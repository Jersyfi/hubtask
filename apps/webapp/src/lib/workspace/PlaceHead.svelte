<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one head of a place's screens (the navigation concept §3; the owner's review R-11): the
  // trail, the title, its ☆ where the place has one, one figure slot on a line of its own that
  // keeps its height empty, the actions, then the view bar at one offset - so switching a
  // collection's views or a hub tool's tabs moves neither the title nor the bar. Notices come
  // after the bar, so they never move it. `CollectionHead` and `ToolHead` say what fills it.
  //
  // Placed before the screen's body, not inside a `Stack`: the bar sticks under the app bar while
  // the page scrolls, which a box around it would end at the head.
  import type { Snippet } from 'svelte';
  import { PageHeader, type Crumb, type MenuItem, type PageAction } from '@hubtask/design-system/components';

  import { page } from '../frame/page.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { placeMark } from './PlaceMark.svelte';

  interface Props {
    /** Which head this is, for whoever looks for it (`data-head`). */
    kind: 'collection' | 'tool';
    title: string;
    subtitle?: string;
    trail: readonly Crumb[];
    trailLabel: string;
    expandLabel: string;
    oncrumb: (id: string) => void;
    titleEnd?: Snippet;
    figure?: Snippet;
    primary?: PageAction;
    secondary?: readonly [] | readonly [PageAction] | readonly [PageAction, PageAction];
    menu?: { label: string; items: readonly MenuItem[]; onselect: (id: string) => void; opener?: string };
    aside?: Snippet;
    /** The view bar. */
    views: Snippet;
    /** On a phone, what stands on the chip's line, named as a toolbar by `toolsLabel`. */
    compactTools?: Snippet;
    toolsLabel?: string;
    /** `data-tour` on the bar, where the tour points at it. */
    tour?: string;
    notices?: Snippet;
  }

  const { kind, title, subtitle, trail, trailLabel, expandLabel, oncrumb, titleEnd, figure, primary, secondary, menu, aside, views, compactTools, toolsLabel, tour, notices }: Props = $props();

  const hasCompactTools = $derived(viewport.isCompact && compactTools !== undefined);
</script>

<!-- The figure's line is drawn whether a figure stands on it, so every head of the place is as tall. -->
{#snippet figureLine()}{#if figure}{@render figure()}{/if}{/snippet}
<div class="head" data-head={kind}>
  <PageHeader
    {title}
    {subtitle}
    isTitleInBar={viewport.isCompact}
    isMenuInBar={viewport.isCompact}
    onmenu={(offered) => page.offer(offered)}
    breadcrumb={{ trail, mark: placeMark, label: trailLabel, expandLabel, onnavigate: oncrumb }}
    {primary}
    {secondary}
    {menu}
    {aside}
    figure={figureLine}
    {titleEnd}
    isFigureBelow
  />
</div>
<!-- On a phone the chip and the tools stand on one line, as the concept's phone draws them. -->
<div class="views" data-tour={tour} data-compact={viewport.isCompact ? '' : undefined}
  role={hasCompactTools ? 'toolbar' : undefined} aria-label={hasCompactTools ? toolsLabel : undefined}>
  {@render views()}
  {#if hasCompactTools && compactTools}<div class="compact-tools">{@render compactTools()}</div>{/if}
</div>
{#if notices}<div class="notices">{@render notices()}</div>{/if}

<style>
  /* No margins of its own: the page kind spaces the head, the bar and the notices (one rhythm). */
  /* The bar stays under the app bar while the view scrolls. */
  .views {
    position: sticky;
    inset-block-start: var(--layout-appbar-height);
    z-index: var(--z-raised);
    background: var(--bg-canvas);
  }

  .views[data-compact] {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-100);
    padding-block-end: var(--sp-100);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }

  .compact-tools { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); margin-inline-start: auto; }

  .notices { display: flex; flex-direction: column; gap: var(--sp-100); }
  .notices:not(:has(*)) { display: none; }
</style>
