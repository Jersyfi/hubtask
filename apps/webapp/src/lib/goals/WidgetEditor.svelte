<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A dashboard widget made or changed (UC-GOL-04 check 1): its title and chart, and what it
  // runs - a saved summary view, a summary asked here, or goals' progress.
  //
  // **A summary asked here is the query language as one line** (ADR-0066, `searchquery.ts`): the
  // reader writes `is:open type:task due:week`, the line compiles to the filter `POST
  // /items:aggregate` takes, and a token the installation does not know is refused by name before
  // anything is sent. The summary's function, measure, grouping and period are chosen beside it;
  // the server holds the document to its grammar on save (`work.ParseSummary`).
  //
  // What a summary counts is one of four sources (F-9): entries, the links between them, the time
  // recorded on them, or the pages of the spaces in scope. Each source offers only what the
  // grammar takes of it - links and pages are counted, time is summed by its minutes, and a page
  // is no entry, so its summary has no line and no collection for a scope.
  //
  // **A second series** (check 15) is the same summary cut by another stamp - resolved beside
  // created -, optionally with a line of its own; a running total draws each period as the sum up
  // to it, and with both the server answers what is left between them, a burndown.

  import { untrack } from 'svelte';
  import { Button, Checkbox, CheckboxGroup, Inline, Input, Select, Stack, Tabs, Textarea } from '@hubtask/design-system/components';
  import type { DashboardWidgetWrite, Goal, SavedView } from '@hubtask/sync-engine';

  import { manifest } from '../data/capabilities.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { groupableFields, queryFields } from '../data/query.ts';
  import { compile, parse, type Context } from '../data/searchquery.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The widget changed, or none for a new one. */
    widget?: DashboardWidgetWrite;
    /** The saved summary views the reader may pick. */
    summaryViews: readonly SavedView[];
    /** The goals the reader may pick. */
    goals: readonly Goal[];
    onsave: (widget: DashboardWidgetWrite) => void;
  }

  const { widget, summaryViews, goals, onsave }: Props = $props();

  type Chart = DashboardWidgetWrite['chart'];
  type Source = DashboardWidgetWrite['source'];
  const CHARTS: readonly Chart[] = ['NUMBER', 'BAR', 'PIE', 'LINE', 'TABLE', 'MATRIX', 'PROGRESS', 'LIST', 'TEXT'];
  const FUNCTIONS = ['COUNT', 'SUM', 'AVG', 'MIN', 'MAX', 'P50', 'P85', 'P95'] as const;
  const PERIODS = ['', 'day', 'week', 'month'] as const;
  const SOURCES = ['entries', 'links', 'time', 'pages'] as const;
  type SummarySource = (typeof SOURCES)[number];
  /** The groupings and stamps a source has of its own, beside an entry's. */
  const OWN_GROUPS: Record<SummarySource, readonly string[]> = {
    entries: [], links: ['link_type'], time: ['work_type', 'billable', 'person'], pages: ['owner', 'template', 'verification'],
  };

  type Aggregate = { source?: string; function?: string; field?: string; group_by?: string; bucket?: { field?: string; period?: string } };
  // svelte-ignore state_referenced_locally
  const asked = (widget?.query ?? {}) as { scope?: { container_id?: string; workspace?: boolean }; filter?: unknown; include_archived?: boolean; aggregate?: Aggregate };
  /** A stored filter is kept as it is until the reader writes a line in its place. */
  // svelte-ignore state_referenced_locally
  const kept = asked.filter;

  // svelte-ignore state_referenced_locally
  const comparedAsked = (widget?.compare?.query ?? {}) as { filter?: unknown; include_archived?: boolean; aggregate?: Aggregate };
  const comparedKept = comparedAsked.filter;

  // svelte-ignore state_referenced_locally
  const draft = $state({
    hasCompare: widget?.compare != null,
    firstTitle: widget?.compare?.first_title ?? '',
    secondTitle: widget?.compare?.title ?? '',
    secondStamp: comparedAsked.aggregate?.bucket?.field ?? 'completed_at',
    secondLine: '',
    cumulative: widget?.cumulative === true,
    title: widget?.title ?? '',
    chart: (widget?.chart ?? 'BAR') as Chart,
    source: (widget?.source ?? 'QUERY') as Source,
    viewId: widget?.view_id ?? '',
    scope: asked.scope?.container_id ?? '',
    line: '',
    summarySource: (SOURCES as readonly string[]).includes(asked.aggregate?.source ?? '') ? (asked.aggregate?.source as SummarySource) : 'entries',
    fn: asked.aggregate?.function ?? 'COUNT',
    field: asked.aggregate?.field ?? '',
    groupBy: asked.aggregate?.group_by ?? '',
    bucketField: asked.aggregate?.bucket?.field ?? 'created_at',
    period: asked.aggregate?.bucket?.period ?? '',
    goalIds: [...(widget?.goal_ids ?? [])] as string[],
    text: widget?.text ?? '',
    column: widget?.column ?? 'due_at',
    target: typeof widget?.target === 'number' ? String(widget.target) : '',
    warnAt: typeof widget?.warn_at === 'number' ? String(widget.warn_at) : '',
    tab: 'data',
  });
  /** A figure's target and warning line, where a number or a line draws them. */
  const aims = $derived(draft.chart === 'NUMBER' || draft.chart === 'LINE');
  const targetValue = $derived(draft.target.trim() === '' ? undefined : Number(draft.target.replace(',', '.')));
  const warnValue = $derived(draft.warnAt.trim() === '' ? undefined : Number(draft.warnAt.replace(',', '.')));

  const reported = $derived(new Set(queryFields(manifest.value).map((field) => field.field)));
  const groups = $derived([
    ...OWN_GROUPS[draft.summarySource],
    ...(draft.summarySource === 'pages' ? [] : groupableFields(manifest.value).map((field) => field.field)),
  ]);
  const stamps = $derived(
    draft.summarySource === 'pages'
      ? ['created_at', 'updated_at']
      : [...(draft.summarySource === 'time' ? ['day'] : []), ...queryFields(manifest.value).filter((field) => field.kind === 'timestamp').map((field) => field.field)],
  );
  const countedOnly = $derived(draft.summarySource === 'links' || draft.summarySource === 'pages');
  const functions = $derived(countedOnly ? (['COUNT'] as const) : FUNCTIONS);

  // A change of source keeps only what the new source takes.
  $effect(() => {
    const kept = { groups, stamps, countedOnly };
    untrack(() => {
      if (kept.countedOnly) draft.fn = 'COUNT';
      if (draft.groupBy && !kept.groups.includes(draft.groupBy)) draft.groupBy = '';
      if (!kept.stamps.includes(draft.bucketField)) draft.bucketField = kept.stamps[0] ?? 'created_at';
      if (draft.summarySource === 'pages' && everyCollection.some((each) => each.id === draft.scope)) draft.scope = '';
    });
  });

  // Every collection the reader sees, for the line's `in:` and for the summary's scope.
  $effect(() => {
    const ids = containers.hubs.map((hub) => hub.id);
    const opened = untrack(() => ids.map((id) => containers.openLevel(id)));
    return () => opened.forEach((close) => close());
  });
  const everyCollection = $derived(containers.hubs.flatMap((hub) => containers.collectionsOf(hub.id)));
  const context = $derived<Context>({
    fields: reported,
    collection: (name) => {
      const wanted = name.trim().toLowerCase();
      return everyCollection.find((each) => each.name.toLowerCase() === wanted || each.id === name.trim())?.id;
    },
    label: () => undefined,
    hasCollection: false,
  });
  const compiled = $derived(compile(parse(draft.line), context));
  const lineError = $derived.by(() => {
    if (compiled.q && compiled.q.trim() !== '') return t('app.dashboards.line_words');
    const first = compiled.problems[0];
    return first ? t(first.code, first.params) : undefined;
  });

  const compiledSecond = $derived(compile(parse(draft.secondLine), context));
  const secondLineError = $derived.by(() => {
    if (compiledSecond.q && compiledSecond.q.trim() !== '') return t('app.dashboards.line_words');
    const first = compiledSecond.problems[0];
    return first ? t(first.code, first.params) : undefined;
  });
  const drawsSeries = $derived(draft.source !== 'GOALS' && (draft.chart === 'BAR' || draft.chart === 'LINE' || draft.chart === 'TABLE'));

  const problem = $derived.by(() => {
    if (draft.title.trim() === '') return t('app.dashboards.widget_title_needed');
    if ((draft.chart === 'TEXT') !== (draft.source === 'TEXT')) return t('dashboards.chart_invalid');
    if (draft.source === 'TEXT' && draft.text.trim() === '') return t('dashboards.text_required');
    if (draft.chart === 'LIST' && draft.source !== 'QUERY') return t('dashboards.list_needs_query');
    if (aims && (targetValue !== undefined && !Number.isFinite(targetValue) || warnValue !== undefined && !Number.isFinite(warnValue))) return t('app.dashboards.aim_number');
    if (aims && warnValue !== undefined && targetValue === undefined) return t('dashboards.warn_needs_target');
    if (aims && warnValue !== undefined && warnValue === targetValue) return t('dashboards.warn_equal');
    if (draft.chart === 'PROGRESS' && draft.source !== 'GOALS') return t('dashboards.chart_invalid');
    if (draft.source === 'VIEW' && draft.viewId === '') return t('dashboards.view_required');
    if (draft.source === 'GOALS' && (draft.goalIds.length === 0 || draft.goalIds.length > 20)) return t('dashboards.goals_count');
    if (draft.source === 'QUERY') {
      if (lineError && draft.summarySource !== 'pages') return lineError;
      if (draft.fn !== 'COUNT' && draft.summarySource !== 'time' && draft.field.trim() === '') return t('app.dashboards.measure_needed');
      if (drawsSeries && draft.cumulative && draft.period === '') return t('dashboards.cumulative_periods');
      if (drawsSeries && draft.cumulative && draft.fn !== 'COUNT' && draft.fn !== 'SUM') return t('dashboards.cumulative_function');
      if (drawsSeries && draft.hasCompare) {
        if (draft.firstTitle.trim() === '' || draft.secondTitle.trim() === '') return t('app.dashboards.series_names_needed');
        if (draft.period === '') return t('app.dashboards.series_period_needed');
        if (secondLineError && draft.summarySource !== 'pages') return secondLineError;
      }
    }
    return undefined;
  });

  function save() {
    if (problem) return;
    const write: DashboardWidgetWrite = {
      ...(widget?.id ? { id: widget.id } : {}),
      title: draft.title.trim(),
      chart: draft.chart,
      source: draft.source,
      x: widget?.x ?? 0,
      y: widget?.y ?? 0,
      w: widget?.w ?? 6,
      h: widget?.h ?? 4,
    };
    if (draft.source === 'VIEW') write.view_id = draft.viewId;
    if (draft.source === 'GOALS') write.goal_ids = [...draft.goalIds];
    if (draft.source === 'TEXT') write.text = draft.text.trim();
    if (aims && targetValue !== undefined) {
      write.target = targetValue;
      if (warnValue !== undefined) write.warn_at = warnValue;
    }
    if (draft.source === 'QUERY' && draft.chart === 'LIST') {
      write.column = draft.column;
      write.query = {
        scope: draft.scope ? { container_id: draft.scope } : { workspace: true },
        ...(draft.line.trim() === '' && kept !== undefined ? { filter: kept } : compiled.filter ? { filter: compiled.filter } : {}),
        sort: [{ field: 'due_at', dir: 'ASC' }],
      };
      onsave(write);
      return;
    }
    if (draft.source === 'QUERY') {
      const aggregate: Aggregate = { function: draft.fn };
      if (draft.summarySource !== 'entries') aggregate.source = draft.summarySource;
      if (draft.fn !== 'COUNT') aggregate.field = draft.summarySource === 'time' ? 'minutes' : draft.field.trim();
      if (draft.groupBy) aggregate.group_by = draft.groupBy;
      if (draft.period) aggregate.bucket = { field: draft.bucketField, period: draft.period };
      write.query = {
        scope: draft.scope ? { container_id: draft.scope } : { workspace: true },
        ...(draft.summarySource === 'pages' ? {} : draft.line.trim() === '' && kept !== undefined
          ? { filter: kept, ...(asked.include_archived ? { include_archived: true } : {}) }
          : { ...(compiled.filter ? { filter: compiled.filter } : {}), ...(compiled.includeArchived ? { include_archived: true } : {}) }),
        aggregate,
      };
      if (drawsSeries && draft.hasCompare && draft.period) {
        write.compare = {
          first_title: draft.firstTitle.trim(),
          title: draft.secondTitle.trim(),
          query: {
            ...write.query,
            ...(draft.summarySource === 'pages' ? {} : draft.secondLine.trim() === ''
              ? comparedKept !== undefined
                ? { filter: comparedKept, ...(comparedAsked.include_archived ? { include_archived: true } : {}) }
                : {}
              : { filter: compiledSecond.filter, ...(compiledSecond.includeArchived ? { include_archived: true } : {}) }),
            aggregate: { ...aggregate, bucket: { field: draft.secondStamp, period: draft.period } },
          },
        };
        if (draft.secondLine.trim() !== '' && !compiledSecond.filter) delete (write.compare.query as { filter?: unknown }).filter;
      }
    }
    // A saved view's second series is asked through the contract; the form keeps it as it was.
    if (draft.source === 'VIEW' && widget?.compare && drawsSeries) write.compare = widget.compare;
    if (drawsSeries && draft.cumulative) write.cumulative = true;
    onsave(write);
  }
