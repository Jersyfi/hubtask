<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one head of a collection's views (the navigation concept §3, the mockup's `coll`; the
  // owner's review R-11): the collection's name, its ☆ - the collection's star; a saved view is
  // starred from the sidebar -, the people's faces and the collection's ⋯ on every view, and one
  // figure slot: a view's own figure - a sprint's days left - where the collection's (*12 open*)
  // would stand. Every view draws this head and only its figure and its verbs differ; a view's
  // own items come first in the ⋯.
  import type { Snippet } from 'svelte';
  import type { MenuItem, PageAction } from '@hubtask/design-system/components';

  import CollectionFigure from '../entries/CollectionFigure.svelte';
  import { containers } from '../data/containers.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { placeActions, type PlaceActions } from './placeActions.ts';
  import PlaceHead from './PlaceHead.svelte';
  import PlacePeople from './PlacePeople.svelte';
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
    /** The view's own items, before the collection's in the head's ⋯. */
    menu?: { label?: string; items: readonly MenuItem[]; onselect: (id: string) => void };
    /**
     * The collection's own items, where the screen runs them itself (the collection's own
     * screen); elsewhere they are the place's (`placeActions`) and run where they belong.
     */
    place?: PlaceActions;
    /** ＋ at the bar's end, and the tools there from `medium` up. */
    add?: { readonly label: string; readonly onselect: () => void };
    tools?: Snippet;
    /** On a phone, what stands on the chip's line instead of the tools. */
    compactTools?: Snippet;
    /** Lines about the whole collection - read only, a failure -, under the bar. */
    notices?: Snippet;
  }

  const { collectionId, current, onnavigate, figure: viewFigure, primary, secondary, menu, place, add, tools, compactTools, notices }: Props = $props();

  const collection = $derived(containers.find(collectionId));
  const hub = $derived(collection?.parent_id ? containers.find(collection.parent_id) : undefined);
  const trail = $derived([
    ...(hub ? [{ id: hub.id, label: hub.name, href: `/hubs/${hub.id}` }] : []),
    ...(collection ? [{ id: collection.id, label: collection.name }] : []),
  ]);

  const own = $derived(place ?? placeActions(collectionId, { onnavigate }));
  /** The view's items, then the collection's under a line - their ids kept apart by a prefix. */
  const headMenu = $derived.by(() => {
    const viewItems = menu?.items ?? [];
    const placeItems = (own?.items ?? []).map((item, index) => ({
      ...item,
      id: `place:${item.id}`,
      hasSeparatorBefore: item.hasSeparatorBefore || (index === 0 && viewItems.length > 0),
    }));
    if (!collection || viewItems.length + placeItems.length === 0) return undefined;
    return {
      label: t('app.workspace.actions', { name: collection.name }),
      items: [...viewItems, ...placeItems],
      onselect: (id: string) => (id.startsWith('place:') ? own?.onselect(id.slice('place:'.length)) : menu?.onselect(id)),
      opener: 'container-menu',
    };
  });
</script>

{#if collection}
  <PlaceHead
    kind="collection"
    title={collection.name}
    subtitle={collection.description ?? undefined}
    {trail}
    trailLabel={t('app.workspace.trail')}
    expandLabel={t('app.workspace.expand_trail')}
    oncrumb={(crumbId) => {
      const crumb = containers.find(crumbId);
      if (crumb) onnavigate(crumb.type === 'HUB' ? `/hubs/${crumbId}` : `/collections/${crumbId}`);
    }}
    {primary}
    {secondary}
    menu={headMenu}
    {compactTools}
    toolsLabel={t('app.toolbar.entries')}
    tour="layouts"
    {notices}
  >
    {#snippet titleEnd()}<StarToggle kind="COLLECTION" id={collection.id} name={collection.name} />{/snippet}
    {#snippet aside()}<PlacePeople placeId={collection.id} />{/snippet}
    {#snippet figure()}
      {#if viewFigure}{@render viewFigure()}{:else if collection.figure}<CollectionFigure figure={collection.figure} name={collection.name} />{/if}
    {/snippet}
    {#snippet views()}
      <PlaceViews placeId={collection.id} {current} {onnavigate} {add} tools={viewport.isCompact ? undefined : tools} />
    {/snippet}
  </PlaceHead>
{/if}
