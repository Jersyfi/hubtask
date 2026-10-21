<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A control that is there, is off, and carries the reason.
  //
  // `domain-model.md` §2 states the rule this component exists for: "setting a field whose
  // capability is not active for the type produces `ErrCapabilityNotSupported` — **not** silent
  // ignoring." A client has three ways to honour that and only one of them is honest. Offering the
  // control anyway builds something the server refuses. **Hiding** it tells the reader nothing.
  // This is the third: it stays, it is off, and it says why. (A control the reader's *role* forbids
  // is a different case and is absent, P-05; that is the caller's to decide.)
  //
  // It knows nothing about a manifest and nothing about a role. It takes a verdict, because which
  // capabilities a type carries is the installation's answer and reading it belongs to the
  // application (`apps/webapp/src/lib/data/capability.ts`) — a design-system component that read
  // `/meta/capabilities` would be a component with a server in it.
  //
  // **Nothing moves** (design-system.md §4). While the installation has not answered, the content
  // keeps its own boxes - dimmed, inert, and with no line of its own - so that an answer of
  // "permitted" changes no position on the page. An area that is refused says why *first*, in one
  // line above its content, which keeps its size. A control in a line (`isInline`) never becomes
  // a column: it keeps its box, and a lock mark over it carries the reason as a bubble.
  //
  // **No sentence of its own.** A refused gate without a reason shows the mark and nothing else:
  // a fallback in English would be a sentence in a component, in one language.

  import type { Snippet } from 'svelte';

  import Reason from './_Reason.svelte';
  import Icon from './Icon.svelte';
  import VisuallyHidden from './VisuallyHidden.svelte';

  /** Whether the thing behind the gate may be used. */
  export type GateStatus = 'permitted' | 'refused' | 'undetermined';

  interface Props {
    status: GateStatus;
    /** Why it cannot be used. Resolved text (ADR-0011): the application renders the code. */
    reason?: string;
    /** What a reader is told while the installation has not answered yet. */
    pendingLabel?: string;
    /**
     * Whether the gated thing is a control in a line - a button in a row - rather than an area. An
     * area says its reason first, above itself; a control in a line keeps its box and says it as a
     * bubble from a lock mark laid over it.
     */
    isInline?: boolean;
    children: Snippet;
  }

  const { status, reason, pendingLabel, isInline = false, children }: Props = $props();

  // Svelte's own id, not a random one: unique per instance and the same on every render, so a
  // prerendered page is identical between builds and a hydrated one matches its markup.
  const instanceId = $props.id();
  const id = `gate-${instanceId}-reason`;
  /** What the inline lock says: the reason, or while undetermined, the pending label. */
  const said = $derived(status === 'undetermined' ? pendingLabel : reason);
  let lock = $state<HTMLElement | null>(null);
</script>

{#if status === 'permitted'}
  {@render children()}
{:else if isInline}
  <span class="inline" data-status={status}>
    <!-- `inert`: out of the tab order and the accessibility tree in one attribute, so a reader
         never reaches a control that does nothing. The lock over it is the one stop. -->
    <span class="behind" inert>{@render children()}</span>
    <button
      type="button"
      class="lock"
      aria-disabled="true"
      aria-describedby={said ? id : undefined}
      bind:this={lock}
    >
      <Icon name={status === 'undetermined' ? 'loader-circle' : 'lock'} size="sm" />
      {#if said}<VisuallyHidden>{said}</VisuallyHidden>{/if}
    </button>
    {#if said}<Reason {id} text={said} control={lock} />{/if}
  </span>
{:else if status === 'undetermined'}
  <!-- No box of its own: the content keeps exactly the boxes it will have once permitted. -->
  <div class="deciding" data-status={status}>
    {#if pendingLabel}<VisuallyHidden>{pendingLabel}</VisuallyHidden>{/if}
    <div class="behind" inert>{@render children()}</div>
  </div>
{:else}
  <div class="gate" data-status={status}>
    <p class="reason" {id}>
      <span class="mark" aria-hidden="true"><Icon name="ban" size="sm" /></span>
      {#if reason}{reason}{/if}
    </p>
    <div class="behind" inert aria-describedby={reason ? id : undefined}>{@render children()}</div>
  </div>
{/if}

<style>
  .gate { display: flex; flex-direction: column; gap: var(--sp-050); min-width: 0; }

  /* Dimmed, but never to the point where the control cannot be read: it is still information —
     what *would* be here — and rule 3 means the dimming is not the only signal. */
  .gate .behind { opacity: 0.5; }

  /* While deciding, the wrappers draw no box: the content is laid out as if they were not there,
     and its own boxes are dimmed instead. */
  .deciding,
  .deciding .behind { display: contents; }
  .deciding .behind > :global(*) { opacity: 0.5; }

  .reason {
    display: flex;
    align-items: start;
    gap: var(--sp-050);
    margin: 0;
    max-width: 48ch;
    /* The line takes the content's width and adds none of its own: the content keeps its size when
       the answer is a refusal, wherever the area stands. */
    contain: inline-size;
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  .mark { display: inline-flex; flex: none; }

  /* A control in a line keeps its own box; the lock lies over it, in the same cell. */
  .inline { display: inline-grid; }
  .inline > .behind,
  .inline > .lock { grid-area: 1 / 1; }
  .inline > .behind { opacity: 0.5; }

  .lock {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    margin: 0;
    padding: 0;
    border: 0;
    background: transparent;
    color: var(--text-secondary);
    cursor: not-allowed;
  }

  .lock:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  /* The one that is waiting rather than refusing turns, so the two are not the same still picture.
     `pending` is the role for an indicator with no beginning and no end (design-system.md §6
     rule 6). */
  .inline[data-status='undetermined'] .lock :global(svg) {
    animation: turn var(--motion-pending-duration) var(--motion-pending-easing) infinite;
  }

  @keyframes turn {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }

  @media (prefers-reduced-motion: reduce) {
    .inline[data-status='undetermined'] .lock :global(svg) { animation: none; }
  }

  :global([data-motion='reduced']) .inline[data-status='undetermined'] .lock :global(svg) { animation: none; }
</style>
