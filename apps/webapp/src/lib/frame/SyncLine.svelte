<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The frame's **one mark** about the copy and the server (F6-06, ADR-0063 decision 5):
  // `SyncStatus` in the app bar, fed by the stream's state `live` already reads and by
  // `engine.queue()` through `queue`. Every word it shows is
  // resolved here, and the moments go through the formats F5-09 built. The conflict a refused
  // change may carry opens the resolver, which is the one write this line can lead to - an
  // ordinary PATCH of the notes, performed by `notes.rewrite`.
  //
  // It also carries the **manifest that could not be read** and the way to ask again (issue 1020).
  // `/meta/capabilities` is what the whole client configures itself from, and until this was here
  // the only retry for it was a button on `/installation` - the page ADR-0063 decision 6 took out
  // of the reader's navigation - so from an entry screen there was no way back but a reload. This
  // mark is on every screen, which is what "a retry where the reader is" means.

  import { ConflictResolver, SyncStatus, type Connection, type RefusedChange, type UnreadInstallation } from '@hubtask/design-system/components';

  import { viewport } from './viewport.svelte.ts';

  import { manifest } from '../data/capabilities.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { live } from '../data/live.svelte.ts';
  import { queue } from '../data/queue.svelte.ts';
  import { formatDateTime, formatRelative } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { session } from '../session.svelte.ts';
  import { items } from '../data/items.svelte.ts';

  $effect(() => {
    if (!session.isSignedIn) return;
    return queue.start();
  });

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

  /**
   * The manifest, where it could not be read. Absent in the ordinary case, which is every case
   * but one - and the one is the whole application running on nothing it was told.
   */
  const unread = $derived.by<UnreadInstallation | undefined>(() => {
    if (!manifest.failure) return undefined;
    const problem = renderProblem(manifest.failure, messages);
    return {
      label: t('app.installation.unread'),
      reason: problem.message,
      reference: problem.reference,
      referenceLabel: t('app.reference'),
      retryLabel: t('app.retry'),
      onRetry: () => void manifest.refresh(),
    };
  });

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
    {unread}
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
