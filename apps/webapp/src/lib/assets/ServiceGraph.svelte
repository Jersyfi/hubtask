<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One service's map (work-surface §6.10, scene invmap 2): the service at the start, what it rests
  // on in columns by steps below it, a curve from each object to what it rests on. An object with an
  // open incident is drawn in the danger tone with the entry's key, a planned change's key beside
  // it, a stage other than *in use* as a lozenge, and the way from the service down to a struck
  // object dashed. Colour is never alone (UX-7): the key, a hidden word and the dash say it too.
  //
  // **No graph library and no length written here** (rule 15): the columns are laid out by CSS and
  // the curves measured from the cards once they stand, as the hub's layered map does. Read only;
  // each card opens its object, each key its entry.
  import { Icon, StateLozenge, VisuallyHidden } from '@hubtask/design-system/components';
  import type { CiClass, CiWorkEntry } from '@hubtask/sync-engine';

  import { classWord, type Words } from '../data/cmdb.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import type { ServiceGraph } from './inventoryscenes.ts';
  import { isQuietStage } from './objectpage.ts';

  interface Props {
    graph: ServiceGraph;
    /** The open incident and the planned change each object carries. */
    marks: ReadonlyMap<string, { readonly struck?: CiWorkEntry; readonly planned?: CiWorkEntry }>;
    /** Objects found wanting or with a firing alert. */
    wanting: ReadonlySet<string>;
    /** The edges on the way of a disruption: dashed. */
    dashed: ReadonlySet<string>;
    classes: readonly CiClass[];
    words: Words;
  }

  const { graph, marks, wanting, dashed, classes, words }: Props = $props();

  const STAGE_TONE: Readonly<Record<string, 'neutral' | 'info' | 'warning'>> = {
    PLANNED: 'neutral', ORDERED: 'info', IN_STOCK: 'info', MAINTENANCE: 'warning', RETIRED: 'neutral', DISPOSED: 'neutral',
  };

  let surface = $state<HTMLDivElement | undefined>(undefined);
  const cards = new Map<string, HTMLElement>();
  let lines = $state<readonly { key: string; d: string; isGroup: boolean; isDashed: boolean }[]>([]);
  let extent = $state({ width: 0, height: 0 });

  function card(node: HTMLElement, id: string) {
    cards.set(id, node);
    return { destroy: () => cards.delete(id) };
  }

  /** A curve from the end of the object that rests to the start of the one it rests on. */
  function measure() {
    if (!surface) return;
    const origin = surface.getBoundingClientRect();
    const isRtl = getComputedStyle(surface).direction === 'rtl';
    extent = { width: surface.scrollWidth, height: surface.scrollHeight };
    const out: { key: string; d: string; isGroup: boolean; isDashed: boolean }[] = [];
    for (const edge of graph.edges) {
      const from = cards.get(edge.source)?.getBoundingClientRect();
      const to = cards.get(edge.target)?.getBoundingClientRect();
      if (!from || !to) continue;
      const x1 = (isRtl ? from.x : from.x + from.width) - origin.x + surface.scrollLeft;
      const y1 = from.top + from.height / 2 - origin.top;
      const x2 = (isRtl ? to.x + to.width : to.x) - origin.x + surface.scrollLeft;
      const y2 = to.top + to.height / 2 - origin.top;
      const bend = (x2 - x1) / 2;
      out.push({ key: edge.key, d: `M ${x1} ${y1} C ${x1 + bend} ${y1}, ${x2 - bend} ${y2}, ${x2} ${y2}`, isGroup: edge.isGroup, isDashed: dashed.has(edge.key) });
    }
    lines = out;
  }

  $effect(() => {
    void graph;
    void dashed;
    if (!surface) return;
    const frame = requestAnimationFrame(measure);
    const observer = new ResizeObserver(() => measure());
    observer.observe(surface);
    return () => {
      cancelAnimationFrame(frame);
      observer.disconnect();
    };
  });

  const columnLabel = (depth: number) => t('app.service_map.column', { count: depth });
</script>

