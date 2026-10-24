<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // **One mark in the bar** about the copy and the server (offline-sync.md §9, design-system.md
  // §11.6): connected, reconnecting or offline; how many changes wait to be sent and how old
  // the oldest is; when the copy last synchronised. Pressed, it says all of it - and it lists
  // every waiting change as what and where, and every refused one with its reason and a dismiss,
  // because a rejection is shown and never swallowed (§9.5).
  //
  // A line on every page would read "Connected" at its quietest: a row of the screen spent on the
  // ordinary case. So the ordinary case is a mark that says nothing until it is asked, and the
  // whole line is behind it.
  //
  // It renders no sentence of its own: every word arrives resolved, because what "offline" is
  // called and how a moment is spelled are the catalogue's and the formats' (ADR-0011). Rule 3:
  // each state carries a mark beside its word, so the line reads in greyscale and in a screen
  // reader alike. The connection state is a `status` live region: a change the reader
  // cannot see is announced, once, when it changes - and nothing else here is, because a count
  // that moved is not news and a heartbeat is not either.

  import Button from './Button.svelte';
  import Drawer from './Drawer.svelte';
  import Icon from './Icon.svelte';
  import Popover from './Popover.svelte';
  import VisuallyHidden from './VisuallyHidden.svelte';
  import type { IconName } from './icons/index.ts';

  /** Where the connection stands. */
  export type Connection = 'connected' | 'reconnecting' | 'offline';

  /**
   * The surface's heading, the most pressing of six states (design-system.md §11.6): a conflict,
   * a refusal, offline, reconnecting, sending, all synchronised - each its tone.
   */
  export type SyncTone = 'ok' | 'busy' | 'warn' | 'bad';

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
    /** The surface's heading - "All synchronised", "1 change not accepted" - resolved. */
    headline?: string;
    /** Its tone; the band's colour and mark follow it. */
    tone?: SyncTone;
    /** Trying now, where something waits for the connection: the surface's one action. */
    onRetry?: () => void;
    retryLabel?: string;
    /** The way to an entry a refusal is about. */
    openLabel?: string;
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
    /** The word on each dismiss. */
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
    headline,
    tone,
    onRetry,
    retryLabel,
    openLabel,
    pendingCount = 0,
    pendingLabel,
    oldestLabel,
    syncedLabel,
    queued = [],
    refused = [],
    listLabel,
    emptyLabel,
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
  /** Without a heading from the caller, the connection is the heading. */
  const toneShown = $derived<SyncTone>(tone ?? (connection === 'connected' ? 'ok' : 'warn'));
  const BAND_MARKS: Record<SyncTone, IconName> = { ok: 'circle-check', busy: 'loader-circle', warn: 'cloud-off', bad: 'circle-alert' };
  /** A conflict is the one refusal with a way through it: its band carries the two-way mark. */
  const isConflict = $derived(refused.some((change) => change.onResolve !== undefined));
  const bandMark = $derived<IconName>(
    isConflict ? 'arrow-right-left' : toneShown === 'warn' && connection === 'reconnecting' ? 'loader-circle' : BAND_MARKS[toneShown],
  );
  /** What puts the dot on the mark: something to read, whether it was refused or never arrived. */
  const hasRefusal = $derived(refused.length > 0);
  /** Nothing to say: connected, nothing waiting, nothing refused. The quietest the mark gets. */
  const isQuiet = $derived(connection === 'connected' && !hasSomething);

  let isOpen = $state(false);
</script>

