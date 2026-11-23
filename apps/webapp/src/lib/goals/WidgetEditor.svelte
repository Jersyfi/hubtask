<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A dashboard widget made or changed (UC-GOL-04 check 1): its title and chart, and what it
  // runs - a saved summary view, a summary asked here, or goals' progress.
  //
  // **A summary asked here is the query line of every list** (Q-1, the chips of `QueryStrip`): the
  // line is read as a collection's (`viewquery.ts`), names become identifiers, and a name nobody
  // here bears is refused by name before anything is sent. The summary's function, measure,
  // grouping and period are chosen beside it; the server holds the document to its grammar
  // (`work.ParseSummary`). Every valid change is handed up as a draft (`ondraft`), which the
  // dashboard computes live without storing it (UC-GOL-04 check 23).
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
  import { Button, Checkbox, Icon, Inline, Input, MultiSelect, Select, Stack, Tabs, Textarea } from '@hubtask/design-system/components';
  import { READER_KEYS, type QueryKey } from '@hubtask/query';
  import type { DashboardWidgetWrite, Goal, SavedView } from '@hubtask/sync-engine';

  import { manifest } from '../data/capabilities.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { groupableFields, queryFields } from '../data/query.ts';
  import { lineOf, readQuery, type Names } from '../data/viewquery.ts';
  import QueryStrip from '../entries/QueryStrip.svelte';
  import { chooseSeveral } from '../forms/chooseSeveral.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The widget changed, or none for a new one. */
    widget?: DashboardWidgetWrite;
    /** The saved summary views the reader may pick. */
    summaryViews: readonly SavedView[];
    /** The goals the reader may pick. */
    goals: readonly Goal[];
    /** The dashboard's conditions, each a chip the widget inherits until it releases its field. */
    inherited?: readonly { readonly field: string; readonly piece: string }[];
    /** The query line's keys, as every list offers them. */
    keys: readonly QueryKey[];
    /** Names in the line, read and written as a collection's line does. */
    names: Names;
    onsave: (widget: DashboardWidgetWrite) => void;
    /** The widget as edited so far, whenever it would save; none while it would not. */
    ondraft?: (widget: DashboardWidgetWrite | undefined) => void;
    oncancel?: () => void;
    /** Opens the list the widget counts, where one can be opened. */
    onlist?: (() => void) | undefined;
  }

  const { widget, summaryViews, goals, inherited = [], keys, names, onsave, ondraft, oncancel, onlist }: Props = $props();

  /** A field by the name a person reads (the query's key word), never its key. */
  function fieldLabel(field: string): string {
    const own = `app.dashboards.field_${field}`;
    if (messages.has(own)) return t(own);
    const key = Object.entries(READER_KEYS).find(([, name]) => name === field)?.[0];
    const word = key ? `app.qkey.${key.replaceAll('.', '_')}` : '';
    return word && messages.has(word) ? t(word) : field.replaceAll('_', ' ');
  }

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
  const asked = (widget?.query ?? {}) as { scope?: { container_id?: string; workspace?: boolean }; q?: unknown; filter?: unknown; include_archived?: boolean; aggregate?: Aggregate };
  /** A stored filter is kept as it is until the reader writes a line in its place. */
  // svelte-ignore state_referenced_locally
  const kept = asked.filter;

  // svelte-ignore state_referenced_locally
  const comparedAsked = (widget?.compare?.query ?? {}) as { filter?: unknown; include_archived?: boolean; aggregate?: Aggregate };
  const comparedKept = comparedAsked.filter;

  /** A stored filter as the line a person reads; the stored one is sent on until the line is changed. */
  function storedLine(filter: unknown): string {
    if (typeof asked.q === 'string' && filter === asked.filter) return asked.q;
    return filter && typeof filter === 'object' ? (lineOf({ filter: filter as never }, names) ?? '') : '';
  }

  // svelte-ignore state_referenced_locally
  const draft = $state({
    hasCompare: widget?.compare != null,
    firstTitle: widget?.compare?.first_title ?? '',
    secondTitle: widget?.compare?.title ?? '',
    secondStamp: comparedAsked.aggregate?.bucket?.field ?? 'completed_at',
    // svelte-ignore state_referenced_locally
    secondLine: storedLine(comparedKept),
    isSecondLineChanged: false,
    cumulative: widget?.cumulative === true,
    title: widget?.title ?? '',
    chart: (widget?.chart ?? 'BAR') as Chart,
    source: (widget?.source ?? 'QUERY') as Source,
    viewId: widget?.view_id ?? '',
    scope: asked.scope?.container_id ?? '',
    // svelte-ignore state_referenced_locally
    line: storedLine(kept),
    isLineChanged: false,
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
    released: [...(widget?.released ?? [])] as string[],
  });
  function toggleReleased(field: string) {
    draft.released = draft.released.includes(field) ? draft.released.filter((each) => each !== field) : [...draft.released, field];
  }
  /** A figure's target and warning line, where a number or a line draws them. */
  const aims = $derived(draft.chart === 'NUMBER' || draft.chart === 'LINE');
  const targetValue = $derived(draft.target.trim() === '' ? undefined : Number(draft.target.replace(',', '.')));
  const warnValue = $derived(draft.warnAt.trim() === '' ? undefined : Number(draft.warnAt.replace(',', '.')));

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
  /** A line read as a filter tree, or the problem that refuses it, by its code or the names nobody bears. */
  function filterOf(line: string): { filter?: unknown; error?: string } {
    if (line.trim() === '') return {};
    const read = readQuery(line, names);
    if (!read.ok) return { error: t(read.error.code) };
    if (read.unknown.length > 0) return { error: t('app.dashboards.filter_unknown', { names: read.unknown.join(', ') }) };
    if (read.query.filter && JSON.stringify(read.query.filter).includes('"field":"text"')) return { error: t('app.dashboards.line_words') };
    return read.query.filter ? { filter: read.query.filter } : {};
  }
  const compiled = $derived(filterOf(draft.line));
  const lineError = $derived(draft.isLineChanged ? compiled.error : undefined);
  const compiledSecond = $derived(filterOf(draft.secondLine));
  const secondLineError = $derived(draft.isSecondLineChanged ? compiledSecond.error : undefined);
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

  /** The widget as it would be saved. */
  function build(): DashboardWidgetWrite {
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
    if (draft.source === 'QUERY' && draft.released.length > 0) write.released = [...draft.released];
    if (aims && targetValue !== undefined) {
      write.target = targetValue;
      if (warnValue !== undefined) write.warn_at = warnValue;
    }
    if (draft.source === 'QUERY' && draft.chart === 'LIST') {
      write.column = draft.column;
      write.query = {
        scope: draft.scope ? { container_id: draft.scope } : { workspace: true },
        ...(!draft.isLineChanged && kept !== undefined ? { filter: kept } : compiled.filter ? { filter: compiled.filter } : {}),
        sort: [{ field: 'due_at', dir: 'ASC' }],
      };
      return write;
    }
    if (draft.source === 'QUERY') {
      const aggregate: Aggregate = { function: draft.fn };
      if (draft.summarySource !== 'entries') aggregate.source = draft.summarySource;
      if (draft.fn !== 'COUNT') aggregate.field = draft.summarySource === 'time' ? 'minutes' : draft.field.trim();
      if (draft.groupBy) aggregate.group_by = draft.groupBy;
      if (draft.period) aggregate.bucket = { field: draft.bucketField, period: draft.period };
      write.query = {
        scope: draft.scope ? { container_id: draft.scope } : { workspace: true },
        ...(draft.summarySource === 'pages' ? {} : !draft.isLineChanged && kept !== undefined
          ? { filter: kept, ...(asked.include_archived ? { include_archived: true } : {}) }
          : compiled.filter ? { filter: compiled.filter } : {}),
        aggregate,
      };
      if (drawsSeries && draft.hasCompare && draft.period) {
        write.compare = {
          first_title: draft.firstTitle.trim(),
          title: draft.secondTitle.trim(),
          query: {
            ...write.query,
            ...(draft.summarySource === 'pages' ? {} : !draft.isSecondLineChanged
              ? comparedKept !== undefined
                ? { filter: comparedKept, ...(comparedAsked.include_archived ? { include_archived: true } : {}) }
                : {}
              : { filter: compiledSecond.filter }),
            aggregate: { ...aggregate, bucket: { field: draft.secondStamp, period: draft.period } },
          },
        };
        if (draft.secondLine.trim() !== '' && !compiledSecond.filter) delete (write.compare.query as { filter?: unknown }).filter;
      }
    }
    // A saved view's second series is asked through the contract; the form keeps it as it was.
    if (draft.source === 'VIEW' && widget?.compare && drawsSeries) write.compare = widget.compare;
    if (drawsSeries && draft.cumulative) write.cumulative = true;
    return write;
  }
  function save() {
    if (!problem) onsave(build());
  }
  // The draft goes up whenever it would save, so the dashboard draws it live.
  $effect(() => {
    const next = problem ? undefined : build();
    untrack(() => ondraft?.(next));
  });
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
    <MultiSelect label={t('app.dashboards.goals')} hint={t('app.dashboards.goals_hint')} selected={draft.goalIds}
      options={goals.map((each) => ({ value: each.id, label: each.title }))} onChange={(next) => (draft.goalIds = [...next])} {...chooseSeveral()} />
  {:else}
    <Select label={t('app.dashboards.summary_source')} bind:value={draft.summarySource}
      options={SOURCES.map((each) => ({ value: each, label: t(`app.dashboards.summary_source_${each}`) }))} />
    <Select label={t('app.dashboards.scope')} bind:value={draft.scope}
      options={[{ value: '', label: t('app.dashboards.scope_workspace') }, ...containers.hubs.map((each) => ({ value: each.id, label: each.name })), ...(draft.summarySource === 'pages' ? [] : everyCollection.map((each) => ({ value: each.id, label: each.name })))]} />
    {#if inherited.length > 0}
      <!-- The dashboard's conditions as chips (§6.5): locked while inherited, released to be ignored. -->
      <div class="inherited" role="group" aria-label={t('app.dashboards.inherited')} data-inherited>
        <span class="quiet">{t('app.dashboards.inherited')}</span>
        <ul class="chips">
          {#each inherited as chip (chip.field + chip.piece)}
            {@const held = !draft.released.includes(chip.field)}
            <li><button type="button" class="chip" aria-pressed={held} onclick={() => toggleReleased(chip.field)}
              title={t(held ? 'app.dashboards.release' : 'app.dashboards.inherit')}>{#if held}<Icon name="lock" size="sm" />{/if}{chip.piece}</button></li>
          {/each}
        </ul>
        <span class="quiet">{t('app.dashboards.inherited_hint')}</span>
      </div>
    {/if}
    {#if draft.summarySource !== 'pages'}
      {@render line(t('app.dashboards.line'), t('app.dashboards.line_hint'), draft.line, lineError, (next) => { draft.line = next; draft.isLineChanged = true; })}
    {/if}
    <Inline gap="100" isWrapping>
      <Select label={t('app.dashboards.function')} bind:value={draft.fn} options={functions.map((each) => ({ value: each, label: t(`app.dashboards.fn_${each}`) }))} />
      {#if draft.fn !== 'COUNT' && draft.summarySource !== 'time'}
        <Input label={t('app.dashboards.measure')} hint={t('app.dashboards.measure_hint')} bind:value={draft.field} spellcheck={false} />
      {/if}
    </Inline>
    <Select label={t('app.dashboards.group_by')} bind:value={draft.groupBy}
      options={[{ value: '', label: t('app.dashboards.no_grouping') }, ...groups.map((field) => ({ value: field, label: fieldLabel(field) }))]} />
    <Inline gap="100" isWrapping>
      <Select label={t('app.dashboards.period')} bind:value={draft.period}
        options={PERIODS.map((each) => ({ value: each, label: t(each === '' ? 'app.dashboards.no_period' : `app.dashboards.period_${each}`) }))} />
      {#if draft.period}
        <Select label={t('app.dashboards.period_field')} bind:value={draft.bucketField}
          options={stamps.map((field) => ({ value: field, label: fieldLabel(field) }))} />
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
          options={stamps.map((field) => ({ value: field, label: fieldLabel(field) }))} />
        {#if draft.summarySource !== 'pages'}
          {@render line(t('app.dashboards.series_second_line'), t('app.dashboards.series_second_line_hint'), draft.secondLine, secondLineError, (next) => { draft.secondLine = next; draft.isSecondLineChanged = true; })}
        {/if}
      {/if}
    {/if}
  {/if}

  {/if}
  </Stack>
  </Tabs>
  <Inline justify="end">
    {#if onlist}<Button tone="subtle" icon="list" onclick={onlist}>{t('app.dashboards.open_list')}</Button>{/if}
    {#if oncancel}<Button onclick={oncancel}>{t('app.links.cancel')}</Button>{/if}
    <Button tone="primary" disabledReason={problem} onclick={save}>{t('app.dashboards.apply')}</Button>
  </Inline>
</Stack>

{#snippet line(label: string, hint: string, value: string, error: string | undefined, onchange: (next: string) => void)}
  <!-- The chips of every list's line (Q-1), not a text field of its own. -->
  <div class="line" role="group" aria-label={label} data-widget-line>
    <span class="line-label">{label}</span>
    <QueryStrip line={value} {keys} {onchange} />
    {#if error}<p class="error" role="alert">{error}</p>{:else}<span class="quiet">{hint}</span>{/if}
  </div>
{/snippet}

<style>
  .line { display: flex; flex-direction: column; gap: var(--sp-050); }
  .line-label { font-size: var(--fs-075); font-weight: var(--fw-semibold); color: var(--text-primary); }
  .error { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .inherited { display: flex; flex-direction: column; gap: var(--sp-050); }
  .chips { display: flex; flex-wrap: wrap; gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  .chip { display: inline-flex; align-items: center; gap: var(--sp-050); padding: var(--sp-025) var(--sp-100); border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-sm); background: var(--bg-surface); color: var(--text-secondary); font: inherit; font-size: var(--fs-075); text-decoration: line-through; cursor: pointer; }
  .chip[aria-pressed='true'] { border-color: var(--accent-primary); background: var(--accent-primary-subtle); color: var(--text-brand); text-decoration: none; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
