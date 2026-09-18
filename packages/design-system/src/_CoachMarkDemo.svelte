<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workbench's wrapper: the mark on its own, in the flow rather than anchored, so its words
  // and its controls can be looked at without a spotlight.

  import CoachMark from './CoachMark.svelte';

  const { hasBack = true }: { hasBack?: boolean } = $props();

  let said = $state('');
</script>

<div class="frame">
  <CoachMark
    caption="Where things arrive before they are work"
    body="The jumble takes what you jot down, forward or dictate. Nothing here is a task yet; you decide what becomes one."
    countLabel="Step 4 of 6"
    nextLabel="Next"
    backLabel="Back"
    skipLabel="Skip the tour"
    {hasBack}
    onNext={() => (said = 'next')}
    onBack={() => (said = 'back')}
    onSkip={() => (said = 'skip')}
  />
  {#if said}<p class="said">Pressed: {said}</p>{/if}
</div>

<style>
  /* The mark positions itself for an anchor; here it sits in the flow. */
  .frame { position: relative; min-block-size: var(--motion-celebration-area); }
  .frame :global(.mark) { position: static; }
  .said { color: var(--text-secondary); font-size: var(--fs-075); }
</style>
