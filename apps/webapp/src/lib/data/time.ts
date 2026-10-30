// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The time screens' arithmetic (UC-TIM-01 … 05): a week from Monday and its days, the week grid of
 * entries by day, minutes typed and shown, a running timer's minutes, a report's group named, the
 * plan's actuals as an indented tree, the export's rows as a CSV file, the workload as a heatmap,
 * the month of absences, and money in a currency's minor unit.
 *
 * Pure, so that "which Monday" and "which cell holds this record" are checked by `node --test`.
 * The server decides every rule - the locked period, the week's state, who sees whose time; this
 * module only lays out what it answered.
 */

import type { Absence, BudgetBurn, EstimatesVsActuals, PlacedTimeRecord, TimeExport, TimeReport, Workload } from '@hubtask/sync-engine';

import { parseDuration } from '../i18n/duration.ts';

const DAY = 86_400_000;

function epochOf(day: string): number | undefined {
  if (!/^\d{4}-\d{2}-\d{2}$/u.test(day)) return undefined;
  const at = Date.parse(`${day}T00:00:00Z`);
  return Number.isFinite(at) ? at : undefined;
}

function dayOf(epoch: number): string {
  return new Date(epoch).toISOString().slice(0, 10);
}

/** The Monday of the week a day lies in, as the server counts weeks; undefined for no date. */
export function weekStartOf(day: string): string | undefined {
  const at = epochOf(day);
  if (at === undefined) return undefined;
  const weekday = (new Date(at).getUTCDay() + 6) % 7;
  return dayOf(at - weekday * DAY);
}

/** A day moved by whole days. */
export function addDays(day: string, by: number): string {
  const at = epochOf(day);
  return at === undefined ? day : dayOf(at + by * DAY);
}

/** The seven days of the week that starts on `monday`. */
export function weekDays(monday: string): string[] {
  return Array.from({ length: 7 }, (_, i) => addDays(monday, i));
}

/** One row of the week grid: the time on one entry (or on none) of one kind of work. */
export interface GridRow {
  readonly key: string;
  readonly entryId: string | null;
  readonly title: string;
  readonly workType: string;
  /** Minutes per day, Monday first; a correction counts with its sign. */
  readonly minutes: readonly number[];
  readonly total: number;
}

/**
 * The week's records as rows of entry and kind of work by day, in the order the rows first
 * appear; a record outside the week's days is left out.
 */
export function weekGrid(records: readonly PlacedTimeRecord[], days: readonly string[]): GridRow[] {
  const rows = new Map<string, { entryId: string | null; title: string; workType: string; minutes: number[] }>();
  for (const record of records) {
    const at = days.indexOf(record.day);
    if (at < 0) continue;
    const key = `${record.entry_id ?? ''}|${record.work_type}`;
    let row = rows.get(key);
    if (!row) {
      row = { entryId: record.entry_id, title: record.entry_title, workType: record.work_type, minutes: days.map(() => 0) };
      rows.set(key, row);
    }
    row.minutes[at] = (row.minutes[at] ?? 0) + record.minutes;
  }
  return [...rows.entries()].map(([key, row]) => ({ key, ...row, total: row.minutes.reduce((sum, m) => sum + m, 0) }));
}

/**
 * Minutes typed for one record: `1:30`, `90` or `1,5` (hours in the reader's notation), between
 * 1 and 1440; with `isSigned` a correction, which may start with a minus and is never 0.
 * Empty is `null`; anything else `undefined`.
 */
export function minutesOf(text: string, separator: string, isSigned = false): number | null | undefined {
  const trimmed = text.trim();
  if (trimmed === '') return null;
  const isNegative = isSigned && /^[-−]/u.test(trimmed);
  const parsed = parseDuration(isNegative ? trimmed.slice(1) : trimmed, separator);
  if (parsed === null || parsed === undefined || parsed < 1 || parsed > 1440) return undefined;
  return isNegative ? -parsed : parsed;
}

/** Signed minutes as `h:mm`, with a minus for a correction that takes time away. */
export function minutesText(minutes: number): string {
  const whole = Math.round(Math.abs(minutes));
  const text = `${Math.floor(whole / 60)}:${String(whole % 60).padStart(2, '0')}`;
  return minutes < 0 ? `−${text}` : text;
}

/** The whole minutes a timer started at `startedAt` has run by `now`; never below zero. */
export function elapsedMinutes(startedAt: string | undefined, now: number): number {
  const at = startedAt ? Date.parse(startedAt) : Number.NaN;
  return Number.isFinite(at) ? Math.max(0, Math.floor((now - at) / 60_000)) : 0;
}

/** Whether a record may still be changed or removed rather than corrected. */
export function isChangeable(record: Pick<PlacedTimeRecord, 'day' | 'source'>, week: { state: string; days: readonly { day: string; locked: boolean }[] }): boolean {
  if (week.state !== 'OPEN' && week.state !== 'REJECTED') return false;
  return !week.days.some((each) => each.day === record.day && each.locked);
}

