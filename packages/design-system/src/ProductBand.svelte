<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A product home's head band (work-surface concept §5, WB-2, WB-5): the product's tile, its name,
  // its figures in one line - the first one set bold - and its primary action. It is `PageHeader`
  // with the product's mark and figures, so a product home and a settings page share one head and
  // one rhythm; on a phone the band is compact, two figures, and the primary floats above the
  // bottom bar as every page's does (§7).
  import type { Snippet } from 'svelte';

  import PageHeader, { type PageAction } from './PageHeader.svelte';
  import ProductTile, { type ProductId } from './ProductTile.svelte';
  import StateLozenge from './StateLozenge.svelte';
  import type { IconName } from './icons/index.ts';
  import type { LozengeVocabulary } from './lozenge.ts';
  import type { MenuItem } from './overlay.ts';
  import type { Crumb } from './structure.ts';

  interface Props {
    product: ProductId;
    icon: IconName;
    /** The product's name: *Service desk*. */
    name: string;
    /** *14 open*, *1 close to breach*, *6 unassigned*: resolved, the first is the headline. */
    figures?: readonly string[];
    primary?: PageAction;
    /** Acts that stand before the primary, or in its place when the view's one primary is lower down. */
    secondary?: readonly [] | readonly [PageAction] | readonly [PageAction, PageAction];
    menu?: { label: string; items: readonly MenuItem[]; onselect: (id: string) => void };
    breadcrumb?: { trail: readonly Crumb[]; label: string; expandLabel: string; onnavigate?: (id: string) => void };
    /** The product's `ViewBar`, under the band. */
    views?: Snippet;
    /**
     * A control of the product's own before its actions - Time's timer (scene time): the
     * caller's drawing, at the actions' height, as `PageHeader` holds it.
     */
    aside?: Snippet;
    /** The state of what the band shows, as a lozenge after its figures - Time's week (scene time). */
    state?: { label: string; vocabulary: LozengeVocabulary; state: string };
  }

  const { product, icon, name, figures = [], primary, secondary, menu, breadcrumb, views, aside, state }: Props = $props();
</script>

<div class="band" data-product={product}>
  <PageHeader title={name} {primary} {secondary} {menu} {breadcrumb} {views} {aside} mark={tile} figure={figures.length > 0 || state ? line : undefined} isFigureBelow />
</div>

{#snippet tile()}
  <span class="tile"><ProductTile {icon} {product} size="xl" /></span>
{/snippet}

{#snippet line()}
  <span class="figures">
    {#each figures as figure, index (index)}
      {#if index > 0}<span class="dot" aria-hidden="true">·</span>{/if}
      <span class="figure" data-lead={index === 0 ? '' : undefined} data-rest={index > 1 ? '' : undefined}>{figure}</span>
    {/each}
    {#if state}{#if figures.length > 0}<span class="dot" aria-hidden="true">·</span>{/if}<span class="state"><StateLozenge label={state.label} vocabulary={state.vocabulary} state={state.state} /></span>{/if}
  </span>
{/snippet}

<style>
  .band { min-inline-size: 0; }
  .tile { display: inline-flex; }

  /* The band as the product homes' mockup draws it: the tile beside a large name, the figures
     under the name (not beside it), and the menu after the primary at the line's end. */
  .band :global(.title) { font-size: var(--fs-600); font-weight: var(--fw-bold); }
  .band :global(.title-line) { gap: var(--sp-050) var(--sp-150); }
  .band :global(.head-figure[data-below]) { padding-inline-start: calc(var(--sp-500) + var(--sp-150)); margin-block-start: calc(-1 * var(--sp-100)); }
  .band :global(.actions .menu) { order: 1; }

  .figures { display: inline-flex; flex-wrap: wrap; align-items: baseline; gap: var(--sp-050); color: var(--text-secondary); font-size: var(--fs-090); }
  .figure[data-lead] { color: var(--text-primary); font-weight: var(--fw-semibold); }
  .dot { color: var(--text-subtle); }
  .state { display: inline-flex; align-self: center; }

  /* A phone: the band is compact - the smaller tile, two figures. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .tile :global(.tile) {
      inline-size: var(--sp-400);
      block-size: var(--sp-400);
    }
    .band :global(.title) { font-size: var(--fs-450); }
    .band :global(.head-figure[data-below]) { padding-inline-start: calc(var(--sp-400) + var(--sp-150)); }
    .figure[data-rest],
    .figure[data-rest] + .dot,
    .dot:has(+ .figure[data-rest]) { display: none; }
  }
</style>
