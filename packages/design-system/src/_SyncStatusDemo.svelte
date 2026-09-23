<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workbench's wrapper: every word the component shows is resolved text, so the wording
  // is written here, and the queue's state is a script the story picks.

  import Stack from './Stack.svelte';
  import SyncStatus, { type Connection, type QueuedChange, type RefusedChange } from './SyncStatus.svelte';
  import TaskRow from './TaskRow.svelte';
  import WorkItemCard from './WorkItemCard.svelte';

  const { mode = 'connected', isSheet = false }: { mode?: 'connected' | 'reconnecting' | 'offline' | 'refused' | 'unread' | 'rows'; isSheet?: boolean } = $props();

  const WORDS: Record<Connection, string> = {
    connected: 'Connected',
    reconnecting: 'Reconnecting…',
    offline: 'Offline',
  };

  const queued: QueuedChange[] = [
    { id: 'q-1', what: 'Title changed', where: 'Write the reference', href: '#write-the-reference' },
    { id: 'q-2', what: 'Completed', where: 'Book the venue', href: '#book-the-venue' },
    { id: 'q-3', what: 'Comment added', where: 'Print the badges' },
  ];

  let refused = $state<RefusedChange[]>([
    { id: 'r-1', what: 'Notes changed', where: 'Send the agenda', reason: 'That entry was deleted for good while you were away. What you wrote is kept here.' },
    { id: 'r-2', what: 'Assigned', where: 'Order lunch', reason: 'You may not change this any more.', onResolve: () => {}, resolveLabel: 'Show both versions' },
  ]);

  const connection = $derived<Connection>(mode === 'reconnecting' ? 'reconnecting' : mode === 'offline' ? 'offline' : 'connected');
  const pending = $derived(mode === 'connected' || mode === 'unread' ? 0 : queued.length);

  /** The manifest the client could not read, and the ask-again. Nothing was written here. */
  let asked = $state(0);
  const unread = $derived(mode === 'unread'
    ? {
        label: 'This installation could not be read, so this client does not know what it permits.',
        reason: asked === 0 ? 'The server could not be reached.' : `Asked again ${asked} time(s).`,
        reference: '01a0e2e0-0000-7000-8000-0000000000ab',
        referenceLabel: 'Reference',
        retryLabel: 'Try again',
        onRetry: () => (asked += 1),
      }
    : undefined);
</script>

{#if mode === 'rows'}
  <Stack gap="200">
    <TaskRow type="TASK" title="Write the reference" completeLabel="Complete" pendingLabel="Waiting to be sent" />
    <TaskRow type="TASK" title="Book the venue" completeLabel="Complete" isCompleted pendingLabel="Waiting to be sent" />
    <WorkItemCard title="Print the badges" pendingLabel="Waiting to be sent" />
  </Stack>
{:else}
  <SyncStatus
    {connection}
    connectionLabel={WORDS[connection]}
    pendingCount={pending}
    pendingLabel={pending > 0 ? `${pending} changes waiting` : undefined}
    oldestLabel={pending > 0 ? 'The oldest is from 4 minutes ago.' : undefined}
    syncedLabel={mode === 'connected' ? 'Synchronised at 21:05' : 'Last synchronised at 20:41'}
    queued={pending > 0 ? queued : []}
    refused={mode === 'refused' ? refused : []}
    {unread}
    listLabel="The copy and the server"
    {isSheet}
    emptyLabel="Nothing is waiting."
    refusedLabel="Not accepted"
    dismissLabel="Dismiss"
    onDismiss={(id) => (refused = refused.filter((change) => change.id !== id))}
  />
{/if}
