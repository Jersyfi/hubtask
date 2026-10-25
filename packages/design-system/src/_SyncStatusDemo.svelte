<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workbench's wrapper: every word the component shows is resolved text, so the wording
  // is written here, and the queue's state is a script the story picks.

  import Stack from './Stack.svelte';
  import SyncStatus, { type Connection, type QueuedChange, type RefusedChange, type SyncTone } from './SyncStatus.svelte';
  import TaskRow from './TaskRow.svelte';
  import WorkItemCard from './WorkItemCard.svelte';

  type Mode = 'connected' | 'busy' | 'reconnecting' | 'offline' | 'refused' | 'conflict' | 'rows';
  const { mode = 'connected', isSheet = false }: { mode?: Mode; isSheet?: boolean } = $props();

  /** The six headings of the concept, each with its tone. */
  const HEADINGS: Record<Exclude<Mode, 'rows'>, { headline: string; tone: SyncTone }> = {
    connected: { headline: 'All synchronised', tone: 'ok' },
    busy: { headline: 'Synchronising…', tone: 'busy' },
    offline: { headline: 'Offline – you keep working here', tone: 'warn' },
    reconnecting: { headline: 'Reconnecting', tone: 'warn' },
    refused: { headline: '1 change not accepted', tone: 'bad' },
    conflict: { headline: 'A conflict in 1 entry', tone: 'warn' },
  };

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
    { id: 'r-1', what: 'Due date changed', where: 'Write the home page', reason: 'The due date lies before the start.', href: '#write-the-home-page' },
  ]);
  let conflicts = $state<RefusedChange[]>([
    { id: 'c-1', what: 'Notes changed', where: 'Quote for Miller', reason: 'It was changed on two devices.', onResolve: () => {}, resolveLabel: 'Resolve' },
  ]);

  const connection = $derived<Connection>(mode === 'reconnecting' ? 'reconnecting' : mode === 'offline' ? 'offline' : 'connected');
  const pending = $derived(mode === 'connected' || mode === 'refused' || mode === 'conflict' ? 0 : queued.length);
  const heading = $derived(mode === 'rows' ? HEADINGS.connected : HEADINGS[mode]);
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
    headline={heading.headline}
    tone={heading.tone}
    onRetry={() => {}}
    retryLabel="Try again"
    openLabel="Open the entry"
    pendingCount={pending}
    pendingLabel={pending > 0 ? `${pending} changes waiting` : undefined}
    oldestLabel={pending > 0 ? 'The oldest is from 4 minutes ago.' : undefined}
    syncedLabel={mode === 'connected' ? 'Synchronised at 21:05' : 'Last synchronised at 20:41'}
    queued={pending > 0 ? queued : []}
    refused={mode === 'refused' ? refused : mode === 'conflict' ? conflicts : []}
    listLabel="The copy and the server"
    {isSheet}
    emptyLabel="Nothing is waiting."
    dismissLabel="Dismiss"
    onDismiss={(id) => {
      refused = refused.filter((change) => change.id !== id);
      conflicts = conflicts.filter((change) => change.id !== id);
    }}
  />
{/if}
