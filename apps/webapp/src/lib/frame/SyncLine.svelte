<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The frame's **one mark** about the copy and the server (F6-06, ADR-0063 decision 5):
  // `SyncStatus` in the app bar, fed by the stream's state `live` already reads and by
  // `engine.queue()` through `queue`. Every word it shows is
  // resolved here, and the moments go through the formats F5-09 built. The conflict a refused
  // change may carry opens the resolver, which is the one write this line can lead to - an
  // ordinary PATCH of the notes, performed by `notes.rewrite`.

  import { ConflictResolver, SyncStatus, type Connection, type RefusedChange } from '@hubtask/design-system/components';

  import { viewport } from './viewport.svelte.ts';

  import { engine } from '../data/engine.ts';
  import { live } from '../data/live.svelte.ts';
  import { queue } from '../data/queue.svelte.ts';
  import { formatDateTime, formatRelative } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { session } from '../session.svelte.ts';
  import { items } from '../data/items.svelte.ts';

  $effect(() => {
    if (!session.isSignedIn) return;
    return queue.start();
  });

  // The mark's three words, from the store's four states: `off` is drawn by nobody - the line is
  // only rendered with a session - and `offline` is the device's own answer, which is what the
  // struck cloud of ADR-0063 decision 5 is for.
  const connection = $derived<Connection>(live.state === 'live' ? 'connected' : live.state === 'reconnecting' ? 'reconnecting' : 'offline');
  const connectionLabel = $derived(t(`app.sync.${connection}`));

  /** When the copy last synchronised: the store's position, read once per change of the queue. */
  let syncedAt = $state<number | undefined>(undefined);
  $effect(() => {
    void queue.count;
    void live.state;
    void engine.storage?.get<{ at: number }>('meta', 'position').then((position) => {
      syncedAt = position?.at;
    });
  });

  const refused = $derived<readonly RefusedChange[]>(
    queue.refused.map((change) => ({
      id: change.id,
      what: change.what,
      where: change.where,
      reason: change.reason,
      href: change.href,
    })),
  );

  /** A conflict on the notes, shown from the list: the entry, and both versions. */
  let resolving = $state<{ itemId: string; conflictId: string; mine: string; theirs: string; preservedCommentId?: string } | undefined>(undefined);
  let isResolverOpen = $state(false);

  const conflicts = $derived<readonly RefusedChange[]>(
    queue.conflicts.filter((conflict) => conflict.field === 'notes').map((conflict) => ({
      id: `conflict-${conflict.id}`,
      what: t('app.sync.what.patch.notes'),
      where: conflict.itemId,
      reason: t('app.sync.conflict.strip'),
      href: `/items/${conflict.itemId}`,
      resolveLabel: t('app.sync.conflict.show'),
      onResolve: () => {
        resolving = {
          itemId: conflict.itemId,
          conflictId: conflict.id,
          mine: String(conflict.mine ?? ''),
          theirs: String(conflict.theirs ?? ''),
          preservedCommentId: conflict.preservedCommentId,
        };
        isResolverOpen = true;
      },
    })),
  );

  async function keep(): Promise<void> {
    isResolverOpen = false;
    if (resolving) await queue.dismissConflict(resolving.conflictId);
    resolving = undefined;
  }

  /** "Write mine again": an ordinary PATCH of the notes from the current version, nothing else. */
  async function rewrite(): Promise<void> {
    if (!resolving) return;
    const { itemId, conflictId, mine } = resolving;
    isResolverOpen = false;
    resolving = undefined;
    await items.rewriteNotes(itemId, mine);
    await queue.dismissConflict(conflictId);
  }
</script>

{#if session.isSignedIn}
  <SyncStatus
    {connection}
    {connectionLabel}
    pendingCount={queue.count}
    pendingLabel={queue.count > 0 ? t('app.sync.pending', { count: queue.count }) : undefined}
    oldestLabel={queue.oldestAt !== undefined ? t('app.sync.oldest', { moment: formatRelative(new Date(queue.oldestAt).toISOString(), messages.locale) }) : undefined}
    syncedLabel={syncedAt !== undefined ? t('app.sync.synced', { moment: formatDateTime(new Date(syncedAt).toISOString(), messages.locale) }) : undefined}
    queued={queue.queued}
    refused={[...conflicts, ...refused]}
    listLabel={t('app.sync.panel')}
    emptyLabel={t('app.sync.empty')}
    refusedLabel={t('app.sync.refused')}
    dismissLabel={t('app.sync.dismiss')}
    onDismiss={(id) => (id.startsWith('conflict-') ? queue.dismissConflict(id.slice('conflict-'.length)) : queue.dismiss(id))}
    isSheet={viewport.isCompact}
  />
  {#if resolving}
    <ConflictResolver
      bind:isOpen={isResolverOpen}
      title={t('app.sync.conflict.title')}
      fieldLabel={t('app.sync.conflict.field')}
      theirs={resolving.theirs}
      theirsLabel={t('app.sync.conflict.theirs')}
      mine={resolving.mine}
      mineLabel={t('app.sync.conflict.mine')}
      preservedHref={resolving.preservedCommentId ? `/items/${resolving.itemId}#comment-${resolving.preservedCommentId}` : undefined}
      preservedLabel={resolving.preservedCommentId ? t('app.sync.conflict.preserved') : undefined}
      keepLabel={t('app.sync.conflict.keep')}
      rewriteLabel={t('app.sync.conflict.rewrite')}
      dismissLabel={t('app.workspace.cancel')}
      onKeep={keep}
      onRewrite={rewrite}
    />
  {/if}
{/if}