{#snippet inside()}
  <!-- The concept's surface: one band across the whole width naming the most pressing state, the
       lines under it on one indent, and at most one action (design-system.md §11.6). -->
  <div class="panel" data-tone={toneShown}>
    <p class="band" data-tone={toneShown}>
      <span class="mark" data-spin={bandMark === 'loader-circle' ? '' : undefined}><Icon name={bandMark} size="sm" /></span>
      <span class="word">{headline ?? connectionLabel}</span>
    </p>
    <div class="body">
      {#if refused.length > 0}
        <ul class="list">
          {#each refused as change (change.id)}
            <li class="change" data-refused>
              <span class="said"><b class="where">{change.where}</b> – {change.reason}</span>
              <span class="actions">
                {#if change.onResolve && change.resolveLabel}
                  <Button size="sm" tone="primary" onclick={change.onResolve}>{change.resolveLabel}</Button>
                {:else if change.href && openLabel}
                  <a class="open" href={change.href}>{openLabel}</a>
                {/if}
                {#if onDismiss && dismissLabel && !change.onResolve}
                  <Button size="sm" tone="secondary" onclick={() => onDismiss(change.id)}>{dismissLabel}</Button>
                {/if}
              </span>
            </li>
          {/each}
        </ul>
      {:else}
        {#if pendingLabel && pendingCount > 0}<p class="detail">{pendingLabel}</p>{/if}
        {#if oldestLabel && pendingCount > 0}<p class="detail">{oldestLabel}</p>{/if}
        {#if syncedLabel && pendingCount === 0}<p class="detail">{syncedLabel}</p>{/if}
        {#if !hasSomething}<p class="detail">{emptyLabel}</p>{/if}
        {#if queued.length > 0}
          <ul class="list queued" aria-label={listLabel}>
            {#each queued as change (change.id)}
              <li class="line">
                <span class="what">{change.what}</span> ·
                {#if change.href}<a class="where" href={change.href}>{change.where}</a>{:else}<span class="where">{change.where}</span>{/if}
              </li>
            {/each}
          </ul>
        {/if}
        {#if onRetry && retryLabel && connection !== 'connected'}
          <span class="actions"><Button size="sm" tone="secondary" onclick={onRetry}>{retryLabel}</Button></span>
        {/if}
      {/if}
    </div>
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
    {#if hasRefusal}<span class="dot" aria-hidden="true"></span>{/if}
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

  /* The surface's padding is given back so the band runs edge to edge, as the concept draws it. */
  .panel { min-inline-size: 30ch; margin: calc(var(--sp-200) * -1); }

  /* The one control: a mark, a count where anything waits, and a dot where anything was refused.
     Drawn as `IconButton` draws one - the same square target, the same radius, the same hover -
     because it stands in a row with them and a control that is a pill beside two rounded squares
     is three shapes for one kind of thing. The inline padding is the count's: with
     nothing waiting the control is square, and it grows around a number rather than reserving
     room for one. */
  .trigger {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--sp-050);
    position: relative;
    min-inline-size: var(--density-control-sm-min);
    min-block-size: var(--density-control-sm-min);
    padding-inline: var(--sp-050);
    border: 0;
    border-radius: var(--r-md);
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
     words. Offline is the one state somebody has to see first.

     **Connected is coloured, including the quiet case.** ADR-0063 decision 5's table says the
     quiet state is `status.success.accent`; drawn in `text.subtle`, the one state that means
     *everything is fine* would be the one state with no colour at all. `data-quiet` says only
     whether there is anything waiting behind the mark. It is not a reason to say nothing in colour. */
  .trigger[data-connection='connected'] { background: var(--status-success-surface); color: var(--status-success-text); }
  .trigger[data-connection='reconnecting'] { background: var(--status-warning-surface); color: var(--status-warning-text); }
  /* Offline is a warning, tinted like the others: a filled block in a row of quiet squares read as
     an alarm, and working offline is a state the product is built for, not a failure. */
  .trigger[data-connection='offline'] { background: var(--status-warning-surface); color: var(--status-warning-text); }

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

  /* The band: the state in its tone across the whole width, its mark before its words. */
  .band {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    margin: 0;
    padding: var(--sp-150) var(--sp-200);
    font-size: var(--fs-100);
    font-weight: var(--fw-semibold);
  }

  .band[data-tone='ok'] { background: var(--status-success-surface); color: var(--status-success-text); }
  .band[data-tone='busy'] { background: var(--accent-primary-subtle); color: var(--text-brand); }
  .band[data-tone='warn'] { background: var(--status-warning-surface); color: var(--status-warning-text); }
  .band[data-tone='bad'] { background: var(--status-danger-surface); color: var(--status-danger-text); }

  /* The lines under the band, on the band's text edge. */
  .body {
    display: flex;
    flex-direction: column;
    gap: var(--sp-050);
    padding: var(--sp-150) var(--sp-200) var(--sp-200) calc(var(--sp-200) + var(--fs-100) + var(--sp-100));
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }

  /* A waiting change is one quiet line: what, then where. */
  .queued { gap: var(--sp-025); }
  .line { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .line .what { color: var(--text-secondary); }

  /* The `pending` role: a continuous indicator with no beginning and no end, so its easing is
     linear and it never arrives (design-system.md §6 rule 6). Transform only; reduced motion
     stops it. */
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

  .said { color: var(--text-secondary); }
  .said .where { color: var(--text-primary); font-weight: var(--fw-semibold); }

  .actions { margin-block-start: var(--sp-100); }

  .open {
    display: inline-flex;
    align-items: center;
    padding-inline: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    color: var(--text-primary);
    font-size: var(--fs-075);
    text-decoration: none;
  }

  .open:hover { background: var(--bg-surface-hover); }

  /* Rule 5: the ring on the links the list carries. */
  .open:focus-visible,
  .where:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }
</style>
