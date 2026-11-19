<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The reader's requests at one portal, every one (UC-SVC-05 check 3): in the desk's own words
  // with what comes next (P-12), loaded at the foot. Where the reader's organisation shares its
  // requests, its colleagues' are one choice away (UC-SVC-11 check 5), asked for only once chosen.

  import { untrack } from 'svelte';
  import { LoadMore, PageHeader, Radio, Skeleton, Stack, WorkPage } from '@hubtask/design-system/components';
  import type { PortalRequestPage, RequestCatalogue } from '@hubtask/sync-engine';

  import { cataloguePath, myRequestsPath, organisationRequestsPath } from '../lib/data/portal.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import FailureState from '../lib/frame/FailureState.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import RequestRows from '../lib/portal/RequestRows.svelte';
  import { renderProblem } from '../lib/problem.ts';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import { offersOrganisation } from '../lib/servicedesk/organisations.ts';

  const { collectionId, onnavigate }: { collectionId: string; onnavigate?: (path: string) => void } = $props();

  const catalogue = untrack(() => resource<RequestCatalogue>(cataloguePath(collectionId)));
  const mine = untrack(() => resource<PortalRequestPage>(myRequestsPath(collectionId)));
  const deskName = $derived(catalogue.state.status === 'ready' ? catalogue.state.data.desk?.name : undefined);
  $effect(() => page.entitle(t('app.portal.mine')));

  const organisation = $derived(mine.state.status === 'ready' ? mine.state.data.organisation : undefined);
  const offered = $derived(offersOrganisation(organisation));
  let scope = $state<'MINE' | 'ORGANISATION'>('MINE');
  let theirs = $state<ReturnType<typeof resource<PortalRequestPage>> | undefined>(undefined);
  $effect(() => {
    if (scope === 'ORGANISATION' && !theirs) theirs = untrack(() => resource<PortalRequestPage>(organisationRequestsPath(collectionId)));
  });
  const shown = $derived(scope === 'ORGANISATION' && offered && theirs ? theirs : mine);
  const requests = $derived(shown.state.status === 'ready' ? shown.state.data.data : []);
  const hasMore = $derived(shown.state.status === 'ready' && Boolean(shown.state.data.page_info.next_cursor));
  const failure = $derived(shown.state.status === 'failed' ? renderProblem(shown.state.error, messages) : undefined);
  let isLoadingMore = $state(false);

  async function more() {
    isLoadingMore = true;
    try {
      await shown.loadMore();
    } finally {
      isLoadingMore = false;
    }
  }
</script>

<WorkPage>
  {#snippet top()}
    <PageHeader title={t('app.portal.mine')} subtitle={deskName} isTitleInBar={viewport.isCompact} />
  {/snippet}
  <Stack gap="150">
    {#if offered}
      <Radio label={t('app.portal.whose')} name="portal-whose" bind:value={scope}
        options={[{ value: 'MINE', label: t('app.portal.whose_mine') },
          { value: 'ORGANISATION', label: t('app.portal.whose_organisation', { name: organisation?.name ?? '' }) }]} />
    {/if}
    {#if failure}
      <FailureState problem={failure} onRetry={() => void shown.refresh()} />
    {:else if shown.state.status !== 'ready'}
      <div aria-busy="true"><Skeleton lines={5} /></div>
    {:else if requests.length === 0}
      <ProductEmpty product="SERVICE_DESK" kind="unused" title={t('app.portal.mine_none')} />
    {:else}
      <div class="card"><RequestRows {requests} {onnavigate} /></div>
      <LoadMore label={t('app.portal.more')} busyLabel={t('app.portal.loading_more')} {hasMore} isBusy={isLoadingMore}
        endLabel={t('app.portal.end')} onLoadMore={() => void more()} />
    {/if}
  </Stack>
</WorkPage>

<style>
  .card { border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); }
</style>
