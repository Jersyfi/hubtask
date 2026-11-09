<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One dashboard widget's figures, drawn (UC-GOL-04 check 8): a number, bars, a pie, a line, a
  // matrix, goals' progress or a table - and **always a table beside the chart** with the same rows,
  // so that a reader who cannot see the shapes reads the numbers (P-13). Colours are the design
  // system's label colours by series (rule 15); the table's swatch says which row is which.
  //
  // Nothing is computed here but the layout: the rows are the server's, counted for this reader.
  // A widget of two series - created beside resolved, and with running totals what is left - is
  // handed over as one table of series, drawn as grouped bars or one line per series.

  import type { Snippet } from 'svelte';

  import { DataTable, ProgressBar, type DataColumn } from '@hubtask/design-system/components';

  import {
    barLayout, lineDots, linePoints, matrixOf, percentOf, pieSlices, scaleTop, seriesBars, seriesLines, seriesOf, totalOf, type FigureRow, type SeriesTable,
  } from '../data/goals.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    title: string;
    chart: 'NUMBER' | 'BAR' | 'PIE' | 'LINE' | 'TABLE' | 'MATRIX' | 'PROGRESS';
    rows: readonly FigureRow[];
    /** Two series or more, aligned by period; drawn in place of `rows` where given. */
    table?: SeriesTable;
    /** Why there are no rows, by the widget's source (user-experience.md §7.7); a general line without. */
    empty?: string;
  }

  const { title, chart, rows, table, empty }: Props = $props();

  const W = 400;
  const H = 160;
  const multi = $derived(table !== undefined && (chart === 'BAR' || chart === 'LINE' || chart === 'TABLE'));
  const multiBars = $derived(multi && table && chart === 'BAR' ? seriesBars(table, W, H) : []);
  const multiLines = $derived(multi && table && chart === 'LINE' ? seriesLines(table, W, H) : []);
  const bars = $derived(chart === 'BAR' ? barLayout(rows, W, H) : []);
  const slices = $derived(chart === 'PIE' ? pieSlices(rows) : []);
  const line = $derived(chart === 'LINE' ? linePoints(rows, W, H) : '');
  const matrix = $derived(chart === 'MATRIX' ? matrixOf(rows) : undefined);
  const total = $derived(totalOf(rows));
  const sliceOf = (key: string) => slices.find((slice) => slice.key === key);
  const columns = $derived<DataColumn[]>([
    { id: 'label', label: t('app.dashboards.col_group'), priority: 1 },
    { id: 'value', label: t('app.dashboards.col_value'), align: 'end', priority: 1, isStatus: true },
  ]);
  // The series' and the matrix' tables are DataTables too (PS-4): a row is a period or a row name,
  // a column a series or a matrix column, the first of them kept on a phone as the row's state.
  const seriesColumns = $derived<DataColumn[]>(table ? [
    { id: 'period', label: t('app.dashboards.col_period'), priority: 1 },
    ...table.series.map((each, s) => ({ id: `s${s}`, label: each.name, align: 'end' as const, priority: (s === 0 ? 1 : 2) as 1 | 2, isStatus: s === 0 })),
  ] : []);
  const seriesRows = $derived(table ? table.labels.map((label, i) => ({ label, i })) : []);
  const matrixColumns = $derived<DataColumn[]>(matrix ? [
    { id: 'row', label: '', priority: 1, isLabelHidden: true },
    ...matrix.columns.map((column, c) => ({ id: `c${c}`, label: column || t('app.dashboards.no_value'), align: 'end' as const, priority: (c === 0 ? 1 : 2) as 1 | 2, isStatus: c === 0 })),
  ] : []);
  const matrixRows = $derived(matrix ? matrix.rows.map((row, r) => ({ row, r })) : []);
  type SeriesRow = { label: string; i: number };
  type MatrixRow = { row: string; r: number };
  const summary = $derived(t('app.dashboards.chart_summary', { title, count: String(rows.length) }));
  const labelText = (row: FigureRow) => row.label === '' ? t('app.dashboards.no_value') : row.label;

  // The axes are words beside the drawing, not in it: the drawing stretches to its box, and a
  // text inside it would stretch with it. The table beside lists the same, so they are hidden
  // from a screen reader.
  const MAX_AXIS_LABELS = 12;
  const axisLabels = $derived(multi && table ? table.labels.map((label) => label || t('app.dashboards.no_value')) : rows.map(labelText));
  const top = $derived(multi && table ? scaleTop(table.series.flatMap((each) => each.values)) : scaleTop(rows.map((row) => row.value)));
  const topText = $derived(new Intl.NumberFormat(messages.locale).format(top));
  const zeroText = $derived(new Intl.NumberFormat(messages.locale).format(0));
  /** Under bars, each bar's label where they fit; under a line, where it starts and ends. */
  const xLabels = $derived(chart === 'LINE'
    ? (axisLabels.length > 1 ? [axisLabels[0]!, axisLabels[axisLabels.length - 1]!] : axisLabels)
    : axisLabels.length <= MAX_AXIS_LABELS ? axisLabels : []);
