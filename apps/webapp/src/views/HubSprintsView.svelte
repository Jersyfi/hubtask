<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The tool *Plan*'s *Sprints* (the navigation concept §2.4, scene plansprints): every collection's
  // sprints gathered - the running ones as cards with their burndown, the planned ones against the
  // velocity, the collections that work without sprints, and the closed ones, committed against
  // done. A sprint belongs to its collection; this page only gathers them, and each card opens
  // the collection's own *Sprint* or *Backlog*.
  //
  // **The server counts; this page draws.** Commitment, burndown and velocity are the answers of
  // `/sprints/{id}` and `/collections/{id}/velocity`, as the sprint's own view reads them.
  import { untrack } from 'svelte';
  import { Button, DataTable, EmptyState, Icon, ProgressBar, Skeleton, StateLozenge, WorkPage, type DataColumn } from '@hubtask/design-system/components';
  import type { ReleaseList, Sprint, SprintDetail, SprintList, Velocity } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { releasesPath } from '../lib/data/releases.svelte.ts';
  import ReleasesFigure from '../lib/planning/ReleasesFigure.svelte';
  import { burndownLine, burndownTop, planLoad, sprintDays, sprintPath, sprintsPath, valueOf, velocityPath } from '../lib/data/sprints.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { formatShortDay } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { todayIn } from '../lib/i18n/zone.ts';
  import { sprintsRunning } from '../lib/product/home.ts';
  import { sprintDaysLeft } from '../lib/today.ts';
  import ToolHead from '../lib/workspace/ToolHead.svelte';

  interface Props {
    hubId: string;
    onnavigate: (path: string) => void;
  }
  const { hubId, onnavigate }: Props = $props();

  /** More than a hub runs at once; the rest stand in their collections. */
  const SHOWN = 50;
  /** The closed sprints the table keeps: the latest, twice the three a velocity is read from. */
  const CLOSED = 6;

  const hub = $derived(containers.find(hubId));
  $effect(() => frame.entitle(hub ? t('app.planning.title_of', { place: hub.name }) : t('app.planning.title')));
  $effect(() => {
    const at = hubId;
    return untrack(() => containers.openLevel(at));
  });

  const collections = $derived(containers.collectionsOf(hubId));
  const key = $derived(collections.map((each) => each.id).join(','));
  let lists = $state<Record<string, SprintList['data']>>({});
  let runs = $state<Record<string, boolean>>({});
  let velocities = $state<Record<string, Velocity>>({});
  $effect(() => {
    const ids = key === '' ? [] : key.split(',');
    return untrack(() => {
      const stops = ids.flatMap((id) => [
        engine.subscribe<SprintList>({ path: sprintsPath(id) }, (next) => {
          if (next.status === 'ready') {
            lists = { ...lists, [id]: next.data.data };
            runs = { ...runs, [id]: next.data.runs_sprints };
          }
          if (next.status === 'failed') lists = { ...lists, [id]: [] };
        }),
        engine.subscribe<Velocity>({ path: velocityPath(id) }, (next) => {
          if (next.status === 'ready') velocities = { ...velocities, [id]: next.data };
        }),
      ]);
      return () => stops.forEach((stop) => stop());
    });
  });
  const ids = $derived(collections.map((each) => each.id));
  const isReading = $derived(containers.isLevelLoading(hubId) || ids.some((id) => lists[id] === undefined));
  const running = $derived(sprintsRunning(lists, ids, SHOWN));
  const planned = $derived(
    ids.flatMap((id) => (lists[id] ?? []).filter((each) => each.state === 'PLANNED')).sort((a, b) => a.start_on.localeCompare(b.start_on)),
  );
  const closed = $derived(
    ids.flatMap((id) => (lists[id] ?? []).filter((each) => each.state === 'CLOSED')).sort((a, b) => b.end_on.localeCompare(a.end_on)).slice(0, CLOSED),
  );
  const without = $derived(collections.filter((each) => lists[each.id] !== undefined && runs[each.id] === false));
  const today = $derived(todayIn(actor.zone));

  // The releases, for the head sentence the tool's *Releases* tab says too.
  let releaseList = $state<ReleaseList['data']>([]);
  $effect(() => {
    const path = releasesPath(hubId);
    return untrack(() => engine.subscribe<ReleaseList>({ path }, (next) => { if (next.status === 'ready') releaseList = next.data.data; }));
  });
  const hasFigure = $derived(releaseList.length > 0 || running.length > 0);

  // The running and planned sprints' own reads: their entries for what is done and planned, the burndown.
  const detailKey = $derived([...running, ...planned].map((each) => each.id).join(','));
  let details = $state<Record<string, SprintDetail>>({});
  $effect(() => {
    const wanted = detailKey === '' ? [] : detailKey.split(',');
    const zone = actor.zone;
    return untrack(() => {
      const stops = wanted.map((id) => engine.subscribe<SprintDetail>({ path: sprintPath(id, zone) }, (next) => {
        if (next.status === 'ready') details = { ...details, [id]: next.data };
      }));
      return () => stops.forEach((stop) => stop());
    });
  });

  const nameOf = (collectionId: string) => containers.find(collectionId)?.name ?? '';
  const span = (sprint: Sprint) => t('app.sprints.span_short', { from: formatShortDay(sprint.start_on, messages.locale), to: formatShortDay(sprint.end_on, messages.locale) });
  const unitOf = (sprint: Sprint) => t(`app.sprints.unit_${sprint.unit}`);
  const inSprint = (sprint: Sprint) => (details[sprint.id]?.entries ?? []).filter((each) => !each.removed_at);
  const doneValue = (sprint: Sprint) => valueOf(inSprint(sprint).filter((each) => each.done), sprint.unit);
  const plannedValue = (sprint: Sprint) => valueOf(inSprint(sprint), sprint.unit);
  /** The velocity the closed table names: only where one collection has one, so it is never a mix. */
  const velocityShown = $derived.by(() => {
    const read = Object.values(velocities).filter((each) => each.average > 0);
    return read.length === 1 ? read[0] : undefined;
  });
  /** The longest bar of the closed table: the most any of them committed or did. */
  const closedColumns = $derived<DataColumn[]>([
    { id: 'sprint', label: t('app.hubsprints.col_sprint'), priority: 1 },
    { id: 'collection', label: t('app.hubsprints.col_collection'), priority: 2 },
    { id: 'committed', label: t('app.hubsprints.col_committed'), align: 'end', priority: 2 },
    { id: 'done', label: t('app.hubsprints.col_done'), align: 'end', priority: 2 },
    { id: 'compare', label: t('app.hubsprints.col_compare'), isLabelHidden: true, priority: 3 },
  ]);
  const closedTop = $derived(Math.max(1, ...closed.map((each) => Math.max(each.committed_value, each.completed_value))));
