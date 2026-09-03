<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The four states one list can be in, side by side, because the point of the group is that they
  // are alternatives rather than components that happen to look similar.
  import Button from './Button.svelte';
  import EmptyState from './EmptyState.svelte';
  import ErrorState from './ErrorState.svelte';
  import ListRow from './ListRow.svelte';
  import Skeleton from './Skeleton.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'loading' }: {
    mode?: 'loading' | 'unused' | 'filtered' | 'settled' | 'failed' | 'rows';
  } = $props();

  const entries = [
    { id: 'a', title: 'Move the socket by the window' },
    { id: 'b', title: 'Order the tiles' },
    { id: 'c', title: 'Book the electrician' },
  ];
</script>

<Stack gap="200">
  {#if mode === 'loading'}
    <!-- `aria-busy` on the container is what a screen reader is told. The placeholders themselves
         are hidden: three announcements of "loading" is how a wait gets worse. -->
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else if mode === 'unused'}
    <EmptyState kind="unused" title="No tasks in this collection yet.">
      {#snippet action()}<Button>Create task</Button>{/snippet}
    </EmptyState>
  {:else if mode === 'filtered'}
    <EmptyState kind="filtered" title="No task matches these filters.">
      {#snippet action()}<Button tone="secondary">Clear filters</Button>{/snippet}
    </EmptyState>
  {:else if mode === 'settled'}
    <!-- The action is passed and deliberately not rendered: §4.3 offers nothing, and the component
         drops it rather than trusting every call site to remember. -->
    <EmptyState kind="settled" title="Nothing waiting.">
      {#snippet action()}<Button>This must not appear</Button>{/snippet}
    </EmptyState>
  {:else if mode === 'failed'}
    <ErrorState
      title="These entries could not be read."
      description="The workspace is reachable again. Try once more."
      retryLabel="Try again"
      referenceLabel="Reference"
      reference="01a06962-4ac3-7d3e-b60e-d60b38507d24"
    />
  {:else}
    <Stack gap="050">
      {#each entries as entry (entry.id)}
        <ListRow href={`/items/${entry.id}`} isSelected={entry.id === 'b'}>{entry.title}</ListRow>
      {/each}
    </Stack>
  {/if}
</Stack>
