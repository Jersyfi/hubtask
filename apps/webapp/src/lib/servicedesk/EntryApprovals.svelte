<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An entry's approvals and their trail (UC-SVC-09 check 4), newest first as the server answers.
  // Absent while the entry has none: a heading over nothing would be a section about nothing.

  import { untrack } from 'svelte';
  import { Stack } from '@hubtask/design-system/components';
  import type { ApprovalList } from '@hubtask/sync-engine';

  import { entryApprovalsPath } from '../data/approvals.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import ApprovalCard from './ApprovalCard.svelte';

  const { itemId }: { itemId: string } = $props();

  // Keyed on the entry by its caller, so the path is fixed for its life.
  const read = resource<ApprovalList>(untrack(() => entryApprovalsPath(itemId)));
  const rows = $derived(read.state.status === 'ready' ? read.state.data.data : []);
</script>

{#if rows.length > 0}
  <div data-entry-approvals>
    <Stack gap="100">
      {#each rows as approval (approval.id)}
        <ApprovalCard {approval} />
      {/each}
    </Stack>
  </div>
{/if}

<style>
</style>
