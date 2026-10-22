<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One entry a duplicates proposal names, by its title (UC-AI-04 check 7).
  //
  // The proposal carries an identifier and never a title: a title in it would be a copy of somebody
  // else's entry inside a record about this one (ai-first.md §2). So the title is read here, from
  // the entry itself, where the permission is asked again - an entry since trashed, or one this
  // reader may not open, is said to be one, and never named.

  import { untrack } from 'svelte';

  import type { WorkItem } from '@hubtask/sync-engine';

  import { itemPath } from '../data/item.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    itemId: string;
  }

  const { itemId }: Props = $props();

  const entry = resource<WorkItem>(untrack(() => itemPath(itemId)));
</script>

{#if entry.state.status === 'ready'}
  <a href={`/items/${itemId}`}>{entry.state.data.title}</a>
{:else if entry.state.status === 'failed'}
  <span class="unopenable">{t('app.ai.neighbour_unopenable')}</span>
{:else}
  <span class="unopenable" aria-busy="true"></span>
{/if}

<style>
  .unopenable { color: var(--text-subtle); }
  a { color: var(--text-brand); overflow-wrap: anywhere; }
  a:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); border-radius: var(--r-sm); }
</style>
