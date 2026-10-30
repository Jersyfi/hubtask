<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Agents* under *Connections*, where the agents module is on (M9): the registered agents with
  // their lifecycle, the acts that wait for a person with what approving would do, the models
  // agents may call with their region, retention and consent, and the metering - with the
  // workspace's kill switch above them (UC-AGT-01 check 9, UC-AGT-02 check 9, UC-AGT-03 check 7).
  //
  // **What the reader may never do is not drawn** (P-05): without MANAGE_MEMBERS the screen shows
  // the agents acting for the reader, lets them stop one and decide what it waits on, and offers
  // no registration, model or switch. The server decides (rule 2).

  import { untrack } from 'svelte';
  import {
    Badge, Banner, Button, Dialog, IconButton, Input, PageHeader, Select, Spinner, Stack, Switch, Table, Tabs, Textarea, type SelectOption,
  } from '@hubtask/design-system/components';
  import type {
    Agent, AgentHalt, WorkspaceAgentPolicy, AgentList, AgentMeteringList, AgentModel, AgentModelList, AgentRequest, AgentRequestList, AgentWrite, AiSource,
  } from '@hubtask/sync-engine';

  import AgentDetail from '../lib/agents/AgentDetail.svelte';
  import { actor } from '../lib/data/account.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import {
    agentHaltPath, agentPolicyPath, agentModelsPath, agents, agentsPath, lifecyclePresses, mayDecide, meteringPath, meteringTotals, previewOf, requestsPath,
    retentionOf, scopesFrom, type LifecyclePress, type Viewer,
  } from '../lib/data/agents.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    /** The tab the address names (`?tab=`); the agents where it names none or an unknown one. */
    tab?: string;
    /** Replaces the address when the tab changes, so a reload or a link keeps it. */
    onnavigate?: (path: string) => void;
  }

  let { tab, onnavigate }: Props = $props();

  $effect(() => page.entitle(t('app.agents.title')));

  const administers = $derived(holdsHere('MANAGE_MEMBERS') === true);
  const viewer = $derived<Viewer>({ administers, accountId: actor.account?.id });

  const TABS = ['agents', 'requests', 'models', 'metering'] as const;
  const current = $derived(TABS.some((each) => each === tab) ? (tab as string) : 'agents');
  const tabs = $derived(
    TABS.filter((each) => administers || each === 'agents' || each === 'requests').map((each) => ({ id: each, label: t(`app.agents.tab_${each}`) })),
  );

  function select(next: string): void {
    onnavigate?.(next === 'agents' ? '/administration/agents' : `/administration/agents?tab=${next}`);
  }

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

  interface Draft {
    name: string;
    purpose: string;
    kind: AgentWrite['kind'];
    owner: string;
    risk: 'MINIMAL' | 'LIMITED';
    scopes: string;
    model: string;
    reversible: 'ALLOW' | 'NOTIFY' | 'APPROVAL' | 'NEVER';
    irreversible: 'APPROVAL' | 'NEVER';
    tokens: string;
    actions: string;
    runs: string;
    accuracy: string;
  }

  let editing = $state<Agent | undefined>(undefined);
  let draft = $state<Draft | undefined>(undefined);
  let isDraftOpen = $state(false);

  function openDraft(of?: Agent) {
    editing = of;
    draft = {
      name: of?.name ?? '',
      purpose: of?.purpose ?? '',
      kind: of?.kind ?? 'TICKET_TRIAGE',
      owner: of?.owner_id ?? actor.account?.id ?? '',
      risk: of?.risk_class ?? 'MINIMAL',
      scopes: (of?.scopes ?? []).join(' '),
      model: of?.model_id ?? '',
      reversible: of?.policy.reversible ?? 'APPROVAL',
      irreversible: of?.policy.irreversible ?? 'APPROVAL',
      tokens: String(of?.budget.tokens_per_day ?? 0),
      actions: String(of?.budget.actions_per_hour ?? 0),
      runs: String(of?.budget.concurrent_runs ?? 0),
      accuracy: String(of?.min_accuracy ?? 0),
    };
    isDraftOpen = true;
  }

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
      ? await act(() => agents.change(editing as Agent, body), t('app.agents.saved'))
      : await act(() => agents.register(body), t('app.agents.registered'));
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
      ? await act(() => agents.changeModel(modelEditing as AgentModel, { ...common, retired: d.retired }), t('app.agents.model_saved'))
      : await act(() => agents.registerModel({ key: d.key, ...common }), t('app.agents.model_registered'));
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
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'agents', label: t('app.agents.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

  <p class="quiet">{t('app.agents.intro')}</p>

  {#if halt?.halted}
    <Banner tone="danger" title={t('app.agents.halted_title')}>
      {t('app.agents.halted_since', { name: nameOf(halt.set_by), at: at(halt.set_at) })}
    </Banner>
  {/if}
  {#if administers && halt}
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
  {#if administers && registeredOnly}
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

  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if agentsRead.state.status !== 'ready'}
    <Spinner label={t('app.agents.reading')} />
  {:else}
    <Tabs label={t('app.agents.title')} {tabs} selected={current} onselect={select}>
      {#if current === 'requests'}
        <Stack gap="150">
          <div class="filter"><Select label={t('app.agents.request_status_label')} size="sm" options={statusOptions} bind:value={requestStatus} /></div>
          {#if requestsRead.state.status === 'failed'}
            <Banner tone="danger">{renderProblem(requestsRead.state.error, messages).message}</Banner>
          {:else}
            <ul class="rows">
              {#each requests as request (request.id)}
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
              {:else}
                <li class="quiet">{t('app.agents.requests_empty')}</li>
              {/each}
            </ul>
          {/if}
        </Stack>
      {:else if current === 'models' && administers}
        <Stack gap="150">
          <p class="quiet">{t('app.agents.models_intro')}</p>
          <div><Button icon="plus" onclick={() => openModel()}>{t('app.agents.new_model')}</Button></div>
          <Table
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
      {:else if current === 'metering' && administers}
        <Table
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
              <td class="number">{row.tokens}</td>
            </tr>
          {/each}
          {#snippet empty()}{t('app.agents.metering_empty')}{/snippet}
        </Table>
      {:else}
        <Stack gap="150">
          {#if administers}<div><Button icon="plus" onclick={() => openDraft()}>{t('app.agents.register')}</Button></div>{/if}
          <ul class="rows">
            {#each all as agent (agent.id)}
              <li class="card">
                <Stack gap="100">
                  <div class="head">
                    <h2 class="name">{agent.name}</h2>
                    <Badge size="sm" tone={statusTone(agent.status)}>{t(`app.agents.status.${agent.status}`)}</Badge>
                    {#if agent.needs_approval && agent.status !== 'DRAFT' && agent.status !== 'RETIRED'}
                      <Badge size="sm" tone="warning">{t('app.agents.needs_approval')}</Badge>
                    {/if}
                  </div>
                  <p class="quiet">{agent.purpose}</p>
                  <p class="meta">
                    {t(`app.agents.kind.${agent.kind}`)} · {t('app.agents.acts_for', { name: nameOf(agent.owner_id) })} · {t(`app.agents.risk.${agent.risk_class}`)}
                    · {modelName(agent.model_id)}
                    {#if agent.approved_at} · {t('app.agents.approved_by', { name: nameOf(agent.approved_by), at: at(agent.approved_at) })}{/if}
                  </p>
                  <p class="meta">
                    {t('app.agents.policy_line', { reversible: t(`app.agents.rule.${agent.policy.reversible ?? 'APPROVAL'}`), irreversible: t(`app.agents.rule.${agent.policy.irreversible ?? 'APPROVAL'}`) })}
                    {#if agent.scopes.length > 0} · {agent.scopes.join(', ')}{/if}
                  </p>
                  <div class="presses">
                    <Button size="sm" tone="secondary" onclick={() => open(agent)}>{t('app.agents.open')}</Button>
                    {#if administers && agent.status !== 'RETIRED'}
                      <Button size="sm" tone="subtle" icon="pencil" onclick={() => openDraft(agent)}>{t('app.agents.edit')}</Button>
                    {/if}
                    {#each lifecyclePresses(agent, viewer) as which (which)}
                      <Button size="sm" tone={which === 'suspend' || which === 'retire' ? 'danger' : 'primary'} onclick={() => press(agent, which)}>
                        {t(`app.agents.press.${which}`)}
                      </Button>
                    {/each}
                  </div>
                </Stack>
              </li>
            {:else}
              <li class="quiet">{t('app.agents.none')}</li>
            {/each}
          </ul>
        </Stack>
      {/if}
    </Tabs>
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
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isModelOpen = false)}>{t('app.agents.cancel')}</Button>
      <Button tone="primary" disabledReason={modelReason} onclick={() => void saveModel()}>{t('app.agents.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .meta { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); overflow-wrap: anywhere; }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }
  .card {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }
  .head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .name { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  .presses { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
  .filter { max-inline-size: 24ch; }
  .line { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
  .number { text-align: end; font-variant-numeric: tabular-nums; }
  td .meta { display: block; }
  .preview { margin: 0; display: grid; grid-template-columns: minmax(0, max-content) minmax(0, 1fr); gap: var(--sp-025) var(--sp-150); }
  .preview dt { color: var(--text-subtle); font-size: var(--fs-075); font-family: var(--font-mono); }
  .preview dd { margin: 0; overflow-wrap: anywhere; }
</style>
