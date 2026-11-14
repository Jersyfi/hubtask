<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An entry's files in a store (UC-INT-14 checks 1, 2): Nextcloud, SharePoint, OneDrive or WebDAV
  // files and folders linked by their address. A link opens the file where it lives, in a new tab,
  // under the store's own permissions; nothing is fetched or copied here.
  //
  // For a reader the section is drawn only where a file is linked or the uploaded ones are drawn in
  // it, as the entry's code is; for a writer it is always there, since linking is what it offers.
  // The store is told by the address unless the writer chooses it. A writer with a file store
  // connection picks the file from its folders instead of pasting it, and on a Nextcloud may link a
  // read-only share (UC-INT-17).

  import { untrack, type Snippet } from 'svelte';
  import { Badge, Button, Icon, Checkbox, Dialog, IconButton, Input, Select, Stack, VisuallyHidden, type SelectOption } from '@hubtask/design-system/components';
  import FailureNote from '../frame/FailureNote.svelte';
  import type { FileLink, FileLinkList, FileStoreConnection, FileStoreItem } from '@hubtask/sync-engine';

  import { STORES, fileHost, fileHref, fileLinksPath } from '../data/files.ts';
  import { files } from '../data/files.svelte.ts';
  import { linkOf } from '../data/filestore.ts';
  import { fileStores } from '../data/filestore.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import StorePicker from './StorePicker.svelte';

  interface Props {
    itemId: string;
    /** Whether the reader may change this entry, which linking and taking off need. */
    mayChange: boolean;
    /** A grid of cards or a list of rows, as the entry's files are drawn (ET-4). */
    view?: 'grid' | 'list';
    /**
     * Where the entry's uploaded files are drawn: handed the linked files' cells and their count, so
     * both stand in one grid or list (ET-4). Absent, the linked files are a list of their own.
     */
    attachments?: Snippet<[Snippet, number]>;
  }

  const { itemId, mayChange, view = 'list', attachments }: Props = $props();

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
  /** The file picked from a store, while the address is still the one it gave. */
  let picked = $state<{ connection: FileStoreConnection; item: FileStoreItem; address: string } | undefined>(undefined);
  let isShared = $state(false);
  const mayShare = $derived(picked !== undefined && picked.address === address && picked.connection.kind === 'NEXTCLOUD');

  function open() {
    address = '';
    store = '';
    name = '';
    refusal = undefined;
    picked = undefined;
    isShared = false;
    isOpen = true;
  }

  function pick(connection: FileStoreConnection, item: FileStoreItem) {
    const link = linkOf(item);
    if (!link) return;
    address = link.address;
    store = link.provider;
    name = link.name;
    picked = { connection, item, address: link.address };
  }

  async function link() {
    isLinking = true;
    refusal = undefined;
    try {
      let linked = address.trim();
      if (mayShare && isShared && picked) {
        linked = (await fileStores.share(picked.connection.id, picked.item.ref)).address;
      }
      await files.link(itemId, linked, store as FileLink['provider'] | '', name.trim());
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

{#if mayChange || links.length > 0 || attachments}
  <section class="files" aria-labelledby="entry-files-head-{itemId}" data-entry-files>
    <div class="head">
      <!-- The section above says *Files*; this names the linked ones for a screen reader only. -->
      <h3 id="entry-files-head-{itemId}" class="title"><VisuallyHidden>{t('app.files.title')}</VisuallyHidden></h3>
      {#if mayChange}
        <Button size="sm" tone="subtle" icon="plus" data-link-file onclick={open}>{t('app.files.add')}</Button>
      {/if}
    </div>
    {#if attachments}
      {@render attachments(cells, links.length)}
    {:else if links.length > 0}
      <ul class="rows" data-view={view}>{@render cells()}</ul>
    {/if}
  </section>
{/if}

{#snippet cells()}
  {#each links as each (each.id)}
    {@const href = fileHref(each.address)}
    <li class="row" data-view={view} data-file-link={each.provider}>
      {#if view === 'grid'}
        <!-- The store's mark on the card, where an uploaded file has its ending. -->
        <span class="picture" aria-hidden="true"><Icon name="link" /></span>
      {/if}
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
{/snippet}

{#if isOpen}
  <Dialog size="m" primary={{ label: t('app.files.confirm'), isBusy: isLinking, disabledReason: address.trim() ? undefined : t('filelink.address_required'), onclick: () => void link() }} bind:isOpen title={t('app.files.dialog_title')} dismissLabel={t('app.files.cancel')}>
    <Stack gap="150">
      <StorePicker picked={picked?.item.ref} onpick={pick} />
      <Input label={t('app.files.address_label')} hint={t('app.files.address_hint')} bind:value={address} maxlength={2048} type="url" />
      <Select label={t('app.files.store_label')} options={storeOptions} bind:value={store} />
      <Input label={t('app.files.name_label')} hint={t('app.files.name_hint')} bind:value={name} maxlength={200} />
      {#if mayShare}
        <Checkbox label={t('app.filestores.share_label')} hint={t('app.filestores.share_hint')} bind:checked={isShared} data-share-file />
      {/if}
      {#if refusal}<FailureNote problem={refusal} />{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.files.cancel')}</Button>
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

  .rows[data-view='grid'] { grid-template-columns: repeat(auto-fill, minmax(calc(var(--sp-1600) + var(--sp-400)), 1fr)); gap: var(--sp-150); }
  /* A linked file's card stands in the uploaded files' grid (ET-4), so it is styled on its own. */
  .row[data-view='grid'] { position: relative; flex-direction: column; align-items: stretch; justify-content: flex-start; gap: var(--sp-025); padding-block-end: var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface); overflow: hidden; }
  .row[data-view='grid'] .picture { margin-block-end: var(--sp-050); }
  .row[data-view='grid'] .what { padding-inline: var(--sp-100); }
  .row[data-view='grid'] .what a { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; text-decoration: none; }
  .row[data-view='grid'] .what a:hover { text-decoration: underline; }
  /* Its acts show on hover or focus, as an uploaded file's do. */
  .row[data-view='grid'] :global([data-unlink-file]) { opacity: 0; }
  .row[data-view='grid']:hover :global([data-unlink-file]), .row[data-view='grid']:focus-within :global([data-unlink-file]) { opacity: 1; }
  .row[data-view='grid'] .end { position: absolute; inset-block-start: var(--sp-050); inset-inline: var(--sp-050); justify-content: space-between; }
  .picture { display: grid; place-items: center; block-size: var(--sp-1000); background: var(--bg-canvas); color: var(--text-subtle); }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the control's press
     reaches layout.target, by its own height or beyond its drawn box. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .what a { position: relative; }
    .what a::after { content: ''; position: absolute; inset-block: min(0%, calc((100% - var(--layout-target)) / 2)); inset-inline: min(0%, calc((100% - var(--layout-target)) / 2)); }
  }
</style>
