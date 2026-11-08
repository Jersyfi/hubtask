<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Workload* of a hub or a collection (UC-TIM-06 check 7): people by weeks, each week what the
  // person's open entries ask against what they can give, coloured room, about right or over - and
  // the words in the cell, so that colour is never the only sign. Where the workspace shows only
  // sums and the reader does not approve time here, one row of the team's weeks, naming nobody.
  //
  // **The server counts**: capacity, holidays, absences, the remaining estimates and their spread.

  import { untrack } from 'svelte';
  import { Button, DataTable, PageHeader, Skeleton, WorkPage, type DataColumn } from '@hubtask/design-system/components';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import type { Workload } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { addDays, heatOf, heatmap, minutesText, weekStartOf, workloadPath } from '../lib/data/time.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { todayIn } from '../lib/i18n/zone.ts';
  import { renderProblem } from '../lib/problem.ts';
  import PlaceStrip from '../lib/workspace/PlaceStrip.svelte';

  interface Props {
    /** The hub or collection whose workload this is. */
    planId: string;
    onnavigate: (path: string) => void;
  }

  const { planId, onnavigate }: Props = $props();
  const WEEKS = 8;

  const place = $derived(containers.find(planId));
  const placePath = $derived(place?.type === 'HUB' ? `/hubs/${planId}` : `/collections/${planId}`);
  $effect(() => frame.entitle(place ? t('app.workload.title_of', { place: place.name }) : t('app.workload.title')));

  const thisWeek = $derived(weekStartOf(todayIn(actor.zone)) ?? todayIn(actor.zone));
  let shift = $state(0);
  const from = $derived(addDays(thisWeek, shift * 7 * WEEKS));

  let answer = $state<Workload | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const path = workloadPath(planId, from, WEEKS);
    return untrack(() => engine.subscribe<Workload>({ path }, (next) => {
      if (next.status === 'ready') { answer = next.data; refusal = undefined; }
      if (next.status === 'failed') refusal = renderProblem(next.error, messages).message;
    }));
  });
  const grid = $derived(answer ? heatmap(answer.cells) : { weeks: [], rows: [] });
  $effect(() => { if (grid.rows.length > 0) accounts.resolve(grid.rows.map((row) => row.accountId)); });
  const weeks = $derived(answer?.aggregated ? answer.team.map((each) => each.week) : grid.weeks);

  // The first week stays on a phone as the row's state; the later ones give way (page-system §4.4).
  const columns = $derived<DataColumn[]>([
    { id: 'person', label: t('app.workload.col_person'), priority: 1 },
    ...weeks.map((week, i) => ({ id: week, label: t('app.workload.week_of', { day: week }), align: 'end' as const, priority: (i === 0 ? 1 : i < 3 ? 2 : 3) as 1 | 2 | 3, isStatus: i === 0 })),
  ]);
  /** The team's one row where the workspace aggregates, else a row per person. */
  type Line = { id: string; isTeam: boolean; index: number };
  const lines = $derived<Line[]>(
    answer?.aggregated
      ? answer.team.length > 0 ? [{ id: '__team', isTeam: true, index: 0 }] : []
      : grid.rows.map((row, index) => ({ id: row.accountId, isTeam: false, index })),
  );
</script>

<WorkPage>
  {#snippet top()}
  <PageHeader
    title={t('app.workload.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={place
      ? {
          trail: [{ id: 'place', label: place.name, href: placePath }, { id: 'workload', label: t('app.workload.title') }],
          label: t('app.workload.trail'),
          expandLabel: t('app.workload.expand_trail'),
          onnavigate: () => onnavigate(placePath),
        }
      : undefined}
  />
  {/snippet}
  {#snippet tabs()}<PlaceStrip placeId={planId} current="time" view="workload" {onnavigate} />{/snippet}

  <div class="controls">
    <Button size="sm" tone="subtle" icon="chevron-left" onclick={() => (shift -= 1)}>{t('app.workload.earlier')}</Button>
    <Button size="sm" tone="subtle" icon="chevron-right" onclick={() => (shift += 1)}>{t('app.workload.later')}</Button>
    {#if shift !== 0}<Button size="sm" tone="subtle" onclick={() => (shift = 0)}>{t('app.workload.now')}</Button>{/if}
  </div>

  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if !answer}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else}
    {#if answer.aggregated}<p class="quiet">{t('app.workload.aggregated')}</p>{/if}
    <DataTable label={t('app.workload.title')} {columns} rows={lines} key={(line: Line) => line.id}>
      {#snippet empty()}<ProductEmpty product="PLANNING" place={{ type: containers.find(planId)?.type === 'HUB' ? 'HUB' : 'COLLECTION', id: planId }} kind="unused" title={t('app.workload.empty')} />{/snippet}
      {#snippet cell(line: Line, column: DataColumn)}
        {#if column.id === 'person'}
          {line.isTeam ? t('app.workload.team') : (accounts.nameOf(grid.rows[line.index]?.accountId ?? '') ?? t('app.people.unnamed'))}
        {:else if line.isTeam && answer}
          {@const week = answer.team.find((each) => each.week === column.id)}
          {#if week}
            <span class="cell" class:over={week.allocated_minutes > week.available_minutes}>
              {t('app.workload.of', { allocated: minutesText(week.allocated_minutes), available: minutesText(week.available_minutes) })}
              <span class="note">{t('app.workload.people', { count: String(week.people) })}</span>
            </span>
          {/if}
        {:else}
          {@const at = grid.rows[line.index]?.cells[weeks.indexOf(column.id)]}
          {@const heat = at ? heatOf(at) : 'unknown'}
          <span class="cell {heat}">
            {#if at && at.known}
              {t('app.workload.of', { allocated: minutesText(at.allocated_minutes), available: minutesText(at.available_minutes) })}
            {:else}
              {t('app.workload.unknown')}
            {/if}
            <span class="note">{t(`app.workload.heat_${heat}`)}</span>
          </span>
        {/if}
      {/snippet}
    </DataTable>
    {#if answer.unscheduled_minutes > 0}
      <p class="quiet">{t('app.workload.unscheduled', { minutes: minutesText(answer.unscheduled_minutes) })}</p>
    {/if}
    <p class="quiet">{t('app.workload.units')}</p>
  {/if}
</WorkPage>

<style>
  .controls { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: center; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .cell { display: block; padding: var(--sp-050) var(--sp-100); border-radius: var(--r-sm); text-align: end; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .note { display: block; color: var(--text-subtle); font-size: var(--fs-075); }
  .room { background: var(--status-success-surface); color: var(--status-success-text); }
  .fit { background: var(--status-info-surface); color: var(--status-info-text); }
  .over { background: var(--status-danger-surface); color: var(--status-danger-text); }
  .unknown { background: var(--bg-surface-sunken); color: var(--text-subtle); }
</style>
