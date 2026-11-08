<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The reader's own file store connections (UC-INT-17 checks 1 and 4), and the page Microsoft
  // sends the browser back to after the sign-in. A connection is the reader's own grant to their
  // own files, so it is profile configuration like a token: nobody else lists it.
  //
  // A Nextcloud is connected by its login flow: its page opens in a new tab, the reader grants
  // access there and comes back to finish; until then the connection waits. Microsoft is a sign-in
  // in this tab that comes back here. A list page (page-system §9.5): connecting is the head's
  // primary and a Dialog M of the store and its server, which holds the waiting login flow too;
  // disconnecting is the row's ⋯ and a Dialog S.

  import { untrack } from 'svelte';

  import { Button, DataTable, Dialog, EmptyState, FilterBar, Input, ListPage, Radio, Stack, type DataColumn, type DataRowMenu, type DialogAction } from '@hubtask/design-system/components';
  import type { FileStoreConnection, FileStoreConnectionList } from '@hubtask/sync-engine';

  import FinalActDialog from '../lib/frame/FinalActDialog.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { profileHead, profileTitle } from '../lib/profile/profilehead.ts';

  import { actor } from '../lib/data/account.svelte.ts';
  import { providerAnswerOf } from '../lib/data/calendarpush.ts';
  import { connectionLabel, fileStoresPath } from '../lib/data/filestore.ts';
  import { fileStores } from '../lib/data/filestore.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    /** The query Microsoft sent the browser back with. */
    providerAnswer?: Record<string, string>;
    /** Called once the answer is handed on, so the address forgets the code. */
    onanswered?: () => void;
  }

  let { providerAnswer, onanswered }: Props = $props();

  const account = $derived(actor.account);
  const read = resource<FileStoreConnectionList>(fileStoresPath);
  const connections = $derived(read.state.status === 'ready' ? read.state.data.data : []);
  const kinds = $derived(read.state.status === 'ready' ? read.state.data.kinds : []);
  let search = $state('');

  let failure = $state<string | undefined>(undefined);
  let connected = $state<FileStoreConnection | undefined>(undefined);
  let asking = $state<FileStoreConnection | undefined>(undefined);
  let server = $state('');
  let isStarting = $state(false);
  /** A Nextcloud login flow begun in another tab, waiting for the reader to come back. */
  let waiting = $state<{ state: string; url: string } | undefined>(undefined);
  let isFinishing = $state(false);

  const fail = (error: unknown) => (failure = renderProblem(error as never, messages).message);

  $effect(() => {
    const answer = untrack(() => (providerAnswer ? providerAnswerOf(providerAnswer) : undefined));
    if (!answer) return;
    void fileStores
      .complete(answer)
      .then((conn) => (connected = conn))
      .catch(fail)
      .finally(() => onanswered?.());
  });

  let isConnecting = $state(false);
  let kind = $state<'NEXTCLOUD' | 'MICROSOFT'>('NEXTCLOUD');
  function openConnect() {
    failure = undefined;
    kind = 'NEXTCLOUD';
    isConnecting = true;
  }

  async function connectNextcloud() {
    isStarting = true;
    failure = undefined;
    connected = undefined;
    try {
      const started = await fileStores.connect('NEXTCLOUD', server.trim());
      if (!started.state) return;
      waiting = { state: started.state, url: started.authorize_url };
      window.open(started.authorize_url, '_blank', 'noopener');
    } catch (error) {
      fail(error);
    } finally {
      isStarting = false;
    }
  }

  async function finishNextcloud() {
    if (!waiting) return;
    isFinishing = true;
    failure = undefined;
    try {
      connected = await fileStores.complete({ state: waiting.state });
      waiting = undefined;
      server = '';
      isConnecting = false;
    } catch (error) {
      fail(error);
    } finally {
      isFinishing = false;
    }
  }

  async function connectMicrosoft() {
    failure = undefined;
    try {
      const started = await fileStores.connect('MICROSOFT');
      window.location.assign(started.authorize_url);
    } catch (error) {
      fail(error);
    }
  }

  const storeName = (conn: FileStoreConnection) => t(conn.kind === 'NEXTCLOUD' ? 'app.filestores.nextcloud' : 'app.filestores.microsoft');

  const shown = $derived(connections.filter((conn) => matchesSearch(search, [connectionLabel(conn, storeName(conn)), storeName(conn)])));

  const columns = $derived<DataColumn[]>([
    { id: 'store', label: t('app.filestores.store_column'), priority: 1 },
    { id: 'used', label: t('app.filestores.used_column'), priority: 2 },
  ]);

  const title = $derived(profileTitle('files'));
  $effect(() => page.entitle(title));

  const rowMenu = (conn: FileStoreConnection): DataRowMenu => ({
    label: t('app.page.more_for', { name: connectionLabel(conn, storeName(conn)) }),
    items: [{ id: 'disconnect', label: t('app.filestores.disconnect_menu'), isDestructive: true }],
  });

  const primary = $derived<DialogAction>(
    waiting
      ? { label: t('app.filestores.finish'), isBusy: isFinishing, onclick: (): void => void finishNextcloud() }
      : kind === 'MICROSOFT'
        ? { label: t('app.filestores.connect_microsoft'), onclick: (): void => void connectMicrosoft() }
        : {
            label: t('app.filestores.connect_nextcloud'),
            isBusy: isStarting,
            disabledReason: server.trim() ? undefined : t('filestore.server_required'),
            onclick: (): void => void connectNextcloud(),
          },
  );
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <ListPage head={profileHead('files', { primary: { label: t('app.filestores.connect'), icon: 'plus', opener: 'connect-file-store', onclick: openConnect } })}>
    {#snippet intro()}<p class="quiet">{t('app.filestores.intro')}</p>{/snippet}
    {#snippet filters()}
      <FilterBar
        searchLabel={t('app.filestores.search')}
        clearSearchLabel={t('app.page.clear_search')}
        bind:search
        countLabel={read.state.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(connections.length) }) : undefined}
      />
    {/snippet}

    {#if connected}<p class="done" role="status">{t('app.filestores.connected', { store: connectionLabel(connected, storeName(connected)) })}</p>{/if}
    {#if failure && !isConnecting}<p class="failure" role="alert">{failure}</p>{/if}

    <DataTable
      label={title}
      {columns}
      rows={shown}
      key={(conn: FileStoreConnection) => conn.id}
      rowLabel={(conn: FileStoreConnection) => connectionLabel(conn, storeName(conn))}
      menu={rowMenu}
      onmenu={(conn: FileStoreConnection) => (asking = conn)}
      actionsLabel={t('app.page.row_actions')}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      shownLabel={shown.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
    >
      {#snippet cell(conn: FileStoreConnection, column: DataColumn)}
        {#if column.id === 'store'}
          <span class="what" data-file-store={conn.kind}>{connectionLabel(conn, storeName(conn))}</span>
        {:else}
          <span class="meta">{formatDateTime(conn.used_at, messages.locale)}</span>
        {/if}
      {/snippet}
      {#snippet empty()}
        {#if connections.length === 0}
          <EmptyState kind="unused" title={t('app.filestores.none')} />
        {:else}
          <EmptyState kind="filtered" title={t('app.filestores.filtered_title')}>
            {#snippet action()}<Button onclick={() => (search = '')}>{t('app.page.clear_search')}</Button>{/snippet}
          </EmptyState>
        {/if}
      {/snippet}
    </DataTable>
  </ListPage>

  <Dialog size="m" title={t('app.filestores.connect_title')} bind:isOpen={isConnecting} dismissLabel={t('app.filestores.cancel')} onClose={() => (waiting = undefined)} {primary}>
    <Stack gap="150">
      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
      {#if waiting}
        <p class="quiet" role="status">{t('app.filestores.waiting')}</p>
        <a href={waiting.url} target="_blank" rel="noopener noreferrer">{t('app.filestores.open_again')}</a>
      {:else}
        {#if kinds.includes('MICROSOFT')}
          <Radio
            label={t('app.filestores.kind_label')}
            bind:value={kind}
            options={[
              { value: 'NEXTCLOUD', label: t('app.filestores.nextcloud') },
              { value: 'MICROSOFT', label: t('app.filestores.microsoft') },
            ]}
          />
        {/if}
        {#if kind === 'NEXTCLOUD'}
          <Input label={t('app.filestores.server_label')} hint={t('app.filestores.server_hint')} bind:value={server} maxlength={512} type="url" data-connect-nextcloud />
        {:else}
          <p class="quiet">{t('app.filestores.microsoft_hint')}</p>
        {/if}
      {/if}
    </Stack>
    {#snippet actions()}
      <Button tone="subtle" onclick={() => { isConnecting = false; waiting = undefined; }}>{t('app.filestores.cancel')}</Button>
    {/snippet}
  </Dialog>

  <FinalActDialog
    isOpen={asking !== undefined}
    title={t('app.filestores.disconnect_title', { store: asking ? connectionLabel(asking, storeName(asking)) : '' })}
    body={t('app.filestores.disconnect_body')}
    confirmLabel={t('app.filestores.disconnect')}
    busyLabel={t('app.filestores.disconnecting')}
    onConfirm={async () => {
      if (!asking) return;
      try {
        await fileStores.disconnect(asking);
      } catch (error) {
        fail(error);
      }
    }}
    onClose={() => (asking = undefined)}
  />
{/if}

<style>
  .what { font-weight: var(--fw-medium); overflow-wrap: anywhere; }
  .meta { color: var(--text-secondary); font-size: var(--fs-075); }
  .quiet { margin: 0; color: var(--text-secondary); }
  .done { margin: 0; color: var(--text-success); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
