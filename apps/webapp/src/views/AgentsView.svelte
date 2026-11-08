<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The agents (user-experience.md §7.8) as a list page (page-system §9.5, §9.6), at the product's
  // home `/agents` and the reader's own `/profile/agents`: the strip *Agents · Waiting for a
  // person · Runs · Settings*, each agent a row whose press opens its record page at
  // `<base>/:id`, its held acts counted in the row and opened from it. *Settings* is a settings
  // page of its own (`AgentsSettingsView`), dispatched where the route is (UC-AGT-01 check 9,
  // UC-AGT-02 check 9, UC-AGT-03 check 7, UC-AGT-07 check 7).
  //
  // **What the reader may never do is not drawn** (P-05): without MANAGE_MEMBERS the list shows
  // the agents acting for the reader, lets them stop one and decide what it waits on, and offers
  // no registration, model or switch. The server decides (rule 2).

  import { untrack } from 'svelte';

  import {
    Badge, Banner, Button, DataTable, EmptyState, FilterBar, ListPage, Radio, Skeleton, ViewBar,
    type DataColumn, type Filter, type DataRowMenu, type ViewLink,
  } from '@hubtask/design-system/components';
  import type { Agent, AgentHalt, AgentJobList, AgentList, AgentModelList, AgentRequest, AgentRequestList } from '@hubtask/sync-engine';

  import AgentDraftDialog from '../lib/agents/AgentDraftDialog.svelte';
  import HeldActCard from '../lib/agents/HeldActCard.svelte';
  import RetireDialog from '../lib/agents/RetireDialog.svelte';
  import { AGENTS_HOME, agentsTrail } from '../lib/agents/trail.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import {
    SHIPPED_ROLES, agentHaltPath, agentJobsPath, agentModelsPath, agents, agentsPath, byState, heldCounts, lifecyclePresses, mayDecide, requestsPath,
    type LifecyclePress, type Viewer,
  } from '../lib/data/agents.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { resource, resourceWhen } from '../lib/data/resource.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { formatDateTime, formatRelative } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import { renderProblem } from '../lib/problem.ts';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import { homeTabs, recentRuns } from '../lib/product/home.ts';

  interface Props {
    /** The tab the address names (`?tab=`); the agents where it names none or an unknown one. */
    tab?: string;
    /** Moves to another address: a tab, an agent's held acts. */
    onnavigate?: (path: string) => void;
    /** Where the list lives: the product's home or the reader's own. */
    base?: string;
    /** The agent whose held acts the address names (`?agent=`), from its row's count. */
    agent?: string;
  }

  let { tab, onnavigate, base = AGENTS_HOME, agent: heldOf }: Props = $props();

  const title = $derived(t('app.agents.title'));
  $effect(() => page.entitle(title));

  const administers = $derived(holdsHere('MANAGE_MEMBERS') === true);
  const viewer = $derived<Viewer>({ administers, accountId: actor.account?.id });

  const current = $derived(tab === 'requests' || tab === 'runs' ? tab : 'agents');
  const hrefOf = (id: string) => (id === 'agents' ? base : `${base}?tab=${id}`);

  const agentsRead = resource<AgentList>(agentsPath);
  const all = $derived(agentsRead.state.status === 'ready' ? agentsRead.state.data.data : []);
  const refusal = $derived(agentsRead.state.status === 'failed' ? renderProblem(agentsRead.state.error, messages) : undefined);
  const byId = $derived(new Map(all.map((each) => [each.id, each])));
  const nameOfAgent = (agentId: string) => byId.get(agentId)?.name ?? t('app.agents.agent_unread');

  $effect(() => {
    const ids = all.flatMap((each) => [each.owner_id, each.approved_by]).filter((id): id is string => Boolean(id));
    if (ids.length > 0) untrack(() => accounts.resolve(ids));
  });
  const nameOf = (id: string | null | undefined) => (id ? (accounts.nameOf(id) ?? t('app.people.unnamed')) : '—');
  const at = (iso: string | null | undefined) => (iso ? formatDateTime(iso, messages.locale) : '—');

  const haltRead = resourceWhen<AgentHalt>(() => (administers ? agentHaltPath : undefined));
  const halt = $derived(haltRead.state.status === 'ready' ? haltRead.state.data : undefined);

  const modelsRead = resourceWhen<AgentModelList>(() => (administers ? agentModelsPath : undefined));
  const models = $derived(modelsRead.state.status === 'ready' ? modelsRead.state.data.data : []);
  const modelName = (modelId: string | null) => (modelId ? (models.find((each) => each.id === modelId)?.name ?? t('app.agents.model_unread')) : t('app.agents.no_model'));

  async function act<T>(run: () => Promise<T>, done: string): Promise<T | undefined> {
    try {
      const out = await run();
      toast.show({ text: done, tone: 'success' });
      return out;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
      return undefined;
    }
  }

  const statusTone = (status: Agent['status']) =>
    status === 'ACTIVE' ? 'success' : status === 'SUSPENDED' ? 'danger' : status === 'RETIRED' ? 'neutral' : 'warning';

  // --- Adding, changing, retiring ---------------------------------------------------------------

  let drafting = $state<{ agent?: Agent; role?: (typeof SHIPPED_ROLES)[number] } | undefined>(undefined);
  let retiring = $state<Agent | undefined>(undefined);

  const primary = $derived(
    administers
      ? {
          label: t('app.agents.add_agent'),
          icon: 'plus' as const,
          onclick: () => (drafting = {}),
          menu: {
            label: t('app.agents.add_from_role'),
            items: SHIPPED_ROLES.map((kind) => ({ id: kind, label: t(`app.agents.kind.${kind}`) })),
            onselect: (id: string) => (drafting = { role: id as (typeof SHIPPED_ROLES)[number] }),
          },
        }
      : undefined,
  );

  function rowMenu(agent: Agent): DataRowMenu | undefined {
    const presses = lifecyclePresses(agent, viewer);
    const items = [
      ...(administers && agent.status !== 'RETIRED' ? [{ id: 'edit', label: t('app.agents.edit') }] : []),
      ...presses.filter((each) => each !== 'retire').map((each) => ({ id: each, label: t(`app.agents.press.${each}`) })),
      ...(presses.includes('retire') ? [{ id: 'retire', label: t('app.agents.retire_menu'), isDestructive: true, hasSeparatorBefore: true }] : []),
    ];
    return items.length > 0 ? { label: t('app.page.more_for', { name: agent.name }), items } : undefined;
  }

  function onRowMenu(agent: Agent, id: string) {
    if (id === 'edit') drafting = { agent };
    else if (id === 'retire') retiring = agent;
    else void act(() => agents.press(agent.id, id as LifecyclePress), t(`app.agents.pressed.${id}`, { name: agent.name }));
  }

  // --- The list ---------------------------------------------------------------------------------

  let search = $state('');
  const ordered = $derived(byState(all));
  const shown = $derived(ordered.filter((each) => matchesSearch(search, [each.name, each.purpose, t(`app.agents.kind.${each.kind}`), nameOf(each.owner_id)])));

  const columns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.agents.col_name'), priority: 1 },
    { id: 'state', label: t('app.agents.col_state'), isStatus: true },
    { id: 'model', label: t('app.agents.model'), priority: 3 },
    { id: 'budget', label: t('app.agents.col_budget'), priority: 3 },
  ]);
  const grouped = (value: number) => new Intl.NumberFormat(messages.locale).format(value);
  const isBudgetZero = (agent: Agent) =>
    (agent.budget.tokens_per_day ?? 0) === 0 || (agent.budget.actions_per_hour ?? 0) === 0 || (agent.budget.concurrent_runs ?? 0) === 0;

  // --- Held acts --------------------------------------------------------------------------------

  type HeldStatus = AgentRequest['status'];
  let requestStatus = $state<HeldStatus>('PENDING');
  // One subscription per status, so switching back shows what was read at once.
  const requestReads = {
    PENDING: resource<AgentRequestList>(requestsPath('PENDING')),
    APPROVED: resource<AgentRequestList>(requestsPath('APPROVED')),
    DECLINED: resource<AgentRequestList>(requestsPath('DECLINED')),
    FAILED: resource<AgentRequestList>(requestsPath('FAILED')),
  };
  const requestsRead = $derived(requestReads[requestStatus]);
  const requests = $derived(requestsRead.state.status === 'ready' ? requestsRead.state.data.data : []);
  const pending = $derived(requestReads.PENDING.state.status === 'ready' ? requestReads.PENDING.state.data.data : []);
  const held = $derived(heldCounts(pending));
  const heldShown = $derived(
    requests
      .filter((each) => !heldOf || each.agent_id === heldOf)
      .filter((each) => matchesSearch(search, [nameOfAgent(each.agent_id), each.use_case])),
  );

  const statusChips = $derived<Filter[]>([
    { id: 'status', label: `${t('app.agents.request_status_label')}: ${t(`app.agents.request_status.${requestStatus}`)}`, isActive: requestStatus !== 'PENDING' },
  ]);

  function decide(request: AgentRequest, approve: boolean) {
    void act(() => agents.decide(request, approve), approve ? t('app.agents.request_approved') : t('app.agents.request_declined'));
  }

  // --- Runs -------------------------------------------------------------------------------------

  // Only the agents whose jobs the reader may read - their own, or any where they administer - and
  // at most twenty asked, read while the tab is open.
  const RUNS_SHOWN = 30;
  const runAgentKey = $derived(
    current === 'runs' ? all.filter((each) => mayDecide(each, viewer)).slice(0, 20).map((each) => each.id).join(',') : '',
  );
  let jobsAt = $state<Record<string, AgentJobList['data']>>({});
  let isRunsRead = $state(false);
  $effect(() => {
    const ids = runAgentKey === '' ? [] : runAgentKey.split(',');
    return untrack(() => {
      let answered = 0;
      isRunsRead = ids.length === 0;
      const stops = ids.map((agentId) => {
        let isFirst = true;
        return engine.subscribe<AgentJobList>({ path: agentJobsPath(agentId) }, (next) => {
          if (next.status === 'ready') jobsAt = { ...jobsAt, [agentId]: next.data.data };
          if ((next.status === 'ready' || next.status === 'failed') && isFirst) {
            isFirst = false;
            answered += 1;
            if (answered === ids.length) isRunsRead = true;
          }
        });
      });
      return () => stops.forEach((stop) => stop());
    });
  });
  type Run = AgentJobList['data'][number] & { readonly agent_id: string };
  const runs = $derived<Run[]>(recentRuns(jobsAt, runAgentKey === '' ? [] : runAgentKey.split(','), RUNS_SHOWN));
  const runsShown = $derived(runs.filter((run) => matchesSearch(search, [nameOfAgent(run.agent_id)])));
  const runTone = (status: string) => (status === 'FAILED' ? 'danger' : status === 'RUNNING' ? 'info' : 'neutral');
  const runPath = (job: Run) => (job.target_type === 'HUB' ? `/hubs/${job.target_id}` : `/items/${job.target_id}`);
  const runTarget = (job: Run) =>
    job.target_type === 'HUB' ? (containers.find(job.target_id)?.name ?? t('app.agents.hub_unread')) : t('app.agents.run_target_entry');
  const runColumns = $derived<DataColumn[]>([
    { id: 'what', label: t('app.agents.col_run'), priority: 1 },
    { id: 'when', label: t('app.agents.col_started'), priority: 2 },
    { id: 'spent', label: t('app.agents.col_proposals'), priority: 3 },
    { id: 'status', label: t('app.agents.col_status'), isStatus: true },
  ]);

  // --- The head and the strip -------------------------------------------------------------------

  const tabs = $derived<ViewLink[]>(
    homeTabs('AGENTS').filter((each) => administers || each.id !== 'settings').map((each) => ({
      id: each.id,
      label: t(each.code),
      href: hrefOf(each.id),
      ...(each.id === 'agents' && agentsRead.state.status === 'ready' ? { count: all.length } : {}),
      ...(each.id === 'requests' && pending.length > 0 ? { count: pending.length } : {}),
    })),
  );

  const figureText = $derived.by(() => {
    if (agentsRead.state.status !== 'ready' || all.length === 0) return '';
    const live = all.filter((each) => each.status === 'ACTIVE').length;
    const drafts = all.filter((each) => each.status === 'DRAFT').length;
    return [
      t('app.agents.figure_live', { count: String(live) }),
      ...(drafts > 0 ? [t('app.agents.figure_drafts', { count: drafts })] : []),
      ...(pending.length > 0 ? [t('app.agents.figure_held', { count: String(pending.length) })] : []),
    ].join(' · ');
  });

  const head = $derived({
    title,
    subtitle: t('app.agents.intro'),
    breadcrumb: agentsTrail(base),
    primary: current === 'agents' ? primary : undefined,
    figure,
  });

  const searchLabel = $derived(current === 'requests' ? t('app.agents.search_held') : current === 'runs' ? t('app.agents.search_runs') : t('app.agents.search'));
  const shownCount = $derived(current === 'requests' ? heldShown.length : current === 'runs' ? runsShown.length : shown.length);
  const listedCount = $derived(current === 'requests' ? requests.length : current === 'runs' ? runs.length : all.length);
