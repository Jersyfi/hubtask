<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What waits for the reader's yes, in the portal's own tab (UC-SVC-09 check 5, PO-3): each with
  // the entry's title, approved or declined with a reason here, the team's entry never opened.
  // Being counted at a level is what lets them, not a permission on the entry.

  import { untrack } from 'svelte';
  import { LoadMore, PageHeader, Skeleton, Stack, WorkPage } from '@hubtask/design-system/components';
  import type { ApprovalList } from '@hubtask/sync-engine';

  import { myApprovalsPath } from '../lib/data/approvals.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import FailureState from '../lib/frame/FailureState.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import ApprovalCard from '../lib/servicedesk/ApprovalCard.svelte';

  const asked = untrack(() => resource<ApprovalList>(myApprovalsPath));
  const waiting = $derived(asked.state.status === 'ready' ? asked.state.data.data : []);
  const hasMore = $derived(asked.state.status === 'ready' && asked.state.data.page?.has_more === true);
  const failure = $derived(asked.state.status === 'failed' ? renderProblem(asked.state.error, messages) : undefined);
  $effect(() => page.entitle(t('app.portal.approvals')));
  let isLoadingMore = $state(false);

  async function more() {
    isLoadingMore = true;
    try {
      await asked.loadMore();
    } finally {
      isLoadingMore = false;
    }
  }
</script>

<WorkPage>
  {#snippet top()}
    <PageHeader title={t('app.portal.approvals')} subtitle={t('app.portal.approvals_sub')} isTitleInBar={viewport.isCompact} />
  {/snippet}
  {#if failure}
    <FailureState problem={failure} onRetry={() => void asked.refresh()} />
  {:else if asked.state.status !== 'ready'}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else if waiting.length === 0}
    <ProductEmpty product="SERVICE_DESK" kind="unused" title={t('app.portal.approvals_none')} />
  {:else}
    <Stack gap="150">
      <div data-portal-approvals>
        <Stack gap="150">
          {#each waiting as approval (approval.id)}
            <ApprovalCard {approval} isPortal />
          {/each}
        </Stack>
      </div>
      <LoadMore label={t('app.portal.more_approvals')} busyLabel={t('app.portal.loading_more')} {hasMore} isBusy={isLoadingMore}
        endLabel={t('app.portal.approvals_end')} onLoadMore={() => void more()} />
    </Stack>
  {/if}
</WorkPage>
