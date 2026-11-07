<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What the rules did, as a log page (page-system §7.6, §9.1): the time range the first chip of
  // the filter bar, the rule, the outcome and what started it the others - on a phone behind
  // *Filters (n)* -, the runs grouped by day in words, each opening its Dialog M (RunDialog), and
  // the next page at the foot. Trying and starting a rule are in its editor (ux-inventory.md §E9).
  //
  // **What the filter shows is counted** - a reader narrowing to one rule and a week reads the
  // week's numbers in the line above the runs, not the workspace's.

  import { untrack } from 'svelte';

  import {
    DataTable, EmptyState, FilterBar, Input, LogPage, Radio, RunStatusBadge, Select, Spinner, Stack,
    type DataColumn, type Filter,
  } from '@hubtask/design-system/components';

  import { adminTrail } from '../lib/administration/trail.ts';
  import RunDialog from '../lib/automation/RunDialog.svelte';
  import { isStale, statusWord, triggerWord } from '../lib/automation/runwords.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { rules } from '../lib/data/rules.svelte.ts';
  import { quotas } from '../lib/data/quotas.svelte.ts';
  import { runs, type Run } from '../lib/data/runs.svelte.ts';
  import { formatDay, formatTime } from '../lib/i18n/datetime.ts';
  import { includedHere, mayHere } from '../lib/data/capability.svelte.ts';
  import LimitSource from '../lib/workspace/LimitSource.svelte';
  import NotInPlan from '../lib/workspace/NotInPlan.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import { renderProblem } from '../lib/problem.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  $effect(() => page.entitle(t('app.runs.title')));

  /**
   * The rule's own screen links here prefiltered: `?rule_id=` in the address is the
   * filter's first value. Read once, at the open - the address is where the link put it, not a
   * store this screen writes back into.
   */
  const asked = typeof location === 'object' ? new URLSearchParams(location.search) : new URLSearchParams();
  let ruleFilter = $state(asked.get('rule_id') ?? '');
  let statusFilter = $state('');
  let triggerFilter = $state('');

  // The time range: a span back from when it was chosen, or a window typed by hand - `from`
  // inclusive, `to` exclusive, on the run's own moment.
  const SPANS: Readonly<Record<string, number>> = { day: 864e5, week: 7 * 864e5, month: 30 * 864e5 };
  let range = $state('');
  let since = $state('');
  let from = $state('');
  let to = $state('');

  function chooseRange(next: string): void {
    range = next;
    from = '';
    to = '';
    const span = SPANS[next];
    since = span ? new Date(Date.now() - span).toISOString() : '';
  }

  const reversed = $derived(from !== '' && to !== '' && new Date(to).getTime() <= new Date(from).getTime());
  const filter = $derived({
    ...(ruleFilter ? { ruleId: ruleFilter } : {}),
    ...(statusFilter ? { status: statusFilter } : {}),
    ...(triggerFilter ? { trigger: triggerFilter } : {}),
    ...(range !== 'custom' && since ? { from: since } : {}),
    ...(range === 'custom' && from && !reversed ? { from: new Date(from).toISOString() } : {}),
    ...(range === 'custom' && to && !reversed ? { to: new Date(to).toISOString() } : {}),
  });

  $effect(() => untrack(() => rules.open()));
  // The hour's standing against automation_runs_per_hour (automation.md §1.5): one line above the
  // runs and the link to Limits, from the same answer the Limits page reads. A reader the quotas
  // refuse sees no line - the standing is theirs to see or not, not this page's.
  $effect(() => untrack(() => quotas.open()));
  const hourly = $derived(quotas.standings.find((standing) => standing.quota === 'automation_runs_per_hour'));
  $effect(() => {
    const current = filter;
    return untrack(() => runs.open(current));
  });

  /** What the last press of *Show more* brought, said by `LoadMore`'s live region; cleared by a new filter. */
  let arrived = $state<string | undefined>(undefined);
  let isLoadingMore = $state(false);
  /** Why the last press of *Show more* brought nothing; cleared by the next press and a new filter. */
  let moreFailed = $state<string | undefined>(undefined);
  $effect(() => {
    void filter;
    arrived = undefined;
    moreFailed = undefined;
  });

  /** The next page, appended, and what arrived said once it has (P-13). */
  async function more(): Promise<void> {
    const current = filter;
    const before = runs.of(current).length;
    moreFailed = undefined;
    isLoadingMore = true;
    try {
      await runs.more(current);
      arrived = t('app.runs.arrived', { count: runs.of(current).length - before });
    } catch (error) {
      // Beside the button that asked, not in the page's banner: the runs already shown are still right.
      moreFailed = renderProblem(error as never, messages).message;
    } finally {
      isLoadingMore = false;
    }
  }

  const reading = $derived(runs.stateOf(filter));
  const listed = $derived(runs.of(filter));
  const refusal = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);

  const ruleName = (id: string) => rules.all.find((rule) => rule.id === id)?.name ?? t('app.runs.rule_gone');

  // The search narrows the runs loaded by their rule's name: the listing has no text search.
  let search = $state('');
  const shown = $derived(listed.filter((run) => matchesSearch(search, [ruleName(run.rule_id)])));

  const statuses = ['RUNNING', 'WAITING', 'SUCCEEDED', 'SKIPPED', 'FAILED', 'ABORTED_LOOP', 'THROTTLED'];
  const triggers = $derived(manifest.value?.automation?.triggers ?? []);

  /** What the filter shows, counted - not the workspace's totals. */
  const counts = $derived({
    runs: listed.length,
    succeeded: listed.filter((run) => run.status === 'SUCCEEDED').length,
    skipped: listed.filter((run) => run.status === 'SKIPPED').length,
    throttled: listed.filter((run) => run.status === 'THROTTLED').length,
    failed: listed.filter((run) => run.status === 'FAILED' || run.status === 'ABORTED_LOOP').length,
  });

  const rangeWords = $derived(range === '' ? t('app.runs.range_any') : t(`app.runs.range_${range}`));
  const chips = $derived<Filter[]>([
    { id: 'when', label: rangeWords, isActive: range !== '' },
    { id: 'rule', label: t('app.runs.chip_rule', { name: ruleFilter ? ruleName(ruleFilter) : t('app.page.any') }), isActive: ruleFilter !== '' },
    { id: 'status', label: t('app.runs.chip_status', { status: statusFilter ? statusWord(statusFilter) : t('app.page.any') }), isActive: statusFilter !== '' },
    { id: 'trigger', label: t('app.runs.chip_trigger', { trigger: triggerFilter ? triggerWord(triggerFilter) : t('app.page.any') }), isActive: triggerFilter !== '' },
  ]);
  const activeFilters = $derived(chips.filter((chip) => chip.isActive).length);

  function clearFilters(): void {
    chooseRange('');
    ruleFilter = '';
    statusFilter = '';
    triggerFilter = '';
    search = '';
  }

  const columns = $derived<DataColumn[]>([
    { id: 'rule', label: t('app.runs.filter_rule'), priority: 1 },
    { id: 'when', label: t('app.runs.when'), priority: 2 },
    { id: 'trigger', label: t('app.runs.filter_trigger'), priority: 3 },
    { id: 'outcome', label: t('app.runs.outcome'), priority: 2, isStatus: true },
  ]);

  let opened = $state<Run | undefined>(undefined);

  /** Whether the reader may change the workspace's automation (`AUTOMATION`), as the server asks. */
  const mayAutomate = $derived(mayHere('AUTOMATION').status === 'permitted');
  /** Where the plan leaves the rules out, nothing is tried, started or replayed; the runs stay readable. */
  const resting = $derived(includedHere('AUTOMATION_RULES') === false);

  const head = $derived({
    title: t('app.runs.title'),
    breadcrumb: adminTrail({ id: 'screen', label: t('app.runs.title') }),
  });