</script>

{#snippet figure()}{#if figureText}<span class="figure" data-agents-figure>{figureText}</span>{/if}{/snippet}

<ListPage {head} refused={refusal ? refusedSnippet : undefined}>
  {#snippet strip()}
    <ViewBar label={title} views={tabs} isCompact={viewport.isCompact} closeLabel={t('app.sheet_close')} {current} onnavigate={(id) => onnavigate?.(hrefOf(id))} />
  {/snippet}
  {#snippet filters()}
    <FilterBar
      {searchLabel}
      clearSearchLabel={t('app.page.clear_search')}
      bind:search
      filters={current === 'requests' ? statusChips : []}
      moreLabel={t('app.page.more_filters')}
      filtersLabel={t('app.page.filters', { count: String(requestStatus !== 'PENDING' ? 1 : 0) })}
      sheetTitle={t('app.page.filters', { count: '0' })}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      showLabel={t('app.page.show_results', { count: String(shownCount) })}
      clearLabel={t('app.page.clear')}
      onclear={() => ((requestStatus = 'PENDING'), (search = ''))}
      countLabel={agentsRead.state.status === 'ready' ? t('app.page.found', { shown: String(shownCount), count: String(listedCount) }) : undefined}
    >
      {#snippet picker(_filter: Filter)}
        <Radio
          label={t('app.agents.request_status_label')}
          bind:value={requestStatus}
          options={(['PENDING', 'APPROVED', 'DECLINED', 'FAILED'] as const).map((each) => ({ value: each, label: t(`app.agents.request_status.${each}`) }))}
        />
      {/snippet}
    </FilterBar>
  {/snippet}

  {#if halt?.halted}
    <Banner tone="danger" title={t('app.agents.halted_title')}>
      {t('app.agents.halted_since', { name: nameOf(halt.set_by), at: at(halt.set_at) })}
    </Banner>
  {/if}

  {#if agentsRead.state.status !== 'ready'}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else if current === 'requests'}
    {#if heldOf}
      <div class="narrowed">
        <span class="chip">{t('app.agents.held_of', { name: nameOfAgent(heldOf) })}</span>
        <Button size="sm" tone="subtle" icon="x" onclick={() => onnavigate?.(hrefOf('requests'))}>{t('app.agents.held_all')}</Button>
      </div>
    {/if}
    {#if requestsRead.state.status === 'failed'}
      <FailureNote problem={renderProblem(requestsRead.state.error, messages)} />
    {:else if requestsRead.state.status !== 'ready'}
      <div aria-busy="true"><Skeleton lines={3} /></div>
    {:else if heldShown.length === 0}
      {#if (heldOf || search) && requests.length > 0}
        <ProductEmpty kind="filtered" title={heldOf ? t('app.agents.held_filtered', { name: nameOfAgent(heldOf) }) : t('app.agents.filtered')}
          clear={{ label: t('app.agents.held_all'), onclick: () => ((search = ''), onnavigate?.(hrefOf('requests'))) }} />
      {:else}
        <ProductEmpty kind="settled" icon="check" title={t('app.agents.requests_empty')} />
      {/if}
    {:else}
      <ul class="rows">
        {#each heldShown as request (request.id)}
          <HeldActCard {request} agentName={nameOfAgent(request.agent_id)} mayDecide={mayDecide(byId.get(request.agent_id), viewer)}
            ondecide={(approve) => decide(request, approve)} />
        {/each}
      </ul>
    {/if}
  {:else if current === 'runs'}
    {#if !isRunsRead && runs.length === 0}
      <div aria-busy="true"><Skeleton lines={4} /></div>
    {:else if runs.length === 0}
      <ProductEmpty kind="settled" icon="zap" title={t('app.agents.runs_none')} description={t('app.agents.runs_none_body')} />
    {:else}
      <DataTable
        label={t('app.agents.tab_runs')}
        columns={runColumns}
        rows={runsShown}
        key={(run: Run) => run.id}
        href={runPath}
        rowLabel={(run: Run) => t('app.agents.run_line', { agent: nameOfAgent(run.agent_id), target: runTarget(run) })}
        shownLabel={t('app.page.showing', { count: String(runsShown.length) })}
      >
        {#snippet cell(run: Run, column: DataColumn)}
          {#if column.id === 'what'}
            <span class="name">{t('app.agents.run_line', { agent: nameOfAgent(run.agent_id), target: runTarget(run) })}</span>
            {#if run.code && messages.has(run.code)}<span class="sub">{t(run.code)}</span>{/if}
          {:else if column.id === 'when'}
            <span class="quiet">{formatRelative(run.started_at, messages.locale)}</span>
          {:else if column.id === 'spent'}
            <span class="quiet">{t('app.agents.run_proposals', { count: run.proposals })} · {t('app.agents.run_tokens', { count: grouped(run.tokens) })}</span>
          {:else}
            <Badge tone={runTone(run.status)} size="sm">{t(`app.agents.job_status.${run.status}`)}</Badge>
          {/if}
        {/snippet}
        {#snippet empty()}<EmptyState kind="filtered" title={t('app.agents.filtered')} />{/snippet}
      </DataTable>
    {/if}
  {:else if all.length === 0}
    <ProductEmpty product="AGENTS"
      kind="unused"
      icon="zap"
      title={t('app.agents.empty_title')}
      description={t('app.agents.empty_body')}
      create={administers ? { label: t('app.agents.add_agent'), onclick: () => (drafting = {}) } : undefined}
      templates={administers ? SHIPPED_ROLES.slice(0, 3).map((kind) => ({ label: t(`app.agents.kind.${kind}`), onclick: () => (drafting = { role: kind }) })) : []}
      whoCan={t('app.agents.empty_who')}
    />
  {:else}
    <DataTable
      label={title}
      {columns}
      rows={shown}
      key={(agent: Agent) => agent.id}
      href={(agent: Agent) => `${base}/${agent.id}`}
      rowLabel={(agent: Agent) => agent.name}
      menu={rowMenu}
      onmenu={onRowMenu}
      actionsLabel={t('app.page.row_actions')}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      shownLabel={t('app.page.showing', { count: String(shown.length) })}
    >
      {#snippet cell(agent: Agent, column: DataColumn)}
        {#if column.id === 'name'}
          <span class="name" data-agent-row={agent.id}>{agent.name}</span>
          <span class="sub">{t(`app.agents.kind.${agent.kind}`)} · {t('app.agents.acts_for', { name: nameOf(agent.owner_id) })}</span>
        {:else if column.id === 'state'}
          <span class="state">
            <Badge size="sm" tone={statusTone(agent.status)}>{t(`app.agents.status.${agent.status}`)}</Badge>
            {#if agent.needs_approval && agent.status !== 'DRAFT' && agent.status !== 'RETIRED'}
              <Badge size="sm" tone="warning">{t('app.agents.needs_approval')}</Badge>
            {/if}
          </span>
        {:else if column.id === 'model'}
          <span class="quiet">{modelName(agent.model_id)}</span>
        {:else if isBudgetZero(agent)}
          <span class="warn" data-agent-budget-zero>{t('app.agents.budget_zero_short')}</span>
        {:else}
          <span class="quiet">{t('app.agents.budget_short', { tokens: grouped(agent.budget.tokens_per_day ?? 0), actions: grouped(agent.budget.actions_per_hour ?? 0) })}</span>
        {/if}
      {/snippet}
      {#snippet verb(agent: Agent)}
        {@const waiting = held.get(agent.id) ?? 0}
        {#if waiting > 0}
          <Button size="sm" tone="primary" icon="clock" onclick={() => onnavigate?.(`${hrefOf('requests')}&agent=${agent.id}`)}>{t('app.agents.held_count', { count: String(waiting) })}</Button>
        {/if}
      {/snippet}
      {#snippet empty()}
        <EmptyState kind="filtered" title={t('app.agents.filtered')}>
          {#snippet action()}<Button onclick={() => (search = '')}>{t('app.people.filtered_clear')}</Button>{/snippet}
        </EmptyState>
      {/snippet}
    </DataTable>
  {/if}
</ListPage>

{#snippet refusedSnippet()}{#if refusal}<FailureNote problem={refusal} />{/if}{/snippet}

<AgentDraftDialog opened={drafting} {models} onclose={() => (drafting = undefined)} />
<RetireDialog agent={retiring} onclose={() => (retiring = undefined)} />

<style>
  .figure { color: var(--text-secondary); font-variant-numeric: tabular-nums; }
  .quiet { color: var(--text-secondary); }
  .name { display: block; font-weight: var(--fw-medium); overflow-wrap: anywhere; }
  .sub { display: block; color: var(--text-subtle); font-size: var(--fs-075); overflow-wrap: anywhere; }
  .state { display: inline-flex; flex-wrap: wrap; gap: var(--sp-050); }
  .warn { color: var(--text-warning); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-100); }
  .narrowed { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .chip { padding: var(--sp-025) var(--sp-100); border-radius: var(--r-full); background: var(--bg-surface-sunken); color: var(--text-primary); font-size: var(--fs-075); }
</style>
