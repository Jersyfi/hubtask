<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The agents (user-experience.md §7.8), at the product's home `/agents` and, with the same
  // anatomy, at the administration's and the reader's own addresses: cards with their role, state,
  // what they may do and their budget, each card's count of held acts opening those acts; the runs
  // newest first in words; adding starts from a shipped role. *Settings* (the kill switch, MCP's
  // registered-only, the models, the metering) is the last tab, for whoever administers agents
  // (UC-AGT-01 check 9, UC-AGT-02 check 9, UC-AGT-03 check 7, UC-AGT-07 check 7).
  //
  // **What the reader may never do is not drawn** (P-05): without MANAGE_MEMBERS the screen shows
  // the agents acting for the reader, lets them stop one and decide what it waits on, and offers
  // no registration, model or switch. The server decides (rule 2).

  import { untrack } from 'svelte';
  import {
    Badge, Banner, Button, Dialog, Icon, IconButton, Input, ListRow, PageHeader, PlaceTabs, Select, Skeleton, Stack, Switch, Table, Textarea,
    type PlaceTab, type SelectOption,
  } from '@hubtask/design-system/components';
  import type {
    Agent, AgentHalt, AgentJobList, WorkspaceAgentPolicy, AgentList, AgentMeteringList, AgentModel, AgentModelList, AgentRequest, AgentRequestList, AgentWrite, AiSource,
  } from '@hubtask/sync-engine';

  import AgentDetail from '../lib/agents/AgentDetail.svelte';
  import { actor } from '../lib/data/account.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import {
    SHIPPED_ROLES, agentHaltPath, agentJobsPath, agentPolicyPath, draftOf, heldCounts, type AgentDraft, agentModelsPath, agents, agentsPath, lifecyclePresses, mayDecide, meteringPath, meteringTotals, previewOf, requestsPath,
    retentionOf, scopesFrom, type LifecyclePress, type Viewer,
  } from '../lib/data/agents.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import { homeTabs, recentRuns } from '../lib/product/home.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatDateTime, formatRelative } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import ViewSectionsAsFile from '../lib/settings/ViewSectionsAsFile.svelte';
  import { pagePart } from '../lib/settings/asFile.ts';

  interface Props {
    /** The tab the address names (`?tab=`); the agents where it names none or an unknown one. */
    tab?: string;
    /** Replaces the address when the tab changes, so a reload or a link keeps it. */
    onnavigate?: (path: string) => void;
    /** Where the screen lives: the product's home, the administration's, or the reader's own. */
    base?: string;
    /** The agent whose held acts the address names (`?agent=`), from its card's count. */
    agent?: string;
  }

  let { tab, onnavigate, base = '/administration/agents', agent: heldOf }: Props = $props();
  const isHome = $derived(base === '/agents');

  $effect(() => page.entitle(t('app.agents.title')));

  const administers = $derived(holdsHere('MANAGE_MEMBERS') === true);
  const viewer = $derived<Viewer>({ administers, accountId: actor.account?.id });

  // `models` and `metering` were tabs of their own; an old link lands on the settings that hold them.
  const current = $derived(
    tab === 'requests' || tab === 'runs' ? tab
    : (tab === 'settings' || tab === 'models' || tab === 'metering') && administers ? 'settings'
    : 'agents',
  );
  const hrefOf = (id: string) => (id === 'agents' ? base : `${base}?tab=${id}`);

  const agentsRead = resource<AgentList>(agentsPath);
  const all = $derived(agentsRead.state.status === 'ready' ? agentsRead.state.data.data : []);
  const refusal = $derived(agentsRead.state.status === 'failed' ? renderProblem(agentsRead.state.error, messages).message : undefined);
  const byId = $derived(new Map(all.map((each) => [each.id, each])));
  const nameOfAgent = (agentId: string) => byId.get(agentId)?.name ?? t('app.agents.agent_unread');

  $effect(() => {
    const ids = all.flatMap((each) => [each.owner_id, each.approved_by]).filter((id): id is string => Boolean(id));
    if (ids.length > 0) untrack(() => accounts.resolve(ids));
  });
  const nameOf = (id: string | null | undefined) => (id ? (accounts.nameOf(id) ?? t('app.people.unnamed')) : '—');
  const at = (iso: string | null | undefined) => (iso ? formatDateTime(iso, messages.locale) : '—');

  // One dialog is open at a time; its refusal is said inside it, which keeps what was typed
  // (user-experience.md §5.10). An action outside a dialog says it in a toast.
  let dialogFailure = $state<string | undefined>(undefined);
  async function act<T>(run: () => Promise<T>, done: string, inDialog = false): Promise<T | undefined> {
    if (inDialog) dialogFailure = undefined;
    try {
      const out = await run();
      toast.show({ text: done, tone: 'success' });
      return out;
    } catch (error) {
      const said = renderProblem(error as never, messages).message;
      if (inDialog) dialogFailure = said;
      else toast.show({ text: said, tone: 'danger' });
      return undefined;
    }
  }

  const statusTone = (status: Agent['status']) =>
    status === 'ACTIVE' ? 'success' : status === 'SUSPENDED' ? 'danger' : status === 'RETIRED' ? 'neutral' : 'warning';

  function press(agent: Agent, which: LifecyclePress) {
    void act(() => agents.press(agent.id, which), t(`app.agents.pressed.${which}`, { name: agent.name }));
  }

  // --- The kill switch -------------------------------------------------------------------------

  const haltRead = resource<AgentHalt>(agentHaltPath);
  const halt = $derived(haltRead.state.status === 'ready' ? haltRead.state.data : undefined);
  // Whether MCP takes registered agents only (UC-AGT-02 check 10): absent for whoever may not read it.
  const policyRead = resource<WorkspaceAgentPolicy>(agentPolicyPath);
  const registeredOnly = $derived(policyRead.state.status === 'ready' ? policyRead.state.data['agents.registered_only'] : undefined);

  // --- An agent opened -------------------------------------------------------------------------

  let opened = $state<Agent | undefined>(undefined);
  let isDetailOpen = $state(false);

  function open(agent: Agent) {
    opened = agent;
    isDetailOpen = true;
  }

  // --- Registering or changing an agent --------------------------------------------------------

  const modelsRead = resource<AgentModelList>(agentModelsPath);
  const models = $derived(modelsRead.state.status === 'ready' ? modelsRead.state.data.data : []);
  const modelOptions = $derived<SelectOption[]>([
    { value: '', label: t('app.agents.no_model') },
    ...models.filter((each) => !each.retired).map((each) => ({ value: each.id, label: each.name })),
  ]);
  const modelName = (modelId: string | null) => (modelId ? (models.find((each) => each.id === modelId)?.name ?? t('app.agents.model_unread')) : t('app.agents.no_model'));

  $effect(() => untrack(() => people.openScope({ scopeType: 'TENANT' })));
  const ownerOptions = $derived<SelectOption[]>(
    people.known().map((id) => ({ value: id, label: accounts.nameOf(id) ?? t('app.people.unnamed') })),
  );

  let editing = $state<Agent | undefined>(undefined);
  let draft = $state<AgentDraft | undefined>(undefined);
  let isDraftOpen = $state(false);

  function openDraft(of?: Agent) {
    editing = of;
    draft = draftOf(of, actor.account?.id ?? '');
    dialogFailure = undefined;
    isDraftOpen = true;
  }

  /** A new agent of a shipped role: its kind, its name and what it does, filled in words. */
  function openRole(kind: (typeof SHIPPED_ROLES)[number]) {
    openDraft();
    if (!draft) return;
    draft = { ...draftOf(undefined, actor.account?.id ?? '', kind), name: t(`app.agents.kind.${kind}`), purpose: t(`app.agents.role_purpose.${kind}`) };
  }

  const primary = $derived(
    administers
      ? {
          label: t('app.agents.add_agent'),
          icon: 'plus' as const,
          onclick: () => openDraft(),
          menu: {
            label: t('app.agents.add_from_role'),
            items: SHIPPED_ROLES.map((kind) => ({ id: kind, label: t(`app.agents.kind.${kind}`) })),
            onselect: (id: string) => openRole(id as (typeof SHIPPED_ROLES)[number]),
          },
        }
      : undefined,
  );

  const whole = (text: string) => (/^\d+$/.test(text.trim()) ? Number.parseInt(text, 10) : Number.NaN);
  const draftReason = $derived(
    !draft ? undefined
    : !draft.name.trim() ? t('agents.name_required')
    : !draft.purpose.trim() ? t('agents.purpose_required')
    : [draft.tokens, draft.actions, draft.runs].some((each) => Number.isNaN(whole(each))) ? t('agents.budget_invalid')
    : !(whole(draft.accuracy) >= 0 && whole(draft.accuracy) <= 100) ? t('agents.min_accuracy_invalid')
    : undefined,
  );

  async function saveDraft() {
    const d = draft;
    if (!d) return;
    const body: AgentWrite = {
      name: d.name.trim(),
      purpose: d.purpose.trim(),
      kind: d.kind,
      risk_class: d.risk,
      scopes: scopesFrom(d.scopes),
      policy: { reversible: d.reversible, irreversible: d.irreversible },
      budget: { tokens_per_day: whole(d.tokens), actions_per_hour: whole(d.actions), concurrent_runs: whole(d.runs) },
      min_accuracy: whole(d.accuracy),
      ...(d.owner ? { owner_id: d.owner } : {}),
      ...(d.model ? { model_id: d.model } : {}),
    };
    const done = editing
      ? await act(() => agents.change(editing as Agent, body), t('app.agents.saved'), true)
      : await act(() => agents.register(body), t('app.agents.registered'), true);
    if (done) isDraftOpen = false;
  }

  const kindOptions = $derived<SelectOption[]>(
    (['TICKET_TRIAGE', 'CI_SUGGESTION', 'DUPLICATE_REVIEW', 'KNOWLEDGE_DRAFT', 'EXTERNAL'] as const).map((each) => ({ value: each, label: t(`app.agents.kind.${each}`) })),
  );
  const riskOptions = $derived<SelectOption[]>((['MINIMAL', 'LIMITED'] as const).map((each) => ({ value: each, label: t(`app.agents.risk.${each}`) })));
  const reversibleOptions = $derived<SelectOption[]>((['ALLOW', 'NOTIFY', 'APPROVAL', 'NEVER'] as const).map((each) => ({ value: each, label: t(`app.agents.rule.${each}`) })));
  const irreversibleOptions = $derived<SelectOption[]>((['APPROVAL', 'NEVER'] as const).map((each) => ({ value: each, label: t(`app.agents.rule.${each}`) })));

  // --- Held acts -------------------------------------------------------------------------------

  let requestStatus = $state<AgentRequest['status']>('PENDING');
  const statusOptions = $derived<SelectOption[]>((['PENDING', 'APPROVED', 'DECLINED', 'FAILED'] as const).map((each) => ({ value: each, label: t(`app.agents.request_status.${each}`) })));
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
  const shown = $derived(heldOf ? requests.filter((each) => each.agent_id === heldOf) : requests);

  const tabs = $derived<PlaceTab[]>(
    homeTabs('AGENTS').filter((each) => administers || each.id !== 'settings').map((each) => ({
      id: each.id,
      label: t(each.code),
      href: hrefOf(each.id),
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

  // --- Runs ------------------------------------------------------------------------------------

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
  const runs = $derived(recentRuns(jobsAt, runAgentKey === '' ? [] : runAgentKey.split(','), RUNS_SHOWN));
  const grouped = (value: number) => new Intl.NumberFormat(messages.locale).format(value);
  const runTone = (status: string) => (status === 'FAILED' ? 'danger' : status === 'RUNNING' ? 'info' : 'neutral');
  const runPath = (job: AgentJobList['data'][number]) => (job.target_type === 'HUB' ? `/hubs/${job.target_id}` : `/items/${job.target_id}`);
  const runTarget = (job: AgentJobList['data'][number]) =>
    job.target_type === 'HUB' ? (containers.find(job.target_id)?.name ?? t('app.agents.hub_unread')) : t('app.agents.run_target_entry');

  function decide(request: AgentRequest, approve: boolean) {
    void act(() => agents.decide(request, approve), approve ? t('app.agents.request_approved') : t('app.agents.request_declined'));
  }

  // --- Models ----------------------------------------------------------------------------------

  const sourceRead = resource<AiSource>('/ai-source');
  const offered = $derived(sourceRead.state.status === 'ready' ? sourceRead.state.data.offered : []);

  interface ModelDraft {
    key: string;
    name: string;
    source: 'OWN' | 'OFFERED';
    offered: string;
    model: string;
    region: AgentModel['region'];
    retention: string;
    retired: boolean;
  }
  let modelEditing = $state<AgentModel | undefined>(undefined);
  let modelDraft = $state<ModelDraft | undefined>(undefined);
  let isModelOpen = $state(false);

  function openModel(of?: AgentModel) {
    modelEditing = of;
    modelDraft = {
      key: of?.key ?? '', name: of?.name ?? '', source: of?.source ?? 'OWN', offered: of?.offered_id ?? '',
      model: of?.model ?? '', region: of?.region ?? 'SELF_HOSTED',
      retention: of?.retention_days === null || of?.retention_days === undefined ? '' : String(of.retention_days), retired: of?.retired ?? false,
    };
    dialogFailure = undefined;
    isModelOpen = true;
  }

  const modelReason = $derived(
    !modelDraft ? undefined
    : !modelEditing && !/^[a-z][a-z0-9_-]{0,63}$/.test(modelDraft.key) ? t('app.agents.model_key_hint')
    : !modelDraft.name.trim() || !modelDraft.model.trim() ? t('app.agents.model_needs_name')
    : modelDraft.source === 'OFFERED' && !modelDraft.offered ? t('app.agents.model_needs_offered')
    : modelDraft.retention.trim() !== '' && Number.isNaN(whole(modelDraft.retention)) ? t('app.agents.retention_hint')
    : undefined,
  );

  async function saveModel() {
    const d = modelDraft;
    if (!d) return;
    const common = {
      name: d.name.trim(), source: d.source, model: d.model.trim(), region: d.region,
      ...(d.source === 'OFFERED' && d.offered ? { offered_id: d.offered } : {}),
      ...(d.retention.trim() !== '' ? { retention_days: whole(d.retention) } : {}),
    };
    const done = modelEditing
      ? await act(() => agents.changeModel(modelEditing as AgentModel, { ...common, retired: d.retired }), t('app.agents.model_saved'), true)
      : await act(() => agents.registerModel({ key: d.key, ...common }), t('app.agents.model_registered'), true);
    if (done) isModelOpen = false;
  }

  const sourceOptions = $derived<SelectOption[]>((['OWN', 'OFFERED'] as const).map((each) => ({ value: each, label: t(`app.agents.source.${each}`) })));
  const offeredOptions = $derived<SelectOption[]>(offered.map((each) => ({ value: each.id, label: each.display_name })));
  const regionOptions = $derived<SelectOption[]>((['SELF_HOSTED', 'EEA', 'ADEQUACY', 'ANY'] as const).map((each) => ({ value: each, label: t(`app.agents.region.${each}`) })));

  // --- Metering --------------------------------------------------------------------------------

  const meteringRead = resource<AgentMeteringList>(meteringPath());
  const totals = $derived(meteringRead.state.status === 'ready' ? meteringTotals(meteringRead.state.data.data) : []);
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.agents.title')}
    subtitle={t('app.agents.intro')}
    isTitleInBar={viewport.isCompact}
    {primary}
    breadcrumb={isHome
      ? {
          trail: [
            { id: 'products', label: t('app.nav.products') },
            { id: 'product', label: t('app.agents.title') },
          ],
          label: t('app.product_home.trail'),
          expandLabel: t('app.admin.expand_trail'),
        }
      : {
          trail: [
            { id: 'administration', label: t('app.admin.title'), href: '/administration' },
            { id: 'agents', label: t('app.agents.title') },
          ],
          label: t('app.admin.trail'),
          expandLabel: t('app.admin.expand_trail'),
        }}
  >
    {#snippet figure()}{#if figureText}<span class="figure" data-agents-figure>{figureText}</span>{/if}{/snippet}
  </PageHeader>

  {#if halt?.halted}
    <Banner tone="danger" title={t('app.agents.halted_title')}>
      {t('app.agents.halted_since', { name: nameOf(halt.set_by), at: at(halt.set_at) })}
    </Banner>
  {/if}

  <PlaceTabs label={t('app.agents.title')} {tabs} {current} onnavigate={(id) => onnavigate?.(hrefOf(id))} />

  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if agentsRead.state.status !== 'ready'}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else if current === 'requests'}
    <Stack gap="150">
      <div class="filters">
        <div class="filter"><Select label={t('app.agents.request_status_label')} size="sm" options={statusOptions} bind:value={requestStatus} /></div>
        {#if heldOf}
          <span class="chip">{t('app.agents.held_of', { name: nameOfAgent(heldOf) })}</span>
          <Button size="sm" tone="subtle" icon="x" onclick={() => onnavigate?.(hrefOf('requests'))}>{t('app.agents.held_all')}</Button>
        {/if}
      </div>
      {#if requestsRead.state.status === 'failed'}
        <Banner tone="danger">{renderProblem(requestsRead.state.error, messages).message}</Banner>
      {:else if requestsRead.state.status !== 'ready'}
        <div aria-busy="true"><Skeleton lines={3} /></div>
      {:else if shown.length === 0}
        {#if heldOf && requests.length > 0}
          <ProductEmpty kind="filtered" title={t('app.agents.held_filtered', { name: nameOfAgent(heldOf) })}
            clear={{ label: t('app.agents.held_all'), onclick: () => onnavigate?.(hrefOf('requests')) }} />
        {:else}
          <ProductEmpty kind="settled" icon="check" title={t('app.agents.requests_empty')} />
        {/if}
      {:else}
        <ul class="rows">
          {#each shown as request (request.id)}
            <li class="card">
              <Stack gap="100">
                <div class="head">
                  <strong class="name">{nameOfAgent(request.agent_id)}</strong>
                  <code>{request.use_case}</code>
                  <Badge size="sm" tone={request.class === 'IRREVERSIBLE' ? 'danger' : 'warning'}>{t(`app.agents.class.${request.class}`)}</Badge>
                  <span class="meta">{at(request.created_at)}</span>
                </div>
                <dl class="preview">
                  {#each previewOf(request.input) as line (line.key)}
                    <dt>{line.key}</dt>
                    <dd>{line.value}</dd>
                  {/each}
                </dl>
                {#if request.status === 'PENDING'}
                  {#if mayDecide(byId.get(request.agent_id), viewer)}
                    <div class="presses">
                      <Button tone="primary" size="sm" icon="check" onclick={() => decide(request, true)}>{t('app.agents.approve_act')}</Button>
                      <Button tone="secondary" size="sm" icon="x" onclick={() => decide(request, false)}>{t('app.agents.decline_act')}</Button>
                    </div>
                  {/if}
                {:else}
                  <p class="meta">
                    {t(`app.agents.request_status.${request.status}`)} · {nameOf(request.decided_by)} · {at(request.decided_at)}
                    {#if request.code} · {messages.has(request.code) ? t(request.code) : request.code}{/if}
                  </p>
                {/if}
              </Stack>
            </li>
          {/each}
        </ul>
      {/if}
    </Stack>
  {:else if current === 'runs'}
    {#if !isRunsRead && runs.length === 0}
      <div aria-busy="true"><Skeleton lines={4} /></div>
    {:else if runs.length === 0}
      <ProductEmpty kind="settled" icon="zap" title={t('app.agents.runs_none')} description={t('app.agents.runs_none_body')} />
    {:else}
      <ul class="list" aria-label={t('app.agents.tab_runs')}>
        {#each runs as run (run.id)}
          {@const line = t('app.agents.run_line', { agent: nameOfAgent(run.agent_id), target: runTarget(run) })}
          <li>
            <ListRow href={runPath(run)} onactivate={() => onnavigate?.(runPath(run))} label={line}>
              {#snippet leading()}<Icon name="zap" size="sm" />{/snippet}
              <span class="line" title={line}>{line}</span>
              <span class="sub">
                {[
                  formatRelative(run.started_at, messages.locale),
                  t('app.agents.run_proposals', { count: run.proposals }),
                  t('app.agents.run_tokens', { count: grouped(run.tokens) }),
                  ...(run.code && messages.has(run.code) ? [t(run.code)] : []),
                ].join(' · ')}
              </span>
              {#snippet trailing()}<Badge tone={runTone(run.status)} size="sm">{t(`app.agents.job_status.${run.status}`)}</Badge>{/snippet}
            </ListRow>
          </li>
        {/each}
      </ul>
    {/if}
  {:else if current === 'settings'}
    <Stack gap="300">
      <div class="file"><ViewSectionsAsFile {...pagePart('/administration/agents')} /></div>
      <section class="section" aria-labelledby="agents-every">
        <Stack gap="150">
          <h2 class="heading" id="agents-every">{t('app.agents.settings_every')}</h2>
          <p class="quiet">{t('app.agents.settings_every_body')}</p>
          {#if halt}
            <div>
              <Button
                tone={halt.halted ? 'primary' : 'danger'}
                icon={halt.halted ? 'play' : 'pause'}
                onclick={() => void act(() => agents.halt(!halt.halted), halt.halted ? t('app.agents.released') : t('app.agents.halted'))}
              >
                {halt.halted ? t('app.agents.release_all') : t('app.agents.halt_all')}
              </Button>
            </div>
          {/if}
          {#if registeredOnly}
            <div data-agents-registered-only>
              <Switch
                label={t('app.agents.registered_only')}
                hint={t('app.agents.registered_only_hint')}
                disabledReason={registeredOnly.locked_by ? t('app.agents.registered_only_locked') : undefined}
                bind:checked={
                  () => registeredOnly.value,
                  (on) => void act(() => agents.registeredOnly(on), on ? t('app.agents.registered_only_on') : t('app.agents.registered_only_off'))
                }
              />
            </div>
          {/if}
        </Stack>
      </section>
      <section class="section" aria-labelledby="agents-models">
        <Stack gap="150">
          <div class="sectionhead">
            <h2 class="heading" id="agents-models">{t('app.agents.tab_models')}</h2>
            <Button size="sm" tone="secondary" icon="plus" onclick={() => openModel()}>{t('app.agents.new_model')}</Button>
          </div>
          <p class="quiet">{t('app.agents.models_intro')}</p>
          <Table isStacked
            label={t('app.agents.tab_models')}
            isLabelHidden
            columns={[
              { id: 'name', label: t('app.agents.col_name') },
              { id: 'source', label: t('app.agents.col_source') },
              { id: 'region', label: t('app.agents.col_region') },
              { id: 'retention', label: t('app.agents.col_retention') },
              { id: 'consent', label: t('app.agents.col_consent') },
              { id: 'edit', label: t('app.agents.edit'), isLabelHidden: true },
            ]}
          >
            {#each models as model (model.id)}
              <tr>
                <td>
                  <strong>{model.name}</strong>
                  <span class="meta">{model.key} · {model.model} · {t('app.agents.revision', { revision: model.revision })}{#if model.retired} · {t('app.agents.retired_model')}{/if}</span>
                </td>
                <td>{t(`app.agents.source.${model.source}`)}</td>
                <td>{t(`app.agents.region.${model.region}`)}</td>
                <td>{t(retentionOf(model.retention_days).code, retentionOf(model.retention_days).params)}</td>
                <td>{t('app.agents.consent_by', { name: nameOf(model.consent_by), at: at(model.consent_at) })}</td>
                <td><IconButton icon="pencil" size="sm" tone="subtle" label={t('app.agents.edit_named', { name: model.name })} onclick={() => openModel(model)} /></td>
              </tr>
            {/each}
            {#snippet empty()}{t('app.agents.models_empty')}{/snippet}
          </Table>
        </Stack>
      </section>
      <section class="section" aria-labelledby="agents-metering">
        <Stack gap="150">
          <h2 class="heading" id="agents-metering">{t('app.agents.tab_metering')}</h2>
          <Table isStacked
            label={t('app.agents.metering_caption')}
            columns={[
              { id: 'agent', label: t('app.agents.col_agent') },
              { id: 'allowed', label: t('app.agents.col_allowed'), align: 'end' },
              { id: 'held', label: t('app.agents.col_held'), align: 'end' },
              { id: 'refused', label: t('app.agents.col_refused'), align: 'end' },
              { id: 'tokens', label: t('app.agents.col_tokens'), align: 'end' },
            ]}
          >
            {#each totals as row (row.agentId)}
              <tr>
                <td>{nameOfAgent(row.agentId)}</td>
                <td class="number">{row.allowed}</td>
                <td class="number">{row.held}</td>
                <td class="number">{row.refused}</td>
                <td class="number">{grouped(row.tokens)}</td>
              </tr>
            {/each}
            {#snippet empty()}{t('app.agents.metering_empty')}{/snippet}
          </Table>
        </Stack>
      </section>
    </Stack>
  {:else if all.length === 0}
    <ProductEmpty
      kind="unused"
      icon="zap"
      title={t('app.agents.empty_title')}
      description={t('app.agents.empty_body')}
      create={administers ? { label: t('app.agents.add_agent'), onclick: () => openDraft() } : undefined}
      templates={administers ? SHIPPED_ROLES.slice(0, 3).map((kind) => ({ label: t(`app.agents.kind.${kind}`), onclick: () => openRole(kind) })) : []}
      whoCan={t('app.agents.empty_who')}
    />
  {:else}
    <ul class="cards">
      {#each all as agent (agent.id)}
        {@const waiting = held.get(agent.id) ?? 0}
        <li class="card" data-agent-card>
          <Stack gap="100">
            <div class="head">
              <h2 class="name">{agent.name}</h2>
              <Badge size="sm" tone={statusTone(agent.status)}>{t(`app.agents.status.${agent.status}`)}</Badge>
              {#if agent.needs_approval && agent.status !== 'DRAFT' && agent.status !== 'RETIRED'}
                <Badge size="sm" tone="warning">{t('app.agents.needs_approval')}</Badge>
              {/if}
            </div>
            <p class="meta">{t(`app.agents.kind.${agent.kind}`)} · {t('app.agents.acts_for', { name: nameOf(agent.owner_id) })} · {modelName(agent.model_id)}</p>
            <p class="purpose">{agent.purpose}</p>
            <ul class="facts">
              <li><Icon name="shield" size="sm" />{t('app.agents.policy_line', { reversible: t(`app.agents.rule.${agent.policy.reversible ?? 'APPROVAL'}`), irreversible: t(`app.agents.rule.${agent.policy.irreversible ?? 'APPROVAL'}`) })}</li>
              {#if (agent.budget.tokens_per_day ?? 0) === 0 || (agent.budget.actions_per_hour ?? 0) === 0 || (agent.budget.concurrent_runs ?? 0) === 0}
                <li class="warn" data-agent-budget-zero><Icon name="triangle-alert" size="sm" />{t('app.agents.budget_zero')}</li>
              {/if}
              <li><Icon name="gauge" size="sm" />{t('app.agents.budget_line', { tokens: grouped(agent.budget.tokens_per_day ?? 0), actions: grouped(agent.budget.actions_per_hour ?? 0), runs: grouped(agent.budget.concurrent_runs ?? 0) })}</li>
              {#if agent.approved_at}<li><Icon name="check" size="sm" />{t('app.agents.approved_by', { name: nameOf(agent.approved_by), at: at(agent.approved_at) })}</li>{/if}
            </ul>
            <div class="presses">
              {#if waiting > 0}
                <Button size="sm" tone="primary" icon="clock" onclick={() => onnavigate?.(`${hrefOf('requests')}&agent=${agent.id}`)}>{t('app.agents.held_count', { count: String(waiting) })}</Button>
              {/if}
              <Button size="sm" tone="secondary" onclick={() => open(agent)}>{t('app.agents.open')}</Button>
              {#if administers && agent.status !== 'RETIRED'}
                <IconButton size="sm" tone="subtle" icon="pencil" label={t('app.agents.edit_named', { name: agent.name })} onclick={() => openDraft(agent)} />
              {/if}
              {#each lifecyclePresses(agent, viewer) as which (which)}
                <Button size="sm" tone={which === 'suspend' || which === 'retire' ? 'danger' : 'primary'} onclick={() => press(agent, which)}>
                  {t(`app.agents.press.${which}`)}
                </Button>
              {/each}
            </div>
          </Stack>
        </li>
      {/each}
    </ul>
  {/if}
</Stack>

{#if isDetailOpen && opened}
  <Dialog bind:isOpen={isDetailOpen} title={opened.name} dismissLabel={t('app.agents.close')} isWide>
    {#key opened.id}<AgentDetail agent={byId.get(opened.id) ?? opened} {viewer} />{/key}
  </Dialog>
{/if}

{#if isDraftOpen && draft}
  <Dialog bind:isOpen={isDraftOpen} title={editing ? t('app.agents.edit_named', { name: editing.name }) : t('app.agents.register')} dismissLabel={t('app.agents.cancel')} isWide>
    <Stack gap="150">
      {#if editing && editing.status !== 'DRAFT'}<Banner tone="info">{t('app.agents.change_hint')}</Banner>{/if}
      <Input label={t('app.agents.name')} bind:value={draft.name} maxlength={120} />
      <Textarea label={t('app.agents.purpose')} hint={t('app.agents.purpose_hint')} rows={3} bind:value={draft.purpose} />
      <Select label={t('app.agents.kind_label')} options={kindOptions} bind:value={draft.kind} />
      <Select label={t('app.agents.owner')} hint={t('app.agents.owner_hint')} options={ownerOptions} bind:value={draft.owner} />
      <Select label={t('app.agents.risk_label')} hint={t('app.agents.risk_hint')} options={riskOptions} bind:value={draft.risk} />
      <Input label={t('app.agents.scopes')} hint={t('app.agents.scopes_hint')} bind:value={draft.scopes} />
      <Select label={t('app.agents.model')} options={modelOptions} bind:value={draft.model} />
      <Select label={t('app.agents.reversible')} options={reversibleOptions} bind:value={draft.reversible} />
      <Select label={t('app.agents.irreversible')} options={irreversibleOptions} bind:value={draft.irreversible} />
      <div class="line">
        <Input label={t('app.agents.tokens_per_day')} inputmode="numeric" bind:value={draft.tokens} />
        <Input label={t('app.agents.actions_per_hour')} inputmode="numeric" bind:value={draft.actions} />
        <Input label={t('app.agents.concurrent_runs')} inputmode="numeric" bind:value={draft.runs} />
      </div>
      <p class="meta">{t('app.agents.budget_hint')}</p>
      <Input label={t('app.agents.min_accuracy')} hint={t('app.agents.min_accuracy_hint')} inputmode="numeric" bind:value={draft.accuracy} />
      {#if dialogFailure}<Banner tone="danger">{dialogFailure}</Banner>{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isDraftOpen = false)}>{t('app.agents.cancel')}</Button>
      <Button tone="primary" disabledReason={draftReason} onclick={() => void saveDraft()}>{editing ? t('app.agents.save') : t('app.agents.register')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if isModelOpen && modelDraft}
  <Dialog bind:isOpen={isModelOpen} title={modelEditing ? t('app.agents.edit_named', { name: modelEditing.name }) : t('app.agents.new_model')} dismissLabel={t('app.agents.cancel')}>
    <Stack gap="150">
      <Banner tone="info">{t('app.agents.model_consent')}</Banner>
      {#if !modelEditing}<Input label={t('app.agents.model_key')} hint={t('app.agents.model_key_hint')} bind:value={modelDraft.key} maxlength={64} />{/if}
      <Input label={t('app.agents.name')} bind:value={modelDraft.name} maxlength={120} />
      <Select label={t('app.agents.col_source')} options={sourceOptions} bind:value={modelDraft.source} />
      {#if modelDraft.source === 'OFFERED'}
        <Select label={t('app.agents.offered')} options={offeredOptions} bind:value={modelDraft.offered} />
      {/if}
      <Input label={t('app.agents.model_name')} bind:value={modelDraft.model} maxlength={200} />
      <Select label={t('app.agents.col_region')} options={regionOptions} bind:value={modelDraft.region} />
      <Input label={t('app.agents.col_retention')} hint={t('app.agents.retention_hint')} inputmode="numeric" bind:value={modelDraft.retention} />
      {#if modelEditing}<Switch label={t('app.agents.retire_model')} bind:checked={modelDraft.retired} />{/if}
      {#if dialogFailure}<Banner tone="danger">{dialogFailure}</Banner>{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isModelOpen = false)}>{t('app.agents.cancel')}</Button>
      <Button tone="primary" disabledReason={modelReason} onclick={() => void saveModel()}>{t('app.agents.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .figure { color: var(--text-secondary); font-variant-numeric: tabular-nums; }
  .quiet { margin: 0; color: var(--text-secondary); }
  .meta { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); overflow-wrap: anywhere; }
  .purpose { margin: 0; color: var(--text-secondary); overflow-wrap: anywhere; }
  .rows, .list { margin: 0; padding: 0; list-style: none; display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-100); }
  .list { gap: var(--sp-050); }
  /* Agents are cards (§7.8), as many to a row as fit, one on a phone. */
  .cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--layout-board-column)), 1fr)); gap: var(--sp-150); margin: 0; padding: 0; list-style: none; }
  .card {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
    min-width: 0;
  }
  .head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .name { margin: 0; min-width: 0; overflow-wrap: anywhere; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  .facts { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); color: var(--text-secondary); font-size: var(--fs-075); }
  .facts li { display: flex; align-items: start; gap: var(--sp-050); overflow-wrap: anywhere; }
  .facts .warn { color: var(--text-warning); }
  .presses { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
  .filters { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }
  .filter { max-inline-size: 24ch; }
  .chip { padding: var(--sp-025) var(--sp-100); border-radius: var(--r-full); background: var(--bg-surface-sunken); color: var(--text-primary); font-size: var(--fs-075); }
  .line { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
  .number { text-align: end; font-variant-numeric: tabular-nums; }
  td .meta { display: block; }
  .preview { margin: 0; display: grid; grid-template-columns: minmax(0, max-content) minmax(0, 1fr); gap: var(--sp-025) var(--sp-150); }
  .preview dt { color: var(--text-subtle); font-size: var(--fs-075); font-family: var(--font-mono); }
  .preview dd { margin: 0; overflow-wrap: anywhere; }
  .file { display: flex; justify-content: flex-end; }
  .heading { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .sectionhead { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .sub { display: block; overflow: hidden; color: var(--text-subtle); font-size: var(--fs-075); text-overflow: ellipsis; white-space: nowrap; }
  .list .line { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
