<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one head of a collection's views (the navigation concept §3, the mockup's `coll`; the
  // owner's review R-11): the trail, the collection's name, its ☆ - the collection's star; a
  // saved view is starred from the sidebar - and one figure slot, then the view bar at one
  // offset. Every view draws this head and only its figure and its verbs differ, so switching
  // views moves neither the title nor the bar. A view's figure stands where the collection's
  // would; without one, the collection's own (*12 open*). Notices come after the bar, so they
  // never move it.
  //
  // Placed before the screen's body, not inside a `Stack`: the bar sticks under the app bar while
  // the page scrolls, which a box around it would end at the head.
  import type { Snippet } from 'svelte';
  import { PageHeader, type MenuItem, type PageAction } from '@hubtask/design-system/components';

  import CollectionFigure from '../entries/CollectionFigure.svelte';
  import { containers } from '../data/containers.svelte.ts';
  import { page } from '../frame/page.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { placeMark } from './PlaceMark.svelte';
  import PlaceViews from './PlaceViews.svelte';
  import StarToggle from './StarToggle.svelte';

  interface Props {
    collectionId: string;
    /** The view the screen is, as the bar names it (`board`, `sprint`, `time`, `view:<id>`). */
    current: string;
    onnavigate: (path: string) => void;
    /** The view's own figure - a sprint's days left -, in the collection's figure's place. */
    figure?: Snippet;
    primary?: PageAction;
    secondary?: readonly [] | readonly [PageAction] | readonly [PageAction, PageAction];
    menu?: { label: string; items: readonly MenuItem[]; onselect: (id: string) => void; opener?: string };
    /** Who is in the collection, beside the actions. */
    aside?: Snippet;
    /** ＋ at the bar's end, and the tools there from `medium` up. */
    add?: { readonly label: string; readonly onselect: () => void };
    tools?: Snippet;
    /** On a phone, what stands on the chip's line instead of the tools. */
    compactTools?: Snippet;
    /** Lines about the whole collection - read only, a failure -, under the bar. */
    notices?: Snippet;
  }

  const { collectionId, current, onnavigate, figure, primary, secondary, menu, aside, add, tools, compactTools, notices }: Props = $props();

  const collection = $derived(containers.find(collectionId));
  const hub = $derived(collection?.parent_id ? containers.find(collection.parent_id) : undefined);
  const trail = $derived([
    ...(hub ? [{ id: hub.id, label: hub.name, href: `/hubs/${hub.id}` }] : []),
    ...(collection ? [{ id: collection.id, label: collection.name }] : []),
  ]);
</script>

{#snippet ownFigure()}
  {#if collection?.figure}<CollectionFigure figure={collection.figure} name={collection.name} />{/if}
{/snippet}

{#if collection}
  <div class="head" data-collection-head>
    <PageHeader
      title={collection.name}
      subtitle={collection.description ?? undefined}
      isTitleInBar={viewport.isCompact}
      isMenuInBar={viewport.isCompact}
      onmenu={(offered) => page.offer(offered)}
      breadcrumb={{
        trail,
        mark: placeMark,
        label: t('app.workspace.trail'),
        expandLabel: t('app.workspace.expand_trail'),
        onnavigate: (crumbId) => {
          const crumb = containers.find(crumbId);
          if (crumb) onnavigate(crumb.type === 'HUB' ? `/hubs/${crumbId}` : `/collections/${crumbId}`);
        },
      }}
      {primary}
      {secondary}
      {menu}
      {aside}
      figure={figure ?? (collection.figure ? ownFigure : undefined)}
    >
      {#snippet titleEnd()}
        <StarToggle kind="COLLECTION" id={collection.id} name={collection.name} />
      {/snippet}
    </PageHeader>
  </div>
  <!-- `data-tour`: where the tour points for "the same entries, two ways". On a phone the chip and
       the tools stand on one line, as the concept's phone draws them. -->
  <div class="views" data-tour="layouts" data-compact={viewport.isCompact ? '' : undefined}
    role={viewport.isCompact && compactTools ? 'toolbar' : undefined} aria-label={viewport.isCompact && compactTools ? t('app.toolbar.entries') : undefined}>
    <PlaceViews placeId={collection.id} {current} {onnavigate} {add} tools={viewport.isCompact ? undefined : tools} />
    {#if viewport.isCompact && compactTools}<div class="compact-tools">{@render compactTools()}</div>{/if}
  </div>
  {#if notices}<div class="notices">{@render notices()}</div>{/if}
{/if}

<style>
  /* The head's own rhythm, whatever the screen's body below uses. */
  .head { margin-block-end: var(--sp-150); }

  /* The bar stays under the app bar while the view scrolls. */
  .views {
    position: sticky;
    inset-block-start: var(--layout-appbar-height);
    z-index: var(--z-raised);
    margin-block-end: var(--sp-200);
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

  .notices { display: flex; flex-direction: column; gap: var(--sp-100); margin-block-end: var(--sp-200); }
  .notices:not(:has(*)) { display: none; }
</style>
