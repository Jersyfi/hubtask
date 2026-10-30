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

  import { ProgressBar, Table, VisuallyHidden, type Column } from '@hubtask/design-system/components';

  import {
    barLayout, linePoints, matrixOf, percentOf, pieSlices, seriesBars, seriesLines, seriesOf, totalOf, type FigureRow, type SeriesTable,
  } from '../data/goals.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    title: string;
    chart: 'NUMBER' | 'BAR' | 'PIE' | 'LINE' | 'TABLE' | 'MATRIX' | 'PROGRESS';
    rows: readonly FigureRow[];
    /** Two series or more, aligned by period; drawn in place of `rows` where given. */
    table?: SeriesTable;
  }

  const { title, chart, rows, table }: Props = $props();

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
  const columns = $derived<Column[]>([
    { id: 'label', label: t('app.dashboards.col_group') },
    { id: 'value', label: t('app.dashboards.col_value'), align: 'end' },
  ]);
  const summary = $derived(t('app.dashboards.chart_summary', { title, count: String(rows.length) }));
  const labelText = (row: FigureRow) => row.label === '' ? t('app.dashboards.no_value') : row.label;
</script>

{#if multi && table}
  {#if table.labels.length === 0}
    <p class="quiet">{t('app.dashboards.no_rows')}</p>
  {:else}
    {@const multiSummary = t('app.dashboards.series_summary', { title, count: String(table.labels.length), series: table.series.map((each) => each.name).join(', ') })}
    <div class="figure" data-chart={chart}>
      {#if chart === 'BAR'}
        <svg class="chart" viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" role="img" aria-label={multiSummary}>
          {#each multiBars as bar (bar.key)}<rect x={bar.x} y={bar.y} width={bar.width} height={bar.height} data-series={bar.series} />{/each}
        </svg>
      {:else if chart === 'LINE'}
        <svg class="chart" viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" role="img" aria-label={multiSummary}>
          {#each multiLines as points, s (s)}<polyline {points} data-series={seriesOf(s)} />{/each}
        </svg>
      {/if}
      <div class="scroll table" class:only={chart === 'TABLE'}>
        <table class="matrix">
          <caption><VisuallyHidden>{title}</VisuallyHidden></caption>
          <thead>
            <tr>
              <th scope="col">{t('app.dashboards.col_period')}</th>
              {#each table.series as each, s (s)}<th scope="col" class="value"><span class="swatch" data-series={seriesOf(s)} aria-hidden="true"></span>{each.name}</th>{/each}
            </tr>
          </thead>
          <tbody>
            {#each table.labels as label, i (label)}
              <tr><th scope="row">{label || t('app.dashboards.no_value')}</th>{#each table.series as each, s (s)}<td class="cell">{each.texts[i]}</td>{/each}</tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}
{:else if rows.length === 0}
  <p class="quiet">{t('app.dashboards.no_rows')}</p>
{:else}
  <div class="figure" data-chart={chart}>
    {#if chart === 'NUMBER'}
      <p class="number-big" aria-label={t('app.dashboards.total_of', { title, total: String(total) })}>{total}</p>
    {:else if chart === 'BAR'}
      <svg class="chart" viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" role="img" aria-label={summary}>
        {#each bars as bar (bar.key)}<rect x={bar.x} y={bar.y} width={bar.width} height={bar.height} data-series={bar.series} />{/each}
      </svg>
    {:else if chart === 'PIE'}
      <svg class="pie" viewBox="-1.05 -1.05 2.1 2.1" role="img" aria-label={summary}>
        {#each slices as slice (slice.key)}<path d={slice.path} data-series={slice.series} />{/each}
      </svg>
    {:else if chart === 'LINE'}
      <svg class="chart" viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" role="img" aria-label={summary}>
        <polyline points={line} data-series="blue" />
      </svg>
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
        <table class="matrix">
          <caption><VisuallyHidden>{title}</VisuallyHidden></caption>
          <thead><tr><td></td>{#each matrix.columns as column (column)}<th scope="col">{column || t('app.dashboards.no_value')}</th>{/each}</tr></thead>
          <tbody>
            {#each matrix.rows as row, r (row)}
              <tr><th scope="row">{row || t('app.dashboards.no_value')}</th>{#each matrix.cells[r] ?? [] as cell, c (c)}<td class="cell">{cell}</td>{/each}</tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else if chart !== 'PROGRESS'}
      <!-- The table beside the chart: the same rows, read rather than seen (P-13). -->
      <div class="scroll table" class:only={chart === 'TABLE' || chart === 'NUMBER'}>
        <Table label={title} isLabelHidden {columns}>
          {#each rows as row, i (row.key)}
            <tr>
              <th scope="row">
                {#if chart === 'BAR' || chart === 'PIE'}<span class="swatch" data-series={chart === 'PIE' ? (sliceOf(row.key)?.series ?? 'slate') : seriesOf(i)} aria-hidden="true"></span>{/if}
                {#if row.goalId}<a href={`/goals/${row.goalId}`}>{labelText(row)}</a>{:else}{labelText(row)}{/if}{#if row.column}<span class="note">{row.column}</span>{/if}
              </th>
              <td class="value">{row.text}</td>
            </tr>
          {/each}
        </Table>
      </div>
    {/if}
  </div>
{/if}

<style>
  .figure { display: flex; flex-wrap: wrap; gap: var(--sp-150); align-items: flex-start; min-width: 0; }
  .figure > .chart, .figure > .pie { flex: 1 1 var(--layout-board-column); min-width: 0; }
  .figure > .table { flex: 1 1 var(--layout-board-column); }
  .figure > .table.only { flex-basis: 100%; }
  .chart { width: 100%; height: var(--sp-1600); background: var(--bg-surface-sunken); border-radius: var(--r-sm); }
  .pie { max-height: var(--sp-1600); aspect-ratio: 1; }
  .chart polyline { fill: none; stroke-width: var(--bw-thick); vector-effect: non-scaling-stroke; }
  .scroll { overflow-x: auto; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .note { margin-inline-start: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); }
  .value, .cell { text-align: end; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .number-big { margin: 0; font-size: var(--fs-700); font-weight: var(--fw-semibold); font-variant-numeric: tabular-nums; }
  .bars { display: flex; flex-direction: column; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; width: 100%; }
  .swatch { display: inline-block; inline-size: var(--sp-100); block-size: var(--sp-100); margin-inline-end: var(--sp-050); border-radius: var(--r-xs); background: var(--series); }
  .matrix { border-collapse: collapse; font-size: var(--fs-075); }
  .matrix th, .matrix td { padding: var(--sp-050) var(--sp-100); border-block-end: var(--bw-hairline) solid var(--border-subtle); }

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
  polyline[data-series] { stroke: var(--series); }
</style>
