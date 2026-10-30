// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The time screens' arithmetic (UC-TIM-01 … 05): a week from Monday and its days, the week grid of
 * entries by day, minutes typed and shown, a running timer's minutes, a report's group named, the
 * plan's actuals as an indented tree, and the export's rows as a CSV file.
 *
 * Pure, so that "which Monday" and "which cell holds this record" are checked by `node --test`.
 * The server decides every rule - the locked period, the week's state, who sees whose time; this
 * module only lays out what it answered.
 */

import type { EstimatesVsActuals, PlacedTimeRecord, TimeExport, TimeReport } from '@hubtask/sync-engine';

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
