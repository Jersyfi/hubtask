<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Waiting for your yes* (UC-SVC-09 check 1): the one inbox of what waits for the reader's decision.
  // The approvals whose current level counts them, names a role they hold there, or belongs to
  // somebody away whose approvals they decide - oldest first, as the server answers, a page at a
  // time; and, where agents are on, the acts an agent holds for the reader's yes, each decided in
  // place with its own presses.

  import { untrack } from 'svelte';
  import { EmptyState, LoadMore, PageHeader, Skeleton, Stack, WorkPage } from '@hubtask/design-system/components';
  import FailureState from '../lib/frame/FailureState.svelte';
  import type { AgentList, AgentRequest, AgentRequestList, ApprovalList } from '@hubtask/sync-engine';

  import HeldActCard from '../lib/agents/HeldActCard.svelte';
  import { actor } from '../lib/data/account.svelte.ts';
  import { agents, agentsPath, mayDecide, requestsPath } from '../lib/data/agents.svelte.ts';
  import { myApprovalsPath } from '../lib/data/approvals.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import ApprovalCard from '../lib/servicedesk/ApprovalCard.svelte';
  import { productBandsOf } from '../lib/today.ts';

  const { onnavigate }: { onnavigate?: (path: string) => void } = $props();

  const list = untrack(() => resource<ApprovalList>(myApprovalsPath));
  const rows = $derived(list.state.status === 'ready' ? list.state.data.data : undefined);
  const hasMore = $derived(list.state.status === 'ready' && list.state.data.page?.has_more === true);
  const failure = $derived(list.state.status === 'failed' ? renderProblem(list.state.error, messages) : undefined);
  $effect(() => page.entitle(t('app.approvals.mine_title')));

  let isLoadingMore = $state(false);
  let moreFailed = $state<string | undefined>(undefined);
  async function loadMore() {
    if (isLoadingMore) return;
    isLoadingMore = true;
    moreFailed = undefined;
    try {
      await list.loadMore();
    } catch {
      moreFailed = t('app.approvals.load_more_failed');
    } finally {
      isLoadingMore = false;
    }
  }

  // The agents' held acts, only where agents are on: `GET /agent-requests` answers those the
  // reader may decide, and the agents' own list gives their names.
  const countsActs = $derived(productBandsOf(manifest.value?.modules_on).includes('agent_acts'));
  let heldActs = $state<AgentRequestList['data'] | undefined>(undefined);
  let agentList = $state<AgentList['data']>([]);
  $effect(() => {
    if (!countsActs) {
      heldActs = undefined;
      return;
    }
    return untrack(() => {
      const stops = [
        engine.subscribe<AgentRequestList>({ path: requestsPath('PENDING') }, (next) => { if (next.status === 'ready') heldActs = next.data.data; }),
        engine.subscribe<AgentList>({ path: agentsPath }, (next) => { if (next.status === 'ready') agentList = next.data.data; }),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });
  const acts = $derived((heldActs ?? []).filter((each) => each.status === 'PENDING'));
  const byId = $derived(new Map(agentList.map((each) => [each.id, each])));
  const viewer = $derived({ administers: holdsHere('MANAGE_MEMBERS') === true, accountId: actor.account?.id });
  async function decideAct(request: AgentRequest, approve: boolean) {
    try {
      await agents.decide(request, approve);
      toast.show({ text: t(approve ? 'app.agents.request_approved' : 'app.agents.request_declined'), tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }
  const isEmpty = $derived(rows !== undefined && rows.length === 0 && acts.length === 0);
</script>

<!-- A work surface, not a list page: each request is decided in its card (page-system §7.8). -->
<WorkPage>
  {#snippet top()}
    <PageHeader title={t('app.approvals.mine_title')} subtitle={t('app.approvals.mine_subtitle')} isTitleInBar={viewport.isCompact} />
  {/snippet}
  {#if countsActs}
    <!-- The agents' acts have a second place, with the decided ones too (accepted: two places stay). -->
    <p class="quiet" data-agent-requests><a href="/profile/agents?tab=requests">{t('app.approvals.agent_requests_link')}</a></p>
  {/if}
  {#if failure}
    <FailureState problem={failure} onRetry={() => void list.refresh()} />
  {:else if !rows}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else if isEmpty}
    <EmptyState kind="unused" icon="user-check" title={t('app.approvals.mine_none')} description={t('app.approvals.mine_none_body')} />
  {:else}
    {#if acts.length > 0}
      <section aria-labelledby="approvals-acts" data-kind="agent_acts">
        <Stack gap="150">
          <h2 class="heading" id="approvals-acts">{t('app.approvals.section_acts', { count: acts.length })}</h2>
          <ul class="rows">
            {#each acts as request (request.id)}
              <HeldActCard {request} agentName={byId.get(request.agent_id)?.name ?? t('app.today.held_act_agent')}
                mayDecide={mayDecide(byId.get(request.agent_id), viewer)} ondecide={(approve) => void decideAct(request, approve)} />
            {/each}
          </ul>
        </Stack>
      </section>
    {/if}
    {#if rows.length > 0}
      <section aria-labelledby="approvals-entries" data-kind="approvals">
        <Stack gap="150">
          {#if acts.length > 0}<h2 class="heading" id="approvals-entries">{t('app.approvals.section_entries')}</h2>{/if}
          {#each rows as approval (approval.id)}
            <ApprovalCard {approval} {onnavigate} />
          {/each}
          {#if hasMore}
            <LoadMore label={t('app.approvals.load_more', { shown: String(rows.length) })} busyLabel={t('app.entries.loading_more')}
              hasMore isBusy={isLoadingMore} {...(moreFailed ? { failedLabel: moreFailed } : {})} onLoadMore={() => void loadMore()} />
          {/if}
        </Stack>
      </section>
    {/if}
  {/if}
</WorkPage>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .heading { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-100); }
</style>
