<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One dashboard widget's content in its kind's shape (UC-GOL-04 check 19, scene wcat): a figure
  // with its change, bars lying down, a ring, up to two lines, a table, a matrix in steps of one
  // colour, goals' progress, a query's first entries or a short text. Every chart has its table as
  // a toggle (`isTable`), the same rows read rather than seen (P-13). Colours are the
  // `chart.categorical` tokens in their fixed order (E-7), never a product's (WB-2); a state is
  // never told by colour alone - an arrow and a word go with it.
  //
  // Nothing is computed here but the layout: the rows are the server's, counted for this reader.

  import { DataTable, ProgressBar, type DataColumn } from '@hubtask/design-system/components';

  import {
    hbars, hpairs, lineDots, lineThrough, matrixOf, matrixSteps, numberFigure, percentOf, ringParts, scaleTop, seriesLines, seriesOf, targetState, textPieces,
    type FigureRow, type SeriesTable,
  } from '../data/goals.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  /** One of a LIST widget's entries, named and formatted by the page. */
  export interface ListEntry {
    readonly id: string;
    readonly key: string;
    readonly title: string;
    readonly href: string;
    readonly column: string;
    readonly tone?: 'danger' | 'warning';
  }

  interface Props {
    title: string;
    chart: 'NUMBER' | 'BAR' | 'PIE' | 'LINE' | 'TABLE' | 'MATRIX' | 'PROGRESS' | 'LIST' | 'TEXT';
    rows: readonly FigureRow[];
    /** Two series or more, aligned by period; drawn in place of `rows` where given. */
    table?: SeriesTable;
    /** A LIST widget's entries. */
    entries?: readonly ListEntry[];
    /** A TEXT widget's words. */
    text?: string;
    target?: number | null;
    warnAt?: number | null;
    /** The table in place of the drawing (§8). */
    isTable?: boolean;
    /** Why there are no rows, by the widget's source (user-experience.md §7.7); a general line without. */
    empty?: string;
  }

  const { title, chart, rows, table, entries = [], text = '', target, warnAt, isTable = false, empty }: Props = $props();

  const W = 400;
  const H = 120;
  const format = (value: number) => new Intl.NumberFormat(messages.locale, { maximumFractionDigits: 2 }).format(value);
  const labelText = (label: string) => (label === '' ? t('app.dashboards.no_value') : label);
  const others = $derived(t('app.dashboards.others'));

  const multi = $derived(table !== undefined && table.series.length > 1 && (chart === 'BAR' || chart === 'LINE' || chart === 'TABLE'));
  const figure = $derived(chart === 'NUMBER' ? numberFigure(rows) : undefined);
  const standing = $derived(figure ? targetState(figure.value, target, warnAt) : undefined);
  const lowerIsBetter = $derived(warnAt !== null && warnAt !== undefined && target !== null && target !== undefined && warnAt >= target);
  const bars = $derived(chart === 'BAR' && !multi ? hbars(rows, others) : []);
  const pairs = $derived(chart === 'BAR' && multi && table ? hpairs(table) : []);
  const ring = $derived(chart === 'PIE' ? ringParts(rows, others) : []);
  const matrix = $derived(chart === 'MATRIX' ? matrixOf(rows) : undefined);
  const steps = $derived(matrix ? matrixSteps(matrix) : []);
  const pieces = $derived(chart === 'TEXT' ? textPieces(text) : []);

  // A line of one series is a table of one: both draw the same way, the second dashed.
  const lineTable = $derived<SeriesTable | undefined>(chart !== 'LINE' ? undefined : multi && table
    ? { labels: table.labels, series: table.series.slice(0, 2) }
    : { labels: rows.map((row) => row.column ?? row.label), series: [{ name: title, values: rows.map((row) => row.value), texts: rows.map((row) => row.text) }] });
  const lineTop = $derived(lineTable ? scaleTop([...lineTable.series.flatMap((each) => each.values), target ?? undefined]) : 0);
  const lines = $derived(lineTable ? seriesLines(scaled(lineTable, lineTop), W, H) : []);
  const targetY = $derived(target !== null && target !== undefined && lineTop > 0 ? H - (Math.max(target, 0) / lineTop) * H : undefined);
  const lineEnds = $derived(lineTable && lineTable.series.length > 1 ? lineTable.series.map((each) => each.name) : []);
  const xLabels = $derived(lineTable ? pickEnds(lineTable.labels.map(labelText)) : []);

  /** The series scaled down, so a target above every value still fits the scale. */
  function scaled(of: SeriesTable, top: number): SeriesTable {
    const own = scaleTop(of.series.flatMap((each) => each.values));
    if (own >= top || own <= 0) return of;
    const ratio = own / top;
    return { labels: of.labels, series: of.series.map((each) => ({ ...each, values: each.values.map((v) => (v === undefined ? v : v * ratio)) })) };
  }
  /** Under a line, its first, middle and last label. */
  function pickEnds(labels: string[]): string[] {
    if (labels.length <= 3) return labels;
    return [labels[0]!, labels[Math.floor((labels.length - 1) / 2)]!, labels[labels.length - 1]!];
  }

  const summary = $derived(t('app.dashboards.chart_summary', { title, count: String(rows.length) }));

  // --- The table every chart has as its toggle --------------------------------------------------
  const columns = $derived<DataColumn[]>([
    { id: 'label', label: t('app.dashboards.col_group'), priority: 1 },
    { id: 'value', label: t('app.dashboards.col_value'), align: 'end', priority: 1, isStatus: true },
  ]);
  const seriesColumns = $derived<DataColumn[]>(table ? [
    { id: 'period', label: t('app.dashboards.col_period'), priority: 1 },
    ...table.series.map((each, s) => ({ id: `s${s}`, label: each.name, align: 'end' as const, priority: (s === 0 ? 1 : 2) as 1 | 2, isStatus: s === 0 })),
  ] : []);
  const seriesRows = $derived(table ? table.labels.map((label, i) => ({ label, i })) : []);
  const matrixColumns = $derived<DataColumn[]>(matrix ? [
    { id: 'row', label: '', priority: 1, isLabelHidden: true },
    ...matrix.columns.map((column, c) => ({ id: `c${c}`, label: labelText(column), align: 'end' as const, priority: (c === 0 ? 1 : 2) as 1 | 2, isStatus: c === 0 })),
  ] : []);
  const matrixRows = $derived(matrix ? matrix.rows.map((row, r) => ({ row, r })) : []);
  const entryColumns = $derived<DataColumn[]>([
    { id: 'key', label: t('app.dashboards.col_key'), priority: 1 },
    { id: 'title', label: t('app.dashboards.col_title'), priority: 1 },
    { id: 'column', label: t('app.dashboards.col_value'), align: 'end', priority: 2, isStatus: true },
  ]);
  type SeriesRow = { label: string; i: number };
  type MatrixRow = { row: string; r: number };
  const isEmpty = $derived(chart === 'TEXT' ? pieces.length === 0 : chart === 'LIST' ? entries.length === 0 : multi ? (table?.labels.length ?? 0) === 0 : rows.length === 0);
