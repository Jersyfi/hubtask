<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The specimen. Not a component: a fixture that exercises every axis, so that the workbench can
  // be shown to work before there is anything to look at (ADR-0029 keeps src/ empty until wave 1
  // builds it deliberately, and a placeholder in there would be a component pretending to exist).
  //
  // It is also the worked example of what wave 1 must do. Read it as one: `start`/`end` and never
  // `left`/`right`, every value a token, states as CSS states rather than variants, a focus ring
  // that survives both themes, motion confined to `opacity` and `transform`, and a disabled
  // control that carries its reason - the CapabilityGate principle applied before that component
  // exists (`ErrCapabilityNotSupported` must never become silent ignoring).

  interface Props {
    /** What the specimen calls itself. The pseudo-locale grows this by 40 %. */
    heading?: string;
    /** Long enough to wrap, so rule 4 has something to break. */
    body?: string;
    /** The reason a disabled control is disabled. A control without one is the defect. */
    unavailableReason?: string;
  }

  const {
    heading = 'A specimen, not a component',
    body =
      'Everything here is a token, aligned to start and end rather than left and right, and ' +
      'operable from the keyboard. Switch a theme, a direction, the text length, motion or the ' +
      'zoom above and what changes is what the rules are about.',
    unavailableReason = 'Not available in this workspace — the capability is off.',
  }: Props = $props();

  let count = $state(0);
</script>

<article class="specimen">
  <h3>{heading}</h3>
  <p>{body}</p>

  <div class="row">
    <button type="button" class="action" onclick={() => (count += 1)}>
      Count up<span class="count" data-workbench-verbatim>{count}</span>
    </button>

    <button type="button" class="action subtle">Secondary</button>

    <span class="gated">
      <button type="button" class="action" disabled aria-describedby="specimen-reason">
        Unavailable
      </button>
      <span id="specimen-reason" class="reason">{unavailableReason}</span>
    </span>
  </div>

  <label class="field">
    <span class="label">A field, so the direction axis has something to flip</span>
    <input type="text" placeholder="Type here" />
  </label>

  <p class="meta" data-workbench-verbatim>
    Data style, tabular figures: <code>2026-09-02T09:41:12Z</code>
  </p>
</article>

<style>
  .specimen {
    max-width: 60ch;
    padding: var(--sp-300);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }

  h3 {
    margin: 0 0 var(--sp-100);
    /* A German compound is one unbreakable word. Rule 4 is not survived by hoping the string is
       short - it is survived by letting it break. This is the lesson the specimen exists to
       teach; switch Text to +40 % with this line removed to see what it costs. */
    overflow-wrap: anywhere;
    font-size: var(--fs-200);
    font-weight: var(--fw-semibold);
    color: var(--text-primary);
  }

  p {
    margin: 0 0 var(--sp-200);
    color: var(--text-secondary);
    overflow-wrap: anywhere;
  }

  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: start;
    gap: var(--sp-150);
    margin-bottom: var(--sp-250);
  }

  /* States are CSS states, never variants (design-system.md §5). A variant matrix that contains
     states explodes, and this is where that starts. */
  .action {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-200);
    border: 1px solid transparent;
    border-radius: var(--r-md);
    background: var(--accent-primary);
    color: var(--text-inverse);
    font: inherit;
    font-weight: var(--fw-medium);
    cursor: pointer;
    /* Rule 6: opacity and transform only. Layout is never animated. */
    transition:
      transform var(--dur-fast) var(--ease-standard),
      opacity var(--dur-fast) var(--ease-standard);
  }

  .action:hover:not(:disabled) {
    background: var(--accent-primary-hover);
  }

  .action:active:not(:disabled) {
    background: var(--accent-primary-pressed);
    transform: scale(0.98);
  }

  .action:disabled {
    background: var(--bg-surface-sunken);
    border-color: var(--border-subtle);
    color: var(--text-subtle);
    cursor: not-allowed;
  }

  .action.subtle {
    background: var(--bg-surface-sunken);
    border-color: var(--border-default);
    color: var(--text-primary);
  }

  .action.subtle:hover {
    background: var(--bg-surface-hover);
  }

  .count {
    padding: 0 var(--sp-050);
    border-radius: var(--r-full);
    background: var(--accent-primary-pressed);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    font-variant-numeric: tabular-nums;
  }

  .gated {
    display: inline-flex;
    flex-direction: column;
    gap: var(--sp-050);
  }

  .reason {
    max-width: 32ch;
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  .field {
    display: block;
    margin-bottom: var(--sp-250);
  }

  .label {
    display: block;
    margin-bottom: var(--sp-050);
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }

  input {
    /* `start`, never `left`: the direction axis exists to catch the other spelling. */
    display: block;
    width: 100%;
    padding: var(--sp-100) var(--sp-150);
    border: 1px solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-canvas);
    color: var(--text-primary);
    font: inherit;
    text-align: start;
  }

  input::placeholder {
    color: var(--text-subtle);
  }

  .meta {
    margin: 0;
    color: var(--text-subtle);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    font-variant-numeric: tabular-nums;
  }

  code {
    font-family: var(--font-mono);
  }

  /* The attribute and the media query together (ADR-0037). The workbench's motion axis sets the
     attribute and enforces nothing; §7 needs the same switch for a user preference, so honouring
     both is the convention every component follows, not a workbench affordance. */
  @media (prefers-reduced-motion: reduce) {
    .action {
      transition-duration: var(--dur-instant);
    }
    .action:active:not(:disabled) {
      transform: none;
    }
  }

  :global([data-motion='reduced']) .action {
    transition-duration: var(--dur-instant);
  }

  :global([data-motion='reduced']) .action:active:not(:disabled) {
    transform: none;
  }
</style>
