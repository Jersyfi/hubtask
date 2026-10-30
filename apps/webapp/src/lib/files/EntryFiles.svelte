<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An entry's files in a store (UC-INT-14 checks 1, 2): Nextcloud, SharePoint, OneDrive or WebDAV
  // files and folders linked by their address. A link opens the file where it lives, in a new tab,
  // under the store's own permissions; nothing is fetched or copied here.
  //
  // For a reader the section is drawn only where a file is linked, as the entry's code is; for a
  // writer it is always there, since linking is what it offers. The store is told by the address
  // unless the writer chooses it.

  import { untrack } from 'svelte';
  import { Badge, Button, Dialog, Icon, IconButton, Input, Select, Stack, type SelectOption } from '@hubtask/design-system/components';
  import type { FileLink, FileLinkList } from '@hubtask/sync-engine';

  import { STORES, fileHost, fileHref, fileLinksPath } from '../data/files.ts';
  import { files } from '../data/files.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    itemId: string;
    /** Whether the reader may change this entry, which linking and taking off need. */
    mayChange: boolean;
  }

  const { itemId, mayChange }: Props = $props();

  // Keyed on the entry by its caller, so the path is fixed for its life.
  const read = resource<FileLinkList>(untrack(() => fileLinksPath(itemId)));
  const links = $derived(read.state.status === 'ready' ? read.state.data.data : []);
  const storeOptions = $derived<SelectOption[]>([
    { value: '', label: t('app.files.store_detected') },
    ...STORES.map((store) => ({ value: store, label: t(`app.files.store.${store}`) })),
  ]);

  let isOpen = $state(false);
  let address = $state('');
  let store = $state<string>('');
  let name = $state('');
  let isLinking = $state(false);
  let refusal = $state<string | undefined>(undefined);

  function open() {
    address = '';
    store = '';
    name = '';
    refusal = undefined;
    isOpen = true;
  }

  async function link() {
    isLinking = true;
    refusal = undefined;
    try {
      await files.link(itemId, address.trim(), store as FileLink['provider'] | '', name.trim());
      isOpen = false;
      toast.show({ text: t('app.files.linked'), tone: 'success' });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isLinking = false;
    }
  }

  async function unlink(each: FileLink) {
    try {
      await files.unlink(each);
      toast.show({ text: t('app.files.unlinked'), tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }
</script>

{#if mayChange || links.length > 0}
  <section class="files" aria-labelledby="entry-files-head-{itemId}" data-entry-files>
    <div class="head">
      <h3 id="entry-files-head-{itemId}" class="title"><Icon name="paperclip" size="sm" />{t('app.files.title')}</h3>
      {#if mayChange}
        <Button size="sm" tone="subtle" icon="plus" data-link-file onclick={open}>{t('app.files.add')}</Button>
      {/if}
    </div>
    {#if links.length === 0}
      <p class="quiet">{t('app.files.none')}</p>
    {:else}
      <ul class="rows">
        {#each links as each (each.id)}
          {@const href = fileHref(each.address)}
          <li class="row" data-file-link={each.provider}>
            <span class="what">
              {#if href}
                <a {href} target="_blank" rel="noopener noreferrer">{each.name}</a>
              {:else}
                <span>{each.name}</span>
              {/if}
              <span class="quiet">{fileHost(each.address)}</span>
            </span>
            <span class="end">
              <Badge size="sm">{t(`app.files.store.${each.provider}`)}</Badge>
              {#if mayChange}
                <IconButton
                  icon="x" size="sm" tone="subtle"
                  label={t('app.files.remove', { name: each.name })}
                  data-unlink-file={each.id}
                  onclick={() => void unlink(each)}
                />
              {/if}
            </span>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
{/if}

{#if isOpen}
  <Dialog bind:isOpen title={t('app.files.dialog_title')} dismissLabel={t('app.files.cancel')}>
    <Stack gap="150">
      <Input label={t('app.files.address_label')} hint={t('app.files.address_hint')} bind:value={address} maxlength={2048} type="url" />
      <Select label={t('app.files.store_label')} options={storeOptions} bind:value={store} />
      <Input label={t('app.files.name_label')} hint={t('app.files.name_hint')} bind:value={name} maxlength={200} />
      {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.files.cancel')}</Button>
      <Button
        tone="primary"
        data-confirm-file
        isBusy={isLinking}
        disabledReason={address.trim() ? undefined : t('filelink.address_required')}
        onclick={() => void link()}
      >{t('app.files.confirm')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .files { display: grid; gap: var(--sp-100); }
  .head { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .title { display: inline-flex; align-items: center; gap: var(--sp-050); margin: 0; font-size: var(--fs-100); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .row { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .what { display: grid; gap: var(--sp-025); min-inline-size: 0; overflow-wrap: anywhere; }
  .what a { color: var(--text-primary); font-weight: var(--fw-medium); }
  .end { display: inline-flex; align-items: center; gap: var(--sp-050); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .refusal { margin: 0; color: var(--text-danger); }
</style>
