<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One entry an answer rests on, as a link by its title (UC-AI-08 check 2).
  //
  // The answer carries identifiers only; the title is read here, from the entry, where the
  // permission is asked again - so an entry this reader can no longer open is left out rather
  // than named.
  import { untrack } from 'svelte';

  import type { WorkItem } from '@hubtask/sync-engine';

  import { itemPath } from '../data/item.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';

  interface Props {
    itemId: string;
  }

  const { itemId }: Props = $props();

  const entry = resource<WorkItem>(untrack(() => itemPath(itemId)));
</script>

{#if entry.state.status === 'ready'}
  <li data-source={itemId}><a href={`/items/${itemId}`}>{entry.state.data.title}</a></li>
{/if}

<style>
  a { color: var(--text-brand); overflow-wrap: anywhere; }
  a:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); border-radius: var(--r-sm); }
</style>
