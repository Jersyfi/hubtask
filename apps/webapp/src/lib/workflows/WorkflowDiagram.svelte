<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A workflow drawn (UC-FLW-02 check 2): one column per category, the statuses stacked in their
  // order, and a line with an arrow for each step from a status it leaves to the one it leads to.
  // A step from any status is written on its target instead, because a line from every card would
  // hide the others.
  //
  // **It draws and selects, it does not edit.** A click on a status or a step selects it, and the
  // list beside it - which holds every action the diagram offers and more - edits it. The lines
  // are measured from the cards after they are laid out, so no length is written here (rule 15).

  import { LabelChip } from '@hubtask/design-system/components';
  import type { WorkflowShape } from '@hubtask/sync-engine';

  import { CATEGORIES, statusLabel, stepLabel } from '../data/workflows.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    shape: WorkflowShape;
    /** `status:<key>` or `step:<key>`, or nothing. */
    selected?: string;
    onselect: (selected: string) => void;
  }

  const { shape, selected, onselect }: Props = $props();

  const initial = $derived(shape.initial ?? shape.statuses.find((status) => status.category !== 'DONE')?.key);
  const columns = $derived(
    CATEGORIES.map((category) => ({
      category,
      statuses: shape.statuses.filter((status) => status.category === category),
    })),
  );

  let surface = $state<HTMLDivElement | undefined>(undefined);
  const cards = new Map<string, HTMLElement>();
  let lines = $state<readonly { key: string; d: string; isSelected: boolean }[]>([]);
  let extent = $state({ width: 0, height: 0 });

  function card(node: HTMLElement, key: string) {
    cards.set(key, node);
    return { destroy: () => cards.delete(key) };
  }

  /** Measures every card and draws a curve from the source's edge to the target's. */
  function measure() {
    if (!surface) return;
    const origin = surface.getBoundingClientRect();
    extent = { width: origin.width, height: origin.height };
    const out: { key: string; d: string; isSelected: boolean }[] = [];
    for (const step of shape.steps) {
      const target = cards.get(step.to)?.getBoundingClientRect();
      if (!target || !step.from?.length) continue;
      for (const from of step.from) {
        const source = cards.get(from)?.getBoundingClientRect();
        if (!source) continue;
        const forward = source.left <= target.left;
        // design-system-lint-ignore: measured screen rectangles; the drawing follows the cards either way.
        const x1 = (forward ? source.right : source.left) - origin.left;
        const y1 = source.top + source.height / 2 - origin.top;
        const sameColumn = Math.abs(source.left - target.left) < source.width / 2;
        // design-system-lint-ignore: measured screen rectangles, as above.
        const x2 = (sameColumn ? (forward ? target.right : target.left) : forward ? target.left : target.right) - origin.left;
        const y2 = target.top + target.height / 2 - origin.top;
        const bend = sameColumn ? source.width / 3 : Math.abs(x2 - x1) / 2;
        const c1 = forward ? x1 + bend : x1 - bend;
        const c2 = sameColumn ? c1 : forward ? x2 - bend : x2 + bend;
        out.push({ key: `${step.key}:${from}`, d: `M ${x1} ${y1} C ${c1} ${y1}, ${c2} ${y2}, ${x2} ${y2}`, isSelected: selected === `step:${step.key}` });
      }
    }
    lines = out;
  }

  $effect(() => {
    // Re-measured whenever the shape or the selection changes, and whenever the surface resizes.
    void shape;
    void selected;
    if (!surface) return;
    const frame = requestAnimationFrame(measure);
    const observer = new ResizeObserver(() => measure());
    observer.observe(surface);
    return () => {
      cancelAnimationFrame(frame);
      observer.disconnect();
    };
  });

  const anyInto = (key: string) => shape.steps.filter((step) => step.to === key && !step.from?.length);
  const outOf = (key: string) => shape.steps.filter((step) => step.from?.includes(key));
</script>

<div class="diagram" bind:this={surface} data-workflow-diagram>
  <svg class="lines" width={extent.width} height={extent.height} aria-hidden="true">
    <defs>
      <marker id="workflow-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" orient="auto-start-reverse">
        <path d="M 0 0 L 10 5 L 0 10 z" class="head" />
      </marker>
    </defs>
    {#each lines as line (line.key)}
      <path d={line.d} class="line" class:selected={line.isSelected} marker-end="url(#workflow-arrow)" />
    {/each}
  </svg>
  {#each columns as column (column.category)}
    <section class="column" aria-label={t(`workflows.category.${column.category.toLowerCase()}`)}>
      <h3 class="heading">{t(`workflows.category.${column.category.toLowerCase()}`)}</h3>
      {#each column.statuses as status (status.key)}
        <div
          class="card"
          class:selected={selected === `status:${status.key}`}
          class:initial={initial === status.key}
          use:card={status.key}
          data-diagram-status={status.key}
        >
          <button type="button" class="pick" onclick={() => onselect(`status:${status.key}`)}>
            <LabelChip name={statusLabel(status, t)} colorToken={status.color_token} />
          </button>
          {#each outOf(status.key) as step (step.key)}
            <button
              type="button"
              class="step"
              class:selected={selected === `step:${step.key}`}
              data-diagram-step={step.key}
              onclick={() => onselect(`step:${step.key}`)}
            >→ {stepLabel(step, shape, t)}</button>
          {/each}
          {#each anyInto(status.key) as step (step.key)}
            <button
              type="button"
              class="step any"
              class:selected={selected === `step:${step.key}`}
              data-diagram-step={step.key}
              onclick={() => onselect(`step:${step.key}`)}
            >{t('app.workflow_designer.from_any', { name: stepLabel(step, shape, t) })}</button>
          {/each}
        </div>
      {/each}
    </section>
  {/each}
</div>

<style>
  .diagram {
    position: relative;
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: var(--sp-400);
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface-sunken);
    overflow-x: auto;
  }

  .lines {
    position: absolute;
    inset-block-start: 0;
    inset-inline-start: 0;
    pointer-events: none;
    overflow: visible;
  }

  .line { fill: none; stroke: var(--border-strong); stroke-width: var(--bw-thick); }
  .line.selected { stroke: var(--accent-primary); }
  .head { fill: var(--border-strong); }

  .column { display: flex; flex-direction: column; gap: var(--sp-150); min-inline-size: 0; }

  .heading {
    margin: 0;
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    color: var(--text-secondary);
    text-transform: uppercase;
  }

  .card {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--sp-050);
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
  }

  .card.initial { border-inline-start-width: var(--bw-thick); }
  .card.selected { border-color: var(--accent-primary); }

  .pick, .step {
    padding: 0;
    border: none;
    background: none;
    font: inherit;
    color: inherit;
    cursor: pointer;
    text-align: start;
  }

  .step { font-size: var(--fs-075); color: var(--text-secondary); }
  .step.any { font-style: italic; }
  .step.selected { color: var(--accent-primary); }

  .pick:focus-visible, .step:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
</style>
