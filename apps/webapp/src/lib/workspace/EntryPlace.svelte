<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where an entry lives, beside it in a list that mixes collections: the collection's mark in its
  // own colour, and its name (UC-WRK-33). The name is what says it - the colour helps the eye find
  // the rows of one place, and never says anything alone (design-system.md §6 rule 3).
  //
  // A collection this client has not read yet is read here, once per collection: a list that mixes
  // collections is exactly the list whose collections the tree may not have opened. Until it
  // arrives the place draws nothing, rather than a name it does not know.

  import { untrack } from 'svelte';

  import { containers } from '../data/containers.svelte.ts';
  import ContainerMark from './ContainerMark.svelte';

  interface Props {
    collectionId: string;
    /**
     * The hub first, in its own colour, then the collection: where a page gathers entries from
     * every hub, the hub is what the eye sorts them by (UC-WRK-34 check 1).
     */
    withHub?: boolean;
  }

  const { collectionId, withHub = false }: Props = $props();

  const collection = $derived(containers.find(collectionId));
  const hub = $derived(withHub && collection?.parent_id ? containers.find(collection.parent_id) : undefined);

  $effect(() => {
    const id = collectionId;
    if (untrack(() => containers.find(id))) return;
    return untrack(() => containers.openSingle(id));
  });
</script>

{#if collection}
  <span class="place">
    {#if hub}
      <ContainerMark look={hub} />
      <span class="name hub">{hub.name}</span>
    {/if}
    <ContainerMark look={collection} />
    <span class="name">{collection.name}</span>
  </span>
{/if}

<style>
  .place {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    min-width: 0;
    max-inline-size: 24ch;
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }

  /* With the hub in front the two names share the width rather than the first taking it all. */
  .place:has(.hub) { max-inline-size: 36ch; }

  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .hub { max-inline-size: 14ch; }
</style>
