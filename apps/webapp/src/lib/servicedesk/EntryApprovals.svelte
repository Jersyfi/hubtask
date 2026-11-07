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
  import { t } from '../i18n/i18n.svelte.ts';
  import ApprovalCard from './ApprovalCard.svelte';

  const { itemId }: { itemId: string } = $props();

  // Keyed on the entry by its caller, so the path is fixed for its life.
  const read = resource<ApprovalList>(untrack(() => entryApprovalsPath(itemId)));
  const rows = $derived(read.state.status === 'ready' ? read.state.data.data : []);
</script>

{#if rows.length > 0}
  <section aria-labelledby="entry-approvals">
    <Stack gap="100">
      <h2 class="heading" id="entry-approvals">{t('app.approvals.entry_title')}</h2>
      {#each rows as approval (approval.id)}
        <ApprovalCard {approval} />
      {/each}
    </Stack>
  </section>
{/if}

<style>
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
</style>
