<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The desk's *Reports* (UC-SVC-02 check 7, the navigation concept §4): how the goals of a period
  // stood, a figure per policy and goal. The period is the first chip of the filter bar and the
  // figures are read again whenever it changes - there is nothing to press. The workspace's, read
  // by a holder of STRUCTURE there (P-05); the server decides (rule 2).
  import { untrack } from 'svelte';
  import { EmptyState, FilterBar, Grid, Input, Radio, Skeleton, Stack, SummaryCard, WorkPage, type Filter } from '@hubtask/design-system/components';
  import type { SlaPolicyList, SlaReport } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { policiesPath, slaReportPath } from '../lib/data/desk.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { addDays, instantOf, todayIn } from '../lib/i18n/zone.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import ProductTabs from '../lib/product/ProductTabs.svelte';
  import { productHead } from '../lib/product/frame.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { reportRows } from '../lib/servicedesk/desk.ts';

  interface Props {
    onnavigate?: (path: string) => void;
  }
  const { onnavigate }: Props = $props();

  $effect(() => page.entitle(t('app.desk.reports_title')));
  const mayRead = $derived(holdsHere('STRUCTURE') === true);

  // Days on the reader's clock, each bound the start of its day in the reader's zone; the last
  // thirty days until a period is chosen.
  const RANGES = ['week', 'month', 'quarter'] as const;
  const DAYS: Record<(typeof RANGES)[number], number> = { week: 7, month: 30, quarter: 90 };
  let range = $state<(typeof RANGES)[number] | 'custom'>('month');
  const today = $derived(todayIn(actor.zone));
  let from = $state(addDays(todayIn(actor.zone), -30));
  let to = $state(addDays(todayIn(actor.zone), 1));
  function choose(next: string): void {
    if (next === 'custom') return;
    range = next as (typeof RANGES)[number];
    from = addDays(today, -DAYS[range]);
    to = addDays(today, 1);
  }

  let report = $state<SlaReport | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    if (!mayRead || from === '' || to === '') return;
    const path = slaReportPath(instantOf(from, '00:00', actor.zone) ?? new Date(from).toISOString(), instantOf(to, '00:00', actor.zone) ?? new Date(to).toISOString());
    return untrack(() => engine.subscribe<SlaReport>({ path }, (next) => {
      if (next.status === 'ready') { report = next.data; refusal = undefined; }
      else if (next.status === 'failed') refusal = renderProblem(next.error as never, messages).message;
    }));
  });

  const policies = untrack(() => resource<SlaPolicyList>(policiesPath));
  const policyName = (key: string) => (policies.state.status === 'ready' ? policies.state.data.data.find((each) => each.key === key)?.name : undefined) ?? key;

  let search = $state('');
  const rows = $derived(report ? reportRows(report) : []);
  const shown = $derived(rows.filter((row) => matchesSearch(search, [policyName(row.policy), t(`app.portal.goal.${row.goal}`)])));
  const rangeLabel = $derived(range === 'custom' ? t('app.desk.range_custom') : t(`app.desk.range_${range}`));
  const chips = $derived<Filter[]>([{ id: 'range', label: t('app.desk.range_chip', { range: rangeLabel }), isActive: true }]);
</script>

{#snippet tabs()}<ProductTabs module="SERVICE_DESK" current="reports" {onnavigate} />{/snippet}

<WorkPage head={productHead('SERVICE_DESK')} {tabs} refused={mayRead ? undefined : notYours}>
  <Stack gap="200">
    <FilterBar
      searchLabel={t('app.desk.reports_search')}
      clearSearchLabel={t('app.page.clear_search')}
      bind:search
      filters={chips}
      moreLabel={t('app.page.more_filters')}
      filtersLabel={t('app.page.filters', { count: '1' })}
      sheetTitle={t('app.page.filters', { count: '1' })}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      showLabel={t('app.page.show_results', { count: String(shown.length) })}
      countLabel={report ? t('app.desk.reports_found', { count: String(shown.length) }) : undefined}
    >
      {#snippet picker(chip: Filter)}
        {#if chip.id === 'range'}
          <Stack gap="150">
            <Radio label={t('app.desk.range')} bind:value={() => range, (next) => choose(next)}
              options={[...RANGES, 'custom'].map((each) => ({ value: each, label: t(`app.desk.range_${each}`) }))} />
            <Input label={t('app.desk.from')} type="date" bind:value={from} oninput={() => (range = 'custom')} />
            <Input label={t('app.desk.to')} type="date" bind:value={to} oninput={() => (range = 'custom')} />
          </Stack>
        {/if}
      {/snippet}
    </FilterBar>

    {#if refusal}
      <FailureNote problem={refusal} />
    {:else if !report}
      <div aria-busy="true"><Skeleton lines={3} /></div>
    {:else if rows.length === 0}
      <EmptyState kind="settled" title={t('app.desk.report_none')} />
    {:else if shown.length === 0}
      <EmptyState kind="filtered" title={t('app.desk.reports_filtered')} />
    {:else}
      <Grid as="ul" columns={{ compact: 1, medium: 2, large: 3 }} gap="200" aria-label={t('app.desk.reports_title')} class="cards">
        {#each shown as row (row.policy + row.goal)}
          <li data-report-row>
            <SummaryCard title={`${policyName(row.policy)} · ${t(`app.portal.goal.${row.goal}`)}`}
              figure={row.metShare !== undefined ? t('app.desk.met_share', { share: String(row.metShare) }) : '—'}
              line={t('app.desk.report_row', { met: String(row.met), breached: String(row.breached), running: String(row.running), paused: String(row.paused) })}
              state={row.breached > 0 ? { label: t('app.desk.report_breached', { count: String(row.breached) }), tone: 'danger' } : undefined} />
          </li>
        {/each}
      </Grid>
    {/if}
  </Stack>
</WorkPage>

{#snippet notYours()}<EmptyState kind="filtered" title={t('app.settings.not_yours')} />{/snippet}
