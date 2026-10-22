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
  }

  const { collectionId }: Props = $props();

  const collection = $derived(containers.find(collectionId));

  $effect(() => {
    const id = collectionId;
    if (untrack(() => containers.find(id))) return;
    return untrack(() => containers.openSingle(id));
  });
</script>

{#if collection}
  <span class="place">
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

  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