</script>

<Stack gap="150">
  <Tabs label={t('app.dashboards.editor_tabs')} tabs={[{ id: 'data', label: t('app.dashboards.tab_data') }, { id: 'look', label: t('app.dashboards.tab_look') }, { id: 'show', label: t('app.dashboards.tab_show') }]} bind:selected={draft.tab}>
  <Stack gap="150">
  {#if draft.tab === 'show'}
    <Input label={t('app.dashboards.widget_title')} bind:value={draft.title} maxlength={200} />
    {#if draft.source === 'TEXT'}
      <Textarea label={t('app.dashboards.text')} hint={t('app.dashboards.text_hint')} bind:value={draft.text} maxlength={2000} />
    {/if}
    {#if draft.chart === 'LIST'}
      <Select label={t('app.dashboards.list_column')} bind:value={draft.column}
        options={['due_at', 'status', 'assignee_id', 'updated_at', 'created_at'].map((field) => ({ value: field, label: t(`app.dashboards.column_${field}`) }))} />
    {/if}
  {:else if draft.tab === 'look'}
    <Select label={t('app.dashboards.chart')} bind:value={draft.chart}
      options={CHARTS.filter((each) => (each !== 'PROGRESS' || draft.source === 'GOALS') && (each !== 'TEXT' || draft.source === 'TEXT') && (each !== 'LIST' || draft.source === 'QUERY')).map((each) => ({ value: each, label: t(`app.dashboards.chart_${each}`) }))} />
    {#if aims}
      <Inline gap="100" isWrapping>
        <Input label={t('app.dashboards.target')} hint={t('app.dashboards.target_hint')} bind:value={draft.target} inputmode="decimal" />
        <Input label={t('app.dashboards.warn_at')} bind:value={draft.warnAt} inputmode="decimal" />
      </Inline>
    {/if}
    {#if drawsSeries}
      <Checkbox label={t('app.dashboards.cumulative')} hint={t('app.dashboards.cumulative_hint')} bind:checked={draft.cumulative} />
    {/if}
  {:else}
  <Select label={t('app.dashboards.source')} bind:value={draft.source}
    options={(['QUERY', 'VIEW', 'GOALS', 'TEXT'] as const).map((each) => ({ value: each, label: t(`app.dashboards.source_${each}`) }))} />
  {#if draft.source === 'TEXT'}
    <p class="quiet">{t('app.dashboards.text_runs_nothing')}</p>
  {:else if draft.source === 'VIEW'}
    <Select label={t('app.dashboards.view')} hint={t('app.dashboards.view_hint')} bind:value={draft.viewId} placeholder={t('app.goals.kr_view_pick')}
      options={summaryViews.map((each) => ({ value: each.id, label: each.name }))} />
  {:else if draft.source === 'GOALS'}
    <CheckboxGroup label={t('app.dashboards.goals')} hint={t('app.dashboards.goals_hint')} selected={draft.goalIds}
      options={goals.map((each) => ({ value: each.id, label: each.title }))} onChange={(next) => (draft.goalIds = [...next])} />
  {:else}
    <Select label={t('app.dashboards.summary_source')} bind:value={draft.summarySource}
      options={SOURCES.map((each) => ({ value: each, label: t(`app.dashboards.summary_source_${each}`) }))} />
    <Select label={t('app.dashboards.scope')} bind:value={draft.scope}
      options={[{ value: '', label: t('app.dashboards.scope_workspace') }, ...containers.hubs.map((each) => ({ value: each.id, label: each.name })), ...(draft.summarySource === 'pages' ? [] : everyCollection.map((each) => ({ value: each.id, label: each.name })))]} />
    {#if draft.summarySource !== 'pages'}
      <Input label={t('app.dashboards.line')} hint={kept !== undefined && draft.line.trim() === '' ? t('app.dashboards.line_kept') : t('app.dashboards.line_hint')} bind:value={draft.line} error={lineError} spellcheck={false} />
    {/if}
    <Inline gap="100" isWrapping>
      <Select label={t('app.dashboards.function')} bind:value={draft.fn} options={functions.map((each) => ({ value: each, label: t(`app.dashboards.fn_${each}`) }))} />
      {#if draft.fn !== 'COUNT' && draft.summarySource !== 'time'}
        <Input label={t('app.dashboards.measure')} hint={t('app.dashboards.measure_hint')} bind:value={draft.field} spellcheck={false} />
      {/if}
    </Inline>
    <Select label={t('app.dashboards.group_by')} bind:value={draft.groupBy}
      options={[{ value: '', label: t('app.dashboards.no_grouping') }, ...groups.map((field) => ({ value: field, label: field }))]} />
    <Inline gap="100" isWrapping>
      <Select label={t('app.dashboards.period')} bind:value={draft.period}
        options={PERIODS.map((each) => ({ value: each, label: t(each === '' ? 'app.dashboards.no_period' : `app.dashboards.period_${each}`) }))} />
      {#if draft.period}
        <Select label={t('app.dashboards.period_field')} bind:value={draft.bucketField}
          options={stamps.map((field) => ({ value: field, label: field }))} />
      {/if}
    </Inline>
  {/if}

  {#if drawsSeries}
    {#if draft.source === 'QUERY'}
      <Checkbox label={t('app.dashboards.series_second')} hint={t('app.dashboards.series_second_hint')} bind:checked={draft.hasCompare} />
      {#if draft.hasCompare}
        <Inline gap="100" isWrapping>
          <Input label={t('app.dashboards.series_first_name')} bind:value={draft.firstTitle} maxlength={200} />
          <Input label={t('app.dashboards.series_second_name')} bind:value={draft.secondTitle} maxlength={200} />
        </Inline>
        <Select label={t('app.dashboards.series_second_stamp')} bind:value={draft.secondStamp}
          options={stamps.map((field) => ({ value: field, label: field }))} />
        {#if draft.summarySource !== 'pages'}
          <Input label={t('app.dashboards.series_second_line')} hint={comparedKept !== undefined && draft.secondLine.trim() === '' ? t('app.dashboards.line_kept') : t('app.dashboards.series_second_line_hint')} bind:value={draft.secondLine} error={secondLineError} spellcheck={false} />
        {/if}
      {/if}
    {/if}
  {/if}

  {/if}
  </Stack>
  </Tabs>
  <Inline justify="end"><Button tone="primary" disabledReason={problem} onclick={save}>{t('app.dashboards.apply')}</Button></Inline>
</Stack>

<style>
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
