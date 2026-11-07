<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A small drawing of a board's objects (user-experience.md §7.2): a board card's preview and a
  // template's in the create gallery. Outlines and fills only, no words - the card says the name -
  // so it stays legible at any size and is hidden from a screen reader as decoration.

  import { tokens } from '@hubtask/design-system';
  import { bounds, connectorPath, outlinePath, strokePath, type Element } from '@hubtask/whiteboard';

  import { BOARD_COLOURS } from '../data/whiteboards.svelte.ts';

  interface Props {
    elements: readonly Element[];
  }

  const { elements }: Props = $props();

  const byId = $derived(new Map(elements.map((e) => [e.id, e])));
  const area = $derived(bounds(elements));
  const pad = $derived(area ? Math.max(24, (area.right - area.left) * 0.04) : 0);
  const viewBox = $derived(area ? `${area.left - pad} ${area.top - pad} ${area.right - area.left + 2 * pad} ${area.bottom - area.top + 2 * pad}` : '0 0 100 50');
  const fill = (e: Element) =>
    e.color_token && (BOARD_COLOURS as readonly string[]).includes(e.color_token) ? tokens.label[e.color_token as (typeof BOARD_COLOURS)[number]].bg : undefined;
</script>

<svg class="thumb" {viewBox} preserveAspectRatio="xMidYMid meet" aria-hidden="true" focusable="false">
  {#each elements as e (e.id)}
    {#if e.kind === 'FRAME'}
      <path class="frame" d={outlinePath(e)} vector-effect="non-scaling-stroke" />
    {:else if e.kind === 'CONNECTOR'}
      <path class="line" d={connectorPath(e, byId)} vector-effect="non-scaling-stroke" />
    {:else if e.kind === 'STROKE'}
      <path class="line" d={strokePath(e)} vector-effect="non-scaling-stroke" />
    {:else if e.kind !== 'TEXT'}
      <path class={`object ${e.kind.toLowerCase()}`} d={outlinePath(e)} style:fill={fill(e)} vector-effect="non-scaling-stroke" />
    {/if}
  {/each}
</svg>

<style>
  .thumb { display: block; inline-size: 100%; block-size: 100%; }
  .frame { fill: none; stroke: var(--border-strong); stroke-width: 1.5; stroke-dasharray: 4 3; }
  .line { fill: none; stroke: var(--text-subtle); stroke-width: 1.5; }
  .object { fill: var(--bg-raised); stroke: var(--border-default); stroke-width: 1; }
  .object.sticky { stroke: none; }
</style>
