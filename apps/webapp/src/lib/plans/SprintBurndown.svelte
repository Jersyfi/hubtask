<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A running sprint's burndown as a small chart (work-surface §6.1): the work left and the ideal
  // line over the sprint's days, on axes that say both. Today's sprint card and the plans' home
  // draw the same one.
  import type { SprintDetail } from '@hubtask/sync-engine';

  import { burndownLine, burndownTop, sprintDays } from '../data/sprints.ts';
  import { formatPlainDay } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  /** How far the lines stand from the chart's top and bottom edge, in its own units. */
  const INSET = 6;

  const { detail }: { detail: SprintDetail } = $props();

  const days = $derived(sprintDays(detail.sprint));
  const top = $derived(burndownTop(detail.sprint, detail.burndown));
  const last = $derived(detail.burndown.at(-1));
  const unit = $derived(t(`app.sprints.unit_${detail.sprint.unit}`));
</script>

{#if detail.burndown.length > 0}
  <div class="burndown" data-burndown>
    <span class="y-top" aria-hidden="true">{top}</span>
    <svg class="spark" viewBox="0 0 400 100" preserveAspectRatio="none" role="img" aria-label={t('app.sprints.burndown_label', { remaining: String(last?.remaining ?? 0), unit })}>
      <polyline class="axis" points="0,0 0,100 400,100" />
      <polyline class="ideal" points={burndownLine(detail.burndown, 'ideal', days, top, 400, 100, INSET)} />
      <polyline class="remaining" points={burndownLine(detail.burndown, 'remaining', days, top, 400, 100, INSET)} />
    </svg>
    <span class="y-zero" aria-hidden="true">0</span>
    <span class="x-days" aria-hidden="true"><span>{formatPlainDay(detail.sprint.start_on, messages.locale)}</span><span>{formatPlainDay(detail.sprint.end_on, messages.locale)}</span></span>
  </div>
{/if}

<style>
  .burndown {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    grid-template-areas: 'top chart' 'zero chart' '. days';
    column-gap: var(--sp-050);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-variant-numeric: tabular-nums;
  }
  .y-top { grid-area: top; align-self: start; text-align: end; }
  .y-zero { grid-area: zero; align-self: end; text-align: end; }
  .x-days { grid-area: days; display: flex; justify-content: space-between; gap: var(--sp-100); }
  .spark { grid-area: chart; inline-size: 100%; block-size: var(--sp-600); overflow: hidden; }
  .spark polyline { fill: none; stroke-width: var(--bw-thick); vector-effect: non-scaling-stroke; }
  .spark .axis { stroke: var(--border-subtle); stroke-width: var(--bw-hairline); }
  .spark .remaining { stroke: var(--accent-primary); }
  .spark .ideal { stroke: var(--border-strong); }
</style>