/** The dimensions a report groups by, in the contract's order. */
export const DIMENSIONS = ['PERSON', 'ENTRY', 'COLLECTION', 'HUB', 'WORK_TYPE', 'DAY', 'WEEK', 'MONTH', 'BILLABLE'] as const;
export type Dimension = (typeof DIMENSIONS)[number];

/** How a report's key is named, given the names the screen knows; the value itself otherwise. */
export interface Names {
  person(id: string): string | undefined;
  place(id: string): string | undefined;
  workType(key: string): string | undefined;
  yes: string;
  no: string;
  none: string;
}

/** The words for one key of a report's group. */
export function keyLabel(key: TimeReport['groups'][number]['keys'][number], names: Names): string {
  if (key.value === '') return names.none;
  switch (key.dimension) {
    case 'ENTRY':
      return key.label || key.value;
    case 'PERSON':
      return names.person(key.value) ?? key.value;
    case 'COLLECTION':
    case 'HUB':
      return names.place(key.value) ?? key.value;
    case 'WORK_TYPE':
      return names.workType(key.value) ?? key.value;
    case 'BILLABLE':
      return key.value === 'true' ? names.yes : names.no;
    default:
      return key.value;
  }
}

/** One row of the actuals table: the entry and how deep it lies below the plan's top entries. */
export interface ActualsRow {
  readonly row: EstimatesVsActuals['data'][number];
  readonly depth: number;
}

/**
 * The plan's entries in tree order - each parent before the entries below it - with their depth;
 * an entry whose parent is not in the answer starts a tree of its own.
 */
export function actualsTree(data: EstimatesVsActuals['data']): ActualsRow[] {
  const ids = new Set(data.map((each) => each.entry_id));
  const children = new Map<string, EstimatesVsActuals['data']>();
  const roots: EstimatesVsActuals['data'] = [];
  for (const each of data) {
    if (each.parent_id && ids.has(each.parent_id)) {
      children.set(each.parent_id, [...(children.get(each.parent_id) ?? []), each]);
    } else roots.push(each);
  }
  const out: ActualsRow[] = [];
  const seen = new Set<string>();
  const walk = (rows: EstimatesVsActuals['data'], depth: number) => {
    for (const row of rows) {
      if (seen.has(row.entry_id)) continue;
      seen.add(row.entry_id);
      out.push({ row, depth });
      walk(children.get(row.entry_id) ?? [], depth + 1);
    }
  };
  walk(roots, 0);
  return out;
}

/** Hours with at most two decimals, as a plain number for a table cell. */
export function hoursText(hours: number): string {
  return String(Math.round(hours * 100) / 100);
}

/**
 * The export's rows as RFC 4180 CSV under the columns the server named: every field quoted where it
 * holds a comma, a quote or a line break, and a cell that a spreadsheet would read as a formula
 * (starting with `=`, `+`, `-`, `@`) prefixed with a quote mark, so that a note never runs.
 */
