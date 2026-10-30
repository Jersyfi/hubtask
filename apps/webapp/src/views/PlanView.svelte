<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Plan* of a hub or a collection (M2, UC-PLN-02 … 05): the schedule as a Gantt chart and as a
  // table, side by side in one screen.
  //
  // **The server schedules; this screen draws and writes.** Every date is the one
  // `GET /plans/{id}/schedule` answered; a drag, a mode, a milestone, a dependency's timing or an
  // accepted move is a write, and the schedule is read again after it. A client that re-scheduled
  // would be a second scheduler, and two schedulers disagree.
  //
  // **The table is the keyboard path** (UC-PLN-02 check 7, SC 2.5.7): every row has its mode, its
  // milestone mark and its proposed move as real controls, every dependency its kind and lag, and
  // the entry's own screen - one press on its title - sets its dates. The chart is the picture of
  // the same answer, never the only way to change it.
  //
  // **What the reader may never do is not drawn** (P-05): without WRITE_ITEMS at the plan the
  // chart's bars are locked and the table shows values, not controls.

  import { untrack } from 'svelte';
  import {
    Banner, Button, Checkbox, Dialog, Inline, Input, PageHeader, Select, Skeleton, Stack, Table, Timeline,
    type Column, type TimelineCarry, type TimelineDependency, type TimelineLink,
  } from '@hubtask/design-system/components';
  import type { BaselineComparison, PlanBaselineList, PlanSchedule } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { dueInputFor, shift, type Window } from '../lib/data/due.ts';
  import { engine } from '../lib/data/engine.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import {
    baselinesPath, dependencyDrawn, ganttRows, lagOf, planning, proposedMoves, schedulePath,
  } from '../lib/data/planning.svelte.ts';
  import { anchorOf, draggedTo, gridOf, scaleOf, windowOf, type Scale } from '../lib/data/schedule.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { firstWeekdayOf } from '../lib/i18n/week.ts';
  import { dateIn, instantOf, todayIn } from '../lib/i18n/zone.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    /** The hub or collection the plan is. */
    planId: string;
    onnavigate: (path: string) => void;
  }

  const { planId, onnavigate }: Props = $props();

  const zone = $derived(actor.zone);
  const today = $derived(todayIn(zone));
  const place = $derived(containers.find(planId));
  const placePath = $derived(place?.type === 'HUB' ? `/hubs/${planId}` : `/collections/${planId}`);
  const rolePath = $derived(
    place?.type === 'HUB' ? { hubId: planId } : { hubId: place?.parent_id ?? undefined, collectionId: planId },
  );

  $effect(() => frame.entitle(place ? t('app.planning.title_of', { place: place.name }) : t('app.planning.title')));
  $effect(() => untrack(() => workspace.open()));
  $effect(() => {
    const path = rolePath;
    untrack(() => people.open(path));
  });

  const mayWrite = $derived(holds(people.roleOf(rolePath, actor.account?.id, roleOrder()) ?? undefined, 'WRITE_ITEMS').status === 'permitted');

  let schedule = $state<PlanSchedule | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  let baselines = $state<PlanBaselineList['data']>([]);
  $effect(() => {
    const path = schedulePath(planId, zone);
    const listed = baselinesPath(planId);
    return untrack(() => {
      const stops = [
        engine.subscribe<PlanSchedule>({ path }, (next) => {
          if (next.status === 'ready') {
            schedule = next.data;
            refusal = undefined;
          }
          if (next.status === 'failed') refusal = renderProblem(next.error, messages).message;
        }),
        engine.subscribe<PlanBaselineList>({ path: listed }, (next) => {
          if (next.status === 'ready') baselines = next.data.data;
        }),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });

  let baselineId = $state('');
  let comparison = $state<BaselineComparison | undefined>(undefined);
  $effect(() => {
    const chosen = baselineId;
    comparison = undefined;
    if (!chosen) return;
    void planning.compare(chosen).then(
      (read) => { if (baselineId === chosen) comparison = read; },
      (error) => { failure = renderProblem(error as never, messages).message; },
    );
  });

  const rows = $derived(schedule ? ganttRows(schedule, comparison, (instant) => dateIn(instant, zone)) : []);
  const byId = $derived(new Map(rows.map((row) => [row.id, row])));
  const titleOf = (id: string) => byId.get(id)?.title ?? t('app.planning.unseen');
  const proposed = $derived(schedule ? proposedMoves(schedule) : []);
  let accepted = $state<string[]>([]);
  const broken = $derived(new Set((schedule?.conflicts ?? []).map((each) => `${each.predecessor_id}>${each.successor_id}`)));

  const timelineRows = $derived(
    rows.map((row) => ({
      id: row.id,
      title: row.title,
      start: row.start,
      due: row.due,
      kind: row.kind,
      isCritical: row.isCritical,
      depth: row.depth,
      baseline: row.baseline,
      isLocked: !mayWrite || row.entry.external || row.entry.summary || row.entry.done,
    })),
  );

  const dependencies = $derived<TimelineDependency[]>(
    (schedule?.dependencies ?? []).map((each) => {
      const isEarly = broken.has(`${each.predecessor_id}>${each.successor_id}`);
      const words = { waiting: titleOf(each.successor_id), waited: titleOf(each.predecessor_id), kind: t(`app.planning.kind_${each.dependency_kind}`), lag: String(each.lag_days) };
      return {
        id: each.link_id,
        from: each.predecessor_id,
        to: each.successor_id,
        isEarly,
        label: t(isEarly ? 'app.planning.dependency_broken' : 'app.planning.dependency', words),
      };
    }),
  );

  // The window, as the timeline's: it opens where the work is, and the reader moves it.
  const weekStart = $derived(firstWeekdayOf(actor.weekStart, messages.locale, manifest.supportedLocales, workspace.weekStartAbove));
  let scale = $state<Scale>('week');
  let shown = $state<Window | undefined>(undefined);
  let anchor = $state<string | undefined>(undefined);
  $effect(() => {
    if (anchor !== undefined || !schedule) return;
    const dated = untrack(() => rows.flatMap((row) => [row.start, row.due]));
    anchor = anchorOf(dated, today, untrack(() => scale), weekStart);
  });
  const current = $derived(shown ?? windowOf(anchor ?? today, scale, weekStart));

  const partsFor = (of: Scale): Intl.DateTimeFormatOptions =>
    of === 'day'
      ? { day: 'numeric', timeZone: 'UTC' }
      : of === 'week'
        ? { day: 'numeric', month: 'short', timeZone: 'UTC' }
        : of === 'year'
          ? { year: 'numeric', timeZone: 'UTC' }
          : { month: 'short', year: of === 'quarter' ? '2-digit' : undefined, timeZone: 'UTC' };
  const gridlines = $derived.by(() => {
    const format = new Intl.DateTimeFormat(messages.locale, partsFor(scale));
    return gridOf(current, scale, weekStart).map((at) => ({ at, label: format.format(Date.parse(`${at}T00:00:00Z`)) }));
  });
  const dayLabel = (day: string | null | undefined) =>
    day ? new Intl.DateTimeFormat(messages.locale, { dateStyle: 'medium', timeZone: 'UTC' }).format(Date.parse(`${day}T00:00:00Z`)) : '—';

  function move(direction: 1 | -1) {
    shown = shift(current, direction);
    announcer.say(t('app.timeline.moved', { from: current.from, to: current.to }));
  }

  let failure = $state<string | undefined>(undefined);

  async function act(run: () => Promise<unknown>, done?: string) {
    failure = undefined;
    try {
      await run();
      if (done) toast.show({ text: done, tone: 'success' });
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }

  /** A bar dragged: its new dates written as the entry's own, then the plan read again. */
  function carried(carry: TimelineCarry) {
    const row = byId.get(carry.id);
    if (!row || !mayWrite || carry.grab === 'place') return;
    const next = draggedTo({ start: row.start, due: row.due }, carry.grab, carry.to - carry.from);
    const start = next.start !== row.start && next.start ? (instantOf(next.start, undefined, zone) ?? undefined) : undefined;
    const due = next.due !== row.due && next.due ? dueInputFor({ date: next.due }, zone) : undefined;
    if (start === undefined && due === undefined) return;
    void act(() => planning.moveEntry(planId, zone, row.id, start, due), t('app.timeline.placed_announced', { title: row.title }));
  }

  /** A dependency drawn from one bar onto another row. */
  function linked(link: TimelineLink) {
    const drawn = dependencyDrawn(link.from, link.to);
    if (!drawn || !mayWrite) return;
    void act(
      () => planning.link(planId, zone, drawn.sourceId, drawn.targetId, drawn.type),
      t('app.planning.linked', { waiting: titleOf(link.to), waited: titleOf(link.from) }),
    );
  }

  function applyMoves(ids: readonly string[]) {
    if (ids.length === 0) return;
    void act(async () => {
      const out = await planning.apply(planId, zone, ids);
      accepted = [];
      announcer.say(t('app.planning.applied', { count: String(out.moved.length) }));
    }, t('app.planning.applied', { count: String(ids.length) }));
  }

  /** The draft of each dependency's timing, by link, until it is saved. */
  const timing = $state<Record<string, { kind: string; lag: string }>>({});
  $effect(() => {
    for (const each of schedule?.dependencies ?? []) {
      untrack(() => {
        timing[each.link_id] ??= { kind: each.dependency_kind, lag: String(each.lag_days) };
      });
    }
  });
  function saveTiming(linkId: string) {
    const draft = timing[linkId];
    const lag = draft ? lagOf(draft.lag) : undefined;
    if (!draft || lag === undefined) {
      failure = t('planning.lag_out_of_range', { maximum: '999' });
      return;
    }
    void act(() => planning.setTiming(planId, zone, linkId, draft.kind as 'FS' | 'SS' | 'FF' | 'SF', lag), t('app.planning.timing_saved'));
  }

  let isTaking = $state(false);
  let baselineName = $state('');
  function take() {
    const name = baselineName.trim();
    if (!name) return;
    isTaking = false;
    baselineName = '';
    void act(() => planning.takeBaseline(planId, name), t('app.planning.baseline_taken', { name }));
  }

  const scales = $derived(
    (['day', 'week', 'month', 'quarter', 'year'] as const).map((value) => ({ value, label: t(`app.planning.scale_${value}`) })),
  );
  const modes = $derived([
    { value: 'MANUAL', label: t('app.planning.mode_MANUAL') },
    { value: 'AUTOMATIC', label: t('app.planning.mode_AUTOMATIC') },
  ]);
  const kinds = $derived((['FS', 'SS', 'FF', 'SF'] as const).map((value) => ({ value, label: t(`app.planning.kind_${value}`) })));
  const columns = $derived<Column[]>([
    { id: 'entry', label: t('app.planning.col_entry') },
    { id: 'start', label: t('app.planning.col_start') },
    { id: 'finish', label: t('app.planning.col_finish') },
    { id: 'duration', label: t('app.planning.col_duration'), align: 'end' },
    { id: 'float', label: t('app.planning.col_float'), align: 'end' },
    { id: 'critical', label: t('app.planning.col_critical') },
    ...(comparison ? [{ id: 'slip', label: t('app.planning.col_slip'), align: 'end' as const }] : []),
    { id: 'mode', label: t('app.planning.col_mode') },
    { id: 'milestone', label: t('app.planning.col_milestone') },
    { id: 'move', label: t('app.planning.col_move') },
  ]);
  const dependencyColumns = $derived<Column[]>([
    { id: 'waiting', label: t('app.planning.col_waiting') },
    { id: 'waited', label: t('app.planning.col_waited') },
    { id: 'kind', label: t('app.planning.col_kind') },
    { id: 'lag', label: t('app.planning.col_lag') },
    { id: 'save', label: t('app.planning.col_save'), isLabelHidden: true },
  ]);
  const slipOf = (id: string) => comparison?.entries.find((each) => each.item_id === id)?.finish_variance_days;
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.planning.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={place
      ? {
          trail: [{ id: 'place', label: place.name, href: placePath }, { id: 'plan', label: t('app.planning.title') }],
          label: t('app.planning.trail'),
          expandLabel: t('app.planning.expand_trail'),
          onnavigate: () => onnavigate(placePath),
        }
      : undefined}
    secondary={mayWrite ? [{ label: t('app.planning.take_baseline'), icon: 'flag', onclick: () => (isTaking = true) }] : []}
  />

  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if !schedule}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else}
    <p class="quiet">
      {schedule.finish ? t('app.planning.ends_on', { date: dayLabel(schedule.finish) }) : t('app.planning.no_dates')}
    </p>

    {#if proposed.length > 0}
      <Banner tone="info" title={t('app.planning.proposes', { count: String(proposed.length) })}>
        {t('app.planning.proposes_hint')}
        {#snippet action()}
          {#if mayWrite}
            <Button size="sm" onclick={() => applyMoves(proposed)}>{t('app.planning.accept_all')}</Button>
          {/if}
        {/snippet}
      </Banner>
    {/if}

    {#if schedule.conflicts.length > 0}
      <section aria-labelledby="plan-conflicts">
        <h2 id="plan-conflicts" class="heading">{t('app.planning.conflicts')}</h2>
        <ul class="plain">
          {#each schedule.conflicts as conflict (`${conflict.predecessor_id}>${conflict.successor_id}`)}
            <li>
              {t('app.planning.conflict', {
                waiting: titleOf(conflict.successor_id),
                waited: titleOf(conflict.predecessor_id),
                date: dayLabel(conflict.earliest),
              })}
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

    <div class="controls" role="group" aria-label={t('app.planning.chart')}>
      <Button size="sm" tone="secondary" icon="chevron-left" onclick={() => move(-1)}>{t('app.timeline.previous')}</Button>
      <Button size="sm" tone="secondary" onclick={() => { anchor = today; shown = undefined; }}>{t('app.timeline.today')}</Button>
      <Button size="sm" tone="secondary" icon="chevron-right" onclick={() => move(1)}>{t('app.timeline.next')}</Button>
      <Select
        label={t('app.planning.zoom')}
        size="sm"
        value={scale}
        options={scales}
        onchange={(event: Event) => { scale = scaleOf((event.currentTarget as HTMLSelectElement).value); shown = undefined; }}
      />
      <Select
        label={t('app.planning.baseline')}
        size="sm"
        value={baselineId}
        options={[{ value: '', label: t('app.planning.baseline_none') }, ...baselines.map((each) => ({ value: each.id, label: each.name }))]}
        onchange={(event: Event) => (baselineId = (event.currentTarget as HTMLSelectElement).value)}
      />
      {#if baselineId && mayWrite}
        <Button size="sm" tone="subtle" icon="trash" onclick={() => { const gone = baselineId; baselineId = ''; void act(() => planning.deleteBaseline(planId, gone), t('app.planning.baseline_deleted')); }}>
          {t('app.planning.baseline_delete')}
        </Button>
      {/if}
      <span class="quiet">{t('app.timeline.window', { from: current.from, to: current.to })}</span>
    </div>

    <div class="window">
      <Timeline
        label={t('app.planning.chart')}
        from={current.from}
        to={current.to}
        {scale}
        rows={timelineRows}
        {gridlines}
        {today}
        todayLabel={t('app.timeline.today_is', { date: dayLabel(today) })}
        undatedLabel={t('app.timeline.undated')}
        emptyLabel={t('app.planning.empty')}
        startHandleLabel={t('app.timeline.move_start')}
        dueHandleLabel={t('app.timeline.move_due')}
        onSelect={(id) => onnavigate(`/items/${id}`)}
        onCarry={carried}
        dependencies={dependencies}
        dependenciesLabel={t('app.timeline.dependencies')}
        onLink={mayWrite ? linked : undefined}
        linkHandleLabel={t('app.planning.draw_dependency')}
      />
    </div>

    <section aria-labelledby="plan-table" class="block">
      <Inline justify="between" align="center">
        <h2 id="plan-table" class="heading">{t('app.planning.table')}</h2>
        {#if mayWrite && accepted.length > 0}
          <Button size="sm" onclick={() => applyMoves(accepted)}>{t('app.planning.accept_chosen', { count: String(accepted.length) })}</Button>
        {/if}
      </Inline>
      <Table label={t('app.planning.table')} isLabelHidden {columns}>
        {#each rows as row (row.id)}
          <tr data-depth={String(Math.min(row.depth, 4))}>
            <th scope="row" class="what">
              <a href={`/items/${row.id}`}>{row.title}</a>
              {#if row.entry.external}<span class="note">{t('app.planning.outside')}</span>{/if}
              {#if row.entry.summary}<span class="note">{t('app.planning.summary')}</span>{/if}
              {#if row.entry.done}<span class="note">{t('app.planning.done')}</span>{/if}
            </th>
            <td>{dayLabel(row.start)}{#if row.entry.moved}<span class="note">{t('app.planning.proposed')}</span>{/if}</td>
            <td>{dayLabel(row.due)}</td>
            <td class="number">{row.entry.duration_days}</td>
            <td class="number">{row.entry.total_float_days}</td>
            <td>{row.isCritical ? t('app.planning.critical') : ''}</td>
            {#if comparison}
              {@const slip = slipOf(row.id)}
              <td class="number">{slip === undefined || slip === null ? '—' : slip > 0 ? `+${slip}` : String(slip)}</td>
            {/if}
            <td>
              {#if mayWrite && !row.entry.external}
                <Select
                  label={t('app.planning.mode_of', { title: row.title })}
                  size="sm"
                  value={row.entry.mode}
                  options={modes}
                  onchange={(event: Event) => {
                    const mode = (event.currentTarget as HTMLSelectElement).value as 'MANUAL' | 'AUTOMATIC';
                    void act(() => planning.setScheduling(planId, zone, row.id, { mode }));
                  }}
                />
              {:else}
                {t(`app.planning.mode_${row.entry.mode}`)}
              {/if}
            </td>
            <td>
              {#if mayWrite && !row.entry.external}
                <Checkbox
                  label={t('app.planning.milestone_of', { title: row.title })}
                  isLabelHidden
                  checked={row.entry.milestone}
                  onchange={(event: Event) => {
                    const milestone = (event.currentTarget as HTMLInputElement).checked;
                    void act(() => planning.setScheduling(planId, zone, row.id, { milestone }));
                  }}
                />
              {:else}
                {row.entry.milestone ? t('app.planning.milestone') : ''}
              {/if}
            </td>
            <td>
              {#if mayWrite && proposed.includes(row.id)}
                <Checkbox
                  label={t('app.planning.accept_of', { title: row.title })}
                  isLabelHidden
                  checked={accepted.includes(row.id)}
                  onchange={(event: Event) => {
                    const isOn = (event.currentTarget as HTMLInputElement).checked;
                    accepted = isOn ? [...accepted, row.id] : accepted.filter((id) => id !== row.id);
                  }}
                />
              {/if}
            </td>
          </tr>
        {/each}
      </Table>
    </section>

    {#if schedule.dependencies.length > 0}
      <section aria-labelledby="plan-dependencies" class="block">
        <h2 id="plan-dependencies" class="heading">{t('app.timeline.dependencies')}</h2>
        <Table label={t('app.timeline.dependencies')} isLabelHidden columns={dependencyColumns}>
          {#each schedule.dependencies as each (each.link_id)}
            {@const draft = timing[each.link_id]}
            <tr>
              <th scope="row">{titleOf(each.successor_id)}</th>
              <td>{titleOf(each.predecessor_id)}</td>
              {#if mayWrite && draft}
                <td><Select label={t('app.planning.kind_of', { title: titleOf(each.successor_id) })} size="sm" bind:value={draft.kind} options={kinds} /></td>
                <td><Input label={t('app.planning.lag_of', { title: titleOf(each.successor_id) })} size="sm" bind:value={draft.lag} inputmode="numeric" /></td>
                <td><Button size="sm" tone="secondary" onclick={() => saveTiming(each.link_id)}>{t('app.planning.save_timing')}</Button></td>
              {:else}
                <td>{t(`app.planning.kind_${each.dependency_kind}`)}</td>
                <td>{each.lag_days}</td>
                <td></td>
              {/if}
            </tr>
          {/each}
        </Table>
      </section>
    {/if}

    {#if schedule.unscheduled.length > 0}
      <p class="quiet">{t('app.planning.unscheduled', { count: String(schedule.unscheduled.length) })}</p>
    {/if}
  {/if}
</Stack>

{#if isTaking}
  <Dialog bind:isOpen={isTaking} title={t('app.planning.take_baseline')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <p class="quiet">{t('app.planning.take_baseline_hint')}</p>
      <Input label={t('app.planning.baseline_name')} bind:value={baselineName} maxlength={120} />
      <Inline justify="end"><Button onclick={take}>{t('app.planning.take')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .controls { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }
  .window { min-width: 0; }
  .block { display: flex; flex-direction: column; gap: var(--sp-100); }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .plain { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: var(--sp-050); }
  .note { margin-inline-start: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); }
  .number { text-align: end; font-variant-numeric: tabular-nums; }
  tr[data-depth='1'] .what { padding-inline-start: var(--sp-150); }
  tr[data-depth='2'] .what { padding-inline-start: var(--sp-300); }
  tr[data-depth='3'] .what { padding-inline-start: var(--sp-400); }
  tr[data-depth='4'] .what { padding-inline-start: var(--sp-500); }
</style>