</script>

{#if isEmpty}
  <p class="quiet" data-widget-empty>{empty ?? t('app.dashboards.no_rows')}</p>
{:else if chart === 'TEXT'}
  <p class="words" data-chart="TEXT">{#each pieces as piece, i (i)}{#if piece.kind === 'bold'}<strong>{piece.text}</strong>{:else if piece.kind === 'link'}<a href={piece.href}>{piece.text}</a>{:else}{piece.text}{/if}{/each}</p>
{:else if isTable}
  <div class="scroll" data-chart={chart} data-table>
    {#if chart === 'LIST'}
      <DataTable label={title} columns={entryColumns} rows={entries} key={(entry: ListEntry) => entry.id}>
        {#snippet cell(entry: ListEntry, column: DataColumn)}
          {#if column.id === 'key'}<span class="key">{entry.key}</span>{:else if column.id === 'title'}<a href={entry.href}>{entry.title}</a>{:else}{entry.column}{/if}
        {/snippet}
      </DataTable>
    {:else if multi && table}
      <DataTable label={title} columns={seriesColumns} rows={seriesRows} key={(row: SeriesRow) => String(row.i)}>
        {#snippet cell(row: SeriesRow, column: DataColumn)}
          {#if column.id === 'period'}{labelText(row.label)}
          {:else}{@const s = Number(column.id.slice(1))}<span class="cell"><span class="swatch" data-series={seriesOf(s)} aria-hidden="true"></span>{table?.series[s]?.texts[row.i] ?? ''}</span>{/if}
        {/snippet}
      </DataTable>
    {:else if matrix}
      <DataTable label={title} columns={matrixColumns} rows={matrixRows} key={(row: MatrixRow) => String(row.r)}>
        {#snippet cell(row: MatrixRow, column: DataColumn)}
          {#if column.id === 'row'}{labelText(row.row)}{:else}<span class="cell">{matrix?.cells[row.r]?.[Number(column.id.slice(1))] ?? ''}</span>{/if}
        {/snippet}
      </DataTable>
    {:else}
      <DataTable label={title} {columns} {rows} key={(row: FigureRow) => row.key}>
        {#snippet cell(row: FigureRow, column: DataColumn)}
          {#if column.id === 'label'}
            {#if row.goalId}<a href={`/goals/${row.goalId}`}>{labelText(row.label)}</a>{:else}{labelText(row.label)}{/if}{#if row.column}<span class="note">{row.column}</span>{/if}
          {:else}<span class="cell">{row.text}</span>{/if}
        {/snippet}
      </DataTable>
    {/if}
  </div>
{:else if chart === 'NUMBER' && figure}
  <div class="number" data-chart="NUMBER">
    <p class="big" aria-label={t('app.dashboards.total_of', { title, total: figure.text })}>{figure.text}</p>
    <p class="change">
      {#if figure.change !== undefined}
        <span data-state={standing ?? 'none'}>{t(figure.change > 0 ? 'app.dashboards.change_up' : figure.change < 0 ? 'app.dashboards.change_down' : 'app.dashboards.change_same', { by: format(Math.abs(figure.change)) })}</span>
      {/if}
      {#if target !== null && target !== undefined}
        <span class="aim">{t(lowerIsBetter ? 'app.dashboards.target_at_most' : 'app.dashboards.target_at_least', { target: format(target) })}{#if warnAt !== null && warnAt !== undefined}{' · '}{t('app.dashboards.warn_from', { warn: format(warnAt) })}{/if}</span>
        {#if standing}<span class="word" data-state={standing}>{t(`app.dashboards.target_${standing}`)}</span>{/if}
      {/if}
    </p>
    {#if figure.spark}
      <svg class="spark" viewBox="0 0 100 24" preserveAspectRatio="none" aria-hidden="true"><polyline points={figure.spark} data-series="1" /></svg>
    {/if}
  </div>
{:else if chart === 'BAR'}
  <ul class="hbars" data-chart="BAR" aria-label={summary}>
    {#if multi}
      {#each pairs as pair, i (i)}
        <li>
          <span class="name">{labelText(pair.label)}</span>
          <span class="track">{#each pair.shares as share, s (s)}<span class="bar" data-series={seriesOf(s)} style:--share={share}></span>{/each}</span>
          <span class="figure">{pair.texts.join('/')}</span>
        </li>
      {/each}
    {:else}
      {#each bars as bar (bar.row.key)}
        <li>
          <span class="name">{labelText(bar.row.label)}</span>
          <span class="track"><span class="bar" data-series="1" style:--share={bar.share}></span></span>
          <span class="figure">{bar.row.text}</span>
        </li>
      {/each}
    {/if}
  </ul>
{:else if chart === 'PIE'}
  <div class="ring" data-chart="PIE">
    <svg viewBox="0 0 42 42" role="img" aria-label={summary}>
      {#each ring as part (part.row.key)}
        <circle cx="21" cy="21" r="15.915" stroke-width="6" data-series={part.series} stroke-dasharray={`${part.share * 100} ${100 - part.share * 100}`} stroke-dashoffset={25 - part.start * 100} />
      {/each}
      <text x="21" y="21" font-size="7" dominant-baseline="central" text-anchor="middle">{percentOf(ring[0]?.share)} %</text>
    </svg>
    <ul class="legend">
      {#each ring as part (part.row.key)}
        <li><span class="swatch" data-series={part.series} aria-hidden="true"></span>{labelText(part.row.label)} <strong>{part.row.text}</strong></li>
      {/each}
    </ul>
  </div>
{:else if chart === 'LINE' && lineTable}
  <div class="plot" data-chart="LINE">
    <div class="y" aria-hidden="true"><span>{format(lineTop)}</span><span>{format(lineTop / 2)}</span></div>
    <div class="area">
      <svg class="chart" viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" role="img" aria-label={summary}>
        <line class="grid" x1="0" x2={W} y1="0" y2="0" /><line class="grid" x1="0" x2={W} y1={H / 2} y2={H / 2} /><line class="grid" x1="0" x2={W} y1={H} y2={H} />
        {#if targetY !== undefined}<line class="target" x1="0" x2={W} y1={targetY} y2={targetY} />{/if}
        {#each lines as points, s (s)}<polyline points={lineThrough(points, W)} data-series={seriesOf(s)} class:second={s === 1} /><path class="dots" d={lineDots(points)} data-series={seriesOf(s)} />{/each}
      </svg>
      {#if lineEnds.length > 0 || targetY !== undefined}
        <p class="ends">
          {#each lineEnds as name, s (s)}<span><span class="dash" class:second={s === 1} data-series={seriesOf(s)} aria-hidden="true"></span>{name}</span>{/each}
          {#if target !== null && target !== undefined}<span><span class="dash target" aria-hidden="true"></span>{t('app.dashboards.target_line', { target: format(target) })}</span>{/if}
        </p>
      {/if}
    </div>
    <div class="x" aria-hidden="true">{#each xLabels as label, i (i)}<span>{label}</span>{/each}</div>
  </div>
{:else if chart === 'TABLE'}
  <div class="scroll" data-chart="TABLE">
    {#if multi && table}
      <DataTable label={title} columns={seriesColumns} rows={seriesRows} key={(row: SeriesRow) => String(row.i)}>
        {#snippet cell(row: SeriesRow, column: DataColumn)}
          {#if column.id === 'period'}{labelText(row.label)}{:else}<span class="cell">{table?.series[Number(column.id.slice(1))]?.texts[row.i] ?? ''}</span>{/if}
        {/snippet}
      </DataTable>
    {:else}
      <DataTable label={title} {columns} {rows} key={(row: FigureRow) => row.key}>
        {#snippet cell(row: FigureRow, column: DataColumn)}
          {#if column.id === 'label'}{labelText(row.label)}{#if row.column}<span class="note">{row.column}</span>{/if}{:else}<span class="cell">{row.text}</span>{/if}
        {/snippet}
      </DataTable>
    {/if}
  </div>
{:else if chart === 'MATRIX' && matrix}
  <div class="matrix" data-chart="MATRIX" style:--columns={matrix.columns.length} role="img" aria-label={summary}>
    <span></span>
    {#each matrix.columns as column, c (c)}<span class="head">{labelText(column)}</span>{/each}
    {#each matrix.rows as row, r (r)}
      <span class="name">{labelText(row)}</span>
      {#each matrix.cells[r] ?? [] as cell, c (c)}<span class="step" data-step={steps[r]?.[c] ?? 0} title={cell}>{cell}</span>{/each}
    {/each}
  </div>
{:else if chart === 'PROGRESS'}
  <ul class="goals" data-chart="PROGRESS">
    {#each rows as row (row.key)}
      <li>
        <span class="goal">{#if row.goalId}<a href={`/goals/${row.goalId}`}>{labelText(row.label)}</a>{:else}{labelText(row.label)}{/if}<span class="figure">{t('app.goals.percent', { percent: row.text || '0' })}</span></span>
        <ProgressBar size="sm" label={t('app.goals.progress_of', { title: labelText(row.label) })} value={percentOf(row.value / 100)} />
      </li>
    {/each}
  </ul>
{:else if chart === 'LIST'}
  <ul class="entries" data-chart="LIST">
    {#each entries as entry (entry.id)}
      <li><a href={entry.href}><span class="key">{entry.key}</span><span class="title">{entry.title}</span></a><span class="column" data-tone={entry.tone ?? 'none'}>{entry.column}</span></li>
    {/each}
  </ul>
{/if}

<style>
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .scroll { overflow-x: auto; }
  .note { margin-inline-start: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); }
  .cell, .figure { font-variant-numeric: tabular-nums; white-space: nowrap; text-align: end; }
  .swatch { display: inline-block; inline-size: var(--sp-100); block-size: var(--sp-100); margin-inline-end: var(--sp-050); border-radius: var(--r-full); background: var(--series); }
  .key { flex: none; font-family: var(--font-mono); font-size: var(--fs-075); color: var(--text-secondary); white-space: nowrap; }
  .goal a { color: var(--text-primary); text-decoration: none; }
  .goal a:hover { text-decoration: underline; }

  .number { display: flex; flex-direction: column; gap: var(--sp-050); }
  .big { margin: 0; font-size: var(--fs-700); font-weight: var(--fw-semibold); line-height: 1; font-variant-numeric: tabular-nums; }
  .change { display: flex; flex-wrap: wrap; gap: var(--sp-100); margin: 0; font-size: var(--fs-075); color: var(--text-secondary); }
  .change [data-state] { padding-inline: var(--sp-050); border-radius: var(--r-xs); }
  .change [data-state='met'] { color: var(--status-success-text); background: var(--status-success-surface); }
  .change [data-state='warn'] { color: var(--status-danger-text); background: var(--status-danger-surface); }
  .word { font-weight: var(--fw-medium); }
  .spark { inline-size: 50%; block-size: var(--sp-300); overflow: visible; }

  .hbars, .legend, .goals, .entries { display: flex; flex-direction: column; margin: 0; padding: 0; list-style: none; min-width: 0; }
  .hbars { gap: var(--sp-100); }
  .hbars li { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 2fr) auto; gap: var(--sp-100); align-items: center; font-size: var(--fs-075); }
  .hbars .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .track { display: flex; gap: var(--bw-thick); min-width: 0; }
  .bar { display: block; inline-size: calc(var(--share) * 1%); block-size: var(--sp-150); border-radius: var(--r-xs); background: var(--series); }

  .ring { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-200); }
  .ring svg { inline-size: var(--sp-1000); block-size: var(--sp-1000); }
  .ring circle { fill: none; stroke: var(--series); }
  .ring text { fill: var(--text-primary); font-weight: var(--fw-semibold); }
  .legend { gap: var(--sp-050); font-size: var(--fs-075); }

  .plot { display: grid; grid-template-columns: auto minmax(0, 1fr); column-gap: var(--sp-050); row-gap: var(--sp-025); }
  .area { grid-column: 2; grid-row: 1; min-width: 0; }
  .chart { inline-size: 100%; block-size: var(--sp-1600); overflow: visible; }
  .chart polyline { fill: none; stroke: var(--series); stroke-width: var(--bw-thick); vector-effect: non-scaling-stroke; }
  .chart .dots { fill: none; stroke: var(--series); stroke-width: var(--sp-050); stroke-linecap: round; vector-effect: non-scaling-stroke; }
  .chart polyline.second { stroke-dasharray: var(--sp-100) var(--sp-050); }
  .chart .grid { stroke: var(--border-subtle); vector-effect: non-scaling-stroke; }
  .chart .target { stroke: var(--text-subtle); stroke-dasharray: var(--sp-025) var(--sp-050); vector-effect: non-scaling-stroke; }
  .y { grid-column: 1; grid-row: 1; display: flex; flex-direction: column; justify-content: space-between; block-size: var(--sp-1600); text-align: end; }
  .x { grid-column: 2; grid-row: 2; display: flex; justify-content: space-between; }
  .y, .x, .ends { color: var(--text-subtle); font-size: var(--fs-075); font-variant-numeric: tabular-nums; }
  .ends { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: var(--sp-150); margin: var(--sp-050) 0 0; }
  .dash { display: inline-block; inline-size: var(--sp-200); margin-inline-end: var(--sp-050); vertical-align: middle; border-block-start: var(--bw-thick) solid var(--series); }
  .dash.second { border-block-start-style: dashed; }
  .dash.target { border-block-start: var(--bw-hairline) dashed var(--text-subtle); }

  .matrix { display: grid; grid-template-columns: auto repeat(var(--columns), minmax(0, 1fr)); gap: var(--bw-thick); font-size: var(--fs-075); }
  .matrix .head { text-align: center; color: var(--text-secondary); }
  .matrix .name { padding-inline-end: var(--sp-100); white-space: nowrap; }
  .step { position: relative; isolation: isolate; text-align: center; font-variant-numeric: tabular-nums; border-radius: var(--r-xs); color: var(--text-primary); }
  .step::before { content: ''; position: absolute; inset: 0; z-index: -1; border-radius: inherit; background: var(--chart-categorical-1); opacity: calc(var(--step-share) * 1); }
  .step[data-step='0'] { --step-share: 0; }
  .step[data-step='1'] { --step-share: 0.2; }
  .step[data-step='2'] { --step-share: 0.4; }
  .step[data-step='3'] { --step-share: 0.6; }
  .step[data-step='4'] { --step-share: 0.8; color: var(--text-inverse); }
  .step[data-step='5'] { --step-share: 1; color: var(--text-inverse); }

  .goals { gap: var(--sp-150); font-size: var(--fs-075); }
  .goal { display: flex; justify-content: space-between; gap: var(--sp-100); }

  .entries li { display: flex; align-items: baseline; gap: var(--sp-100); padding-block: var(--sp-100); border-block-end: var(--bw-hairline) solid var(--border-subtle); font-size: var(--fs-075); }
  .entries a { display: flex; gap: var(--sp-100); flex: 1 1 auto; min-width: 0; color: var(--text-primary); text-decoration: none; }
  .entries a:hover .title { text-decoration: underline; }
  .entries .title { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .column { flex: none; font-variant-numeric: tabular-nums; color: var(--text-secondary); }
  .column[data-tone='danger'], .column[data-tone='warning'] { padding-inline: var(--sp-050); border-radius: var(--r-xs); }
  .column[data-tone='danger'] { color: var(--status-danger-text); background: var(--status-danger-surface); }
  .column[data-tone='warning'] { color: var(--status-warning-text); background: var(--status-warning-surface); }

  .words { margin: 0; font-size: var(--fs-075); line-height: var(--lh-normal); }

  [data-series='1'] { --series: var(--chart-categorical-1); }
  [data-series='2'] { --series: var(--chart-categorical-2); }
  [data-series='3'] { --series: var(--chart-categorical-3); }
  [data-series='4'] { --series: var(--chart-categorical-4); }
  [data-series='5'] { --series: var(--chart-categorical-5); }
  [data-series='6'] { --series: var(--chart-categorical-6); }
  [data-series='7'] { --series: var(--chart-categorical-7); }
</style>