export function exportCsv(answer: TimeExport): string {
  const cell = (value: unknown): string => {
    let text = value === null || value === undefined ? '' : String(value);
    if (typeof value === 'string' && /^[=+\-@\t\r]/u.test(text)) text = `'${text}`;
    return /[",\r\n]/u.test(text) ? `"${text.replaceAll('"', '""')}"` : text;
  };
  const lines = [answer.columns.map(cell).join(',')];
  for (const row of answer.rows) {
    const record = row as unknown as Record<string, unknown>;
    lines.push(answer.columns.map((column) => cell(record[column])).join(','));
  }
  return `${lines.join('\r\n')}\r\n`;
}

/** A day of the week grid's head - "Mon 2" - in the reader's language; the day itself if unread. */
export function dayHeading(day: string, locale: string): string {
  const at = epochOf(day);
  if (at === undefined) return day;
  return new Intl.DateTimeFormat(locale, { weekday: 'short', day: 'numeric', timeZone: 'UTC' }).format(at);
}

// --- The workload (UC-TIM-06) --------------------------------------------------------------------

/** How full a person's week is: nothing known, room, about right, or more than they can give. */
export type Heat = 'unknown' | 'room' | 'fit' | 'over';

/** The heat of a week as the server marked it. */
export function heatOf(cell: Pick<Workload['cells'][number], 'known' | 'over' | 'under'>): Heat {
  if (!cell.known) return 'unknown';
  if (cell.over) return 'over';
  return cell.under ? 'room' : 'fit';
}

/** One row of the heatmap: a person and their weeks in the order of `weeks`. */
export interface HeatRow {
  readonly accountId: string;
  readonly cells: readonly (Workload['cells'][number] | undefined)[];
}

/** The workload's cells as people by weeks, the weeks in order and each person once. */
export function heatmap(cells: Workload['cells']): { weeks: string[]; rows: HeatRow[] } {
  const weeks = [...new Set(cells.map((cell) => cell.week))].sort();
  const people = new Map<string, Map<string, Workload['cells'][number]>>();
  for (const cell of cells) {
    let row = people.get(cell.account_id);
    if (!row) { row = new Map(); people.set(cell.account_id, row); }
    row.set(cell.week, cell);
  }
  return { weeks, rows: [...people.entries()].map(([accountId, row]) => ({ accountId, cells: weeks.map((week) => row.get(week)) })) };
}

// --- Absences (UC-TIM-07) ------------------------------------------------------------------------

/** The days of a month (`YYYY-MM`), the first first; empty for no month. */
export function monthDays(month: string): string[] {
  if (!/^\d{4}-\d{2}$/u.test(month)) return [];
  const first = epochOf(`${month}-01`);
  if (first === undefined) return [];
  const out: string[] = [];
  for (let at = first; dayOf(at).startsWith(month); at += DAY) out.push(dayOf(at));
  return out;
}

/** A month moved by whole months. */
export function addMonths(month: string, by: number): string {
  const [year, number] = month.split('-').map(Number);
  if (!year || !number) return month;
  const index = year * 12 + (number - 1) + by;
  return `${String(Math.floor(index / 12)).padStart(4, '0')}-${String((index % 12) + 1).padStart(2, '0')}`;
}

/** One person's month: the absence on each day, in the order of the days. */
export interface AbsenceRow {
  readonly accountId: string;
  readonly days: readonly (Absence | undefined)[];
}

/** The absences laid out as people by the days given, each person once, in the order they appear. */
export function absenceGrid(absences: readonly Absence[], days: readonly string[]): AbsenceRow[] {
  const people = new Map<string, (Absence | undefined)[]>();
  for (const absence of absences) {
    let row = people.get(absence.account_id);
    if (!row) { row = days.map(() => undefined); people.set(absence.account_id, row); }
    days.forEach((day, i) => { if (day >= absence.from && day <= absence.to) row[i] = absence; });
  }
  return [...people.entries()].map(([accountId, row]) => ({ accountId, days: row }));
}

// --- Money (UC-TIM-08) ---------------------------------------------------------------------------

/** How many digits of a currency's minor unit there are: 2 for EUR, 0 for JPY. */
export function minorDigits(currency: string): number {
  try {
    return new Intl.NumberFormat('en', { style: 'currency', currency }).resolvedOptions().maximumFractionDigits ?? 2;
  } catch {
    return 2;
  }
}

/** An amount in a currency's minor unit as the reader writes money; the bare number for no currency. */
export function moneyText(minor: number, currency: string, locale: string): string {
  if (!currency) return String(minor);
  const digits = minorDigits(currency);
  try {
    return new Intl.NumberFormat(locale, { style: 'currency', currency }).format(minor / 10 ** digits);
  } catch {
    return `${(minor / 10 ** digits).toFixed(digits)} ${currency}`;
  }
}

/**
 * An amount typed in the currency's major unit - `90`, `90,5` - as its minor unit; null for an
 * empty field and undefined for one that is no amount.
 */
export function minorOf(text: string, currency: string, separator: string): number | null | undefined {
  const trimmed = text.trim();
  if (trimmed === '') return null;
  const digits = minorDigits(currency);
  const pattern = new RegExp(`^(\\d+)(?:${separator === ',' ? ',' : '\\.'}(\\d{0,${digits}}))?$`, 'u');
  const match = pattern.exec(trimmed);
  if (!match) return undefined;
  const whole = Number(match[1]);
  const fraction = (match[2] ?? '').padEnd(digits, '0');
  return whole * 10 ** digits + (fraction ? Number(fraction) : 0);
}

/** How much of a budget its burn used, 0 to any number of hundredths; null where there is no budget. */
export function burnShare(burn: Pick<BudgetBurn, 'budget_minutes' | 'minutes' | 'budget_amount_minor' | 'amount_minor'>, of: 'time' | 'money'): number | null {
  const [used, budget] = of === 'time' ? [burn.minutes, burn.budget_minutes] : [burn.amount_minor, burn.budget_amount_minor];
  return budget > 0 ? Math.round((used / budget) * 100) : null;
}

/** Hours typed - `120`, `7,5` - as whole minutes; null for an empty field, undefined for no number. */
export function minutesOfHours(text: string, separator: string): number | null | undefined {
  const trimmed = text.trim();
  if (trimmed === '') return null;
  const pattern = separator === ',' ? /^(\d+)(?:,(\d+))?$/u : /^(\d+)(?:\.(\d+))?$/u;
  const match = pattern.exec(trimmed);
  if (!match) return undefined;
  return Math.round(Number(`${match[1]}.${match[2] ?? '0'}`) * 60);
}
