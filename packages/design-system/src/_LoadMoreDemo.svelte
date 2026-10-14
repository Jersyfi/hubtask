<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import ListRow from './ListRow.svelte';
  import LoadMore from './LoadMore.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'more' }: { mode?: 'more' | 'end' | 'busy' | 'failed' } = $props();

  let rows = $state(['Move the socket', 'Order the tiles', 'Book the electrician']);
  let arrived = $state('');
  let isBusy = $state(false);
  let failed = $state('');

  function loadMore() {
    failed = '';
    isBusy = true;
    // A page appended, never replacing what is there — which is what the engine does, and the
    // reason this is a button rather than a scroll.
    setTimeout(() => {
      // The workbench's stand-in for a page that did not come back: the rows stay, the reason is
      // said, and the control is the retry.
      if (mode === 'failed') {
        failed = 'The next page did not arrive. Check the connection and try again.';
        isBusy = false;
        return;
      }
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
    failedLabel={failed}
    onLoadMore={loadMore}
  />
</Stack>