</script>

<LogPage {head}>
  {#snippet intro()}<p class="quiet">{t('app.runs.intro')}</p>{/snippet}
  {#snippet filters()}
    <FilterBar
      searchLabel={t('app.runs.search')}
      clearSearchLabel={t('app.page.clear_search')}
      bind:search
      filters={chips}
      moreLabel={t('app.page.more_filters')}
      filtersLabel={t('app.page.filters', { count: String(activeFilters) })}
      sheetTitle={t('app.page.filters', { count: '0' })}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      showLabel={t('app.page.show_results', { count: String(shown.length) })}
      clearLabel={t('app.page.clear')}
      onclear={clearFilters}
      countLabel={reading.status === 'ready' ? t('app.runs.found', { count: String(shown.length) }) : undefined}
    >
      {#snippet picker(chip: Filter)}
        {#if chip.id === 'when'}
          <Stack gap="150">
            <Radio
              label={t('app.runs.when')}
              bind:value={() => (range === 'custom' ? '' : range), (next) => chooseRange(next)}
              options={['', 'day', 'week', 'month'].map((each) => ({ value: each, label: each === '' ? t('app.runs.range_any') : t(`app.runs.range_${each}`) }))}
            />
            <Input label={t('app.runs.filter_from')} type="datetime-local" bind:value={from} oninput={() => (range = 'custom')} />
            <Input label={t('app.runs.filter_to')} type="datetime-local" bind:value={to} oninput={() => (range = 'custom')} error={reversed ? t('app.runs.window_reversed') : undefined} />
          </Stack>
        {:else if chip.id === 'rule'}
          <Select label={t('app.runs.filter_rule')} bind:value={ruleFilter} placeholder={t('app.runs.all_rules')}
            options={rules.all.map((rule) => ({ value: rule.id, label: rule.name }))} />
        {:else if chip.id === 'status'}
          <Select label={t('app.runs.filter_status')} bind:value={statusFilter} placeholder={t('app.runs.all_statuses')}
            options={statuses.map((status) => ({ value: status, label: statusWord(status) }))} />
        {:else}
          <Select label={t('app.runs.filter_trigger')} bind:value={triggerFilter} placeholder={t('app.runs.all_triggers')}
            options={triggers.map((kind) => ({ value: kind, label: triggerWord(kind) }))} />
        {/if}
      {/snippet}
    </FilterBar>
  {/snippet}

  {#if resting}<NotInPlan body={t('app.plan.runs_body')} />{/if}

  <div class="summary">
    <dl class="strip" data-strip>
      {#each [['runs', counts.runs], ['succeeded', counts.succeeded], ['skipped', counts.skipped], ['throttled', counts.throttled], ['failed', counts.failed]] as [key, value] (key)}
        <div class="stat"><dd>{value}</dd><dt>{t(`app.runs.strip_${key}`)}</dt></div>
      {/each}
    </dl>
    {#if hourly}
      <p class="quiet small" data-hourly>
        {#if hourly.limit > 0}
          {t('app.runs.this_hour', { used: String(hourly.used ?? 0), limit: String(hourly.limit) })}
        {:else}
          {t('app.runs.this_hour_unlimited', { used: String(hourly.used ?? 0) })}
        {/if}
        <LimitSource standing={hourly} />
        <a href="/administration/quotas">{t('app.quotas.title')}</a>
      </p>
    {/if}
  </div>

  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="quiet"><Spinner label={t('app.runs.reading')} /> <span>{t('app.runs.reading')}</span></p>
  {:else if refusal}
    <FailureNote problem={refusal} />
  {:else}
    <DataTable
      label={t('app.runs.list_title')}
      {columns}
      rows={shown}
      key={(run: Run) => run.id}
      onopen={(run: Run) => (opened = run)}
      rowLabel={(run: Run) => ruleName(run.rule_id)}
      group={(run: Run) => formatDay(run.started_at, messages.locale)}
      shownLabel={listed.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
      more={runs.moreAfter(filter) || arrived || moreFailed
        ? {
            label: t('app.runs.more'),
            busyLabel: t('app.runs.loading_more'),
            hasMore: Boolean(runs.moreAfter(filter)),
            isBusy: isLoadingMore,
            arrivedLabel: arrived,
            failedLabel: moreFailed,
            onLoadMore: () => void more(),
          }
        : undefined}
    >
      {#snippet cell(run: Run, column: DataColumn)}
        {#if column.id === 'rule'}
          <span class="name" data-run={run.id}>{ruleName(run.rule_id)}</span>
        {:else if column.id === 'when'}
          <span class="quiet">{formatTime(run.started_at, messages.locale)}</span>
        {:else if column.id === 'trigger'}
          <span class="quiet">{triggerWord(run.trigger)}</span>
        {:else}
          <RunStatusBadge
            status={run.status}
            label={isStale(run) ? t('app.runs.status_crashed') : statusWord(run.status)}
            isDryRun={run.is_dry_run ?? false}
            dryRunLabel={t('app.runs.dry_run')}
          />
        {/if}
      {/snippet}
      {#snippet empty()}<EmptyState kind="unused" title={t('app.runs.none')} />{/snippet}
    </DataTable>
  {/if}
</LogPage>

<RunDialog run={opened} {ruleName} mayReplay={mayAutomate && !resting} onClose={() => (opened = undefined)} />

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .small { font-size: var(--fs-075); }
  .name { color: var(--text-primary); font-weight: var(--fw-medium); }

  /* The counts read as one line of words above the runs, not as tiles (PS-16). */
  .summary { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: var(--sp-100) var(--sp-300); }
  .strip { margin: 0; display: flex; flex-wrap: wrap; gap: var(--sp-100) var(--sp-250); }
  .stat { display: inline-flex; align-items: baseline; gap: var(--sp-050); }
  .stat dd { margin: 0; font-weight: var(--fw-semibold); font-variant-numeric: tabular-nums; }
  .stat dt { color: var(--text-secondary); }

  /* The way to *Limits* reads as a link in both themes, as the overview's does. */
  [data-hourly] a { color: var(--text-brand); }
</style>
