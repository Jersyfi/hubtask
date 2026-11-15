<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Time* (M7, user-experience.md §7.6): the reader's week, the weeks waiting for their decision,
  // and where the time went, under one head - the week's total and state as its figure, the one
  // timer beside the acts, *Record time* the primary until every working day holds time, then *Hand in
  // the week* (HOME-time-week) - and its views as the home's view bar (the
  // navigation concept §3). Its settings are a page of their own, the bar's last tab.
  //
  // **My week** (UC-TIM-02 check 8): the week from Monday as a grid of entries by day, the records
  // under it - changed and removed while the week is open and the day unlocked, corrected
  // otherwise (UC-TIM-03 check 3) -, copying last week, handing it in and taking it back.
  //
  // **To approve** (UC-TIM-03 check 7): the weeks handed in that the server lets this reader
  // decide; one is read whole, then approved or sent back with a reason.
  //
  // **Report** (UC-TIM-04 check 6, UC-TIM-05): sums by up to two dimensions as a table, and the
  // records handed over as a CSV file.
  //
  // **Absences** (UC-TIM-07 check 7) and **Capacity** (UC-TIM-06 check 7): the shared calendar of
  // who is away, and the hours the reader can give each weekday.
  //
  // **The server decides every rule**: who sees whose week, what is locked, what may change. A
  // refusal is said where it happened, in the server's words.

  import { create } from '../lib/create/create.svelte.ts';
  import { homeTabs } from '../lib/product/home.ts';
  import { untrack } from 'svelte';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import {
    Banner, Button, Checkbox, Dialog, Icon, IconButton, Input, PageHeader, ProductBand, Select, Skeleton, Stack, Switch, Textarea, ViewBar, WorkPage,
    DataTable, type DataColumn, type DataRowMenu, type ViewLink,
  } from '@hubtask/design-system/components';
  import type { CapacityList, PlacedTimeRecord, TimeReport, Timesheet, TimesheetList, TimesheetWeek, WorkTypeList } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import {
    CAPACITIES, DIMENSIONS, WORK_TYPES, addDays, dayCapacities, dayHeading, exportCsv, groupKey, isChangeable, isWeekLogged, keyLabel, minutesOf, minutesText, queuePath, time,
    weekDays, weekGrid, weekPath, weekStartOf, type RecordFields,
  } from '../lib/data/time.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatPlainDay } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { decimalSeparatorOf } from '../lib/i18n/number.ts';
  import { todayIn } from '../lib/i18n/zone.ts';
  import { platform } from '../lib/platform/index.ts';
  import { renderProblem } from '../lib/problem.ts';
  import AbsenceCalendar from '../lib/time/AbsenceCalendar.svelte';
  import CapacityEditor from '../lib/time/CapacityEditor.svelte';
  import RunningTimer from '../lib/time/RunningTimer.svelte';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import { isTimeUnused, owesDefaultKinds, writeDefaultKinds } from '../lib/time/kinds.ts';
  import { timeWays } from '../lib/time/ways.ts';
  import TemplateDialog from '../lib/settings/TemplateDialog.svelte';
  import { WorkspaceStarts } from '../lib/product/workspaceStarts.svelte.ts';

  interface Props {
    /** The tab the address names (`?tab=`); the week where it names none or an unknown one. */
    tab?: string;
    /** Any day of the week to open (`?week=`); this week without. */
    week?: string;
    onnavigate: (path: string) => void;
  }

  const { tab, week, onnavigate }: Props = $props();

  // The time's tabs are the home's (the shared vocabulary, `homeTabs`): Settings last, for whoever
  // may change one of the kinds of work, the lock and the rates (user-experience.md §5.1, §8.2).
  const TABS = homeTabs('TIME');
  const maySetUp = $derived(
    holdsHere('STRUCTURE') === true || holdsHere('APPROVE_TIME') === true || holdsHere('MANAGE_COSTS') === true || holdsHere('MANAGE_MEMBERS') === true,
  );
  const offered = $derived(TABS.filter((each) => each.id !== 'settings' || maySetUp));
  const current = $derived(offered.some((each) => each.id === tab) ? (tab as string) : 'week');
  const homeViews = $derived<ViewLink[]>(offered.map((each) => ({ id: each.id, label: t(each.code), href: each.path })));
  function select(next: string) {
    onnavigate(next === 'week' ? '/time' : next === 'settings' ? '/time/settings' : `/time?tab=${next}`);
  }

  $effect(() => frame.entitle(t('app.time.title')));

  const zone = $derived(actor.zone);
  const today = $derived(todayIn(zone));
  const thisWeek = $derived(weekStartOf(today) ?? today);
  const monday = $derived((week && weekStartOf(week)) || thisWeek);
  const days = $derived(weekDays(monday));
  const locale = $derived(messages.locale);
  const separator = $derived(decimalSeparatorOf(locale, manifest.supportedLocales));

  let failure = $state<string | undefined>(undefined);
  /** A refusal while a dialog is open, said inside it (§5.10); one dialog is open at a time. */
  let dialogFailure = $state<string | undefined>(undefined);
  async function act(run: () => Promise<unknown>, done?: string, inDialog = false): Promise<boolean> {
    failure = undefined;
    dialogFailure = undefined;
    try {
      await run();
      if (done) toast.show({ text: done, tone: 'success' });
      return true;
    } catch (error) {
      const message = renderProblem(error as never, messages).message;
      if (inDialog) dialogFailure = message;
      else failure = message;
      return false;
    }
  }

  // --- The kinds of work, for names and choices ---------------------------------------------------

  let kinds = $state<WorkTypeList['data']>([]);
  /** Answered at all (the page may draw), and answered with the kinds (the start page may stand). */
  let areKindsRead = $state(false);
  let areKindsKnown = $state(false);
  $effect(() => untrack(() => engine.subscribe<WorkTypeList>({ path: WORK_TYPES }, (next) => {
    if (next.status === 'ready') { kinds = next.data.data; areKindsKnown = true; }
    if (next.status === 'ready' || next.status === 'failed') areKindsRead = true;
  })));
  const kindName = (key: string) => kinds.find((each) => each.key === key)?.name ?? key;
  const offeredKinds = $derived(kinds.filter((each) => !each.archived));
  // The sets of kinds of work the workspace may start from (scene emptytime), beside the usual kinds
  // the first visit writes (kinds.ts), for whoever may change the workspace's structure.
  const starts = new WorkspaceStarts(() => 'TIME', () => holdsHere('STRUCTURE') === true);

  // --- My week ------------------------------------------------------------------------------------

  let sheet = $state<TimesheetWeek | undefined>(undefined);
  let sheetRefusal = $state<string | undefined>(undefined);
  $effect(() => {
    const path = weekPath(monday);
    sheet = undefined;
    sheetRefusal = undefined;
    return untrack(() => engine.subscribe<TimesheetWeek>({ path }, (next) => {
      if (next.status === 'ready') sheet = next.data;
      if (next.status === 'failed') sheetRefusal = renderProblem(next.error, messages).message;
    }));
  });
  const grid = $derived(sheet ? weekGrid(sheet.records, days) : []);
  const isOpen = $derived(sheet?.state === 'OPEN' || sheet?.state === 'REJECTED');
  // Time before its first use: the start page in place of the band, the tabs and the week.
  // The band's timer waits until the week and the kinds are read: it writes the usual kinds (§4),
  // which would end the start page before it was drawn.
  const isSettled = $derived(current !== 'week' || (areKindsRead && (sheet !== undefined || sheetRefusal !== undefined)));
  const isStart = $derived(current === 'week' && isTimeUnused(areKindsKnown ? kinds : undefined, sheet?.records.length));
  const ways = $derived(timeWays(starts.cards, t, { timer: () => onnavigate('/'), week: () => void recordFirst() }));
  // On a phone the head pins it where the thumb is (user-experience.md §5.2).
  const firstRecord = $derived(isOpen && (offeredKinds.length > 0 || holdsHere('STRUCTURE') === true)
    ? { label: t('app.time.first_record'), icon: 'plus' as const, onclick: () => void recordFirst() } : undefined);
  // The first record waits for the usual kinds its dialog offers, written by its own press (§4).
  let isRecordingFirst = $state(false);
  async function recordFirst() {
    if (owesDefaultKinds(kinds, holdsHere('STRUCTURE') === true)) {
      if (await act(() => writeDefaultKinds((key, fields) => time.defineWorkType(key, fields), t))) isRecordingFirst = true;
      return;
    }
    openRecord(undefined);
  }
  $effect(() => {
    if (isRecordingFirst && offeredKinds.length > 0) untrack(() => { isRecordingFirst = false; openRecord(undefined); });
  });
  const entryChoices = $derived.by(() => {
    const seen = new Map<string, string>();
    for (const record of sheet?.records ?? []) if (record.entry_id && !seen.has(record.entry_id)) seen.set(record.entry_id, record.entry_title);
    return [{ value: '', label: t('app.time.no_entry') }, ...[...seen].map(([value, label]) => ({ value, label: label || t('app.time.entry_unreadable') }))];
  });
  const gridColumns = $derived<DataColumn[]>([
    { id: 'entry', label: t('app.time.col_entry'), priority: 1 },
    { id: 'kind', label: t('app.time.col_kind'), priority: 3 },
    ...days.map((day) => ({ id: day, label: dayHeading(day, locale), align: 'end' as const, priority: 2 as const })),
    { id: 'total', label: t('app.time.col_total'), align: 'end', priority: 1, isStatus: true },
  ]);
  const recordColumns = $derived<DataColumn[]>([
    { id: 'entry', label: t('app.time.col_entry'), priority: 1 },
    { id: 'day', label: t('app.time.col_day'), priority: 2 },
    { id: 'kind', label: t('app.time.col_kind'), priority: 2 },
    { id: 'minutes', label: t('app.time.col_minutes'), align: 'end', priority: 1, isStatus: true },
    { id: 'note', label: t('app.time.col_note'), priority: 3 },
  ]);
  /** A row of a week's grid, the totals last: the grid's foot is its last row (DataTable has no foot row). */
  type GridLine = { key: string; entryId?: string | null; title?: string; workType?: string; minutes: readonly number[]; total: number; isTotal?: boolean; capacity?: readonly number[]; isAdd?: boolean };
  /**
   * The rows, then - while the week takes time - a row that adds an entry, then the totals, then -
   * on the reader's own week - the day's capacity as a bar (scene time).
   */
  const gridLines = (rows: readonly GridLine[], totals?: { days: readonly { minutes: number }[]; minutes: number }, perDay?: readonly number[], isAdding = false): GridLine[] => {
    if (!totals) return [...rows];
    const lines = isAdding ? [...rows, { key: '__add', minutes: [], total: 0, isAdd: true }] : [...rows];
    const total = { key: '__total', minutes: totals.days.map((day) => day.minutes), total: totals.minutes, isTotal: true };
    return perDay && perDay.some((each) => each > 0) ? [...lines, total, { ...total, key: '__capacity', isTotal: false, capacity: perDay }] : [...lines, total];
  };
  /** The week's state for the band's lozenge (scene time). */
  const sheetState = (state: Timesheet['state']) => state.toLowerCase();
  // The verb repeats in the menu: a phone draws the menu's sheet, not the verb.
  const isCorrectable = (record: PlacedTimeRecord) =>
    !!sheet && record.source !== 'CORRECTION' && (sheet.state === 'APPROVED' || sheet.days.some((d) => d.day === record.day && d.locked));
  const recordMenu = (record: PlacedTimeRecord): DataRowMenu | undefined => {
    const label = t('app.page.more_for', { name: record.entry_title || t('app.time.no_entry') });
    if (sheet && isChangeable(record, sheet)) {
      return { label, items: [{ id: 'change', label: t('app.time.change') }, { id: 'remove', label: t('app.time.remove'), isDestructive: true, hasSeparatorBefore: true }] };
    }
    return isCorrectable(record) ? { label, items: [{ id: 'correct', label: t('app.time.correct') }] } : undefined;
  };
  function onRecordMenu(record: PlacedTimeRecord, id: string) {
    if (id === 'change') openRecord(record);
    else if (id === 'correct') openCorrection(record);
    else if (id === 'remove') void act(() => time.remove(record, [monday]), t('app.time.removed'));
  }

  /** The record dialog: a new record (`editing` undefined) or a change to one. */
  let isRecording = $state(false);
  let editing = $state<PlacedTimeRecord | undefined>(undefined);
  const draft = $state({ day: '', entry: '', kind: '', minutes: '', note: '', billable: true, isBillableTouched: false });
  let draftError = $state<string | undefined>(undefined);
  // *Erstellen* › time opens this screen's own dialog (C-3).
  $effect(() => {
    if (create.wished === 'time' && untrack(() => create.take('time'))) untrack(() => openRecord(undefined));
  });
  function openRecord(record: PlacedTimeRecord | undefined, day?: string) {
    editing = record;
    draft.day = record?.day ?? day ?? (days.includes(today) ? today : monday);
    draft.entry = record?.entry_id ?? '';
    draft.kind = record?.work_type ?? offeredKinds[0]?.key ?? '';
    draft.minutes = record ? minutesText(record.minutes) : '';
    draft.note = record?.note ?? '';
    draft.billable = record?.billable ?? (kinds.find((each) => each.key === draft.kind)?.billable ?? true);
    draft.isBillableTouched = record !== undefined;
    draftError = undefined;
    dialogFailure = undefined;
    isRecording = true;
  }
  $effect(() => {
    if (!isRecording || draft.isBillableTouched) return;
    const kind = kinds.find((each) => each.key === draft.kind);
    if (kind) untrack(() => (draft.billable = kind.billable));
  });
  async function saveRecord() {
    const minutes = minutesOf(draft.minutes, separator);
    if (minutes === null || minutes === undefined) {
      draftError = t('time.minutes_out_of_range', { maximum: '1440' });
      return;
    }
    const fields: RecordFields = {
      entry_id: draft.entry || null, work_type: draft.kind, day: draft.day, minutes, note: draft.note, billable: draft.billable,
    };
    const weeks = [monday, weekStartOf(draft.day) ?? monday];
    const changing = editing;
    const ok = await act(
      () => (changing ? time.update(changing, fields, weeks) : time.log(fields, weeks)),
      changing ? t('app.time.changed') : t('app.time.recorded', { minutes: minutesText(minutes) }),
      true,
    );
    if (ok) isRecording = false;
  }

  /** The correction dialog: signed minutes on a day of an open week, naming the record corrected. */
  let correcting = $state<PlacedTimeRecord | undefined>(undefined);
  let isCorrecting = $state(false);
  const correction = $state({ day: '', minutes: '', note: '' });
  let correctionError = $state<string | undefined>(undefined);
  function openCorrection(record: PlacedTimeRecord) {
    correcting = record;
    correction.day = today;
    correction.minutes = '';
    correction.note = '';
    correctionError = undefined;
    dialogFailure = undefined;
    isCorrecting = true;
  }
  async function saveCorrection() {
    const record = correcting;
    const minutes = minutesOf(correction.minutes, separator, true);
    if (!record || minutes === null || minutes === undefined) {
      correctionError = t('app.time.correction_minutes');
      return;
    }
    const weeks = [monday, weekStartOf(correction.day) ?? monday];
    const ok = await act(() => time.correct(record.id, { minutes, day: correction.day, note: correction.note }, weeks), t('app.time.corrected'), true);
    if (ok) isCorrecting = false;
  }

  const weekText = $derived(t('app.time.week_of', { from: formatPlainDay(days[0], locale), to: formatPlainDay(days[6], locale) }));
  /** The reader's capacity each day of the week (UC-TIM-01): what a day's bar is measured against. */
  let capacities = $state<CapacityList['data']>([]);
  $effect(() => untrack(() => engine.subscribe<CapacityList>({ path: CAPACITIES }, (next) => { if (next.status === 'ready') capacities = next.data.data; })));
  const capacity = $derived(dayCapacities(capacities, actor.account?.id, monday));
  const weekCapacity = $derived(capacity?.reduce((sum, each) => sum + each, 0) ?? 0);
  /** The band's figures (scene time): the week against its capacity, the week, its state. */
  const bandFigures = $derived(
    sheet && current === 'week'
      ? [
          weekCapacity > 0 ? t('app.time.week_of_capacity', { minutes: minutesText(sheet.minutes), capacity: minutesText(weekCapacity) }) : t('app.time.week_total', { minutes: minutesText(sheet.minutes) }),
          weekText,
        ]
      : [],
  );
  /** On a phone a day of the strip is chosen and its bookings are listed under it (scene time). */
  let chosenDay = $state<string | undefined>(undefined);
  const shownDay = $derived(chosenDay && days.includes(chosenDay) ? chosenDay : days.includes(today) ? today : days[0]!);
  const dayRecords = $derived(sheet ? sheet.records.filter((record) => record.day === shownDay) : []);
  /** Each day of the week with its minutes, for the strip a day is recorded from. */
  const strip = $derived(days.map((day) => {
    const at = sheet?.days.find((each) => each.day === day);
    return { day, minutes: at?.minutes ?? 0, isLocked: at?.locked === true, isToday: day === today, capacity: capacity?.[days.indexOf(day)] ?? 0 };
  }));
  const mayRecord = $derived(isOpen && offeredKinds.length > 0);
  const longDay = (day: string) => new Intl.DateTimeFormat(locale, { weekday: 'long', month: 'long', day: 'numeric', timeZone: 'UTC' }).format(Date.parse(`${day}T00:00:00Z`));
  const recordPrimary = $derived(mayRecord ? { label: t('app.time.add'), icon: 'plus' as const, onclick: () => openRecord(undefined) } : undefined);
  // One primary per view (HOME-time-week): recording, until every working day holds time; then
  // handing the week in, and recording steps back to a secondary act in the head.
  const isHandInFirst = $derived(isOpen && !!sheet && isWeekLogged(strip.map((each) => each.minutes), capacity));

  function goWeek(by: number) {
    const next = addDays(monday, by * 7);
    onnavigate(next === thisWeek ? '/time' : `/time?week=${next}`);
  }

  // --- To approve ---------------------------------------------------------------------------------

  /** Twelve weeks back to the end of this one: what a lead decides is rarely older. */
  const queue = $derived(queuePath(addDays(thisWeek, -7 * 12), addDays(thisWeek, 6)));
  let waiting = $state<Timesheet[] | undefined>(undefined);
  let queueRefusal = $state<string | undefined>(undefined);
  $effect(() => {
    if (current !== 'approvals') return;
    const path = queue;
    return untrack(() => engine.subscribe<TimesheetList>({ path }, (next) => {
      if (next.status === 'ready') { waiting = next.data.data.filter((each) => each.account_id !== actor.account?.id); queueRefusal = undefined; }
      if (next.status === 'failed') queueRefusal = renderProblem(next.error, messages).message;
    }));
  });
  $effect(() => { if (waiting) accounts.resolve(waiting.map((each) => each.account_id)); });
  const personName = (id: string) => accounts.nameOf(id) ?? t('app.people.unnamed');

  let deciding = $state<Timesheet | undefined>(undefined);
  let decided = $state<TimesheetWeek | undefined>(undefined);
  $effect(() => {
    const chosen = deciding;
    decided = undefined;
    if (!chosen) return;
    const path = weekPath(chosen.week_start, chosen.account_id);
    return untrack(() => engine.subscribe<TimesheetWeek>({ path }, (next) => { if (next.status === 'ready') decided = next.data; }));
  });
  const decidedDays = $derived(deciding ? weekDays(deciding.week_start) : []);
  const decidedGrid = $derived(decided ? weekGrid(decided.records, decidedDays) : []);
  const decidedColumns = $derived<DataColumn[]>([
    { id: 'entry', label: t('app.time.col_entry'), priority: 1 },
    { id: 'kind', label: t('app.time.col_kind'), priority: 3 },
    ...decidedDays.map((day) => ({ id: day, label: dayHeading(day, locale), align: 'end' as const, priority: 2 as const })),
    { id: 'total', label: t('app.time.col_total'), align: 'end', priority: 1, isStatus: true },
  ]);
  const queueColumns = $derived<DataColumn[]>([
    { id: 'person', label: t('app.time.col_person'), priority: 1 },
    { id: 'week', label: t('app.time.col_week'), priority: 2 },
    { id: 'minutes', label: t('app.time.col_total'), align: 'end', priority: 1, isStatus: true },
  ]);
  let isRejecting = $state(false);
  let reason = $state('');
  async function approve(sheetOf: Timesheet) {
    const ok = await act(() => time.approve(sheetOf.week_start, sheetOf.account_id, queue), t('app.time.approved', { name: personName(sheetOf.account_id) }));
    if (ok) deciding = undefined;
  }
  async function reject() {
    const sheetOf = deciding;
    if (!sheetOf || reason.trim() === '') return;
    const ok = await act(() => time.reject(sheetOf.week_start, sheetOf.account_id, reason.trim(), queue), t('app.time.sent_back', { name: personName(sheetOf.account_id) }), true);
    if (ok) { isRejecting = false; deciding = undefined; reason = ''; }
  }

  // --- Report -------------------------------------------------------------------------------------

  const report = $state({ from: '', to: '', mine: true, first: 'ENTRY', second: '', costs: false });
  // Costs are MANAGE_COSTS's (UC-TIM-05 check 5, UC-TIM-08 check 1): not offered to anybody else.
  const mayCost = $derived(holdsHere('MANAGE_COSTS') === true || holdsHere('MANAGE_MEMBERS') === true);
  $effect(() => {
    if (report.from === '') untrack(() => { report.from = `${today.slice(0, 8)}01`; report.to = today; });
  });
  const reportBody = $derived({
    from: report.from, to: report.to, mine: report.mine,
    group_by: [report.first, report.second].filter((each, i, all) => each !== '' && all.indexOf(each) === i),
  });
  let answer = $state<TimeReport | undefined>(undefined);
  let reportRefusal = $state<string | undefined>(undefined);
  $effect(() => {
    if (current !== 'report' || !report.from || !report.to) return;
    const body = reportBody;
    answer = undefined;
    reportRefusal = undefined;
    return untrack(() => engine.subscribe<TimeReport>({ path: '/time-records:report', body }, (next) => {
      if (next.status === 'ready') answer = next.data;
      if (next.status === 'failed') reportRefusal = renderProblem(next.error, messages).message;
    }));
  });
  /** The largest group of the report, for its bars. */
  const reportPeak = $derived(Math.max(1, ...(answer?.groups ?? []).map((group) => group.minutes)));
  $effect(() => {
    const ids = (answer?.groups ?? []).flatMap((group) => group.keys.filter((key) => key.dimension === 'PERSON').map((key) => key.value));
    if (ids.length > 0) accounts.resolve(ids);
  });
  const names = $derived({
    person: (id: string) => accounts.nameOf(id),
    place: (id: string) => containers.find(id)?.name,
    workType: (key: string) => kinds.find((each) => each.key === key)?.name,
    yes: t('app.time.billable_yes'), no: t('app.time.billable_no'), none: t('app.time.none'),
  });
  const dimensionOptions = $derived(DIMENSIONS.map((each) => ({ value: each, label: t(`app.time.dimension_${each}`) })));
  const reportColumns = $derived<DataColumn[]>([
    ...(answer?.group_by ?? []).map((each, i) => ({ id: `key${i}`, label: t(`app.time.dimension_${each}`), priority: (i === 0 ? 1 : 2) as 1 | 2 })),
    { id: 'minutes', label: t('app.time.col_total'), align: 'end', priority: 1, isStatus: true },
    { id: 'billable', label: t('app.time.col_billable'), align: 'end', priority: 3 },
    { id: 'records', label: t('app.time.col_records'), align: 'end', priority: 3 },
  ]);
  let isExporting = $state(false);
  async function exportRecords() {
    isExporting = true;
    await act(async () => {
      const rows = await time.export({ from: report.from, to: report.to, mine: report.mine, costs: mayCost && report.costs });
      platform.saveFile(new Blob([exportCsv(rows)], { type: 'text/csv;charset=utf-8' }), `time-${report.from}-${report.to}.csv`);
      if (rows.truncated) toast.show({ text: t('app.time.export_truncated'), tone: 'warning' });
    }, t('app.time.exported'));
    isExporting = false;
  }
  let isExportingSheet = $state(false);
  async function exportSpreadsheet() {
    isExportingSheet = true;
    await act(async () => {
      const file = await time.exportSpreadsheet({ from: report.from, to: report.to, mine: report.mine, costs: mayCost && report.costs });
      platform.saveFile(file.body, `time-${report.from}-${report.to}.xlsx`);
      if (file.headers.get('export-truncated') === 'true') toast.show({ text: t('app.time.export_truncated'), tone: 'warning' });
    }, t('app.time.exported'));
    isExportingSheet = false;
  }
