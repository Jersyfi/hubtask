<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workbench's wrapper: a row the celebration sits over, a control that plays the tier
  // again, and the sentence resolved here because the component takes resolved text.

  import Button from './Button.svelte';
  import Celebration from './Celebration.svelte';
  import Stack from './Stack.svelte';
  import TaskRow from './TaskRow.svelte';

  const { tier = 1 }: { tier?: 1 | 2 | 3 } = $props();

  const SENTENCES: Record<1 | 2 | 3, string> = {
    1: 'Done.',
    2: 'That was the last one under Print the badges: the work package is complete.',
    3: 'Everything in Errands is done.',
  };

  let playing = $state(true);
  let key = $state(0);

  function again() {
    key += 1;
    playing = true;
  }
</script>

<Stack gap="200">
  <div class="slot">
    <TaskRow type="TASK" title="Book the venue" completeLabel="Complete" isCompleted />
    {#if playing}
      {#key key}
        <Celebration {tier} announcement={SENTENCES[tier]} onDone={() => (playing = false)} />
      {/key}
    {/if}
  </div>
  <div>
    <Button size="sm" tone="secondary" onclick={again}>Play it again</Button>
  </div>
</Stack>

<style>
  /* The slot is whatever the caller makes it: here a row with room beneath for tier 3. */
  .slot { position: relative; min-block-size: var(--motion-celebration-area); }
</style>
