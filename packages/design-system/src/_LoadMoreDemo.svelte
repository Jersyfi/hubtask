<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import ListRow from './ListRow.svelte';
  import LoadMore from './LoadMore.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'more' }: { mode?: 'more' | 'end' | 'busy' } = $props();

  let rows = $state(['Move the socket', 'Order the tiles', 'Book the electrician']);
  let arrived = $state('');
  let isBusy = $state(false);

  function loadMore() {
    isBusy = true;
    // A page appended, never replacing what is there — which is what F2-03 taught the engine and
    // the reason this is a button rather than a scroll.
    setTimeout(() => {
      rows = [...rows, 'Seal the joints', 'Fit the splashback'];
      arrived = '2 more entries';
      isBusy = false;
    }, 600);
  }
</script>

<Stack gap="050">
  {#each rows as row (row)}
    <ListRow href="/items/x">{row}</ListRow>
  {/each}
  <LoadMore
    label="Show more"
    busyLabel="Loading"
    hasMore={mode !== 'end'}
    isBusy={mode === 'busy' || isBusy}
    arrivedLabel={arrived}
    endLabel="That is all of them."
    onLoadMore={loadMore}
  />
</Stack>
