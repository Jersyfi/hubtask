<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The shape of what is coming, so the page does not jump when it arrives.
  //
  // That is the whole requirement and it is a layout one rather than a decorative one: a list that
  // renders nothing while it loads and then rows afterwards moves everything below it, and a
  // reader who had started reading loses their place. So a skeleton takes the **height of the row
  // it stands in for**, which is why it is sized from the same density tokens the row is.
  //
  // It is hidden from the accessibility tree. A screen reader is told the list is busy by the
  // container (`aria-busy`), and announcing "loading, loading, loading" once per placeholder row
  // is how a wait becomes worse for the reader who can least afford it.
  //
  // The shimmer is deliberately absent. Rule 6 confines motion to opacity and transform, and an
  // animated gradient is neither - it is a moving `background-position`, which is the one property
  // that cannot be composited. What it has instead is a slow opacity pulse through F2-01's
  // `pending` role, and reduced motion takes even that away.

  interface Props {
    /** How many placeholder rows. A list knows roughly how many it had last time. */
    lines?: number;
    /** `row` stands in for a list row; `text` for a paragraph; `block` for a card. */
    shape?: 'row' | 'text' | 'block';
  }

  const { lines = 3, shape = 'row' }: Props = $props();
</script>

<div class="skeleton" data-shape={shape} aria-hidden="true">
  {#each Array.from({ length: lines }, (_, index) => index) as line (line)}
    <div class="bar"></div>
  {/each}
</div>

<style>
  .skeleton {
    display: flex;
    flex-direction: column;
    gap: var(--density-row-gap);
  }

  .bar {
    border-radius: var(--r-sm);
    background: var(--bg-surface-sunken);
    animation: pulse var(--motion-pending-duration) var(--motion-pending-easing) infinite alternate;
  }

  /* The heights are the density scale's, so a placeholder is exactly as tall as the thing it
     stands in for and nothing shifts when the data lands. */
  .skeleton[data-shape='row'] .bar { block-size: var(--density-control-md-min); }
  .skeleton[data-shape='text'] .bar { block-size: var(--sp-200); }
  .skeleton[data-shape='block'] .bar { block-size: var(--sp-1000); }

  /* Rule 4: the last line is short, the way a paragraph's is. It reads as text rather than as a
     stack of identical bars, without any of them carrying a width literal. */
  .skeleton[data-shape='text'] .bar:last-child { inline-size: 60%; }

  @keyframes pulse {
    from { opacity: 1; }
    to { opacity: 0.55; }
  }

  @media (prefers-reduced-motion: reduce) {
    .bar { animation: none; }
  }

  :global([data-motion='reduced']) .bar { animation: none; }
</style>