</script>

{#snippet burndown(sprint: Sprint)}
  {@const detail = details[sprint.id]}
  {@const top = detail ? burndownTop(sprint, detail.burndown) : 0}
  {#if detail && top > 0}
    <figure class="burn" data-burndown>
      <svg viewBox="0 0 260 64" preserveAspectRatio="none" role="img" aria-label={t('app.hubsprints.burndown_of', { name: sprint.name })}>
        <polyline class="ideal" points={burndownLine(detail.burndown, 'ideal', sprintDays(sprint), top, 260, 64, 4)} />
        <polyline class="remaining" points={burndownLine(detail.burndown, 'remaining', sprintDays(sprint), top, 260, 64, 4)} />
      </svg>
      <figcaption>{t('app.hubsprints.burndown_caption')}</figcaption>
    </figure>
  {/if}
{/snippet}

{#snippet headFigure()}<ReleasesFigure releases={releaseList} running={running[0]} {today} />{/snippet}
<WorkPage>
  {#snippet top()}<ToolHead {hubId} tool="plan" current="sprints" {onnavigate} figure={hasFigure ? headFigure : undefined} />{/snippet}

  {#if isReading && running.length === 0 && planned.length === 0 && closed.length === 0}
    <div aria-busy="true"><Skeleton lines={3} /></div>
  {:else if running.length === 0 && planned.length === 0 && closed.length === 0 && without.length === 0}
    <EmptyState kind="unused" title={t('app.sprints.none_title')} description={t('app.sprints.none_hint')} />
  {:else}
    <div class="sections">
      {#if running.length > 0}
        <section aria-labelledby="hub-sprints-running" class="section">
          <h2 class="label" id="hub-sprints-running">{t('app.hubsprints.running')}</h2>
          {#each running as sprint (sprint.id)}
            {@const velocity = velocities[sprint.collection_id]}
            <article class="card" data-running-sprint={sprint.id} aria-label={sprint.name}>
              <div class="card-head">
                <div class="what">
                  <span class="line"><StateLozenge size="sm" tone="info" label={t('app.sprints.state_ACTIVE')} /><span class="quiet">{nameOf(sprint.collection_id)} · {span(sprint)}</span></span>
                  <h3 class="name">{sprint.name}</h3>
                  {#if sprint.goal}<span class="quiet">{t('app.hubsprints.goal', { goal: sprint.goal })}</span>{/if}
                </div>
                <Button tone="secondary" icon="arrow-right" onclick={() => onnavigate(`/collections/${sprint.collection_id}/sprint`)}>{t('app.sprints.open_sprint')}</Button>
              </div>
              <div class="figures">
                <dl>
                  <div><dt>{t('app.hubsprints.left')}</dt><dd>{t('app.hubsprints.days', { count: String(sprintDaysLeft(sprint.end_on, today)) })}</dd></div>
                  <div><dt>{t('app.hubsprints.done')}</dt><dd>{t('app.hubsprints.done_of', { done: String(doneValue(sprint)), all: String(sprint.committed_value), unit: unitOf(sprint) })}</dd></div>
                  <div><dt>{t('app.hubsprints.entries')}</dt><dd>{inSprint(sprint).length}</dd></div>
                  {#if velocity && velocity.average > 0}
                    <div><dt>{t('app.hubsprints.velocity')}</dt><dd>{t('app.hubsprints.value', { value: String(velocity.average), unit: t(`app.sprints.unit_${velocity.unit}`) })}</dd></div>
                  {/if}
                </dl>
                {@render burndown(sprint)}
              </div>
            </article>
          {/each}
        </section>
      {/if}

      {#if planned.length > 0 || without.length > 0}
        <section aria-labelledby="hub-sprints-next" class="section">
          <h2 class="label" id="hub-sprints-next">{t('app.hubsprints.next')}</h2>
          {#each planned as sprint (sprint.id)}
            {@const velocity = velocities[sprint.collection_id]}
            {@const load = velocity ? planLoad(plannedValue(sprint), velocity.average) : undefined}
            <article class="card row" data-planned-sprint={sprint.id} aria-label={sprint.name}>
              <div class="what">
                <span class="line"><StateLozenge size="sm" tone="neutral" label={t('app.sprints.state_PLANNED')} /><span class="quiet">{nameOf(sprint.collection_id)} · {span(sprint)}</span></span>
                <h3 class="name small">{sprint.name}</h3>
              </div>
              {#if velocity && load}
                <div class="load" data-load>
                  <span>{t('app.hubsprints.planned_load', { planned: String(plannedValue(sprint)), velocity: String(velocity.average), unit: unitOf(sprint) })}</span>
                  <ProgressBar size="sm" label={t('app.hubsprints.planned_of', { name: sprint.name })} value={load.percent} />
                </div>
              {/if}
              <Button tone="subtle" onclick={() => onnavigate(`/collections/${sprint.collection_id}/backlog`)}>{t('app.hubsprints.plan_in_backlog')}</Button>
            </article>
          {/each}
          {#each without as collection (collection.id)}
            <article class="card row quiet-card" data-without-sprints={collection.id}>
              <Icon name="square-check" size="sm" />
              <span class="what"><span><b>{collection.name}</b> <span class="quiet">{t('app.hubsprints.without')}</span></span></span>
              <Button tone="subtle" onclick={() => onnavigate(`/collections/${collection.id}/sprint`)}>{t('app.hubsprints.turn_on')}</Button>
            </article>
          {/each}
        </section>
      {/if}

      {#if closed.length > 0}
        <section aria-labelledby="hub-sprints-closed" class="card closed-card">
          <div class="closed-head">
            <h2 class="label" id="hub-sprints-closed">{t('app.hubsprints.closed')}</h2>
            {#if velocityShown}<span class="quiet">{t('app.sprints.velocity', { value: String(velocityShown.average), unit: t(`app.sprints.unit_${velocityShown.unit}`) })}</span>{/if}
          </div>
          <div data-closed-sprints>
            <DataTable label={t('app.hubsprints.closed')} columns={closedColumns} rows={closed}
              key={(sprint: Sprint) => sprint.id} rowLabel={(sprint: Sprint) => sprint.name}>
              {#snippet cell(sprint: Sprint, column: DataColumn)}
                {#if column.id === 'sprint'}<span data-closed-sprint={sprint.id}>{sprint.name}</span>
                {:else if column.id === 'collection'}<span class="quiet">{nameOf(sprint.collection_id)}</span>
                {:else if column.id === 'committed'}<span class="number" data-committed>{sprint.committed_value}</span>
                {:else if column.id === 'done'}<span class="number" data-done>{sprint.completed_value}</span>
                {:else}
                  <span class="versus" role="img" aria-label={t('app.hubsprints.committed_vs_done', { committed: String(sprint.committed_value), done: String(sprint.completed_value) })}>
                    <!-- design-system-lint-ignore: a bar's length is its value's share of the longest. -->
                    <i class="committed" style:inline-size={`${(sprint.committed_value / closedTop) * 100}%`}></i>
                    <!-- design-system-lint-ignore: a bar's length is its value's share of the longest. -->
                    <i class="achieved" style:inline-size={`${(sprint.completed_value / closedTop) * 100}%`}></i>
                  </span>
                {/if}
              {/snippet}
            </DataTable>
          </div>
        </section>
      {/if}
    </div>
  {/if}
</WorkPage>

<style>
  .sections { display: flex; flex-direction: column; gap: var(--sp-300); }
  .section { display: flex; flex-direction: column; gap: var(--sp-150); }
  .label { margin: 0; color: var(--text-secondary); font-size: var(--fs-065); font-weight: var(--fw-semibold); text-transform: uppercase; }
  .card { display: flex; flex-direction: column; gap: var(--sp-200); padding: var(--sp-250) var(--sp-300); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); min-inline-size: 0; }
  .card.row { flex-direction: row; flex-wrap: wrap; align-items: center; }
  .quiet-card { border-style: dashed; background: transparent; }
  .closed-card { gap: var(--sp-150); padding: var(--sp-200); }
  .card-head { display: flex; flex-wrap: wrap; align-items: flex-start; gap: var(--sp-150); }
  .what { display: flex; flex: 1; flex-direction: column; gap: var(--sp-050); min-inline-size: 0; }
  .line { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .name { margin: 0; font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  .name.small { font-size: var(--fs-200); }
  .quiet { color: var(--text-secondary); font-size: var(--fs-075); }
  .figures { display: flex; flex-wrap: wrap; align-items: flex-end; gap: var(--sp-300); }
  dl { display: flex; flex: 1; flex-wrap: wrap; gap: var(--sp-400); margin: 0; }
  dt { color: var(--text-subtle); font-size: var(--fs-075); }
  dd { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .burn { display: flex; flex-direction: column; align-items: flex-end; gap: var(--sp-050); margin: 0; }
  .burn svg { inline-size: var(--layout-field-md); block-size: var(--sp-600); }
  .burn figcaption { color: var(--text-subtle); font-size: var(--fs-075); }
  .ideal { fill: none; stroke: var(--text-subtle); stroke-width: var(--bw-thick); stroke-dasharray: var(--sp-050) var(--sp-050); vector-effect: non-scaling-stroke; }
  .remaining { fill: none; stroke: var(--accent-primary); stroke-width: var(--bw-thick); vector-effect: non-scaling-stroke; }
  .load { display: flex; flex-direction: column; gap: var(--sp-050); inline-size: var(--layout-field-md); max-inline-size: 100%; font-size: var(--fs-075); }
  .closed-head { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-150); }
  .number { font-variant-numeric: tabular-nums; }
  .versus { display: flex; flex-direction: column; gap: var(--sp-025); min-inline-size: var(--layout-field-sm); }
  .versus i { display: block; block-size: var(--sp-050); border-radius: var(--r-full); }
  .committed { background: var(--text-subtle); }
  .achieved { background: var(--accent-primary); }
</style>
