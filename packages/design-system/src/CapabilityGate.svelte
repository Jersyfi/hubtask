<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A control that is there, is off, and carries the reason.
  //
  // `domain-model.md` §2 states the rule this component exists for: "setting a field whose
  // capability is not active for the type produces `ErrCapabilityNotSupported` — **not** silent
  // ignoring." A client has three ways to honour that and only one of them is honest. Offering the
  // control anyway builds something the server refuses. **Hiding** it tells the reader nothing —
  // and is the tempting one, because a hidden control is a screen that looks clean. This is the
  // third: it stays, it is off, and it says why.
  //
  // It knows nothing about a manifest and nothing about a role. It takes a verdict, because which
  // capabilities a type carries is the installation's answer and reading it belongs to the
  // application (`apps/webapp/src/lib/data/capability.ts`) — a design-system component that read
  // `/meta/capabilities` would be a component with a server in it.
  //
  // The three states are the point of the API. `undetermined` is the one a screen gets wrong:
  // until the manifest is read nothing is known, and a control shown as available in the meantime
  // is one that disappears a moment later.

  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';

  /** Whether the thing behind the gate may be used. */
  export type GateStatus = 'permitted' | 'refused' | 'undetermined';

  interface Props {
    status: GateStatus;
    /**
     * Why it cannot be used. Resolved text (ADR-0011) — the application renders the code the
     * manifest implied, and this component never writes a sentence.
     *
     * Required in spirit rather than in the type: a `refused` gate with no reason renders a
     * fallback that says so, because a control that is off for no stated reason is exactly the
     * silent ignoring the rule forbids, and failing loudly is better than failing invisibly.
     */
    reason?: string;
    /** What a reader is told while the installation has not answered yet. */
    pendingLabel?: string;
    children: Snippet;
  }

  const { status, reason, pendingLabel, children }: Props = $props();

  const id = `gate-${Math.random().toString(36).slice(2, 9)}`;
</script>

{#if status === 'permitted'}
  {@render children()}
{:else}
  <!-- `inert` rather than removing the children: the control stays where it was, at the size it
       was, so nothing around it moves — and `inert` takes the whole subtree out of the tab order
       and the accessibility tree in one attribute, which is what stops a reader tabbing into a
       control that does nothing. -->
  <div class="gate" data-status={status} aria-describedby={id}>
    <div class="behind" inert>{@render children()}</div>
    <p class="reason" {id}>
      <span class="mark" aria-hidden="true">
        <Icon name={status === 'undetermined' ? 'loader-circle' : 'ban'} size="sm" />
      </span>
      <!-- The fallback is deliberately blunt. A gate that closed a control and said nothing would
           be the defect this component was written to prevent, so it is visible rather than quiet. -->
      {status === 'undetermined' ? (pendingLabel ?? '…') : (reason ?? 'No reason was given.')}
    </p>
  </div>
{/if}

<style>
  .gate { display: flex; flex-direction: column; gap: var(--sp-050); min-width: 0; }

  /* Dimmed, but never to the point where the control cannot be read: it is still information —
     what *would* be here — and rule 3 means the dimming is not the only signal. The mark and the
     sentence carry it. */
  .behind { opacity: 0.5; }

  .reason {
    display: flex;
    align-items: start;
    gap: var(--sp-050);
    margin: 0;
    max-width: 48ch;
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  .mark { display: inline-flex; flex: none; }

  /* The one that is waiting rather than refusing turns, so the two are not the same still picture.
     `pending` is the role for an indicator with no beginning and no end (F2-01). */
  .gate[data-status='undetermined'] .mark {
    animation: turn var(--motion-pending-duration) var(--motion-pending-easing) infinite;
  }

  @keyframes turn {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }

  @media (prefers-reduced-motion: reduce) {
    .gate[data-status='undetermined'] .mark { animation: none; }
  }

  :global([data-motion='reduced']) .gate[data-status='undetermined'] .mark { animation: none; }
</style>
