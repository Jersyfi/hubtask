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
  import { DataTable, PageHeader, Skeleton, WorkPage, type DataColumn } from '@hubtask/design-system/components';
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
  import PlaceStrip from '../lib/workspace/PlaceStrip.svelte';

  interface Props {
    /** The hub or collection whose plan this is. */
    planId: string;
    onnavigate: (path: string) => void;
  }

  const { planId, onnavigate }: Props = $props();

  const place = $derived(containers.find(planId));
  const placePath = $derived(place?.type === 'HUB' ? `/hubs/${planId}` : `/collections/${planId}`);
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

  const columns = $derived<DataColumn[]>([
    { id: 'entry', label: t('app.actuals.col_entry'), priority: 1 },
    { id: 'estimate', label: t('app.actuals.col_estimate'), align: 'end', priority: 2 },
    { id: 'actual', label: t('app.actuals.col_actual'), align: 'end', priority: 2 },
    { id: 'rolled_estimate', label: t('app.actuals.col_rolled_estimate'), align: 'end', priority: 3 },
    { id: 'rolled_actual', label: t('app.actuals.col_rolled_actual'), align: 'end', priority: 3 },
    { id: 'remaining', label: t('app.actuals.col_remaining'), align: 'end', priority: 2 },
    { id: 'variance', label: t('app.actuals.col_variance'), align: 'end', priority: 1, isStatus: true },
  ]);
  type Line = (typeof rows)[number];
</script>

<WorkPage>
  {#snippet top()}
  <PageHeader
    title={t('app.actuals.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={place
      ? {
          trail: [{ id: 'place', label: place.name, href: placePath }, { id: 'actuals', label: t('app.actuals.title') }],
          label: t('app.actuals.trail'),
          expandLabel: t('app.actuals.expand_trail'),
          onnavigate: () => onnavigate(placePath),
        }
      : undefined}
  />
  {/snippet}
  {#snippet tabs()}<PlaceStrip placeId={planId} current="time" view="actuals" {onnavigate} />{/snippet}

  {#if mayCost}<BudgetPanel {planId} /><BillingRates {planId} />{/if}

  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if !answer}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else}
    <p class="quiet">{t('app.actuals.summary', { estimate: hoursClockText(answer.estimate_hours), actual: minutesText(answer.minutes) })}</p>
    <DataTable label={t('app.actuals.title')} {columns} {rows} key={(line: Line) => line.row.entry_id}>
      {#snippet empty()}<ProductEmpty product="PLANNING" place={{ type: containers.find(planId)?.type === 'HUB' ? 'HUB' : 'COLLECTION', id: planId }} kind="unused" title={t('app.actuals.empty')} />{/snippet}
      {#snippet cell({ row, depth }: Line, column: DataColumn)}
        {#if column.id === 'entry'}<span class="indent" style:--depth={depth}><a href={`/items/${row.entry_id}`}>{row.title}</a></span>{#if row.done}<span class="note">{t('app.actuals.done')}</span>{/if}
        {:else if column.id === 'estimate'}<span class="number">{row.estimate_hours === null ? '' : hoursClockText(row.estimate_hours)}</span>
        {:else if column.id === 'actual'}<span class="number">{minutesText(row.minutes)}</span>
        {:else if column.id === 'rolled_estimate'}<span class="number">{hoursClockText(row.rolled_estimate_hours)}</span>
        {:else if column.id === 'rolled_actual'}<span class="number">{minutesText(row.rolled_minutes)}</span>
        {:else if column.id === 'remaining'}<span class="number">{hoursClockText(row.remaining_hours)}</span>
        {:else}<span class="number" class:over={row.variance_hours > 0}>{hoursClockText(row.variance_hours)}</span>{/if}
      {/snippet}
    </DataTable>
    <p class="quiet">{t('app.actuals.units')}</p>
  {/if}
</WorkPage>

<style>
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .note { margin-inline-start: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); }
  .number { text-align: end; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .over { color: var(--text-danger); }
  .indent { display: inline-block; padding-inline-start: calc(var(--depth) * var(--sp-200)); }
</style>
