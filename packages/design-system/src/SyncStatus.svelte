<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One line in the frame's header about the copy and the server (F6-06, offline-sync.md §9):
  // connected, reconnecting or offline; how many changes wait to be sent and how old the oldest
  // is; when the copy last synchronised. Opened, it lists every waiting change as what and where,
  // and every refused one with its reason and a dismiss - a rejection is shown, never swallowed
  // (§9.5).
  //
  // It renders no sentence of its own: every word arrives resolved, because what "offline" is
  // called and how a moment is spelled are the catalogue's and the formats' (ADR-0011). Rule 3:
  // each state carries a mark beside its word, so the line reads in greyscale and in a screen
  // reader alike. The connection state is a `status` live region (F5-12): a change the reader
  // cannot see is announced, once, when it changes - and nothing else here is, because a count
  // that moved is not news and a heartbeat is not either.

  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import Popover from './Popover.svelte';
  import Stack from './Stack.svelte';
  import type { IconName } from './icons/index.ts';

  /** Where the connection stands. */
  export type Connection = 'connected' | 'reconnecting' | 'offline';

  /** One change waiting to be sent: what it did, and where. Resolved text. */
  export interface QueuedChange {
    readonly id: string;
    /** "Title changed", "Completed", "Comment added" - the kind, in the catalogue's words. */
    readonly what: string;
    /** The entry's title, or what the caller has for it. The reader's own words. */
    readonly where: string;
    readonly href?: string;
  }

  /** One change the server refused: what, where, and why - and the way out. */
  export interface RefusedChange {
    readonly id: string;
    readonly what: string;
    readonly where: string;
    /** The reason, resolved from the server's code (`sync.gone`, `forbidden`). */
    readonly reason: string;
    readonly href?: string;
    /** Present for a conflict the person may act on: opens the resolver. */
    readonly onResolve?: () => void;
    readonly resolveLabel?: string;
  }

  interface Props {
    connection: Connection;
    /** The word for the connection state. Resolved. */
    connectionLabel: string;
    /** How many changes wait. Zero renders no count. */
    pendingCount?: number;
    /** "3 changes waiting" - resolved, with the count already in it. */
    pendingLabel?: string;
    /** "oldest 4 minutes ago" - resolved, through the formats. */
    oldestLabel?: string;
    /** "synchronised at 21:05" - resolved. Absent while nothing has synchronised yet. */
    syncedLabel?: string;
    queued?: readonly QueuedChange[];
    refused?: readonly RefusedChange[];
    /** What the list is called, for the reader who arrives on it without seeing the trigger. */
    listLabel: string;
    /** "Nothing waiting" - the list when it is empty. */
    emptyLabel: string;
    /** The heading over the refused changes, and the word on each dismiss. */
    refusedLabel?: string;
    dismissLabel?: string;
    onDismiss?: (id: string) => void;
  }

  const {
    connection,
    connectionLabel,
    pendingCount = 0,
    pendingLabel,
    oldestLabel,
    syncedLabel,
    queued = [],
    refused = [],
    listLabel,
    emptyLabel,
    refusedLabel,
    dismissLabel,
    onDismiss,
  }: Props = $props();

  const MARKS: Record<Connection, IconName> = {
    connected: 'cloud-check',
    reconnecting: 'loader-circle',
    offline: 'cloud-off',
  };

  const hasSomething = $derived(queued.length > 0 || refused.length > 0);
</script>

<div class="sync" data-connection={connection}>
  <!-- The connection state, announced when it changes. The mark turns with the `pending` role
       while reconnecting - a continuous indicator with no end - and stands still otherwise. -->
  <span class="state" role="status">
    <span class="mark" data-spin={connection === 'reconnecting' ? '' : undefined}><Icon name={MARKS[connection]} size="sm" /></span>
    <span class="word">{connectionLabel}</span>
  </span>

  {#if pendingCount > 0 || refused.length > 0}
    <Popover label={listLabel}>
      {#snippet trigger(props)}
        <Button size="sm" tone="subtle" icon={refused.length > 0 ? 'circle-alert' : 'cloud-upload'} {...props}>
          {#if pendingCount > 0}{pendingLabel ?? String(pendingCount)}{:else}{refusedLabel}{/if}
        </Button>
      {/snippet}
      <Stack gap="150">
        {#if oldestLabel}<p class="detail">{oldestLabel}</p>{/if}
        {#if !hasSomething}
          <p class="detail">{emptyLabel}</p>
        {/if}
        {#if queued.length > 0}
          <ul class="list">
            {#each queued as change (change.id)}
              <li class="change">
                <span class="what">{change.what}</span>
                {#if change.href}<a class="where" href={change.href}>{change.where}</a>{:else}<span class="where">{change.where}</span>{/if}
              </li>
            {/each}
          </ul>
        {/if}
        {#if refused.length > 0}
          <section class="refused" aria-label={refusedLabel}>
            {#if refusedLabel}<h3 class="heading">{refusedLabel}</h3>{/if}
            <ul class="list">
              {#each refused as change (change.id)}
                <li class="change" data-refused>
                  <span class="what">{change.what}</span>
                  {#if change.href}<a class="where" href={change.href}>{change.where}</a>{:else}<span class="where">{change.where}</span>{/if}
                  <span class="reason">{change.reason}</span>
                  <span class="actions">
                    {#if change.onResolve && change.resolveLabel}
                      <Button size="sm" onclick={change.onResolve}>{change.resolveLabel}</Button>
                    {/if}
                    {#if onDismiss && dismissLabel}
                      <Button size="sm" tone="subtle" icon="x" onclick={() => onDismiss(change.id)}>{dismissLabel}</Button>
                    {/if}
                  </span>
                </li>
              {/each}
            </ul>
          </section>
        {/if}
      </Stack>
    </Popover>
  {/if}

  {#if syncedLabel}
    <span class="synced">{syncedLabel}</span>
  {/if}
</div>

<style>
  .sync {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-100);
    font-size: var(--fs-075);
    color: var(--text-secondary);
  }

  .state {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
  }

  /* Rule 3: the colour says it and so does the word beside it. */
  [data-connection='offline'] .state { color: var(--text-warning); }
  [data-connection='connected'] .state { color: var(--text-success); }

  .mark { display: inline-flex; }

  /* The `pending` role: a continuous indicator with no beginning and no end, so its easing is
     linear and it never arrives (design-system.md §9). Transform only; reduced motion stops it. */
  .mark[data-spin] {
    animation: turn var(--motion-pending-duration) var(--motion-pending-easing) infinite;
  }

  @keyframes turn {
    to { transform: rotate(1turn); }
  }

  @media (prefers-reduced-motion: reduce) {
    .mark[data-spin] { animation: none; }
  }

  :global([data-motion='reduced']) .mark[data-spin] { animation: none; }

  .detail {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }

  .list {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
  }

  .change {
    display: grid;
    gap: var(--sp-025);
  }

  .what { color: var(--text-primary); }

  .where { color: var(--text-secondary); }

  .reason { color: var(--text-danger); }

  .actions {
    display: flex;
    gap: var(--sp-050);
    flex-wrap: wrap;
  }

  .heading {
    margin: 0;
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    color: var(--text-secondary);
  }

  .synced { color: var(--text-secondary); }

  /* Rule 5: the ring on the links the list carries. */
  .where:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }
</style>
