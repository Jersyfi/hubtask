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

  import { untrack } from 'svelte';
  import { Button, CheckboxGroup, Inline, Input, Select, Stack } from '@hubtask/design-system/components';
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
  const CHARTS: readonly Chart[] = ['NUMBER', 'BAR', 'PIE', 'LINE', 'TABLE', 'MATRIX', 'PROGRESS'];
  const FUNCTIONS = ['COUNT', 'SUM', 'AVG', 'MIN', 'MAX'] as const;
  const PERIODS = ['', 'day', 'week', 'month'] as const;

  type Aggregate = { function?: string; field?: string; group_by?: string; bucket?: { field?: string; period?: string } };
  // svelte-ignore state_referenced_locally
  const asked = (widget?.query ?? {}) as { scope?: { container_id?: string; workspace?: boolean }; filter?: unknown; include_archived?: boolean; aggregate?: Aggregate };
  /** A stored filter is kept as it is until the reader writes a line in its place. */
  // svelte-ignore state_referenced_locally
  const kept = asked.filter;

  // svelte-ignore state_referenced_locally
  const draft = $state({
    title: widget?.title ?? '',
    chart: (widget?.chart ?? 'BAR') as Chart,
    source: (widget?.source ?? 'QUERY') as Source,
    viewId: widget?.view_id ?? '',
    scope: asked.scope?.container_id ?? '',
    line: '',
    fn: asked.aggregate?.function ?? 'COUNT',
    field: asked.aggregate?.field ?? '',
    groupBy: asked.aggregate?.group_by ?? '',
    bucketField: asked.aggregate?.bucket?.field ?? 'created_at',
    period: asked.aggregate?.bucket?.period ?? '',
    goalIds: [...(widget?.goal_ids ?? [])] as string[],
  });

  const reported = $derived(new Set(queryFields(manifest.value).map((field) => field.field)));
  const groups = $derived(groupableFields(manifest.value));
  const stamps = $derived(queryFields(manifest.value).filter((field) => field.kind === 'timestamp'));

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

  const problem = $derived.by(() => {
    if (draft.title.trim() === '') return t('app.dashboards.widget_title_needed');
    if (draft.chart === 'PROGRESS' && draft.source !== 'GOALS') return t('dashboards.chart_invalid');
    if (draft.source === 'VIEW' && draft.viewId === '') return t('dashboards.view_required');
    if (draft.source === 'GOALS' && (draft.goalIds.length === 0 || draft.goalIds.length > 20)) return t('dashboards.goals_count');
    if (draft.source === 'QUERY') {
      if (lineError) return lineError;
      if (draft.fn !== 'COUNT' && draft.field.trim() === '') return t('app.dashboards.measure_needed');
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
    if (draft.source === 'QUERY') {
      const aggregate: Aggregate = { function: draft.fn };
      if (draft.fn !== 'COUNT') aggregate.field = draft.field.trim();
      if (draft.groupBy) aggregate.group_by = draft.groupBy;
      if (draft.period) aggregate.bucket = { field: draft.bucketField, period: draft.period };
      write.query = {
        scope: draft.scope ? { container_id: draft.scope } : { workspace: true },
        ...(draft.line.trim() === '' && kept !== undefined
          ? { filter: kept, ...(asked.include_archived ? { include_archived: true } : {}) }
          : { ...(compiled.filter ? { filter: compiled.filter } : {}), ...(compiled.includeArchived ? { include_archived: true } : {}) }),
        aggregate,
      };
    }
    onsave(write);
  }
</script>

<Stack gap="150">
  <Input label={t('app.dashboards.widget_title')} bind:value={draft.title} maxlength={200} />
  <Select label={t('app.dashboards.source')} bind:value={draft.source}
    options={(['QUERY', 'VIEW', 'GOALS'] as const).map((each) => ({ value: each, label: t(`app.dashboards.source_${each}`) }))} />
  <Select label={t('app.dashboards.chart')} bind:value={draft.chart}
    options={CHARTS.filter((each) => each !== 'PROGRESS' || draft.source === 'GOALS').map((each) => ({ value: each, label: t(`app.dashboards.chart_${each}`) }))} />

  {#if draft.source === 'VIEW'}
    <Select label={t('app.dashboards.view')} hint={t('app.dashboards.view_hint')} bind:value={draft.viewId} placeholder={t('app.goals.kr_view_pick')}
      options={summaryViews.map((each) => ({ value: each.id, label: each.name }))} />
  {:else if draft.source === 'GOALS'}
    <CheckboxGroup label={t('app.dashboards.goals')} hint={t('app.dashboards.goals_hint')} selected={draft.goalIds}
      options={goals.map((each) => ({ value: each.id, label: each.title }))} onChange={(next) => (draft.goalIds = [...next])} />
  {:else}
    <Select label={t('app.dashboards.scope')} bind:value={draft.scope}
      options={[{ value: '', label: t('app.dashboards.scope_workspace') }, ...containers.hubs.map((each) => ({ value: each.id, label: each.name })), ...everyCollection.map((each) => ({ value: each.id, label: each.name }))]} />
    <Input label={t('app.dashboards.line')} hint={kept !== undefined && draft.line.trim() === '' ? t('app.dashboards.line_kept') : t('app.dashboards.line_hint')} bind:value={draft.line} error={lineError} spellcheck={false} />
    <Inline gap="100" isWrapping>
      <Select label={t('app.dashboards.function')} bind:value={draft.fn} options={FUNCTIONS.map((each) => ({ value: each, label: t(`app.dashboards.fn_${each}`) }))} />
      {#if draft.fn !== 'COUNT'}
        <Input label={t('app.dashboards.measure')} hint={t('app.dashboards.measure_hint')} bind:value={draft.field} spellcheck={false} />
      {/if}
    </Inline>
    <Select label={t('app.dashboards.group_by')} bind:value={draft.groupBy}
      options={[{ value: '', label: t('app.dashboards.no_grouping') }, ...groups.map((field) => ({ value: field.field, label: field.field }))]} />
    <Inline gap="100" isWrapping>
      <Select label={t('app.dashboards.period')} bind:value={draft.period}
        options={PERIODS.map((each) => ({ value: each, label: t(each === '' ? 'app.dashboards.no_period' : `app.dashboards.period_${each}`) }))} />
      {#if draft.period}
        <Select label={t('app.dashboards.period_field')} bind:value={draft.bucketField}
          options={stamps.map((field) => ({ value: field.field, label: field.field }))} />
      {/if}
    </Inline>
  {/if}

  <Inline justify="end"><Button disabledReason={problem} onclick={save}>{t('app.goals.save')}</Button></Inline>
</Stack>
