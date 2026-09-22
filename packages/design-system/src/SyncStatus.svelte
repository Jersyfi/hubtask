<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // **One mark in the bar** about the copy and the server (F6-06, offline-sync.md §9, ADR-0063
  // decision 5): connected, reconnecting or offline; how many changes wait to be sent and how old
  // the oldest is; when the copy last synchronised. Pressed, it says all of it - and it lists
  // every waiting change as what and where, and every refused one with its reason and a dismiss,
  // because a rejection is shown and never swallowed (§9.5).
  //
  // It was a line of every page, and at its quietest that line read "Connected": a row of the
  // screen spent on the ordinary case. Now the ordinary case is a mark that says nothing until it
  // is asked, and what it used to print is behind it, whole.
  //
  // It renders no sentence of its own: every word arrives resolved, because what "offline" is
  // called and how a moment is spelled are the catalogue's and the formats' (ADR-0011). Rule 3:
  // each state carries a mark beside its word, so the line reads in greyscale and in a screen
  // reader alike. The connection state is a `status` live region (F5-12): a change the reader
  // cannot see is announced, once, when it changes - and nothing else here is, because a count
  // that moved is not news and a heartbeat is not either.

  import Button from './Button.svelte';
  import Drawer from './Drawer.svelte';
  import Icon from './Icon.svelte';
  import Popover from './Popover.svelte';
  import Stack from './Stack.svelte';
  import VisuallyHidden from './VisuallyHidden.svelte';
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
    /**
     * A sheet from the bottom rather than a surface beside the mark.
     *
     * The same choice `AccountMenu` makes: on `compact` there is no room beside a bar's control,
     * so what it opens comes from the edge. Width is the caller's question, never this
     * component's.
     */
    isSheet?: boolean;
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
    isSheet = false,
  }: Props = $props();

  const MARKS: Record<Connection, IconName> = {
    connected: 'cloud-check',
    reconnecting: 'loader-circle',
    offline: 'cloud-off',
  };

  const hasSomething = $derived(queued.length > 0 || refused.length > 0);
  /** Nothing to say: connected, nothing waiting, nothing refused. The quietest the mark gets. */
  const isQuiet = $derived(connection === 'connected' && !hasSomething);

  let isOpen = $state(false);
</script>

{#snippet inside()}
  <!-- A floor on the width, so the surface beside a bar's control is not squeezed into a column
       of two words by the edge it is anchored against. -->
  <div class="panel">
    <Stack gap="150">
    <p class="detail" data-connection={connection}>
      <span class="mark"><Icon name={MARKS[connection]} size="sm" /></span>
      <span class="word">{connectionLabel}</span>
    </p>
    {#if syncedLabel}<p class="detail">{syncedLabel}</p>{/if}
    {#if pendingLabel && pendingCount > 0}<p class="detail">{pendingLabel}</p>{/if}
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
  </div>
{/snippet}

{#snippet mark(props: Record<string, unknown>)}
  <!-- The one control. Its name is the state and the count, so a reader who cannot see the mark
       is told what it says rather than "sync". The mark turns with the `pending` role while
       reconnecting - a continuous indicator with no end - and stands still otherwise. -->
  <button
    type="button"
    class="trigger"
    data-connection={connection}
    data-quiet={isQuiet ? '' : undefined}
    aria-label={pendingCount > 0 && pendingLabel ? `${connectionLabel} — ${pendingLabel}` : connectionLabel}
    {...props}
  >
    <span class="mark" data-spin={connection === 'reconnecting' ? '' : undefined}><Icon name={MARKS[connection]} size="sm" /></span>
    {#if pendingCount > 0}<span class="count">{pendingCount}</span>{/if}
    {#if refused.length > 0}<span class="dot" aria-hidden="true"></span>{/if}
  </button>
{/snippet}

<div class="sync" data-connection={connection}>
  <!-- The state is announced when it changes, whether or not anything is open: a reader who cannot
       see the mark is told that the connection went, once, and not told the heartbeat. -->
  <VisuallyHidden as="span"><span role="status">{connectionLabel}</span></VisuallyHidden>

  {#if isSheet}
    {@render mark({ onclick: () => (isOpen = true), 'aria-expanded': isOpen, 'aria-haspopup': 'dialog' })}
    <Drawer bind:isOpen edge="block-end" title={listLabel} dismissLabel={dismissLabel ?? listLabel}>
      {@render inside()}
    </Drawer>
  {:else}
    <Popover label={listLabel}>
      {#snippet trigger(props)}{@render mark(props)}{/snippet}
      {@render inside()}
    </Popover>
  {/if}
</div>

<style>
  .sync {
    display: inline-flex;
    align-items: center;
    font-size: var(--fs-075);
    color: var(--text-secondary);
  }

  .panel { min-inline-size: 24ch; }

  /* The one control: a mark, a count where anything waits, and a dot where anything was refused. */
  .trigger {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    position: relative;
    min-inline-size: var(--density-control-sm-min);
    min-block-size: var(--density-control-sm-min);
    padding-inline: var(--sp-100);
    border: 0;
    border-radius: var(--r-full);
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-075);
    cursor: pointer;
  }

  .trigger:hover { background: var(--bg-surface-hover); color: var(--text-primary); }

  .trigger:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  /* Rule 3: the state is a colour *and* a mark of its own, and the accessible name says it in
     words. Connected with nothing waiting is the quietest the frame gets — the ordinary case
     spends no colour on itself. Offline is the one state somebody has to see first. */
  .trigger[data-quiet] { color: var(--text-subtle); }
  .trigger[data-connection='connected']:not([data-quiet]) { color: var(--status-success-text); }
  .trigger[data-connection='reconnecting'] { color: var(--status-warning-text); }
  .trigger[data-connection='offline'] {
    background: var(--status-warning-accent);
    color: var(--text-inverse);
    font-weight: var(--fw-semibold);
  }

  .count { font-variant-numeric: tabular-nums; }

  /* Something was refused and is waiting to be read. */
  .dot {
    position: absolute;
    inset-block-start: var(--sp-050);
    inset-inline-end: var(--sp-050);
    inline-size: var(--sp-100);
    block-size: var(--sp-100);
    border-radius: var(--r-full);
    background: var(--status-danger-accent);
  }

  .mark { display: inline-flex; }

  .detail[data-connection='connected'] { color: var(--status-success-text); }
  .detail[data-connection='offline'] { color: var(--status-danger-text); }

  .detail .word { margin-inline-start: var(--sp-050); }

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
    display: flex;
    align-items: center;
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

  /* Rule 5: the ring on the links the list carries. */
  .where:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }
</style>
