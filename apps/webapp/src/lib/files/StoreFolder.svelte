<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One folder of the reader's own file store (UC-INT-17 check 2), read through their connection:
  // folders first, a folder opened by choosing it, a file picked for the entry's link. Keyed on
  // the folder by its caller, so the path is fixed for its life.

  import { untrack } from 'svelte';
  import { Icon } from '@hubtask/design-system/components';
  import type { FileStoreFolder, FileStoreItem } from '@hubtask/sync-engine';

  import { fileStoreFolderPath, sortItems } from '../data/filestore.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    connectionId: string;
    folder: string;
    /** The reference of the picked item, drawn as chosen. */
    picked?: string;
    onopen: (item: FileStoreItem) => void;
    onpick: (item: FileStoreItem) => void;
  }

  const { connectionId, folder, picked, onopen, onpick }: Props = $props();

  const read = resource<FileStoreFolder>(untrack(() => fileStoreFolderPath(connectionId, folder)));
  const items = $derived(read.state.status === 'ready' ? sortItems(read.state.data.data, messages.locale) : []);
</script>

<div class="folder" data-store-folder={folder}>
  {#if read.state.status === 'failed'}
    <p class="refusal" role="alert">{renderProblem(read.state.error, messages).message}</p>
  {:else if read.state.status !== 'ready'}
    <p class="quiet" role="status">{t('app.filestores.loading')}</p>
  {:else if items.length === 0}
    <p class="quiet">{t('app.filestores.empty_folder')}</p>
  {:else}
    <ul class="items">
      {#each items as item (item.ref)}
        <li class:chosen={item.ref === picked}>
          {#if item.folder}
            <button type="button" class="item" data-store-item={item.ref} onclick={() => onopen(item)}>
              <Icon name="folder" size="sm" /><span class="name">{item.name}</span><Icon name="chevron-right" size="sm" />
            </button>
          {:else}
            <button type="button" class="item" data-store-item={item.ref} aria-pressed={item.ref === picked} onclick={() => onpick(item)}>
              <Icon name="file-text" size="sm" /><span class="name">{item.name}</span>
            </button>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .folder { display: grid; gap: var(--sp-050); }
  .items { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); max-block-size: 50vh; overflow-y: auto; }
  .item {
    display: flex; align-items: center; gap: var(--sp-050); inline-size: 100%;
    padding: var(--sp-050) var(--sp-100); border: 0; border-radius: var(--r-sm);
    background: none; color: var(--text-primary); font: inherit; text-align: start; cursor: pointer;
  }
  .item:hover { background: var(--bg-surface-hover); }
  .item:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); }
  .chosen .item { background: var(--bg-surface-pressed); font-weight: var(--fw-medium); }
  .name { flex: 1; min-inline-size: 0; overflow-wrap: anywhere; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .refusal { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
