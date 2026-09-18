<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The celebration slot (design-system.md §7, F6-13): one component, three tiers, an asset per
  // tier that fills the slot within the tokens' guardrails - `--motion-celebration-*-duration`
  // for how long, `--motion-celebration-area` for the most it may claim of the screen,
  // `--motion-celebration-travel` for the furthest anything in it moves.
  //
  // **Never blocking.** The slot is `inert` and hidden from the accessibility tree: nothing in it
  // takes focus, nothing in it takes a click, and the next completion is not delayed by it - the
  // caller unmounts it when `onDone` fires and may mount the next before that. What a reader who
  // is not watching gets is the one sentence the caller resolved, announced once through a
  // `status` region beside the slot, in the voice voice-and-tone.md §7 asks for: the caller
  // writes it, and it does no work with an exclamation mark.
  //
  // **Under reduced motion every tier is rule 6's colour change**: the slot tints for the tier's
  // duration and nothing moves. Switching off motion never switches off the acknowledgement.
  //
  // Which animation carries which tier is an asset, not a decision of §7's: tier 1 is a ring that
  // settles on the row, tier 2 a sweep of the brand colour across the parent row, tier 3 a handful
  // of marks in the two brand colours rising and fading within the slot. All three animate opacity
  // and transform only.

  import VisuallyHidden from './VisuallyHidden.svelte';

  interface Props {
    /** Which moment (§7's table). */
    tier: 1 | 2 | 3;
    /** The one sentence a screen reader hears, once. Resolved text (ADR-0011). */
    announcement: string;
    /** Called when the tier's duration has passed and the slot may be unmounted. */
    onDone?: () => void;
  }

  const { tier, announcement, onDone }: Props = $props();

  // The end is read from the element rather than timed here: the duration is the token's, and a
  // timer written beside it would be a second copy of the value. Under reduced motion the tint
  // runs the same duration, so the slot ends on the same event.
  function ended() {
    onDone?.();
  }

  // Six marks for tier 3, placed by hand rather than drawn: nothing here is random (§7), and a
  // person who completes two collections sees the same moment twice.
  const marks = [0, 1, 2, 3, 4, 5];
</script>

<div class="celebration" data-tier={tier} aria-hidden="true" inert>
  {#if tier === 1}
    <span class="ring" onanimationend={ended}></span>
  {:else if tier === 2}
    <span class="sweep" onanimationend={ended}></span>
  {:else}
    <span class="marks">
      {#each marks as mark (mark)}
        <span class="mark" style:--mark={mark} onanimationend={mark === 0 ? ended : undefined}></span>
      {/each}
    </span>
  {/if}
</div>
<!-- Said once, beside the slot: the acknowledgement is not part of what is hidden. -->
<VisuallyHidden role="status">{announcement}</VisuallyHidden>

<style>
  .celebration {
    position: absolute;
    inset: 0;
    pointer-events: none;
    overflow: hidden;
  }

  /* §7's guardrail: the box is the most any tier may claim. */
  .celebration[data-tier='3'] {
    inset: auto 0 0 0;
    block-size: min(100%, var(--motion-celebration-area));
  }

  /* Tier 1: a ring that settles on the row - the micro-animation every completion gets. */
  .ring {
    position: absolute;
    inset-block: 0;
    inset-inline-start: 0;
    inline-size: var(--motion-celebration-travel);
    border-radius: var(--r-md);
    background-color: var(--accent-primary-subtle);
    opacity: 0;
    animation: settle var(--motion-celebration-duration) var(--motion-celebration-easing) both;
  }

  @keyframes settle {
    0% { opacity: 0; transform: scale(0.6); }
    40% { opacity: 1; transform: scale(1); }
    100% { opacity: 0; transform: scale(1.1); }
  }

  /* Tier 2: a sweep of the brand colour across the parent row, once. */
  .sweep {
    position: absolute;
    inset-block: 0;
    inset-inline-start: 0;
    inline-size: 100%;
    background-color: var(--accent-signature-subtle);
    opacity: 0;
    transform-origin: 0 50%;
    animation: sweep var(--motion-celebration-tier2-duration) var(--motion-celebration-tier2-easing) both;
  }

  @keyframes sweep {
    0% { opacity: 0; transform: scaleX(0); }
    30% { opacity: 1; }
    70% { opacity: 1; transform: scaleX(1); }
    100% { opacity: 0; transform: scaleX(1); }
  }

  /* Tier 3: marks in the brand colours, rising no further than `travel` and fading. */
  .marks {
    position: absolute;
    inset: 0;
    display: grid;
    grid-template-columns: repeat(6, 1fr);
    align-items: end;
    justify-items: center;
  }

  .mark {
    inline-size: var(--sp-150);
    block-size: var(--sp-150);
    border-radius: var(--r-full);
    background-color: var(--accent-primary);
    opacity: 0;
    /* The odd marks in the signature colour, the even in the primary: both brand colours, never one alone. */
    animation: rise var(--motion-celebration-tier3-duration) var(--motion-celebration-tier3-easing) both;
    animation-delay: calc(var(--mark) * var(--dur-instant));
  }

  .mark:nth-child(odd) { background-color: var(--accent-signature); }

  @keyframes rise {
    0% { opacity: 0; transform: translateY(0) scale(0.6); }
    20% { opacity: 1; transform: translateY(calc(var(--motion-celebration-travel) * -0.4)) scale(1); }
    65% { opacity: 1; transform: translateY(calc(var(--motion-celebration-travel) * -0.8)) scale(1); }
    100% { opacity: 0; transform: translateY(calc(var(--motion-celebration-travel) * -1)) scale(0.8); }
  }

  /* Rule 6: under a reduced preference nothing moves; the slot tints for the tier's duration -
     the colour change that is the floor - and ends on the same event. */
  @media (prefers-reduced-motion: reduce) {
    .ring, .sweep, .mark { animation-name: tint; animation-delay: unset; transform: none; }
  }

  :global([data-motion='reduced']) .ring,
  :global([data-motion='reduced']) .sweep,
  :global([data-motion='reduced']) .mark {
    animation-name: tint;
    animation-delay: unset;
    transform: none;
  }

  @keyframes tint {
    0% { opacity: 0; }
    30% { opacity: 1; }
    100% { opacity: 0; }
  }

</style>
