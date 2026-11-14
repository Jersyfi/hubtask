<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One agent as its record page (page-system §5.1, §9.5) at `<base>/:id`: its state and what it
  // may do in the head, its lists - the ledger, its jobs with *Run on…*, its evaluation set and the
  // runs on it - as regions, retiring it in the danger zone (UC-AGT-02 check 9, UC-AGT-04 check 6,
  // UC-AGT-07 check 7). The ledger never holds an input or an answer, so neither does this.

  import { untrack } from 'svelte';

  import {
    Badge, Breadcrumb, Button, DataTable, EmptyState, ObjectHead, RecordPage, Select, Skeleton, Stack, StateLozenge, StatusButton,
    type DataColumn, type RecordList, type DataRowMenu, type SelectOption, type StatusCategory, type StatusRef, type WorkflowStep,
  } from '@hubtask/design-system/components';
  import type { Agent, AgentActionList, AgentCaseList, AgentJobList, AgentModelList, AgentRequestList, AgentRunList } from '@hubtask/sync-engine';

  import AgentDraftDialog from '../lib/agents/AgentDraftDialog.svelte';
  import CaseDialog from '../lib/agents/CaseDialog.svelte';
  import RetireDialog from '../lib/agents/RetireDialog.svelte';
  import RunOnDialog from '../lib/agents/RunOnDialog.svelte';
  import { AGENTS_HOME } from '../lib/agents/trail.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import {
    agentActionsPath, agentCasesPath, agentEvaluationsPath, agentJobsPath, agentModelsPath, agentPath, agents, heldCounts, jobTarget, lifecyclePresses,
    mayStartJob, requestsPath, selfEvaluation, type LifecyclePress, type Viewer,
  } from '../lib/data/agents.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { resource, resourceWhen } from '../lib/data/resource.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    id: string;
    base?: string;
    onnavigate: (path: string) => void;
  }

  const { id, base = AGENTS_HOME, onnavigate }: Props = $props();

  const administers = $derived(holdsHere('MANAGE_MEMBERS') === true);
  const viewer = $derived<Viewer>({ administers, accountId: actor.account?.id });

  // The page is keyed on the agent by its caller, so each path is fixed for its life.
  const key = untrack(() => id);
  const one = resource<Agent>(agentPath(key));
  const ledger = resource<AgentActionList>(agentActionsPath(key));
  const jobs = resource<AgentJobList>(agentJobsPath(key));
  const cases = resource<AgentCaseList>(agentCasesPath(key));
  const runs = resource<AgentRunList>(agentEvaluationsPath(key));
  const pendingRead = resource<AgentRequestList>(requestsPath('PENDING'));
  const modelsRead = resourceWhen<AgentModelList>(() => (administers ? agentModelsPath : undefined));
  const models = $derived(modelsRead.state.status === 'ready' ? modelsRead.state.data.data : []);

  const agent = $derived(one.state.status === 'ready' ? one.state.data : undefined);
  const title = $derived(agent?.name ?? t('app.agents.title'));
  $effect(() => page.entitle(title));

  $effect(() => {
    const ids = [agent?.owner_id, agent?.approved_by].filter((each): each is string => Boolean(each));
    if (ids.length > 0) untrack(() => accounts.resolve(ids));
  });
  const nameOf = (person: string | null | undefined) => (person ? (accounts.nameOf(person) ?? t('app.people.unnamed')) : '—');
  const at = (iso: string | null | undefined) => (iso ? formatDateTime(iso, messages.locale) : '—');
  const grouped = (value: number) => new Intl.NumberFormat(messages.locale).format(value);
  const modelName = (modelId: string | null | undefined) => (modelId ? (models.find((each) => each.id === modelId)?.name ?? t('app.agents.model_unread')) : t('app.agents.no_model'));

  // Approved is a good standing, not a warning (the agents' scenes draw it green); a draft still waits.
  const statusTone = (status: Agent['status']) =>
    status === 'ACTIVE' || status === 'APPROVED' ? 'success' : status === 'SUSPENDED' ? 'danger' : status === 'RETIRED' ? 'neutral' : 'warning';
  /** Every toned standing says it with a mark too (UX-7); an approval that no longer covers the model warns. */
  const STATUS_GLYPH = { DRAFT: 'pencil', APPROVED: 'check', ACTIVE: 'zap', SUSPENDED: 'circle-alert', RETIRED: 'archive' } as const;
  const isApprovalDue = (each: Agent) => each.needs_approval && each.status !== 'DRAFT' && each.status !== 'RETIRED';

  async function act(run: () => Promise<unknown>, done: string) {
    try {
      await run();
      toast.show({ text: done, tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }

  // --- The head ---------------------------------------------------------------------------------

  let drafting = $state<{ agent: Agent } | undefined>(undefined);
  let retiring = $state<Agent | undefined>(undefined);
  let isRunOpen = $state(false);
  let isCaseOpen = $state(false);

  const presses = $derived(agent ? lifecyclePresses(agent, viewer) : []);
  const waiting = $derived(pendingRead.state.status === 'ready' ? (heldCounts(pendingRead.state.data.data).get(key) ?? 0) : 0);
  // The head's next step (WB-1): putting it live, else approving it; the other presses stay in ⋯.
  const nextPress = $derived(presses.find((each) => each === 'activate') ?? presses.find((each) => each === 'approve'));
  const menuItems = $derived([
    ...(waiting > 0 ? [{ id: 'held', label: t('app.agents.held_count', { count: String(waiting) }) }] : []),
    ...presses.filter((each) => each !== 'retire' && each !== nextPress).map((each) => ({ id: each, label: t(`app.agents.press.${each}`) })),
  ]);
  function onMenu(item: string) {
    if (!agent) return;
    if (item === 'held') onnavigate(`${base}?tab=requests&agent=${agent.id}`);
    else void act(() => agents.press(agent.id, item as LifecyclePress), t(`app.agents.pressed.${item}`, { name: agent.name }));
  }

  // The status's ⌄ (H-2): the lifecycle steps the reader may take from here, each to the status it
  // leads to; retiring stays in the danger zone, where its cost is said.
  const PRESS_TO = { approve: 'APPROVED', activate: 'ACTIVE', suspend: 'SUSPENDED' } as const;
  const categoryOf = (status: Agent['status']): StatusCategory => (status === 'ACTIVE' ? 'progress' : status === 'RETIRED' ? 'done' : 'todo');
  const statusRef = (status: Agent['status'], label = t(`app.agents.status.${status}`)): StatusRef => ({ id: status, label, category: categoryOf(status) });
  const statusSteps = $derived<WorkflowStep[]>(
    presses.flatMap((press) => (press === 'retire' ? [] : [{ id: press, label: t(`app.agents.press.${press}`), to: statusRef(PRESS_TO[press]), isNext: press === nextPress }])),
  );

  const meta = $derived(agent ? [t('app.agents.acts_for', { name: nameOf(agent.owner_id) }), modelName(agent.model_id)].join(' · ') : undefined);
  const statusLabel = $derived(
    agent
      ? agent.needs_approval && agent.status !== 'DRAFT' && agent.status !== 'RETIRED'
        ? `${t(`app.agents.status.${agent.status}`)} · ${t('app.agents.needs_approval')}`
        : t(`app.agents.status.${agent.status}`)
      : '',
  );
  // The budget as the head's fact: a day's tokens and how much of it today used (the scene's line).
  const budgetFact = $derived.by(() => {
    if (!agent) return undefined;
    const perDay = agent.budget.tokens_per_day ?? 0;
    if (perDay === 0) return t('app.agents.budget_none');
    const today = one.state.status === 'ready' ? (one.state.data.usage?.tokens_today ?? 0) : 0;
    return t('app.agents.budget_fact', { tokens: grouped(perDay), share: String(Math.min(100, Math.round((100 * today) / perDay))) });
  });

  const about = $derived(
    agent
      ? {
          title: t('app.agents.about'),
          rows: [
            { label: t('app.agents.purpose'), value: agent.purpose },
            { label: t('app.agents.policy_label'), value: t('app.agents.policy_line', { reversible: t(`app.agents.rule.${agent.policy.reversible ?? 'APPROVAL'}`), irreversible: t(`app.agents.rule.${agent.policy.irreversible ?? 'APPROVAL'}`) }) },
            {
              label: t('app.agents.budget_label'),
              value: (agent.budget.tokens_per_day ?? 0) === 0 || (agent.budget.actions_per_hour ?? 0) === 0 || (agent.budget.concurrent_runs ?? 0) === 0
                ? t('app.agents.budget_zero')
                : t('app.agents.budget_line', { tokens: grouped(agent.budget.tokens_per_day ?? 0), actions: grouped(agent.budget.actions_per_hour ?? 0), runs: grouped(agent.budget.concurrent_runs ?? 0) }),
            },
            { label: t('app.agents.bar_label'), value: agent.min_accuracy > 0 ? t('app.agents.bar', { min_accuracy: agent.min_accuracy }) : t('app.agents.no_bar') },
            ...(agent.approved_at ? [{ label: t('app.agents.approved_label'), value: t('app.agents.approved_by', { name: nameOf(agent.approved_by), at: at(agent.approved_at) }) }] : []),
          ],
        }
      : undefined,
  );

  // --- The lists --------------------------------------------------------------------------------

  const target = $derived(agent ? jobTarget(agent.kind) : undefined);
  const mayRun = $derived(agent ? mayStartJob(agent, viewer) : false);
  const countOf = (state: { status: string; data?: { data: readonly unknown[] } }) => (state.status === 'ready' ? String(state.data?.data.length ?? 0) : undefined);
  /** An empty part is one line, its sentence beside its heading (WB-3). */
  const emptyOf = (state: { status: string; data?: { data: readonly unknown[] } }, code: string) => (state.status === 'ready' && (state.data?.data.length ?? 0) === 0 ? { emptyLabel: t(code) } : {});

  const lists = $derived<RecordList[]>([
    { id: 'ledger', title: t('app.agents.ledger'), count: countOf(ledger.state as never), ...emptyOf(ledger.state as never, 'app.agents.ledger_empty') },
    ...(target
      ? [{ id: 'jobs', title: t('app.agents.jobs'), count: countOf(jobs.state as never), ...emptyOf(jobs.state as never, 'app.agents.jobs_empty'), ...(mayRun ? { add: { label: t('app.agents.run_on'), icon: 'play' as const, onclick: () => (isRunOpen = true) } } : {}) }]
      : []),
    { id: 'cases', title: t('app.agents.cases'), count: countOf(cases.state as never), ...emptyOf(cases.state as never, 'app.agents.cases_empty'), ...(administers ? { add: { label: t('app.agents.add_case'), icon: 'plus' as const, onclick: () => (isCaseOpen = true) } } : {}) },
    { id: 'runs', title: t('app.agents.runs'), count: countOf(runs.state as never), ...emptyOf(runs.state as never, 'app.agents.runs_empty') },
  ]);

  const failureOf = (state: { status: string; error?: unknown }) => (state.status === 'failed' ? renderProblem(state.error as never, messages) : undefined);

  type Ledger = AgentActionList['data'][number];
  type Job = AgentJobList['data'][number];
  type Case = AgentCaseList['data'][number];
  type Evaluation = AgentRunList['data'][number];

  const ledgerColumns = $derived<DataColumn[]>([
    { id: 'at', label: t('app.agents.col_at'), priority: 2 },
    { id: 'use_case', label: t('app.agents.col_use_case'), priority: 1 },
    { id: 'class', label: t('app.agents.col_class'), priority: 3 },
    { id: 'decision', label: t('app.agents.col_decision'), isStatus: true },
    { id: 'tokens', label: t('app.agents.col_tokens'), align: 'end', priority: 3 },
  ]);
  const jobColumns = $derived<DataColumn[]>([
    { id: 'started', label: t('app.agents.col_started'), priority: 1 },
    { id: 'target', label: t('app.agents.col_target'), priority: 2 },
    { id: 'status', label: t('app.agents.col_status'), isStatus: true },
    { id: 'proposals', label: t('app.agents.col_proposals'), align: 'end', priority: 3 },
    { id: 'tokens', label: t('app.agents.col_tokens'), align: 'end', priority: 3 },
  ]);
  const caseColumns = $derived<DataColumn[]>([
    { id: 'title', label: t('app.agents.case_title'), priority: 1 },
    { id: 'expected', label: t('app.agents.case_expected'), priority: 2 },
  ]);
  const runColumns = $derived<DataColumn[]>([
    { id: 'at', label: t('app.agents.col_at'), priority: 1 },
    { id: 'revision', label: t('app.agents.col_revision'), align: 'end', priority: 3 },
    { id: 'passed', label: t('app.agents.col_passed'), align: 'end', priority: 2 },
    { id: 'accuracy', label: t('app.agents.col_accuracy'), isStatus: true },
  ]);

  const caseMenu = (each: Case): DataRowMenu | undefined =>
    administers ? { label: t('app.page.more_for', { name: each.title }), items: [{ id: 'remove', label: t('app.agents.remove_case', { title: each.title }), isDestructive: true }] } : undefined;

  // --- The agent answering its own set ----------------------------------------------------------

  const answersItself = $derived(agent ? selfEvaluation(agent.kind) : undefined);
  let evaluationHub = $state('');
  // A triage's labels are chosen from one collection's; none chosen answers no labels.
  let evaluationCollection = $state('');
  const labelsFrom = $derived(agent?.kind === 'TICKET_TRIAGE' && evaluationHub !== '');
  const hubOptions = $derived<SelectOption[]>(containers.hubs.map((each) => ({ value: each.id, label: each.name })));
  const collectionOptions = $derived<SelectOption[]>([
    { value: '', label: t('app.agents.evaluation_no_labels') },
    ...containers.collectionsOf(evaluationHub).map((each) => ({ value: each.id, label: each.name })),
  ]);
  $effect(() => {
    if (labelsFrom) return containers.openLevel(evaluationHub);
  });

  function evaluate() {
    const hub = answersItself === 'hub' ? evaluationHub : undefined;
    const collection = labelsFrom && evaluationCollection !== '' ? evaluationCollection : undefined;
    void act(() => agents.evaluate(key, hub, collection), t('app.agents.evaluated'));
  }
</script>

<RecordPage
  record={{
    kind: t('app.agents.record_kind'),
    title,
    back: { label: t('app.agents.title'), href: base, onnavigate: () => onnavigate(base) },
    status: agent ? { label: statusLabel, tone: statusTone(agent.status) } : undefined,
    facts: meta,
    edit: agent && administers && agent.status !== 'RETIRED' ? { label: t('app.agents.edit'), icon: 'pencil', onclick: () => agent && (drafting = { agent }) } : undefined,
    menu: agent && menuItems.length > 0 ? { label: t('app.page.more_for', { name: title }), items: menuItems, onselect: onMenu } : undefined,
  }}
  head={agent ? agentHead : undefined}
  lists={agent ? lists : []}
  facts={about}
  danger={agent && presses.includes('retire')
    ? { title: t('app.page.danger_zone'), acts: [{ id: 'retire', title: t('app.agents.retire_act'), cost: t('app.agents.retire_cost'), actionLabel: t('app.agents.retire_menu') }], onact: () => (retiring = agent) }
    : undefined}
  refused={one.state.status === 'failed' ? notHere : undefined}
>
  {#snippet list(item: RecordList)}
    {#if !agent}
      <Skeleton lines={3} />
    {:else if item.id === 'ledger'}
      {#if failureOf(ledger.state)}
        <FailureNote problem={failureOf(ledger.state) ?? ''} />
      {:else if ledger.state.status === 'ready' && ledger.state.data.data.length === 0}
        <p class="quiet">{t('app.agents.ledger_empty')}</p>
      {:else}
        <DataTable label={item.title} columns={ledgerColumns} rows={ledger.state.status === 'ready' ? ledger.state.data.data : []} key={(row: Ledger) => row.id} isBusy={ledger.state.status !== 'ready'}>
          {#snippet cell(row: Ledger, column: DataColumn)}
            {#if column.id === 'at'}{at(row.at)}
            {:else if column.id === 'use_case'}<code>{row.use_case}</code>
            {:else if column.id === 'class'}{t(`app.agents.class.${row.class}`)}
            {:else if column.id === 'decision'}
              <Badge size="sm" tone={row.decision === 'REFUSED' ? 'danger' : row.decision === 'HELD' ? 'warning' : 'success'}>{t(`app.agents.decision.${row.decision}`)}</Badge>
              {#if row.code}<span class="sub">{messages.has(row.code) ? t(row.code) : row.code}</span>{/if}
            {:else}<span class="number">{row.tokens}</span>{/if}
          {/snippet}
          {#snippet empty()}<EmptyState kind="settled" title={t('app.agents.ledger_empty')} />{/snippet}
        </DataTable>
      {/if}
    {:else if item.id === 'jobs'}
      <Stack gap="100">
        {#if !mayRun && agent.status !== 'ACTIVE'}<p class="quiet">{t('app.agents.runs_when_live')}</p>{/if}
        {#if failureOf(jobs.state)}
          <FailureNote problem={failureOf(jobs.state) ?? ''} />
        {:else if jobs.state.status === 'ready' && jobs.state.data.data.length === 0}
          <p class="quiet">{t('app.agents.jobs_empty')}</p>
        {:else}
          <DataTable label={item.title} columns={jobColumns} rows={jobs.state.status === 'ready' ? jobs.state.data.data : []} key={(job: Job) => job.id} isBusy={jobs.state.status !== 'ready'}
            href={(job: Job) => (job.target_type === 'HUB' ? undefined : `/items/${job.target_id}`)}>
            {#snippet cell(job: Job, column: DataColumn)}
              {#if column.id === 'started'}{at(job.started_at)}
              {:else if column.id === 'target'}{job.target_type === 'HUB' ? (containers.find(job.target_id)?.name ?? t('app.agents.hub_unread')) : t('app.agents.open_entry')}
              {:else if column.id === 'status'}
                <Badge size="sm" tone={job.status === 'FAILED' ? 'danger' : job.status === 'RUNNING' ? 'info' : 'success'}>{t(`app.agents.job_status.${job.status}`)}</Badge>
                {#if job.code}<span class="sub">{messages.has(job.code) ? t(job.code) : job.code}</span>{/if}
              {:else if column.id === 'proposals'}<span class="number">{job.proposals}</span>
              {:else}<span class="number">{job.tokens}</span>{/if}
            {/snippet}
            {#snippet empty()}<EmptyState kind="settled" title={t('app.agents.jobs_empty')} />{/snippet}
          </DataTable>
        {/if}
      </Stack>
    {:else if item.id === 'cases'}
      {#if failureOf(cases.state)}
        <FailureNote problem={failureOf(cases.state) ?? ''} />
      {:else if cases.state.status === 'ready' && cases.state.data.data.length === 0}
        <p class="quiet">{t('app.agents.cases_empty')}</p>
      {:else}
        <DataTable label={item.title} columns={caseColumns} rows={cases.state.status === 'ready' ? cases.state.data.data : []} key={(each: Case) => each.id} isBusy={cases.state.status !== 'ready'}
          rowLabel={(each: Case) => each.title} menu={caseMenu} onmenu={(each: Case) => void act(() => agents.removeCase(key, each.id), t('app.agents.case_removed'))}
          actionsLabel={t('app.page.row_actions')} sheetDismissLabel={t('app.page.sheet_close')} sheetGrabLabel={t('app.page.sheet_grab')}>
          {#snippet cell(each: Case, column: DataColumn)}
            {#if column.id === 'title'}<span class="name">{each.title}</span>
            {:else}<code class="sub">{JSON.stringify(each.expected)}</code>{/if}
          {/snippet}
          {#snippet empty()}<EmptyState kind="unused" title={t('app.agents.cases_empty')} />{/snippet}
        </DataTable>
      {/if}
    {:else}
      <Stack gap="150">
        {#if administers && answersItself}
          <div class="line">
            {#if answersItself === 'hub'}
              <Select label={t('app.agents.evaluation_hub')} size="sm" options={hubOptions} bind:value={evaluationHub} onchange={() => (evaluationCollection = '')} />
            {/if}
            {#if labelsFrom}
              <Select label={t('app.agents.evaluation_collection')} size="sm" options={collectionOptions} bind:value={evaluationCollection} />
            {/if}
            <Button
              icon="play"
              disabledReason={answersItself === 'hub' && !evaluationHub ? t('app.agents.evaluation_hub_needed') : undefined}
              onclick={evaluate}
            >{t('app.agents.answer_itself')}</Button>
          </div>
        {/if}
        {#if runs.state.status === 'ready' && runs.state.data.data.length === 0}
          <p class="quiet">{t('app.agents.runs_empty')}</p>
        {:else}
        <DataTable label={item.title} columns={runColumns} rows={runs.state.status === 'ready' ? runs.state.data.data : []} key={(each: Evaluation) => each.id} isBusy={runs.state.status !== 'ready'}>
          {#snippet cell(each: Evaluation, column: DataColumn)}
            {#if column.id === 'at'}{at(each.at)}
            {:else if column.id === 'revision'}<span class="number">{each.model_revision}</span>
            {:else if column.id === 'passed'}<span class="number">{t('app.agents.passed_of', { passed: each.passed, cases: each.cases })}</span>
            {:else}<Badge size="sm" tone={each.accuracy >= agent.min_accuracy ? 'success' : 'danger'}>{t('app.agents.percent', { value: each.accuracy })}</Badge>{/if}
          {/snippet}
          {#snippet empty()}<EmptyState kind="settled" title={t('app.agents.runs_empty')} />{/snippet}
        </DataTable>
        {/if}
      </Stack>
    {/if}
  {/snippet}
</RecordPage>

{#snippet agentHead()}
  {#if agent}
    {@const shown = agent}
    <!-- One head for every object (WB-1): its state, putting it live as the next step, changing it
         beside, the budget as its fact; the other presses in ⋯, retiring in the danger zone. -->
    <ObjectHead hasActsOnPhone
      objectKey={{ label: t('app.agents.record_kind'), icon: 'zap', product: 'agents' }}
      type={{ label: t(`app.agents.kind.${shown.kind}`) }}
      {meta}
      title={shown.name}
      titleLabel={t('app.agents.record_kind')}
      next={nextPress ? { label: t(`app.agents.press.${nextPress}`), onclick: () => onMenu(nextPress) } : undefined}
      secondary={administers && shown.status !== 'RETIRED' ? { label: t('app.agents.edit'), onclick: () => (drafting = { agent: shown }) } : undefined}
      person={{ name: nameOf(shown.owner_id), src: accounts.pictureOf(shown.owner_id) }}
      trail={viewport.isCompact ? undefined : agentTrail}
      menu={menuItems.length > 0 ? { label: t('app.page.more_for', { name: title }), items: menuItems, onselect: onMenu } : undefined}
    >
      {#snippet status()}
        {#if statusSteps.length > 0}
          <span data-agent-status><StatusButton current={statusRef(shown.status, statusLabel)} steps={statusSteps}
            look={{ tone: isApprovalDue(shown) ? 'warning' : statusTone(shown.status), icon: isApprovalDue(shown) ? 'triangle-alert' : STATUS_GLYPH[shown.status] }}
            label={t('app.agents.status_button', { state: statusLabel })}
            menuTitle={t('app.agents.status_from', { state: t(`app.agents.status.${shown.status}`) })}
            onstep={(step) => onMenu(step.id)}
            closeLabel={t('app.workflows.close')} grabLabel={t('app.workflows.close')} /></span>
        {:else}
          <StateLozenge size="md" tone={isApprovalDue(shown) ? 'warning' : statusTone(shown.status)} icon={isApprovalDue(shown) ? 'triangle-alert' : STATUS_GLYPH[shown.status]} label={statusLabel} />
        {/if}
      {/snippet}
      {#snippet facts()}{#if budgetFact}<span data-budget-fact>{budgetFact}</span>{/if}{/snippet}
    </ObjectHead>
  {/if}
{/snippet}

{#snippet agentTrail()}
  <Breadcrumb label={t('app.goals.trail')} trail={[{ id: 'agents', label: t('app.agents.title'), href: base }, { id: 'agent', label: title }]}
    expandLabel={t('app.goals.expand_trail')} onnavigate={() => onnavigate(base)} />
{/snippet}

{#snippet notHere()}
  <FailureNote problem={failureOf(one.state) ?? ''} />
{/snippet}

<AgentDraftDialog opened={drafting} {models} onclose={() => (drafting = undefined)} />
<RetireDialog agent={retiring} onclose={() => (retiring = undefined)} />
{#if agent}
  <RunOnDialog {agent} isOpen={isRunOpen} onclose={() => (isRunOpen = false)} />
  <CaseDialog agentId={key} isOpen={isCaseOpen} onclose={() => (isCaseOpen = false)} />
{/if}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .name { display: block; font-weight: var(--fw-medium); overflow-wrap: anywhere; }
  .sub { display: block; color: var(--text-subtle); font-size: var(--fs-075); overflow-wrap: anywhere; }
  .number { font-variant-numeric: tabular-nums; }
  .line { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: end; }
</style>
