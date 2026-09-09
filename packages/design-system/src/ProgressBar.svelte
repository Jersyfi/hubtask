<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // How far something has got, or that it has not finished.
  //
  // **The element is `<progress>`, and that is what makes the determinate case possible at all.**
  // A bar drawn as a filled div needs a width, a width is arithmetic, and arithmetic in an
  // attribute is an inline style — which ADR-0028's `style-src 'self'` refuses, in production only
  // (design-system.md §4, wave 0). The platform's own element takes a number and draws the
  // proportion itself, so there is nothing here to compute and nothing to inline.
  //
  // **Indeterminate is a first-class case, not a missing number.** A job answers `progress: null`
  // while it is queued or while the server cannot say how much work there is, and a bar sitting at
  // zero would be a claim that nothing has happened. Leaving `value` off puts the element in its
  // indeterminate state, and the track pulses instead of filling.
  //
  // **The number is not the message.** A screen reader reading "37" learns nothing; `valueLabel`
  // is the sentence the caller writes — "37 % uploaded", "3 of 8 done" — and it is what is
  // announced. Polite, because whoever started this is not waiting on it the way they wait on a
  // failure, and an assertive region would interrupt them at every percent.

  import type { ControlSize } from './control.ts';

  interface Props {
    /**
     * The accessible name of what is progressing. Resolved text, never a sentence written here
     * (ADR-0011).
     */
    label: string;
    /**
     * How far, from 0 to 100. Absent is **indeterminate** — something is happening and how far is
     * not knowable — rather than zero.
     */
    value?: number;
    /**
     * The progress as a sentence, for a reader who is not watching a bar. Shown beside it and
     * announced politely as it changes.
     */
    valueLabel?: string;
    size?: ControlSize;
  }

  const { label, value, valueLabel, size = 'md' }: Props = $props();
</script>

<div class="progress" data-size={size}>
  <!-- `max` is 100 because the caller's number is a percentage; the element does the rest. With
       `value` absent the browser draws its indeterminate state, which is why there is no branch
       here for the two cases. -->
  <progress class="bar" max="100" {value} aria-label={label} data-state={value === undefined ? 'indeterminate' : 'determinate'}></progress>
  {#if valueLabel}
    <span class="value" aria-live="polite">{valueLabel}</span>
  {/if}
</div>

<style>
  .progress { display: flex; align-items: center; gap: var(--sp-100); min-width: 0; }

  /* `appearance: none` in all three places the platform draws one, because a bar styled in one
     engine and native in another is two components. */
  .bar {
    appearance: none;
    flex: 1 1 auto;
    min-width: 0;
    border: none;
    border-radius: var(--r-full);
    background: var(--bg-surface-sunken);
    block-size: var(--sp-100);
  }

  /* Two sizes, because `ControlSize` is two: how prominent this bar is beside what it belongs to. */
  .progress[data-size='sm'] .bar { block-size: var(--sp-050); }

  .bar::-webkit-progress-bar { background: var(--bg-surface-sunken); border-radius: var(--r-full); }
  .bar::-webkit-progress-value { background: var(--accent-primary); border-radius: var(--r-full); }
  .bar::-moz-progress-bar { background: var(--accent-primary); border-radius: var(--r-full); }

  /* Indeterminate: the track breathes rather than filling. Opacity only — rule 6 — so nothing
     beside it moves, and a reader who cannot see the animation still has `valueLabel`. */
  .bar[data-state='indeterminate'] {
    background: var(--accent-primary-subtle);
    animation: breathe var(--motion-pending-duration) var(--motion-pending-easing) infinite alternate;
  }

  .value { color: var(--text-secondary); font-size: var(--fs-075); }

  @keyframes breathe {
    from { opacity: 0.45; }
    to { opacity: 1; }
  }

  @media (prefers-reduced-motion: reduce) {
    .bar[data-state='indeterminate'] { animation: none; opacity: 1; }
  }

  :global([data-motion='reduced']) .bar[data-state='indeterminate'] { animation: none; opacity: 1; }
</style>
