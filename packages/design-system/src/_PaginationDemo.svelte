<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Pagination from './Pagination.svelte';
  import Stack from './Stack.svelte';
  import { pageOf } from './table.ts';

  const { mode = 'middle' }: { mode?: 'middle' | 'ends' | 'short' } = $props();

  const SIZE = 12;

  let page = $state(9);

  /** The four positions the width test is about, drawn at once rather than described. */
  const positions = [1, 2, 19, 20];

  const range = (total: number, at: number): string => {
    const shape = pageOf(Array.from({ length: total }), at, SIZE);
    return `${shape.firstRow}–${shape.lastRow} of ${shape.total}`;
  };

  const words = {
    previousLabel: 'Previous page',
    nextLabel: 'Next page',
    pageLabel: (n: number) => `Page ${n}`,
    gapLabel: (from: number, to: number) => `Pages ${from} to ${to}`,
  };
</script>

{#if mode === 'ends'}
  <Stack gap="100">
    {#each positions as at (at)}
      <Pagination
        total={240}
        pageSize={SIZE}
        page={at}
        label={`Pages of entries, at page ${at}`}
        rangeLabel={range(240, at)}
        onPage={() => {}}
        {...words}
      />
    {/each}
  </Stack>
{:else if mode === 'short'}
  <Stack gap="100">
    {#each [48, 84, SIZE] as total (total)}
      <Pagination
        total={total}
        pageSize={SIZE}
        page={1}
        label={`Pages of ${total} entries`}
        rangeLabel={range(total, 1)}
        onPage={() => {}}
        {...words}
      />
    {/each}
  </Stack>
{:else}
  <Pagination
    total={1091}
    pageSize={SIZE}
    {page}
    label="Pages of entries"
    rangeLabel={range(1091, page)}
    onPage={(n) => (page = n)}
    {...words}
  />
{/if}
