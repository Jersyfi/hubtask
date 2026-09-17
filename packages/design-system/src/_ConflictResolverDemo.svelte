<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workbench's wrapper. "Write mine again" is the caller's PATCH: the demo performs none
  // and records what was chosen, which is what the story shows.

  import Button from './Button.svelte';
  import ConflictResolver from './ConflictResolver.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'open' }: { mode?: 'open' | 'archived' } = $props();

  let isOpen = $state(false);
  let outcome = $state<string | undefined>(undefined);
  let notes = $state('Venue confirmed for the 12th. Catering still open — ask Priya about the vegan option.');

  const mine = 'Venue confirmed for the 12th. Catering booked with Greenleaf; vegan option included.';

  function keep() {
    isOpen = false;
    outcome = 'Kept the server’s version.';
  }

  function rewrite() {
    isOpen = false;
    notes = mine;
    outcome = 'Wrote mine again: PATCH /items/{id} { notes } from the current version.';
  }
</script>

<Stack gap="200">
  <Button tone="primary" onclick={() => (isOpen = true)}>Show both versions</Button>
  <p>Notes now: {notes}</p>
  {#if outcome}<p role="status">{outcome}</p>{/if}
  <ConflictResolver
    bind:isOpen
    title="Two versions of the notes"
    fieldLabel="Notes on ‘Send the agenda’ — somebody else changed them while you were away. Theirs is in place; yours was kept as a comment."
    theirs={notes}
    theirsLabel="Theirs, in place"
    {mine}
    mineLabel="Yours, from this device"
    preservedHref="#comment-preserved"
    preservedLabel="Your version, as a comment on the entry"
    keepLabel="Keep theirs"
    rewriteLabel="Write mine again"
    rewriteDisabledReason={mode === 'archived' ? 'This entry is archived and read-only.' : undefined}
    dismissLabel="Close"
    onKeep={keep}
    onRewrite={rewrite}
  />
</Stack>
