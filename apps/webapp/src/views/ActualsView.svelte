<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Estimates and actuals* of a hub or a collection (UC-TIM-04 checks 2 and 6): every entry the
  // reader may read with its estimate in hours, the time recorded on it, both rolled up with the
  // entries below, the hours remaining and the variance - a table in the plan's tree order.
  //
  // **The server counts**: the rolling up, the remaining hours that never fall below zero, and
  // whether an aggregated workspace lets this reader see it at all (check 4). Above it, for whoever
  // manages costs, the plan's budget and its burn (UC-TIM-08 check 8) and its billing rates (check 9).

  import { untrack } from 'svelte';
  import { PageHeader, Skeleton, Stack, Table, type Column } from '@hubtask/design-system/components';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import type { EstimatesVsActuals } from '@hubtask/sync-engine';

  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { actualsPath, actualsTree, hoursClockText, minutesText } from '../lib/data/time.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import BillingRates from '../lib/time/BillingRates.svelte';
  import BudgetPanel from '../lib/time/BudgetPanel.svelte';
  import PlaceViews from '../lib/workspace/PlaceViews.svelte';

  interface Props {
    /** The hub or collection whose plan this is. */
    planId: string;
    /** The tool *Time*'s tab of a hub: its actuals, or its budget and rates (§2.4). A collection's *Time* view holds both. */
    part?: 'actuals' | 'budget';
    onnavigate: (path: string) => void;
  }

  const { planId, part = 'actuals', onnavigate }: Props = $props();

  const place = $derived(containers.find(planId));
  const placePath = $derived(place?.type === 'HUB' ? `/hubs/${planId}` : `/collections/${planId}`);
  const isHub = $derived(place?.type === 'HUB');
  const title = $derived(isHub ? t('app.nav.tool_time') : t('app.actuals.title'));
  $effect(() => frame.entitle(place ? t('app.actuals.title_of', { place: place.name }) : t('app.actuals.title')));

  let answer = $state<EstimatesVsActuals | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const path = actualsPath(planId);
    return untrack(() => engine.subscribe<EstimatesVsActuals>({ path }, (next) => {
      if (next.status === 'ready') { answer = next.data; refusal = undefined; }
      if (next.status === 'failed') refusal = renderProblem(next.error, messages).message;
    }));
  });
  const rows = $derived(answer ? actualsTree(answer.data) : []);
  // The budget and the billing rates are MANAGE_COSTS's (UC-TIM-08 check 1): not drawn for anybody else (P-05).
  const mayCost = $derived(holdsHere('MANAGE_COSTS') === true || holdsHere('MANAGE_MEMBERS') === true);

  const columns = $derived<Column[]>([
    { id: 'entry', label: t('app.actuals.col_entry') },
    { id: 'estimate', label: t('app.actuals.col_estimate'), align: 'end' },
    { id: 'actual', label: t('app.actuals.col_actual'), align: 'end' },
    { id: 'rolled_estimate', label: t('app.actuals.col_rolled_estimate'), align: 'end' },
    { id: 'rolled_actual', label: t('app.actuals.col_rolled_actual'), align: 'end' },
    { id: 'remaining', label: t('app.actuals.col_remaining'), align: 'end' },
    { id: 'variance', label: t('app.actuals.col_variance'), align: 'end' },
  ]);
</script>

<Stack gap="300">
  <PageHeader
    {title}
    isTitleInBar={viewport.isCompact}
    secondary={place && !isHub ? [{ label: t('app.workload.title'), icon: 'users', onclick: () => onnavigate(`/collections/${planId}/workload`) }] : []}
    breadcrumb={place
      ? {
          trail: [{ id: 'place', label: place.name, href: placePath }, { id: 'actuals', label: title }],
          label: t('app.actuals.trail'),
          expandLabel: t('app.actuals.expand_trail'),
          onnavigate: () => onnavigate(placePath),
        }
      : undefined}
  />
  <PlaceViews placeId={planId} tool="time" current={isHub ? part : 'time'} {onnavigate} />

  {#if mayCost && (!isHub || part === 'budget')}<BudgetPanel {planId} /><BillingRates {planId} />{/if}

  {#if isHub && part === 'budget'}
    <!-- The budget tab holds the budget and the rates alone. -->
  {:else if refusal}
    <FailureNote problem={refusal} />
  {:else if !answer}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else}
    <p class="quiet">{t('app.actuals.summary', { estimate: hoursClockText(answer.estimate_hours), actual: minutesText(answer.minutes) })}</p>
    <div class="scroll">
      <Table label={t('app.actuals.title')} isLabelHidden {columns}>
        {#snippet empty()}<ProductEmpty product="PLANNING" place={{ type: containers.find(planId)?.type === 'HUB' ? 'HUB' : 'COLLECTION', id: planId }} kind="unused" title={t('app.actuals.empty')} />{/snippet}
        {#each rows as { row, depth } (row.entry_id)}
          <tr>
            <th scope="row"><span class="indent" style:--depth={depth}><a href={`/items/${row.entry_id}`}>{row.title}</a></span>{#if row.done}<span class="note">{t('app.actuals.done')}</span>{/if}</th>
            <td class="number">{row.estimate_hours === null ? '' : hoursClockText(row.estimate_hours)}</td>
            <td class="number">{minutesText(row.minutes)}</td>
            <td class="number">{hoursClockText(row.rolled_estimate_hours)}</td>
            <td class="number">{minutesText(row.rolled_minutes)}</td>
            <td class="number">{hoursClockText(row.remaining_hours)}</td>
            <td class="number" class:over={row.variance_hours > 0}>{hoursClockText(row.variance_hours)}</td>
          </tr>
        {/each}
      </Table>
    </div>
    <p class="quiet">{t('app.actuals.units')}</p>
  {/if}
</Stack>

<style>
  .scroll { overflow-x: auto; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .note { margin-inline-start: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); }
  .number { text-align: end; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .over { color: var(--text-danger); }
  .indent { display: inline-block; padding-inline-start: calc(var(--depth) * var(--sp-200)); }
</style>
