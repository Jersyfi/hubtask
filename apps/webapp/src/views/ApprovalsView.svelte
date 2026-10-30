<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Waiting for my yes* (UC-SVC-09 check 1): the approvals whose current level counts the reader,
  // names a role they hold there, or belongs to somebody away whose approvals they decide - oldest
  // first, as the server answers, each decided in place.

  import { untrack } from 'svelte';
  import { EmptyState, ErrorState, PageHeader, Skeleton, Stack } from '@hubtask/design-system/components';
  import type { ApprovalList } from '@hubtask/sync-engine';

  import { myApprovalsPath } from '../lib/data/approvals.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import ApprovalCard from '../lib/servicedesk/ApprovalCard.svelte';

  const { onnavigate }: { onnavigate?: (path: string) => void } = $props();

  const list = untrack(() => resource<ApprovalList>(myApprovalsPath));
  const rows = $derived(list.state.status === 'ready' ? list.state.data.data : undefined);
  const failure = $derived(list.state.status === 'failed' ? renderProblem(list.state.error, messages) : undefined);
  $effect(() => page.entitle(t('app.approvals.mine_title')));
</script>

<Stack gap="300">
  <PageHeader title={t('app.approvals.mine_title')} subtitle={t('app.approvals.mine_subtitle')} isTitleInBar={viewport.isCompact} />
  {#if failure}
    <ErrorState title={failure.message} reference={failure.reference} referenceLabel={t('app.reference')}
      retryLabel={t('app.retry')} onRetry={() => void list.refresh()} />
  {:else if !rows}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else if rows.length === 0}
    <EmptyState kind="unused" icon="user-check" title={t('app.approvals.mine_none')} description={t('app.approvals.mine_none_body')} />
  {:else}
    {#each rows as approval (approval.id)}
      <ApprovalCard {approval} {onnavigate} />
    {/each}
  {/if}
</Stack>
