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
  import { Banner, Button, PageHeader, Skeleton, Stack, Table, type Column } from '@hubtask/design-system/components';
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

  const columns = $derived<Column[]>([
    { id: 'person', label: t('app.workload.col_person') },
    ...weeks.map((week) => ({ id: week, label: t('app.workload.week_of', { day: week }), align: 'end' as const })),
  ]);
</script>

<Stack gap="300">
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

  <div class="controls">
    <Button size="sm" tone="subtle" icon="chevron-left" onclick={() => (shift -= 1)}>{t('app.workload.earlier')}</Button>
    <Button size="sm" tone="subtle" icon="chevron-right" onclick={() => (shift += 1)}>{t('app.workload.later')}</Button>
    {#if shift !== 0}<Button size="sm" tone="subtle" onclick={() => (shift = 0)}>{t('app.workload.now')}</Button>{/if}
  </div>

  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if !answer}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else}
    {#if answer.aggregated}<p class="quiet">{t('app.workload.aggregated')}</p>{/if}
    <div class="scroll">
      <Table label={t('app.workload.title')} isLabelHidden {columns}>
        {#snippet empty()}{t('app.workload.empty')}{/snippet}
        {#if answer.aggregated}
          {#if answer.team.length > 0}
            <tr>
              <th scope="row">{t('app.workload.team')}</th>
              {#each answer.team as week (week.week)}
                <td class="cell" class:over={week.allocated_minutes > week.available_minutes}>
                  {t('app.workload.of', { allocated: minutesText(week.allocated_minutes), available: minutesText(week.available_minutes) })}
                  <span class="note">{t('app.workload.people', { count: String(week.people) })}</span>
                </td>
              {/each}
            </tr>
          {/if}
        {:else}
          {#each grid.rows as row (row.accountId)}
            <tr>
              <th scope="row">{accounts.nameOf(row.accountId) ?? t('app.people.unnamed')}</th>
              {#each row.cells as cell, i (i)}
                {@const heat = cell ? heatOf(cell) : 'unknown'}
                <td class="cell {heat}">
                  {#if cell && cell.known}
                    {t('app.workload.of', { allocated: minutesText(cell.allocated_minutes), available: minutesText(cell.available_minutes) })}
                  {:else}
                    {t('app.workload.unknown')}
                  {/if}
                  <span class="note">{t(`app.workload.heat_${heat}`)}</span>
                </td>
              {/each}
            </tr>
          {/each}
        {/if}
      </Table>
    </div>
    {#if answer.unscheduled_minutes > 0}
      <p class="quiet">{t('app.workload.unscheduled', { minutes: minutesText(answer.unscheduled_minutes) })}</p>
    {/if}
    <p class="quiet">{t('app.workload.units')}</p>
  {/if}
</Stack>

<style>
  .controls { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: center; }
  .scroll { overflow-x: auto; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .cell { text-align: end; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .note { display: block; color: var(--text-subtle); font-size: var(--fs-075); }
  .room { background: var(--status-success-surface); color: var(--status-success-text); }
  .fit { background: var(--status-info-surface); color: var(--status-info-text); }
  .over { background: var(--status-danger-surface); color: var(--status-danger-text); }
  .unknown { background: var(--bg-surface-sunken); color: var(--text-subtle); }
</style>