</script>

{#snippet plot(drawing: Snippet)}
  <div class="plot" data-plot={chart}>
    <div class="y" aria-hidden="true"><span>{topText}</span><span>{zeroText}</span></div>
    {@render drawing()}
    {#if xLabels.length > 0}
      <div class="x" class:ends={chart === 'LINE' && xLabels.length > 1} aria-hidden="true">{#each xLabels as label, i (i)}<span>{label}</span>{/each}</div>
    {/if}
  </div>
{/snippet}

{#if multi && table}
  {#if table.labels.length === 0}
    <p class="quiet" data-widget-empty>{empty ?? t('app.dashboards.no_rows')}</p>
  {:else}
    {@const multiSummary = t('app.dashboards.series_summary', { title, count: String(table.labels.length), series: table.series.map((each) => each.name).join(', ') })}
    <div class="figure" data-chart={chart}>
      {#if chart === 'BAR'}
        {#snippet multiBarDrawing()}
          <svg class="chart" viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" role="img" aria-label={multiSummary}>
            {#each multiBars as bar (bar.key)}<rect x={bar.x} y={bar.y} width={bar.width} height={bar.height} data-series={bar.series} />{/each}
          </svg>
        {/snippet}
        {@render plot(multiBarDrawing)}
      {:else if chart === 'LINE'}
        {#snippet multiLineDrawing()}
          <svg class="chart" viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" role="img" aria-label={multiSummary}>
            {#each multiLines as points, s (s)}<polyline {points} data-series={seriesOf(s)} /><path class="dots" d={lineDots(points)} data-series={seriesOf(s)} />{/each}
          </svg>
        {/snippet}
        {@render plot(multiLineDrawing)}
      {/if}
      <div class="scroll table" class:only={chart === 'TABLE'}>
        <DataTable label={title} columns={seriesColumns} rows={seriesRows} key={(row: SeriesRow) => String(row.i)}>
          {#snippet cell(row: SeriesRow, column: DataColumn)}
            {#if column.id === 'period'}{row.label || t('app.dashboards.no_value')}
            {:else}{@const s = Number(column.id.slice(1))}<span class="cell"><span class="swatch" data-series={seriesOf(s)} aria-hidden="true"></span>{table?.series[s]?.texts[row.i] ?? ''}</span>{/if}
          {/snippet}
        </DataTable>
      </div>
    </div>
  {/if}
{:else if rows.length === 0}
  <p class="quiet" data-widget-empty>{empty ?? t('app.dashboards.no_rows')}</p>
{:else}
  <div class="figure" data-chart={chart}>
    {#if chart === 'NUMBER'}
      <p class="number-big" aria-label={t('app.dashboards.total_of', { title, total: String(total) })}>{total}</p>
    {:else if chart === 'BAR'}
      {#snippet barDrawing()}
        <svg class="chart" viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" role="img" aria-label={summary}>
          {#each bars as bar (bar.key)}<rect x={bar.x} y={bar.y} width={bar.width} height={bar.height} data-series={bar.series} />{/each}
        </svg>
      {/snippet}
      {@render plot(barDrawing)}
    {:else if chart === 'PIE'}
      <svg class="pie" viewBox="-1.05 -1.05 2.1 2.1" role="img" aria-label={summary}>
        {#each slices as slice (slice.key)}<path d={slice.path} data-series={slice.series} />{/each}
      </svg>
    {:else if chart === 'LINE'}
      {#snippet lineDrawing()}
        <svg class="chart" viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" role="img" aria-label={summary}>
          <polyline points={line} data-series="blue" /><path class="dots" d={lineDots(line)} data-series="blue" />
        </svg>
      {/snippet}
      {@render plot(lineDrawing)}
    {:else if chart === 'PROGRESS'}
      <ul class="bars">
        {#each rows as row (row.key)}
          <li>
            {#if row.goalId}<a href={`/goals/${row.goalId}`}>{labelText(row)}</a>{:else}{labelText(row)}{/if}
            <ProgressBar size="sm" label={t('app.goals.progress_of', { title: labelText(row) })} value={percentOf(row.value / 100)} valueLabel={t('app.goals.percent', { percent: row.text || '0' })} />
          </li>
        {/each}
      </ul>
    {/if}

    {#if chart === 'MATRIX' && matrix}
      <div class="scroll">
        <DataTable label={title} columns={matrixColumns} rows={matrixRows} key={(row: MatrixRow) => String(row.r)}>
          {#snippet cell(row: MatrixRow, column: DataColumn)}
            {#if column.id === 'row'}{row.row || t('app.dashboards.no_value')}
            {:else}<span class="cell">{matrix?.cells[row.r]?.[Number(column.id.slice(1))] ?? ''}</span>{/if}
          {/snippet}
        </DataTable>
      </div>
    {:else if chart !== 'PROGRESS'}
      <!-- The table beside the chart: the same rows, read rather than seen (P-13). -->
      <div class="scroll table" class:only={chart === 'TABLE' || chart === 'NUMBER'}>
        <DataTable label={title} {columns} {rows} key={(row: FigureRow) => row.key}>
          {#snippet cell(row: FigureRow, column: DataColumn)}
            {#if column.id === 'label'}
              {@const i = rows.indexOf(row)}
              {#if chart === 'BAR' || chart === 'PIE'}<span class="swatch" data-series={chart === 'PIE' ? (sliceOf(row.key)?.series ?? 'slate') : seriesOf(i)} aria-hidden="true"></span>{/if}
              {#if row.goalId}<a href={`/goals/${row.goalId}`}>{labelText(row)}</a>{:else}{labelText(row)}{/if}{#if row.column}<span class="note">{row.column}</span>{/if}
            {:else}<span class="value">{row.text}</span>{/if}
          {/snippet}
        </DataTable>
      </div>
    {/if}
  </div>
{/if}

<style>
  .figure { display: flex; flex-wrap: wrap; gap: var(--sp-150); align-items: flex-start; min-width: 0; }
  .figure > .plot, .figure > .pie { flex: 1 1 var(--layout-board-column); min-width: 0; }
  .plot { display: grid; grid-template-columns: auto minmax(0, 1fr); column-gap: var(--sp-050); row-gap: var(--sp-025); }
  .plot > .chart { grid-column: 2; grid-row: 1; overflow: visible; }
  .y { grid-column: 1; grid-row: 1; display: flex; flex-direction: column; justify-content: space-between; text-align: end; }
  .x { grid-column: 2; grid-row: 2; display: flex; min-width: 0; }
  .x > span { flex: 1 1 0; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: center; }
  .x.ends { justify-content: space-between; }
  .x.ends > span { flex: 0 1 auto; }
  .y, .x { color: var(--text-subtle); font-size: var(--fs-075); font-variant-numeric: tabular-nums; }
  .figure > .table { flex: 1 1 var(--layout-board-column); }
  .figure > .table.only { flex-basis: 100%; }
  .chart { width: 100%; height: var(--sp-1600); background: var(--bg-surface-sunken); border-radius: var(--r-sm); }
  .pie { max-height: var(--sp-1600); aspect-ratio: 1; }
  .chart polyline { fill: none; stroke-width: var(--bw-thick); vector-effect: non-scaling-stroke; }
  .chart .dots { fill: none; stroke-width: var(--sp-100); stroke-linecap: round; vector-effect: non-scaling-stroke; }
  .scroll { overflow-x: auto; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .note { margin-inline-start: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); }
  .value, .cell { text-align: end; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .number-big { margin: 0; font-size: var(--fs-700); font-weight: var(--fw-semibold); font-variant-numeric: tabular-nums; }
  .bars { display: flex; flex-direction: column; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; width: 100%; }
  .swatch { display: inline-block; inline-size: var(--sp-100); block-size: var(--sp-100); margin-inline-end: var(--sp-050); border-radius: var(--r-xs); background: var(--series); }

  [data-series='blue'] { --series: var(--label-blue-fg); }
  [data-series='teal'] { --series: var(--label-teal-fg); }
  [data-series='amber'] { --series: var(--label-amber-fg); }
  [data-series='violet'] { --series: var(--label-violet-fg); }
  [data-series='green'] { --series: var(--label-green-fg); }
  [data-series='magenta'] { --series: var(--label-magenta-fg); }
  [data-series='orange'] { --series: var(--label-orange-fg); }
  [data-series='lime'] { --series: var(--label-lime-fg); }
  [data-series='red'] { --series: var(--label-red-fg); }
  [data-series='slate'] { --series: var(--label-slate-fg); }
  rect[data-series], path[data-series] { fill: var(--series); }
  path[data-series] { stroke: var(--bg-surface); stroke-width: var(--bw-hairline); vector-effect: non-scaling-stroke; }
  polyline[data-series], path.dots[data-series] { stroke: var(--series); }
</style>
