<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Who else has this entry open, and who is writing a reply (UC-SVC-03 check 4): two handlers
  // learn of each other before both answer the same requester. The entry's presence document is
  // awareness alone - the connection is opened read-only and nothing of it is stored - and it is
  // the stream's: offline, nobody is shown rather than somebody stale.

  import { untrack } from 'svelte';
  import { Doc, LiveDocument, type LiveSocket } from '@hubtask/editor';

  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { othersHere, presencePath, type Present } from './presence.ts';

  const { itemId, replying = false }: { itemId: string; replying?: boolean } = $props();

  let others = $state<Present[]>([]);
  let live: LiveDocument | undefined;

  $effect(() => {
    const id = itemId;
    return untrack(() => {
      const doc = new Doc();
      const connection = new LiveDocument(doc, {
        open: () => engine.liveSocket<WebSocket>(presencePath(id)) as unknown as LiveSocket,
        writable: false,
        onStatus: (status) => { if (status !== 'connected') others = []; },
      });
      live = connection;
      const changed = () => { others = othersHere(connection.awareness.getStates(), doc.clientID, actor.account?.id); };
      connection.awareness.setLocalState({ account_id: actor.account?.id, name: actor.account?.display_name ?? '', replying });
      connection.awareness.on('change', changed);
      return () => {
        connection.awareness.off('change', changed);
        connection.destroy();
        doc.destroy();
        live = undefined;
        others = [];
      };
    });
  });

  $effect(() => {
    const now = replying;
    untrack(() => live?.awareness.setLocalStateField('replying', now));
  });

  $effect(() => {
    accounts.resolve(others.map((one) => one.accountId).filter((id): id is string => id !== undefined));
  });

  const nameOf = (one: Present) => (one.accountId ? accounts.nameOf(one.accountId) : undefined) ?? one.name ?? t('app.whiteboards.someone');
</script>

{#if others.length > 0}
  <p class="presence" class:is-replying={others.some((one) => one.replying)} role="status" data-entry-presence>
    {#each others as one, index (one.accountId ?? index)}
      <span class="who" data-replying={one.replying || undefined}>
        {t(one.replying ? 'app.entries.presence.replying' : 'app.entries.presence.here', { name: nameOf(one) })}
      </span>
    {/each}
  </p>
{/if}

<style>
  .presence {
    margin: 0;
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-050) var(--sp-100);
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }
  .presence.is-replying { color: var(--text-warning); }
  .who[data-replying] { font-weight: var(--fw-medium); }
</style>