<div class="graph" bind:this={surface} data-service-graph>
  <svg class="lines" width={extent.width} height={extent.height} aria-hidden="true">
    {#each lines as line (line.key)}
      <path d={line.d} class="line" class:group={line.isGroup} class:struck={line.isDashed} data-map-line={line.key} data-dashed={line.isDashed ? '' : undefined} />
    {/each}
  </svg>
  {#each graph.columns as column, depth (depth)}
    <ul class="column" aria-label={columnLabel(depth)} data-map-depth={depth}>
      {#each column as node (node.ci_id)}
        {@const mark = marks.get(node.ci_id)}
        <li class="node" use:card={node.ci_id} data-map-node={node.ci_id} data-struck={mark?.struck ? '' : undefined} data-service={depth === 0 ? '' : undefined}>
          <span class="name">
            {#if wanting.has(node.ci_id) && !mark?.struck}<span class="flag" aria-hidden="true"><Icon name="triangle-alert" size="sm" /></span><VisuallyHidden>{t('app.service_map.wanting')}</VisuallyHidden>{/if}
            <a href={`/cis/${node.ci_id}`} title={classWord(node.class, classes, words)}>{node.name}</a>
          </span>
          {#if isQuietStage(node.stage) && STAGE_TONE[node.stage]}
            <StateLozenge label={t(`app.assets.stage.${node.stage}`)} tone={STAGE_TONE[node.stage]} />
          {/if}
          {#if mark?.struck}
            <a class="chip danger" href={`/items/${mark.struck.item_id}`} title={mark.struck.title} data-map-struck>
              <VisuallyHidden>{t('app.service_map.struck')}</VisuallyHidden>{mark.struck.key ?? mark.struck.title}
            </a>
          {/if}
          {#if mark?.planned}
            <a class="chip" href={`/items/${mark.planned.item_id}`} title={mark.planned.title} data-map-planned>
              <VisuallyHidden>{t('app.service_map.planned')}</VisuallyHidden>{mark.planned.key ?? mark.planned.title}
            </a>
          {/if}
        </li>
      {/each}
    </ul>
  {/each}
</div>

<style>
  .graph {
    position: relative;
    display: grid;
    grid-auto-flow: column;
    grid-auto-columns: max-content;
    align-items: center;
    column-gap: var(--sp-600);
    padding: var(--sp-400) var(--sp-300);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface-sunken);
    overflow-x: auto;
  }
  .lines { position: absolute; inset-block-start: 0; inset-inline-start: 0; pointer-events: none; overflow: visible; }
  .line { fill: none; stroke: var(--border-strong); stroke-width: var(--bw-thick); }
  .line.group { stroke-dasharray: var(--sp-025) var(--sp-050); }
  /* The way of a disruption is told by its dash, not by a colour the status gate could not set on a surface. */
  .line.struck { stroke: var(--text-primary); stroke-dasharray: var(--sp-050) var(--sp-050); }
  .column { position: relative; display: flex; flex-direction: column; align-items: flex-start; justify-content: center; gap: var(--sp-300); margin: 0; padding: 0; list-style: none; }
  .node {
    display: flex; align-items: center; gap: var(--sp-050);
    padding: var(--sp-050) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    white-space: nowrap;
  }
  .node[data-service] { border-width: var(--bw-thick); border-color: var(--accent-primary); font-weight: var(--fw-semibold); }
  .node[data-struck] { border-color: var(--status-danger-border); border-width: var(--bw-thick); background: var(--status-danger-surface); }
  .name { display: inline-flex; align-items: center; gap: var(--sp-050); }
  .name a { color: var(--text-primary); text-decoration: none; }
  .name a:hover { text-decoration: underline; }
  .flag { display: inline-flex; color: var(--text-warning); }
  .chip {
    padding: 0 var(--sp-050);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-sm);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    color: var(--text-secondary);
    text-decoration: none;
  }
  .chip.danger { border-color: var(--status-danger-border); background: var(--status-danger-surface); color: var(--status-danger-text); }
  .chip:hover { text-decoration: underline; }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the control's press
     reaches layout.target, by its own height or beyond its drawn box. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .name a { display: inline-flex; align-items: center; min-block-size: var(--layout-target); }
    .node { position: relative; min-block-size: var(--layout-target); }
    .name a::after { content: ''; position: absolute; inset: 0; }
    .chip { position: relative; z-index: var(--z-raised); }
    .chip::after { content: ''; position: absolute; inset-block: min(0%, calc((100% - var(--layout-target)) / 2)); inset-inline: min(0%, calc((100% - var(--layout-target)) / 2)); }
  }
</style>
