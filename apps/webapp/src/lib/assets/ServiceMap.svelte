<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's service map drawn (UC-AST-11 check 5): one band per layer - offerings, services,
  // application services, software and data, virtual and cloud resources, what they stand on -
  // with its CIs in the order `mapLayers` gives, and a line from each CI to the one it rests on; a
  // group's member is joined to its service by a dashed line, because no relation joins them.
  //
  // **No graph library and no length written here** (rule 15): the bands are laid out by CSS, and
  // the lines are measured from the cards once they stand, as the workflow diagram does. The map
  // is read-only; each card opens its CI.

  import { untrack } from 'svelte';
  import { Banner, Spinner } from '@hubtask/design-system/components';
  import type { CiClass, ServiceMap } from '@hubtask/sync-engine';

  import { classWord, isGroupEdge, mapLayers, serviceMapPath, type Words } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    hubId: string;
    /** Only this service and what it rests on; the whole hub when absent. */
    serviceId?: string;
    classes: readonly CiClass[];
    words: Words;
  }

  const { hubId, serviceId, classes, words }: Props = $props();

  let map = $state<ServiceMap | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const path = serviceMapPath(hubId, serviceId);
    return untrack(() => {
      map = undefined;
      refusal = undefined;
      return engine.subscribe<ServiceMap>({ path }, (state) => {
        if (state.status === 'ready') map = state.data;
        if (state.status === 'failed') refusal = renderProblem(state.error, messages).message;
      });
    });
  });
  const layers = $derived(map ? mapLayers(map) : []);

  let surface = $state<HTMLDivElement | undefined>(undefined);
  const cards = new Map<string, HTMLElement>();
  let lines = $state<readonly { key: string; d: string; group: boolean }[]>([]);
  let extent = $state({ width: 0, height: 0 });

  function card(node: HTMLElement, id: string) {
    cards.set(id, node);
    return { destroy: () => cards.delete(id) };
  }

  /** Measures every card and draws a curve from the bottom of the one that suffers to the top of the one it rests on. */
  function measure() {
    if (!surface || !map) return;
    const origin = surface.getBoundingClientRect();
    extent = { width: surface.scrollWidth, height: surface.scrollHeight };
    const out: { key: string; d: string; group: boolean }[] = [];
    for (const edge of map.edges) {
      const from = cards.get(edge.source)?.getBoundingClientRect();
      const to = cards.get(edge.target)?.getBoundingClientRect();
      if (!from || !to) continue;
      const x1 = from.left + from.width / 2 - origin.left + surface.scrollLeft;
      const y1 = from.bottom - origin.top;
      const x2 = to.left + to.width / 2 - origin.left + surface.scrollLeft;
      const y2 = to.top - origin.top;
      const bend = Math.abs(y2 - y1) / 2;
      out.push({ key: `${edge.source}>${edge.target}:${edge.type}`, d: `M ${x1} ${y1} C ${x1} ${y1 + bend}, ${x2} ${y2 - bend}, ${x2} ${y2}`, group: isGroupEdge(edge.type) });
    }
    lines = out;
  }

  $effect(() => {
    void layers;
    if (!surface) return;
    const frame = requestAnimationFrame(measure);
    const observer = new ResizeObserver(() => measure());
    observer.observe(surface);
    return () => {
      cancelAnimationFrame(frame);
      observer.disconnect();
    };
  });
</script>

{#if refusal}
  <Banner tone="danger">{refusal}</Banner>
{:else if !map}
  <Spinner label={t('app.assets.loading')} />
{:else if layers.length === 0}
  <p class="meta">{t('app.service_map.empty')}</p>
{:else}
  {#if map.truncated}<Banner tone="warning">{t('app.service_map.truncated')}</Banner>{/if}
  <div class="map" bind:this={surface} data-service-map>
    <svg class="lines" width={extent.width} height={extent.height} aria-hidden="true">
      <defs>
        <marker id="service-map-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" orient="auto-start-reverse">
          <path d="M 0 0 L 10 5 L 0 10 z" class="head" />
        </marker>
      </defs>
      {#each lines as line (line.key)}
        <path d={line.d} class="line" class:group={line.group} marker-end="url(#service-map-arrow)" />
      {/each}
    </svg>
    {#each layers as band (band.layer)}
      <section class="band" aria-label={t(`app.service_map.layer.${band.layer}`)} data-map-layer={band.layer}>
        <h3 class="heading">{t(`app.service_map.layer.${band.layer}`)}</h3>
        <ul class="cards">
          {#each band.nodes as node (node.ci_id)}
            <li class="card" use:card={node.ci_id} data-map-node={node.ci_id}>
              <a href={`/cis/${node.ci_id}`}>{node.name}</a>
              <span class="meta">{classWord(node.class, classes, words)}</span>
            </li>
          {/each}
        </ul>
      </section>
    {/each}
  </div>
  <p class="meta">{t('app.service_map.legend')}</p>
{/if}

<style>
  .map {
    position: relative;
    display: grid;
    gap: var(--sp-400);
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface-sunken);
    overflow-x: auto;
  }
  .lines { position: absolute; inset-block-start: 0; inset-inline-start: 0; pointer-events: none; overflow: visible; }
  .line { fill: none; stroke: var(--border-strong); stroke-width: var(--bw-thick); }
  .line.group { stroke-dasharray: var(--sp-050) var(--sp-050); }
  .head { fill: var(--border-strong); }
  .band { display: grid; gap: var(--sp-100); }
  .heading { margin: 0; font-size: var(--fs-075); font-weight: var(--fw-medium); color: var(--text-secondary); text-transform: uppercase; }
  .cards { margin: 0; padding: 0; list-style: none; display: flex; flex-wrap: nowrap; justify-content: safe center; gap: var(--sp-200); }
  .card {
    position: relative;
    display: grid;
    gap: var(--sp-025);
    flex: none;
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .card a { color: var(--text-primary); font-weight: var(--fw-medium); white-space: nowrap; }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; white-space: nowrap; }
</style>