</script>

<WorkPage {top} {toolbar}>
  {#snippet tabs()}{#if !isStart}<ViewBar label={t('app.time.title')} views={homeViews} isCompact={viewport.isCompact} closeLabel={t('app.sheet_close')} {current} onnavigate={select} />{/if}{/snippet}
  {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

  {#if current === 'week'}

    {#if sheetRefusal}
      <FailureNote problem={sheetRefusal} />
    {:else if !sheet || !areKindsRead}
      <div aria-busy="true"><Skeleton lines={6} /></div>
    {:else}
      {#if sheet.state === 'REJECTED' && sheet.reason}
        <Banner tone="warning" title={t('app.time.sent_back_title')}>{sheet.reason}</Banner>
      {/if}

      <!-- A phone's day strip belongs to a week in use; the start page stands alone (emptytime). -->
      {#if viewport.isCompact && !isStart}
        <ol class="days" aria-label={t('app.time.days', { week: weekText })}>
          {#each strip as each (each.day)}
            <li>
              <button type="button" class="day" data-today={each.isToday ? '' : undefined} data-locked={each.isLocked ? '' : undefined} aria-pressed={each.day === shownDay}
                aria-label={t('app.time.show_day', { day: longDay(each.day), minutes: minutesText(each.minutes) })} onclick={() => (chosenDay = each.day)}>
                <span class="weekday">{dayHeading(each.day, locale)}</span>
                <span class="sum" data-none={each.minutes === 0 ? '' : undefined}>{each.minutes === 0 ? '–' : minutesText(each.minutes)}</span>
              </button>
            </li>
          {/each}
        </ol>
        <section class="block" aria-labelledby="time-day" data-day-records>
          <h2 id="time-day" class="eyebrow">{longDay(shownDay)}</h2>
          {#if dayRecords.length > 0}
            <ul class="bookings">
              {#each dayRecords as record (record.id)}
                <li class="booking">
                  {#if record.entry_id}<a href={`/items/${record.entry_id}`} class="booking-title">{record.entry_title || t('app.time.entry_unreadable')}</a>{:else}<span class="booking-title">{t('app.time.no_entry')}</span>{/if}
                  <span class="number strong">{minutesText(record.minutes)}</span>
                  {#if isChangeable(record, sheet)}<IconButton size="sm" icon="pencil" label={t('app.time.change')} onclick={() => openRecord(record)} />{/if}
                </li>
              {/each}
            </ul>
          {/if}
          {#if mayRecord && !strip.find((each) => each.day === shownDay)?.isLocked}
            <span><Button size="sm" tone="subtle" icon="plus" onclick={() => openRecord(undefined, shownDay)}>{t('app.time.record_on', { day: longDay(shownDay), minutes: minutesText(dayRecords.reduce((sum, each) => sum + each.minutes, 0)) })}</Button></span>
          {/if}
        </section>
      {/if}

      {#if isStart}
        <ProductEmpty product="TIME" place={{ type: 'TENANT' }} kind="unused" icon="clock" title={t('app.nav.time')} description={t('app.product_empty.sentence_time')}
          create={firstRecord} isInHead
          isStart templates={ways} templateShape="row"
          side={maySetUp ? { label: t('app.time.empty_side_kinds'), onclick: () => select('settings') } : undefined} />
      {:else if sheet.records.length === 0}
        <ProductEmpty product="TIME" place={{ type: 'TENANT' }} kind={isOpen ? 'unused' : 'settled'} icon="clock" title={t('app.time.week_empty')}
          description={isOpen ? t('app.time.week_empty_body') : undefined} create={recordPrimary} isInHead={current === 'week'} isProductHome
          templates={isOpen ? starts.cards : []} templateShape="row"
          side={maySetUp ? { label: t('app.time.empty_side_kinds'), onclick: () => select('settings') } : { label: t('app.time.empty_side'), onclick: () => select('capacity') }} />
      {:else}
        {#if !viewport.isCompact}
          <section class="block" aria-labelledby="time-grid">
            <h2 id="time-grid" class="heading">{t('app.time.grid', { minutes: minutesText(sheet.minutes) })}</h2>
            <DataTable label={t('app.time.grid', { minutes: minutesText(sheet.minutes) })} columns={gridColumns} rows={gridLines(grid, sheet, capacity, mayRecord)} key={(row: GridLine) => row.key}>
              {#snippet cell(row: GridLine, column: DataColumn)}{@render gridCell(row, column, days, true)}{/snippet}
            </DataTable>
          </section>
        {/if}

        {#if !viewport.isCompact}
        <section class="block" aria-labelledby="time-records">
          <h2 id="time-records" class="heading">{t('app.time.records', { count: String(sheet.records.length) })}</h2>
          <DataTable label={t('app.time.records', { count: String(sheet.records.length) })} columns={recordColumns}
            rows={[...sheet.records].sort((a, b) => a.day.localeCompare(b.day))} key={(record: PlacedTimeRecord) => record.id}
            rowLabel={(record: PlacedTimeRecord) => record.entry_title || t('app.time.no_entry')}
            menu={recordMenu} onmenu={onRecordMenu}
            actionsLabel={t('app.time.col_actions')} sheetDismissLabel={t('app.page.sheet_close')} sheetGrabLabel={t('app.page.sheet_grab')}>
            {#snippet cell(record: PlacedTimeRecord, column: DataColumn)}
              {#if column.id === 'day'}<span class="nowrap">{dayHeading(record.day, locale)}</span>
              {:else if column.id === 'entry'}{#if record.entry_id}<a href={`/items/${record.entry_id}`}>{record.entry_title || t('app.time.entry_unreadable')}</a>{:else}{t('app.time.no_entry')}{/if}
              {:else if column.id === 'kind'}{kindName(record.work_type)}{#if record.billable}<span class="note">{t('app.time.billable_yes')}</span>{/if}
              {:else if column.id === 'minutes'}<span class="number">{minutesText(record.minutes)}</span>{#if record.source === 'CORRECTION'}<span class="note">{t('app.time.a_correction')}</span>{/if}
              {:else}<span class="clip">{record.note}</span>{/if}
            {/snippet}
            {#snippet verb(record: PlacedTimeRecord)}
              {#if isChangeable(record, sheet!)}
                <IconButton size="sm" icon="pencil" label={t('app.time.change')} onclick={() => openRecord(record)} />
              {:else if isCorrectable(record)}
                <Button size="sm" tone="subtle" onclick={() => openCorrection(record)}>{t('app.time.correct')}</Button>
              {/if}
            {/snippet}
          </DataTable>
        </section>
        {/if}
      {/if}
      {#if isOpen && !isStart}
        <!-- Handing in, with what happens next (scene time): the week goes for approval and is locked.
             Not on the start page, where nothing has been recorded yet (emptytime). -->
        <div class="hand-in" data-hand-in>
          <p>{t('app.time.hand_in_next')}</p>
          <Button tone={isHandInFirst ? 'primary' : 'secondary'} icon="send" disabledReason={sheet.minutes <= 0 ? t('timesheets.empty') : undefined}
            onclick={() => void act(() => time.submit(monday), t('app.time.handed_in', { minutes: minutesText(sheet?.minutes ?? 0) }))}>{t('app.time.hand_in_week')}</Button>
        </div>
      {/if}
    {/if}

  {:else if current === 'approvals'}
    {#if queueRefusal}
      <FailureNote problem={queueRefusal} />
    {:else if !waiting}
      <div aria-busy="true"><Skeleton lines={4} /></div>
    {:else if waiting.length === 0}
      <ProductEmpty kind="settled" icon="check" title={t('app.time.queue_empty')} description={t('app.time.queue_empty_body')} />
    {:else}
      <DataTable label={t('app.time.queue')} columns={queueColumns} rows={waiting} key={(each: Timesheet) => `${each.account_id}:${each.week_start}`}
        onopen={(each: Timesheet) => (deciding = each)} rowLabel={(each: Timesheet) => personName(each.account_id)} actionsLabel={t('app.time.col_actions')}>
        {#snippet cell(each: Timesheet, column: DataColumn)}
          {#if column.id === 'person'}{personName(each.account_id)}
          {:else if column.id === 'week'}{t('app.time.week_of', { from: formatPlainDay(each.week_start, messages.locale), to: formatPlainDay(addDays(each.week_start, 6), messages.locale) })}
          {:else}<span class="number">{minutesText(each.submitted_minutes)}</span>{/if}
        {/snippet}
        {#snippet verb(each: Timesheet)}
          <Button size="sm" tone="subtle" icon="check" onclick={() => void approve(each)}>{t('app.time.approve')}</Button>
        {/snippet}
      </DataTable>
    {/if}

    {#if deciding}
      <section class="block" aria-labelledby="time-deciding">
        <div class="toolbar">
          <h2 id="time-deciding" class="heading">{t('app.time.week_of_person', { name: personName(deciding.account_id), from: formatPlainDay(deciding.week_start, messages.locale) })}</h2>
          <div class="acts">
            <Button size="sm" icon="check" onclick={() => deciding && void approve(deciding)}>{t('app.time.approve')}</Button>
            <Button size="sm" tone="secondary" onclick={() => { reason = ''; dialogFailure = undefined; isRejecting = true; }}>{t('app.time.send_back')}</Button>
            <Button size="sm" tone="subtle" icon="x" onclick={() => (deciding = undefined)}>{t('app.time.close_week')}</Button>
          </div>
        </div>
        {#if !decided}
          <Skeleton lines={3} />
        {:else}
          <DataTable label={t('app.time.grid', { minutes: minutesText(decided.minutes) })} columns={decidedColumns} rows={decidedGrid} key={(row: GridLine) => row.key}>
            {#snippet cell(row: GridLine, column: DataColumn)}{@render gridCell(row, column, decidedDays, false)}{/snippet}
          </DataTable>
        {/if}
      </section>
    {/if}

  {:else if current === 'report'}
    <div class="controls">
      <Input label={t('app.time.from')} type="date" size="sm" bind:value={report.from} />
      <Input label={t('app.time.to')} type="date" size="sm" bind:value={report.to} />
      <Select label={t('app.time.group_by')} size="sm" bind:value={report.first} options={dimensionOptions} />
      <Select label={t('app.time.then_by')} size="sm" bind:value={report.second} options={[{ value: '', label: t('app.time.none') }, ...dimensionOptions]} />
      <Switch label={t('app.time.mine_only')} bind:checked={report.mine} />
      {#if mayCost}<Switch label={t('app.time.export_with_costs')} bind:checked={report.costs} />{/if}
      <Button size="sm" tone="secondary" icon="file-text" isBusy={isExporting} busyLabel={t('app.time.export')} onclick={() => void exportRecords()}>{t('app.time.export')}</Button>
      <Button size="sm" tone="secondary" icon="file-text" isBusy={isExportingSheet} busyLabel={t('app.time.export_spreadsheet')} onclick={() => void exportSpreadsheet()}>{t('app.time.export_spreadsheet')}</Button>
    </div>
    {#if reportRefusal}
      <FailureNote problem={reportRefusal} />
    {:else if !answer}
      <div aria-busy="true"><Skeleton lines={4} /></div>
    {:else if answer.groups.length === 0}
      <ProductEmpty kind="settled" icon="clock" title={t('app.time.report_empty')} description={t('app.time.report_empty_body')} />
    {:else}
      <h2 class="heading">{t('app.time.report_total', { minutes: minutesText(answer.minutes) })}</h2>
      {#if answer.withheld_groups > 0}
        {#if answer.withheld_minutes_hidden}
          <p class="quiet">{t('app.time.withheld_hidden', { groups: String(answer.withheld_groups) })}</p>
        {:else}
          <p class="quiet">{t('app.time.withheld', { groups: String(answer.withheld_groups), minutes: minutesText(answer.withheld_minutes) })}</p>
        {/if}
      {/if}
      <DataTable label={t('app.time.tab_report')} columns={reportColumns} rows={answer.groups} key={(group: TimeReport['groups'][number]) => groupKey(group)}>
        {#snippet cell(group: TimeReport['groups'][number], column: DataColumn)}
          {#if column.id === 'minutes'}
            <span class="share">
              <span class="bar" aria-hidden="true" style:inline-size={`${Math.round((group.minutes / reportPeak) * 100)}%`}></span>
              <span class="number">{minutesText(group.minutes)}</span>
            </span>
          {:else if column.id === 'billable'}<span class="number">{minutesText(group.billable_minutes)}</span>
          {:else if column.id === 'records'}<span class="number">{group.records}</span>
          {:else}{@const key = group.keys[Number(column.id.slice(3))]}{key ? keyLabel(key, names) : ''}{/if}
        {/snippet}
      </DataTable>
    {/if}
  {:else if current === 'absences'}
    <AbsenceCalendar />
  {:else if current === 'capacity'}
    <CapacityEditor />
  {/if}
</WorkPage>

<TemplateDialog template={starts.opened} {onnavigate} onClose={() => (starts.opened = undefined)} />

{#snippet gridCell(row: GridLine, column: DataColumn, columnDays: readonly string[], isLinked: boolean)}
  {#if row.isAdd}
    {#if column.id === 'entry'}<button type="button" class="add-row" data-add-entry onclick={() => openRecord(undefined)}><Icon name="plus" size="sm" /><span>{t('app.time.add_row')}</span></button>{/if}
  {:else if row.capacity}
    {#if column.id === 'entry'}{t('app.time.capacity_row')}
    {:else if column.id !== 'kind' && column.id !== 'total'}{@const at = columnDays.indexOf(column.id)}{@const of = row.capacity[at] ?? 0}
      {#if of > 0}<span class="cap" data-capacity={of} role="img" aria-label={t('app.time.day_of_capacity', { minutes: minutesText(row.minutes[at] ?? 0), capacity: minutesText(of) })}><span class="cap-fill" style:inline-size={`${Math.min(100, Math.round(((row.minutes[at] ?? 0) / of) * 100))}%`}></span></span>{/if}
    {/if}
  {:else if column.id === 'entry'}
    {#if row.isTotal}<span class="strong">{t('app.time.col_total')}</span>
    {:else if row.entryId && isLinked}<a class="clip" href={`/items/${row.entryId}`} title={row.title || undefined}>{row.title || t('app.time.entry_unreadable')}</a>
    {:else if row.entryId}{row.title || t('app.time.entry_unreadable')}{:else}{t('app.time.no_entry')}{/if}
  {:else if column.id === 'kind'}{row.workType ? kindName(row.workType) : ''}
  {:else if column.id === 'total'}<span class="number strong">{minutesText(row.total)}</span>
  {:else}{@const minutes = row.minutes[columnDays.indexOf(column.id)] ?? 0}<span class="number">{minutes === 0 && !row.isTotal ? '' : minutesText(minutes)}</span>{/if}
{/snippet}

{#snippet top()}
  {#if isStart}
    <!-- The start page's name is its one heading (scene emptytime); a phone's bar still carries it. -->
    {#if viewport.isCompact}<PageHeader title={t('app.nav.time')} isTitleInBar primary={firstRecord} />{/if}
  {:else}
  <div class="top" data-head="time">
    <ProductBand product="time" icon="clock" name={current === 'week' ? t('app.time.tab_week') : t('app.time.title')} figures={bandFigures}
      state={sheet && current === 'week' ? { label: t(`app.time.state_${sheet.state}`), vocabulary: 'timesheet', state: sheetState(sheet.state) } : undefined}
      primary={current === 'week' && !isHandInFirst ? recordPrimary : undefined}
      secondary={current === 'week' && isHandInFirst && recordPrimary ? [recordPrimary] : []}
      breadcrumb={{
        trail: [
          { id: 'products', label: t('app.nav.products') },
          { id: 'product', label: t('app.nav.time') },
        ],
        label: t('app.product_home.trail'),
        expandLabel: t('app.admin.expand_trail'),
      }}>
      {#snippet aside()}{#if !viewport.isCompact && isSettled}<RunningTimer {onnavigate} />{/if}{/snippet}
    </ProductBand>
    <!-- On a phone the timer is a card of its own above the week (scene time), where an entry's
         whole title wraps (user-experience.md §5.12). -->
    {#if viewport.isCompact && current === 'week' && isSettled}<RunningTimer {onnavigate} isCard />{/if}
  </div>
  {/if}
{/snippet}

{#snippet toolbar()}
  {#if current === 'week' && !isStart}
    <div class="toolbar">
      <div class="nav">
        <Button size="sm" tone="subtle" icon="chevron-left" onclick={() => goWeek(-1)}>{t('app.time.week_before')}</Button>
        <h2 class="heading" aria-live="polite">{weekText}</h2>
        <Button size="sm" tone="subtle" icon="chevron-right" onclick={() => goWeek(1)}>{t('app.time.week_after')}</Button>
        {#if monday !== thisWeek}<Button size="sm" tone="subtle" onclick={() => onnavigate('/time')}>{t('app.time.this_week')}</Button>{/if}
      </div>
      {#if sheet}
        <div class="acts">
          {#if isOpen}
            <Button size="sm" tone="secondary" icon="copy" onclick={() => void act(async () => {
              const copied = await time.copyPrevious(monday);
              toast.show({ text: t('app.time.copied', { count: String(copied.copied) }), tone: 'success' });
            })}>{t('app.time.copy_previous')}</Button>
          {:else if sheet.state === 'SUBMITTED'}
            <Button size="sm" tone="secondary" icon="rotate-ccw" onclick={() => void act(() => time.withdraw(monday), t('app.time.taken_back'))}>{t('app.time.take_back')}</Button>
          {/if}
        </div>
      {/if}
    </div>
  {/if}
{/snippet}

{#if isRecording}
  <Dialog bind:isOpen={isRecording} title={editing ? t('app.time.change') : t('app.time.add')} dismissLabel={t('app.links.cancel')} size="m"
    primary={{ label: editing ? t('app.time.save') : t('app.time.record'), onclick: () => void saveRecord() }}>
    <Stack gap="150">
      <Select label={t('app.time.col_day')} bind:value={draft.day} options={days.map((day) => ({ value: day, label: dayHeading(day, locale) }))} />
      <Select label={t('app.time.col_entry')} bind:value={draft.entry} options={entryChoices} />
      <Select label={t('app.time.work_type')} bind:value={draft.kind}
        options={(editing && !offeredKinds.some((k) => k.key === draft.kind) ? [{ value: draft.kind, label: kindName(draft.kind) }] : []).concat(offeredKinds.map((each) => ({ value: each.key, label: each.name })))} />
      <Input label={t('app.time.col_minutes')} hint={t('app.time.minutes_hint')} bind:value={draft.minutes} error={draftError} />
      <Textarea label={t('app.time.col_note')} bind:value={draft.note} maxlength={2000} />
      <Checkbox label={t('app.time.billable')} checked={draft.billable}
        onchange={(event: Event) => { draft.billable = (event.currentTarget as HTMLInputElement).checked; draft.isBillableTouched = true; }} />
      {#if dialogFailure}<FailureNote problem={dialogFailure} />{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isRecording = false)}>{t('app.links.cancel')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if isCorrecting && correcting}
  <Dialog bind:isOpen={isCorrecting} title={t('app.time.correct_title', { minutes: minutesText(correcting.minutes), day: formatPlainDay(correcting.day, messages.locale) })} dismissLabel={t('app.links.cancel')} size="m"
    primary={{ label: t('app.time.correct'), onclick: () => void saveCorrection() }}>
    <Stack gap="150">
      <p class="quiet">{t('app.time.correct_hint')}</p>
      <Input label={t('app.time.col_day')} type="date" bind:value={correction.day} />
      <Input label={t('app.time.correction_label')} hint={t('app.time.correction_hint')} bind:value={correction.minutes} error={correctionError} />
      <Textarea label={t('app.time.col_note')} bind:value={correction.note} maxlength={2000} />
      {#if dialogFailure}<FailureNote problem={dialogFailure} />{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isCorrecting = false)}>{t('app.links.cancel')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if isRejecting && deciding}
  <Dialog bind:isOpen={isRejecting} title={t('app.time.send_back_title', { name: personName(deciding.account_id) })} dismissLabel={t('app.links.cancel')} size="s"
    primary={{ label: t('app.time.send_back'), disabledReason: reason.trim() === '' ? t('app.time.reason_needed') : undefined, onclick: () => void reject() }}>
    <Stack gap="150">
      <Textarea label={t('app.time.reason')} hint={t('app.time.reason_hint')} bind:value={reason} maxlength={500} />
      {#if dialogFailure}<FailureNote problem={dialogFailure} />{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isRejecting = false)}>{t('app.links.cancel')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .top { display: grid; gap: var(--sp-200); min-inline-size: 0; }
  .toolbar { display: flex; flex: 1; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .nav, .acts { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050); }
  .controls { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }
  .block { display: flex; flex-direction: column; gap: var(--sp-100); min-width: 0; }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .note { display: inline-flex; align-items: center; gap: var(--sp-025); margin-inline-start: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); }
  .number { text-align: end; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .strong { font-weight: var(--fw-medium); }
  .nowrap { white-space: nowrap; }
  .clip { display: inline-block; max-inline-size: var(--layout-board-column); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .days { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  /* No global `box-sizing`: a locked day is a `div`, whose padding and border would add to its 100%. */
  .day {
    box-sizing: border-box;
    position: relative; display: flex; flex-direction: column; align-items: flex-start; gap: var(--sp-025);
    inline-size: 100%; min-inline-size: 0; block-size: 100%; padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md);
    background: var(--bg-surface); color: var(--text-primary); font: inherit; text-align: start;
  }
  button.day { cursor: pointer; }
  button.day:hover { border-color: var(--border-strong); }
  button.day:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--bw-ring); }
  .day[data-today] { border-color: var(--accent-primary); box-shadow: inset 0 0 0 var(--bw-hairline) var(--accent-primary); }
  .day[data-locked] { background: var(--bg-surface-sunken); }
  .weekday { overflow: hidden; color: var(--text-secondary); font-size: var(--fs-075); text-overflow: ellipsis; white-space: nowrap; max-inline-size: 100%; }
  .sum { font-size: var(--fs-200); font-weight: var(--fw-medium); font-variant-numeric: tabular-nums; }
  .sum[data-none] { color: var(--text-subtle); }
  /* A day's capacity in the grid: neutral, never the product's colour (WB-2). */
  .cap { display: block; inline-size: 100%; min-inline-size: var(--sp-500); block-size: var(--sp-050); border-radius: var(--r-full); background: var(--bg-surface-sunken); overflow: hidden; }
  .cap-fill { display: block; block-size: 100%; border-radius: inherit; background: var(--text-secondary); }
  .day[aria-pressed='true'] { border-color: var(--accent-primary); background: var(--accent-primary-subtle); }
  .eyebrow { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-medium); text-transform: uppercase; }
  /* design-system-lint-ignore: tracking in em follows the type step it spaces */
  .eyebrow { letter-spacing: 0.06em; }
  .bookings { display: grid; gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  .booking { display: grid; grid-template-columns: minmax(0, 1fr) auto auto; align-items: center; gap: var(--sp-100); padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface); }
  .booking-title { overflow: hidden; color: var(--text-primary); text-overflow: ellipsis; white-space: nowrap; }
  .hand-in { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .hand-in p { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .add-row { display: inline-flex; align-items: center; gap: var(--sp-050); padding: 0; border: 0; background: none; color: var(--text-secondary); font: inherit; cursor: pointer; white-space: nowrap; }
  .add-row:hover { color: var(--text-primary); }
  .add-row:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--bw-ring); border-radius: var(--r-sm); }
  .share { display: inline-flex; align-items: center; justify-content: flex-end; gap: var(--sp-100); inline-size: 100%; min-inline-size: var(--sp-800); }
  .bar { display: block; max-inline-size: var(--sp-800); block-size: var(--sp-050); border-radius: var(--r-full); background: var(--accent-primary); }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .day { padding: var(--sp-050); }
    .sum { font-size: var(--fs-100); }
  }
</style>
