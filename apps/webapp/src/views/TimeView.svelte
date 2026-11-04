<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Time* (M7, user-experience.md §7.6): the reader's week, the weeks waiting for their decision,
  // and where the time went, under one head - the week's total and state as its figure, the one
  // timer beside the acts, *Record time* the primary - and one strip of tabs.
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

  import { untrack } from 'svelte';
  import {
    Badge, Banner, Button, Checkbox, Dialog, Icon, IconButton, Inline, Input, PageHeader, PlaceTabs, Select, Skeleton, Stack, Switch, Table, Textarea,
    type Column, type PlaceTab,
  } from '@hubtask/design-system/components';
  import type { PlacedTimeRecord, TimeReport, Timesheet, TimesheetList, TimesheetWeek, WorkTypeList } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import {
    DIMENSIONS, WORK_TYPES, addDays, dayHeading, exportCsv, isChangeable, keyLabel, minutesOf, minutesText, queuePath, time,
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
  import TimeSettingsView from './TimeSettingsView.svelte';
  import RunningTimer from '../lib/time/RunningTimer.svelte';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';

  interface Props {
    /** The tab the address names (`?tab=`); the week where it names none or an unknown one. */
    tab?: string;
    /** Any day of the week to open (`?week=`); this week without. */
    week?: string;
    onnavigate: (path: string) => void;
  }

  const { tab, week, onnavigate }: Props = $props();

  const TABS = [
    { id: 'week', code: 'app.time.tab_week' },
    { id: 'approvals', code: 'app.time.tab_approvals' },
    { id: 'report', code: 'app.time.tab_report' },
    { id: 'absences', code: 'app.time.tab_absences' },
    { id: 'capacity', code: 'app.time.tab_capacity' },
    // Last (user-experience.md §5.1, §8.2): the kinds of work, the lock and the rates, for whoever
    // may change one of them.
    { id: 'settings', code: 'app.time.tab_settings' },
  ] as const;
  const maySetUp = $derived(
    holdsHere('STRUCTURE') === true || holdsHere('APPROVE_TIME') === true || holdsHere('MANAGE_COSTS') === true || holdsHere('MANAGE_MEMBERS') === true,
  );
  const offered = $derived(TABS.filter((each) => each.id !== 'settings' || maySetUp));
  const current = $derived(offered.some((each) => each.id === tab) ? (tab as string) : 'week');
  const tabs = $derived<PlaceTab[]>(offered.map((each) => ({ id: each.id, label: t(each.code), href: each.id === 'week' ? '/time' : `/time?tab=${each.id}` })));
  function select(next: string) {
    onnavigate(next === 'week' ? '/time' : `/time?tab=${next}`);
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
  $effect(() => untrack(() => engine.subscribe<WorkTypeList>({ path: WORK_TYPES }, (next) => { if (next.status === 'ready') kinds = next.data.data; })));
  const kindName = (key: string) => kinds.find((each) => each.key === key)?.name ?? key;
  const offeredKinds = $derived(kinds.filter((each) => !each.archived));

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
  const stateTone = (state: Timesheet['state']) => (state === 'APPROVED' ? 'success' : state === 'REJECTED' ? 'warning' : state === 'SUBMITTED' ? 'info' : 'neutral');
  const entryChoices = $derived.by(() => {
    const seen = new Map<string, string>();
    for (const record of sheet?.records ?? []) if (record.entry_id && !seen.has(record.entry_id)) seen.set(record.entry_id, record.entry_title);
    return [{ value: '', label: t('app.time.no_entry') }, ...[...seen].map(([value, label]) => ({ value, label: label || t('app.time.entry_unreadable') }))];
  });
  const gridColumns = $derived<Column[]>([
    { id: 'entry', label: t('app.time.col_entry') },
    { id: 'kind', label: t('app.time.col_kind') },
    ...days.map((day) => ({ id: day, label: dayHeading(day, locale), align: 'end' as const })),
    { id: 'total', label: t('app.time.col_total'), align: 'end' },
  ]);
  const recordColumns = $derived<Column[]>([
    { id: 'day', label: t('app.time.col_day') },
    { id: 'entry', label: t('app.time.col_entry') },
    { id: 'kind', label: t('app.time.col_kind') },
    { id: 'minutes', label: t('app.time.col_minutes'), align: 'end' },
    { id: 'note', label: t('app.time.col_note') },
    { id: 'actions', label: t('app.time.col_actions'), isLabelHidden: true },
  ]);

  /** The record dialog: a new record (`editing` undefined) or a change to one. */
  let isRecording = $state(false);
  let editing = $state<PlacedTimeRecord | undefined>(undefined);
  const draft = $state({ day: '', entry: '', kind: '', minutes: '', note: '', billable: true, isBillableTouched: false });
  let draftError = $state<string | undefined>(undefined);
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
  const figureText = $derived(
    sheet ? `${t('app.time.week_total', { minutes: minutesText(sheet.minutes) })} · ${t(`app.time.state_${sheet.state}`)}` : '',
  );
  /** Each day of the week with its minutes, for the strip a day is recorded from. */
  const strip = $derived(days.map((day) => {
    const at = sheet?.days.find((each) => each.day === day);
    return { day, minutes: at?.minutes ?? 0, isLocked: at?.locked === true, isToday: day === today };
  }));
  const mayRecord = $derived(isOpen && offeredKinds.length > 0);
  const longDay = (day: string) => new Intl.DateTimeFormat(locale, { weekday: 'long', month: 'long', day: 'numeric', timeZone: 'UTC' }).format(Date.parse(`${day}T00:00:00Z`));
  const recordPrimary = $derived(mayRecord ? { label: t('app.time.add'), icon: 'plus' as const, onclick: () => openRecord(undefined) } : undefined);

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
  const decidedColumns = $derived<Column[]>([
    { id: 'entry', label: t('app.time.col_entry') },
    { id: 'kind', label: t('app.time.col_kind') },
    ...decidedDays.map((day) => ({ id: day, label: dayHeading(day, locale), align: 'end' as const })),
    { id: 'total', label: t('app.time.col_total'), align: 'end' },
  ]);
  const queueColumns = $derived<Column[]>([
    { id: 'person', label: t('app.time.col_person') },
    { id: 'week', label: t('app.time.col_week') },
    { id: 'minutes', label: t('app.time.col_total'), align: 'end' },
    { id: 'actions', label: t('app.time.col_actions'), isLabelHidden: true },
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
  const reportColumns = $derived<Column[]>([
    ...(answer?.group_by ?? []).map((each) => ({ id: each, label: t(`app.time.dimension_${each}`) })),
    { id: 'minutes', label: t('app.time.col_total'), align: 'end' },
    { id: 'billable', label: t('app.time.col_billable'), align: 'end' },
    { id: 'records', label: t('app.time.col_records'), align: 'end' },
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

<Stack gap="300">
  <PageHeader
    title={t('app.time.title')}
    subtitle={t('app.modules.hint_time')}
    isTitleInBar={viewport.isCompact}
    primary={current === 'week' ? recordPrimary : undefined}
    breadcrumb={{
      trail: [
        { id: 'products', label: t('app.nav.products') },
        { id: 'product', label: t('app.nav.time') },
      ],
      label: t('app.product_home.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  >
    {#snippet figure()}{#if current === 'week' && figureText}<span class="figure" data-time-figure>{figureText}</span>{/if}{/snippet}
    {#snippet aside()}<RunningTimer {onnavigate} />{/snippet}
  </PageHeader>

  <PlaceTabs label={t('app.time.title')} {tabs} {current} onnavigate={select} />

  {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

  {#if current === 'week'}
    <div class="toolbar">
      <div class="nav">
        <Button size="sm" tone="subtle" icon="chevron-left" onclick={() => goWeek(-1)}>{t('app.time.week_before')}</Button>
        <h2 class="heading" aria-live="polite">{weekText}</h2>
        <Button size="sm" tone="subtle" icon="chevron-right" onclick={() => goWeek(1)}>{t('app.time.week_after')}</Button>
        {#if monday !== thisWeek}<Button size="sm" tone="subtle" onclick={() => onnavigate('/time')}>{t('app.time.this_week')}</Button>{/if}
      </div>
      {#if sheet}
        <div class="acts">
          <Badge size="sm" tone={stateTone(sheet.state)}>{t(`app.time.state_${sheet.state}`)}</Badge>
          {#if isOpen}
            <Button size="sm" tone="secondary" icon="copy" onclick={() => void act(async () => {
              const copied = await time.copyPrevious(monday);
              toast.show({ text: t('app.time.copied', { count: String(copied.copied) }), tone: 'success' });
            })}>{t('app.time.copy_previous')}</Button>
            <Button size="sm" icon="send" disabledReason={sheet.minutes <= 0 ? t('timesheets.empty') : undefined}
              onclick={() => void act(() => time.submit(monday), t('app.time.handed_in', { minutes: minutesText(sheet?.minutes ?? 0) }))}>{t('app.time.hand_in')}</Button>
          {:else if sheet.state === 'SUBMITTED'}
            <Button size="sm" tone="secondary" icon="rotate-ccw" onclick={() => void act(() => time.withdraw(monday), t('app.time.taken_back'))}>{t('app.time.take_back')}</Button>
          {/if}
        </div>
      {/if}
    </div>

    {#if sheetRefusal}
      <Banner tone="danger">{sheetRefusal}</Banner>
    {:else if !sheet}
      <div aria-busy="true"><Skeleton lines={6} /></div>
    {:else}
      {#if sheet.state === 'REJECTED' && sheet.reason}
        <Banner tone="warning" title={t('app.time.sent_back_title')}>{sheet.reason}</Banner>
      {/if}

      <ol class="days" aria-label={t('app.time.days', { week: weekText })}>
        {#each strip as each (each.day)}
          <li>
            {#if mayRecord && !each.isLocked}
              <button type="button" class="day" data-today={each.isToday ? '' : undefined}
                aria-label={t('app.time.record_on', { day: longDay(each.day), minutes: minutesText(each.minutes) })}
                onclick={() => openRecord(undefined, each.day)}>
                <span class="weekday">{dayHeading(each.day, locale)}</span>
                <span class="sum" data-none={each.minutes === 0 ? '' : undefined}>{minutesText(each.minutes)}</span>
                <span class="add" aria-hidden="true"><Icon name="plus" size="sm" /></span>
              </button>
            {:else}
              <div class="day" data-today={each.isToday ? '' : undefined} data-locked={each.isLocked ? '' : undefined}>
                <span class="weekday">{dayHeading(each.day, locale)}</span>
                <span class="sum" data-none={each.minutes === 0 ? '' : undefined}>{minutesText(each.minutes)}</span>
                {#if each.isLocked}<span class="note"><Icon name="lock" size="sm" /><span>{t('app.time.locked')}</span></span>{/if}
              </div>
            {/if}
          </li>
        {/each}
      </ol>

      {#if sheet.records.length === 0}
        <ProductEmpty kind={isOpen ? 'unused' : 'settled'} icon="clock" title={t('app.time.week_empty')}
          description={isOpen ? t('app.time.week_empty_body') : undefined} create={recordPrimary} isInHead={current === 'week'} />
      {:else}
        {#if !viewport.isCompact}
          <section class="block" aria-labelledby="time-grid">
            <h2 id="time-grid" class="heading">{t('app.time.grid', { minutes: minutesText(sheet.minutes) })}</h2>
            <Table label={t('app.time.grid', { minutes: minutesText(sheet.minutes) })} isLabelHidden columns={gridColumns}>
              {#each grid as row (row.key)}
                <tr>
                  <th scope="row">{#if row.entryId}<a href={`/items/${row.entryId}`}>{row.title || t('app.time.entry_unreadable')}</a>{:else}{t('app.time.no_entry')}{/if}</th>
                  <td>{kindName(row.workType)}</td>
                  {#each row.minutes as minutes, i (days[i])}<td class="number">{minutes === 0 ? '' : minutesText(minutes)}</td>{/each}
                  <td class="number strong">{minutesText(row.total)}</td>
                </tr>
              {/each}
              <tr class="foot">
                <th scope="row">{t('app.time.col_total')}</th>
                <td></td>
                {#each sheet.days as day (day.day)}
                  <td class="number">{minutesText(day.minutes)}</td>
                {/each}
                <td class="number strong">{minutesText(sheet.minutes)}</td>
              </tr>
            </Table>
          </section>
        {/if}

        <section class="block" aria-labelledby="time-records">
          <h2 id="time-records" class="heading">{t('app.time.records', { count: String(sheet.records.length) })}</h2>
          <Table isStacked label={t('app.time.records', { count: String(sheet.records.length) })} isLabelHidden columns={recordColumns}>
            {#each [...sheet.records].sort((a, b) => a.day.localeCompare(b.day)) as record (record.id)}
              <tr>
                <td class="nowrap">{dayHeading(record.day, locale)}</td>
                <th scope="row">{#if record.entry_id}<a href={`/items/${record.entry_id}`}>{record.entry_title || t('app.time.entry_unreadable')}</a>{:else}{t('app.time.no_entry')}{/if}</th>
                <td>{kindName(record.work_type)}{#if record.billable}<span class="note">{t('app.time.billable_yes')}</span>{/if}</td>
                <td class="number">{minutesText(record.minutes)}{#if record.source === 'CORRECTION'}<span class="note">{t('app.time.a_correction')}</span>{/if}</td>
                <td class="clip">{record.note}</td>
                <td>
                  <Inline gap="050">
                    {#if isChangeable(record, sheet)}
                      <IconButton size="sm" icon="pencil" label={t('app.time.change')} onclick={() => openRecord(record)} />
                      <IconButton size="sm" icon="trash" label={t('app.time.remove')} onclick={() => void act(() => time.remove(record, [monday]), t('app.time.removed'))} />
                    {:else if record.source !== 'CORRECTION' && (sheet.state === 'APPROVED' || sheet.days.some((d) => d.day === record.day && d.locked))}
                      <Button size="sm" tone="subtle" onclick={() => openCorrection(record)}>{t('app.time.correct')}</Button>
                    {/if}
                  </Inline>
                </td>
              </tr>
            {/each}
          </Table>
        </section>
      {/if}
    {/if}

  {:else if current === 'approvals'}
    {#if queueRefusal}
      <Banner tone="danger">{queueRefusal}</Banner>
    {:else if !waiting}
      <div aria-busy="true"><Skeleton lines={4} /></div>
    {:else if waiting.length === 0}
      <ProductEmpty kind="settled" icon="check" title={t('app.time.queue_empty')} description={t('app.time.queue_empty_body')} />
    {:else}
      <Table isStacked label={t('app.time.queue')} columns={queueColumns}>
        {#each waiting as each (`${each.account_id}:${each.week_start}`)}
          <tr>
            <th scope="row">{personName(each.account_id)}</th>
            <td>{t('app.time.week_of', { from: formatPlainDay(each.week_start, messages.locale), to: formatPlainDay(addDays(each.week_start, 6), messages.locale) })}</td>
            <td class="number">{minutesText(each.submitted_minutes)}</td>
            <td>
              <Inline gap="050">
                <Button size="sm" tone="secondary" onclick={() => (deciding = each)}>{t('app.time.open_week')}</Button>
                <Button size="sm" icon="check" onclick={() => void approve(each)}>{t('app.time.approve')}</Button>
              </Inline>
            </td>
          </tr>
        {/each}
      </Table>
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
          <Table isStacked label={t('app.time.grid', { minutes: minutesText(decided.minutes) })} isLabelHidden columns={decidedColumns}>
            {#each decidedGrid as row (row.key)}
              <tr>
                <th scope="row">{row.entryId ? row.title || t('app.time.entry_unreadable') : t('app.time.no_entry')}</th>
                <td>{kindName(row.workType)}</td>
                {#each row.minutes as minutes, i (decidedDays[i])}<td class="number">{minutes === 0 ? '' : minutesText(minutes)}</td>{/each}
                <td class="number strong">{minutesText(row.total)}</td>
              </tr>
            {/each}
          </Table>
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
      <Banner tone="danger">{reportRefusal}</Banner>
    {:else if !answer}
      <div aria-busy="true"><Skeleton lines={4} /></div>
    {:else if answer.groups.length === 0}
      <ProductEmpty kind="settled" icon="clock" title={t('app.time.report_empty')} description={t('app.time.report_empty_body')} />
    {:else}
      <h2 class="heading">{t('app.time.report_total', { minutes: minutesText(answer.minutes) })}</h2>
      {#if answer.withheld_groups > 0}
        <p class="quiet">{t('app.time.withheld', { groups: String(answer.withheld_groups), minutes: minutesText(answer.withheld_minutes) })}</p>
      {/if}
      <Table isStacked label={t('app.time.tab_report')} isLabelHidden columns={reportColumns}>
        {#each answer.groups as group, i (i)}
          <tr>
            {#each group.keys as key, k (k)}
              {#if k === 0}<th scope="row">{keyLabel(key, names)}</th>{:else}<td>{keyLabel(key, names)}</td>{/if}
            {/each}
            <td class="number">
              <span class="share">
                <span class="bar" aria-hidden="true" style:inline-size={`${Math.round((group.minutes / reportPeak) * 100)}%`}></span>
                <span>{minutesText(group.minutes)}</span>
              </span>
            </td>
            <td class="number">{minutesText(group.billable_minutes)}</td>
            <td class="number">{group.records}</td>
          </tr>
        {/each}
      </Table>
    {/if}
  {:else if current === 'absences'}
    <AbsenceCalendar />
  {:else if current === 'capacity'}
    <CapacityEditor />
  {:else if current === 'settings'}
    <TimeSettingsView isEmbedded />
  {/if}
</Stack>

{#if isRecording}
  <Dialog bind:isOpen={isRecording} title={editing ? t('app.time.change') : t('app.time.add')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Select label={t('app.time.col_day')} bind:value={draft.day} options={days.map((day) => ({ value: day, label: dayHeading(day, locale) }))} />
      <Select label={t('app.time.col_entry')} bind:value={draft.entry} options={entryChoices} />
      <Select label={t('app.time.work_type')} bind:value={draft.kind}
        options={(editing && !offeredKinds.some((k) => k.key === draft.kind) ? [{ value: draft.kind, label: kindName(draft.kind) }] : []).concat(offeredKinds.map((each) => ({ value: each.key, label: each.name })))} />
      <Input label={t('app.time.col_minutes')} hint={t('app.time.minutes_hint')} bind:value={draft.minutes} error={draftError} />
      <Textarea label={t('app.time.col_note')} bind:value={draft.note} maxlength={2000} />
      <Checkbox label={t('app.time.billable')} checked={draft.billable}
        onchange={(event: Event) => { draft.billable = (event.currentTarget as HTMLInputElement).checked; draft.isBillableTouched = true; }} />
      {#if dialogFailure}<Banner tone="danger">{dialogFailure}</Banner>{/if}
      <Inline justify="end"><Button onclick={() => void saveRecord()}>{editing ? t('app.time.save') : t('app.time.record')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

{#if isCorrecting && correcting}
  <Dialog bind:isOpen={isCorrecting} title={t('app.time.correct_title', { minutes: minutesText(correcting.minutes), day: formatPlainDay(correcting.day, messages.locale) })} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <p class="quiet">{t('app.time.correct_hint')}</p>
      <Input label={t('app.time.col_day')} type="date" bind:value={correction.day} />
      <Input label={t('app.time.correction_label')} hint={t('app.time.correction_hint')} bind:value={correction.minutes} error={correctionError} />
      <Textarea label={t('app.time.col_note')} bind:value={correction.note} maxlength={2000} />
      {#if dialogFailure}<Banner tone="danger">{dialogFailure}</Banner>{/if}
      <Inline justify="end"><Button onclick={() => void saveCorrection()}>{t('app.time.correct')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

{#if isRejecting && deciding}
  <Dialog bind:isOpen={isRejecting} title={t('app.time.send_back_title', { name: personName(deciding.account_id) })} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Textarea label={t('app.time.reason')} hint={t('app.time.reason_hint')} bind:value={reason} maxlength={500} />
      {#if dialogFailure}<Banner tone="danger">{dialogFailure}</Banner>{/if}
      <Inline justify="end"><Button disabledReason={reason.trim() === '' ? t('app.time.reason_needed') : undefined} onclick={() => void reject()}>{t('app.time.send_back')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .toolbar { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .nav, .acts { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050); }
  .controls { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }
  .block { display: flex; flex-direction: column; gap: var(--sp-100); min-width: 0; }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .figure { color: var(--text-secondary); font-variant-numeric: tabular-nums; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .note { display: inline-flex; align-items: center; gap: var(--sp-025); margin-inline-start: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); }
  .number { text-align: end; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .strong { font-weight: var(--fw-medium); }
  .nowrap { white-space: nowrap; }
  .clip { max-inline-size: var(--layout-board-column); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .foot th, .foot td { border-block-start: var(--bw-hairline) solid var(--border-strong); }
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
  .add { position: absolute; inset-block-start: var(--sp-050); inset-inline-end: var(--sp-050); color: var(--text-subtle); opacity: 0; }
  button.day:hover .add, button.day:focus-visible .add { opacity: 1; }
  .share { display: inline-flex; align-items: center; justify-content: flex-end; gap: var(--sp-100); inline-size: 100%; min-inline-size: var(--sp-800); }
  .bar { display: block; max-inline-size: var(--sp-800); block-size: var(--sp-050); border-radius: var(--r-full); background: var(--accent-primary); }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .day { padding: var(--sp-050); }
    .sum { font-size: var(--fs-100); }
    .note span { display: none; }
  }
</style>
