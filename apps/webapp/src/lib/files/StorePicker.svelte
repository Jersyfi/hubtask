<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Link a file*'s picker (UC-INT-17 check 2): the reader's own file store connections, a folder
  // trail from the store's top, and a file or folder picked for the link. Without a connection it
  // says where one is made; the address can always be pasted instead.

  import { Button, Select, type SelectOption } from '@hubtask/design-system/components';
  import type { FileStoreConnection, FileStoreConnectionList, FileStoreItem } from '@hubtask/sync-engine';

  import { FILE_STORE_RETURN, backTo, connectionLabel, enterFolder, fileStoresPath, type FolderStep } from '../data/filestore.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import StoreFolder from './StoreFolder.svelte';

  interface Props {
    picked?: string;
    onpick: (connection: FileStoreConnection, item: FileStoreItem) => void;
  }

  const { picked, onpick }: Props = $props();

  const read = resource<FileStoreConnectionList>(fileStoresPath);
  const connections = $derived(read.state.status === 'ready' ? read.state.data.data : []);

  let chosenId = $state('');
  let trail = $state<FolderStep[]>([]);

  const storeName = (conn: FileStoreConnection) => t(conn.kind === 'NEXTCLOUD' ? 'app.filestores.nextcloud' : 'app.filestores.microsoft');
  const chosen = $derived(connections.find((c) => c.id === chosenId) ?? connections[0]);
  const options = $derived<SelectOption[]>(connections.map((c) => ({ value: c.id, label: connectionLabel(c, storeName(c)) })));
  const steps = $derived(trail.length > 0 && chosen ? trail : chosen ? [{ ref: '', name: storeName(chosen) }] : []);
  const here = $derived(steps.at(-1)?.ref ?? '');

  function choose(id: string) {
    chosenId = id;
    trail = [];
  }
</script>

{#if read.state.status === 'ready'}
  <div class="picker" data-store-picker>
    {#if connections.length === 0}
      <p class="quiet">
        {t('app.filestores.picker_none')}
        <a href={FILE_STORE_RETURN}>{t('app.filestores.picker_connect')}</a>
      </p>
    {:else if chosen}
      {#if connections.length > 1}
        <Select label={t('app.filestores.picker_store')} {options} value={chosen.id} onchange={(event) => choose((event.currentTarget as HTMLSelectElement).value)} />
      {/if}
      <nav class="trail" aria-label={t('app.filestores.trail')}>
        {#each steps as step, index (step.ref)}
          {#if index > 0}<span class="sep" aria-hidden="true">/</span>{/if}
          {#if index < steps.length - 1}
            <Button size="sm" tone="subtle" onclick={() => (trail = backTo(steps, index))}>{step.name}</Button>
          {:else}
            <span class="current" aria-current="location">{step.name}</span>
          {/if}
        {/each}
      </nav>
      {#key `${chosen.id}|${here}`}
        <StoreFolder
          connectionId={chosen.id}
          folder={here}
          {picked}
          onopen={(item) => (trail = enterFolder(steps, item))}
          onpick={(item) => onpick(chosen, item)}
        />
      {/key}
    {/if}
  </div>
{/if}

<style>
  .picker { display: grid; gap: var(--sp-100); padding-block-end: var(--sp-100); border-block-end: var(--bw-hairline) solid var(--border-default); }
  .trail { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-025); }
  .sep { color: var(--text-secondary); }
  .current { font-weight: var(--fw-medium); padding-inline: var(--sp-050); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
