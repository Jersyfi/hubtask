<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The reader's own file store connections (UC-INT-17 checks 1 and 4), and the page Microsoft
  // sends the browser back to after the sign-in. A connection is the reader's own grant to their
  // own files, so it is profile configuration like a token: nobody else lists it.
  //
  // A Nextcloud is connected by its login flow: its page opens in a new tab, the reader grants
  // access there and comes back to finish; until then the connection waits. Microsoft is a sign-in
  // in this tab that comes back here.

  import { untrack } from 'svelte';

  import { Button, EmptyState, Input, Stack, Table } from '@hubtask/design-system/components';
  import type { FileStoreConnection, FileStoreConnectionList } from '@hubtask/sync-engine';

  import FinalActDialog from '../lib/frame/FinalActDialog.svelte';
  import SettingsHead from '../lib/frame/SettingsHead.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { providerAnswerOf } from '../lib/data/calendarpush.ts';
  import { connectionLabel, fileStoresPath } from '../lib/data/filestore.ts';
  import { fileStores } from '../lib/data/filestore.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
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

  const columns = $derived([
    { id: 'store', label: t('app.filestores.store_column') },
    { id: 'used', label: t('app.filestores.used_column') },
    { id: 'stop', label: t('app.filestores.disconnect'), isLabelHidden: true },
  ]);
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <Stack gap="300">
    <SettingsHead row="files" />

    <Stack gap="150">
      <p class="quiet">{t('app.filestores.intro')}</p>
      {#if connected}<p class="done" role="status">{t('app.filestores.connected', { store: connectionLabel(connected, storeName(connected)) })}</p>{/if}
      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

      {#if connections.length === 0}
        <p class="quiet">{t('app.filestores.none')}</p>
      {:else}
        <Table label={t('app.filestores.title')} isLabelHidden {columns}>
          {#each connections as conn (conn.id)}
            <tr data-file-store={conn.kind}>
              <th scope="row" class="what">{connectionLabel(conn, storeName(conn))}</th>
              <td class="meta">{formatDateTime(conn.used_at, messages.locale)}</td>
              <td><Button size="sm" tone="danger" onclick={() => (asking = conn)}>{t('app.filestores.disconnect')}</Button></td>
            </tr>
          {/each}
        </Table>
      {/if}
    </Stack>

    <Stack gap="150">
      <h2 class="head">{t('app.filestores.nextcloud')}</h2>
      {#if waiting}
        <p class="quiet" role="status">{t('app.filestores.waiting')}</p>
        <div class="row">
          <Button tone="primary" isBusy={isFinishing} data-finish-nextcloud onclick={() => void finishNextcloud()}>{t('app.filestores.finish')}</Button>
          <a href={waiting.url} target="_blank" rel="noopener noreferrer">{t('app.filestores.open_again')}</a>
          <Button tone="subtle" onclick={() => (waiting = undefined)}>{t('app.filestores.cancel')}</Button>
        </div>
      {:else}
        <div class="row">
          <Input label={t('app.filestores.server_label')} hint={t('app.filestores.server_hint')} bind:value={server} maxlength={512} type="url" />
          <Button
            tone="secondary"
            isBusy={isStarting}
            data-connect-nextcloud
            disabledReason={server.trim() ? undefined : t('filestore.server_required')}
            onclick={() => void connectNextcloud()}
          >{t('app.filestores.connect_nextcloud')}</Button>
        </div>
      {/if}
    </Stack>

    {#if kinds.includes('MICROSOFT')}
      <Stack gap="150">
        <h2 class="head">{t('app.filestores.microsoft')}</h2>
        <p class="quiet">{t('app.filestores.microsoft_hint')}</p>
        <div class="row"><Button tone="secondary" data-connect-microsoft onclick={() => void connectMicrosoft()}>{t('app.filestores.connect_microsoft')}</Button></div>
      </Stack>
    {/if}
  </Stack>

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
  .head { margin: 0; font-size: var(--fs-100); }
  .row { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }
  .quiet { margin: 0; color: var(--text-secondary); max-width: 64ch; }
  .done { margin: 0; color: var(--text-success); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
