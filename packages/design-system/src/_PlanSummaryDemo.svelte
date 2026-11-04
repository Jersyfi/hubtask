<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import PlanSummary from './PlanSummary.svelte';

  const { mode = 'import' }: { mode?: 'import' | 'again' | 'empty' } = $props();
</script>

{#if mode === 'again'}
  <PlanSummary
    label="What the import would do"
    emptyLabel="The file holds nothing to import."
    lines={[{ text: 'Everything in the file is already here: 251 things are left as they are.', tone: 'keep' }]}
  />
{:else if mode === 'empty'}
  <PlanSummary label="What the import would do" emptyLabel="The file holds nothing to import." lines={[]} />
{:else}
  <PlanSummary
    label="What the import would do"
    emptyLabel="The file holds nothing to import."
    lines={[
      { text: 'Creates 1 collection, 6 columns, 9 labels and 248 entries.', tone: 'add' },
      { text: '2 rows would not land: row 17 has no title, row 88 a date that is not understood.', tone: 'refuse' },
    ]}
    reach="Under the hub Marketing. Nothing is written until you import."
  />
{/if}
