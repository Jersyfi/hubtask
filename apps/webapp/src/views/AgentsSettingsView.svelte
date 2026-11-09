<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The agents' settings (page-system §9.6): a settings page of *Every agent* (the kill switch and
  // MCP's registered-only), *Models* and *Metering*, each at `/agents/settings/<id>` - in Your
  // settings at `<base>?tab=settings&section=<id>`. `?tab=models` and `?tab=metering`, tabs of
  // their own once, land on their section. For whoever
  // administers agents (UC-AGT-01 check 9, UC-AGT-02 check 10, UC-AGT-03 check 7); the server
  // decides (rule 2).

  import { Badge, Button, DataTable, EmptyState, SettingRow, SettingsLayout, SettingsSection, Switch, type DataColumn, type SectionLink } from '@hubtask/design-system/components';
  import type { AgentHalt, AgentList, AgentMeteringList, AgentModel, AgentModelList, WorkspaceAgentPolicy } from '@hubtask/sync-engine';

  import ModelDialog from '../lib/agents/ModelDialog.svelte';
  import { AGENTS_HOME, agentsTrail } from '../lib/agents/trail.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { agentHaltPath, agentModelsPath, agentPolicyPath, agents, agentsPath, meteringPath, meteringTotals, retentionOf } from '../lib/data/agents.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { resource, resourceWhen } from '../lib/data/resource.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import TableSection from '../lib/page/TableSection.svelte';
  import { renderProblem } from '../lib/problem.ts';
  import ViewSectionsAsFile from '../lib/settings/ViewSectionsAsFile.svelte';
  import { pagePart } from '../lib/settings/asFile.ts';
  import { viewAsFileAction } from '../lib/settings/viewasfile.svelte.ts';
  import ProductTabs from '../lib/product/ProductTabs.svelte';
  import { productHead } from '../lib/product/frame.svelte.ts';

  interface Props {
    /** The tab the address names: `settings`, or `models`/`metering` from an older link. */
    tab?: string;
    /** The section the address names; none is the list of sections. */
    section?: string;
    base?: string;
    /** Replaces the address: a move between sections. */
    onreplace?: (path: string) => void;
    onnavigate?: (path: string) => void;
  }

  const { tab, section, base = AGENTS_HOME, onreplace, onnavigate }: Props = $props();

  const title = $derived(t('app.agents.settings_title'));
  $effect(() => page.entitle(title));

  const administers = $derived(holdsHere('MANAGE_MEMBERS') === true);
  const isHome = $derived(base === AGENTS_HOME);
  const here = $derived(isHome ? `${AGENTS_HOME}/settings` : `${base}?tab=settings`);
  const sectionAt = (id: string) => (isHome ? `${here}/${id}` : `${here}&section=${id}`);

  const SECTIONS = [
    { id: 'every', code: 'app.agents.settings_every' },
    { id: 'models', code: 'app.agents.tab_models' },
    { id: 'metering', code: 'app.agents.tab_metering' },
  ] as const;
  const isKnown = (id: string | undefined) => id !== undefined && SECTIONS.some((each) => each.id === id);
  const named = $derived(isKnown(section) ? section : tab === 'models' || tab === 'metering' ? tab : undefined);
  const shown = $derived(named ?? 'every');
  const view = $derived<'list' | 'detail'>(named ? 'detail' : 'list');

  const haltRead = resourceWhen<AgentHalt>(() => (administers ? agentHaltPath : undefined));
  const halt = $derived(haltRead.state.status === 'ready' ? haltRead.state.data : undefined);
  const policyRead = resourceWhen<WorkspaceAgentPolicy>(() => (administers ? agentPolicyPath : undefined));
  const registeredOnly = $derived(policyRead.state.status === 'ready' ? policyRead.state.data['agents.registered_only'] : undefined);
  const modelsRead = resourceWhen<AgentModelList>(() => (administers ? agentModelsPath : undefined));
  const models = $derived(modelsRead.state.status === 'ready' ? modelsRead.state.data.data : []);
  const meteringRead = resourceWhen<AgentMeteringList>(() => (administers ? meteringPath() : undefined));
  const totals = $derived(meteringRead.state.status === 'ready' ? meteringTotals(meteringRead.state.data.data) : []);
  const agentsRead = resource<AgentList>(agentsPath);
  const nameOfAgent = (id: string) =>
    (agentsRead.state.status === 'ready' ? agentsRead.state.data.data.find((each) => each.id === id)?.name : undefined) ?? t('app.agents.agent_unread');

  $effect(() => {
    const ids = [halt?.set_by, ...models.map((each) => each.consent_by)].filter((id): id is string => Boolean(id));
    if (ids.length > 0) accounts.resolve(ids);
  });
  const nameOf = (id: string | null | undefined) => (id ? (accounts.nameOf(id) ?? t('app.people.unnamed')) : '—');
  const at = (iso: string | null | undefined) => (iso ? formatDateTime(iso, messages.locale) : '—');
  const grouped = (value: number) => new Intl.NumberFormat(messages.locale).format(value);

  async function act(run: () => Promise<unknown>, done: string) {
    try {
      await run();
      toast.show({ text: done, tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }

  let modelOpened = $state<AgentModel | null | undefined>(undefined);
  let isFileOpen = $state(false);
  const fileAction = $derived(viewAsFileAction(() => (isFileOpen = true)));

  const modelColumns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.agents.col_name'), priority: 1 },
    { id: 'source', label: t('app.agents.col_source'), priority: 2 },
    { id: 'region', label: t('app.agents.col_region'), priority: 2 },
    { id: 'retention', label: t('app.agents.col_retention'), priority: 3 },
    { id: 'consent', label: t('app.agents.col_consent'), priority: 3 },
  ]);
  type Total = ReturnType<typeof meteringTotals>[number];
  const meteringColumns = $derived<DataColumn[]>([
    { id: 'agent', label: t('app.agents.col_agent'), priority: 1 },
    { id: 'allowed', label: t('app.agents.col_allowed'), align: 'end' },
    { id: 'held', label: t('app.agents.col_held'), align: 'end' },
    { id: 'refused', label: t('app.agents.col_refused'), align: 'end' },
    { id: 'tokens', label: t('app.agents.col_tokens'), align: 'end', priority: 2 },
  ]);

  function statusOf(id: string): Pick<SectionLink, 'status' | 'statusTone'> {
    if (id === 'every' && halt) return halt.halted ? { status: t('app.agents.status_halted'), statusTone: 'danger' } : { status: t('app.agents.status_acting'), statusTone: 'set' };
    if (id === 'models' && modelsRead.state.status === 'ready') return { status: String(models.length) };
    return {};
  }
  const links = $derived<SectionLink[]>(SECTIONS.map((each) => ({ id: each.id, label: t(each.code), href: sectionAt(each.id), ...statusOf(each.id) })));
</script>

{#snippet tabs()}<ProductTabs module="AGENTS" current="settings" {onnavigate} />{/snippet}

<!-- The agents' last tab (the navigation concept §4): the home's head and `ViewBar` stay. -->
<SettingsLayout
  head={{ ...productHead('AGENTS'), breadcrumb: agentsTrail(base), secondary: fileAction ? ([fileAction] as const) : ([] as const) }}
  tabs={isHome ? tabs : undefined}
  nav={{ label: t('app.settings_page.sections', { name: title }), sections: links, current: shown, onselect: (next) => onreplace?.(sectionAt(next)) }}
  {view}
  back={{ label: t('app.product_home.tab_settings'), onback: () => onreplace?.(here) }}
>
  {#if shown === 'every'}
    <SettingsSection title={t('app.agents.settings_every')} description={t('app.agents.settings_every_body')}>
      {#if halt}
        <SettingRow label={t('app.agents.halt_row')} help={halt.halted ? t('app.agents.halted_since', { name: nameOf(halt.set_by), at: at(halt.set_at) }) : t('app.agents.halt_help')}>
          {#snippet control()}
            <Button
              tone={halt.halted ? 'primary' : 'danger'}
              icon={halt.halted ? 'play' : 'pause'}
              onclick={() => void act(() => agents.halt(!halt.halted), halt.halted ? t('app.agents.released') : t('app.agents.halted'))}
            >
              {halt.halted ? t('app.agents.release_all') : t('app.agents.halt_all')}
            </Button>
          {/snippet}
        </SettingRow>
      {/if}
      {#if registeredOnly}
        <div data-agents-registered-only>
          <SettingRow label={t('app.agents.registered_only')} help={t('app.agents.registered_only_hint')} lockReason={registeredOnly.locked_by ? t('app.agents.registered_only_locked') : undefined}>
            {#snippet control()}
              <Switch
                label={t('app.agents.registered_only')}
                isLabelHidden
                disabledReason={registeredOnly.locked_by ? t('app.agents.registered_only_locked') : undefined}
                bind:checked={
                  () => registeredOnly.value,
                  (on) => void act(() => agents.registeredOnly(on), on ? t('app.agents.registered_only_on') : t('app.agents.registered_only_off'))
                }
              />
            {/snippet}
          </SettingRow>
        </div>
      {/if}
    </SettingsSection>
  {:else if shown === 'models'}
    <TableSection title={t('app.agents.tab_models')} note={String(models.length)} description={t('app.agents.models_intro')}>
      {#snippet acts()}<Button icon="plus" data-list-add onclick={() => (modelOpened = null)}>{t('app.agents.new_model')}</Button>{/snippet}
      {#if modelsRead.state.status === 'failed'}
        <FailureNote problem={renderProblem(modelsRead.state.error, messages)} />
      {:else}
        <DataTable
          label={t('app.agents.tab_models')}
          columns={modelColumns}
          rows={models}
          key={(model: AgentModel) => model.id}
          onopen={(model: AgentModel) => (modelOpened = model)}
          rowLabel={(model: AgentModel) => model.name}
        >
          {#snippet cell(model: AgentModel, column: DataColumn)}
            {#if column.id === 'name'}
              <span class="name">{model.name}{#if model.retired} <Badge size="sm" tone="neutral">{t('app.agents.retired_model')}</Badge>{/if}</span>
              <span class="sub">{model.key} · {model.model} · {t('app.agents.revision', { revision: model.revision })}</span>
            {:else if column.id === 'source'}
              {t(`app.agents.source.${model.source}`)}
            {:else if column.id === 'region'}
              {t(`app.agents.region.${model.region}`)}
            {:else if column.id === 'retention'}
              {t(retentionOf(model.retention_days).code, retentionOf(model.retention_days).params)}
            {:else}
              <span class="sub">{t('app.agents.consent_by', { name: nameOf(model.consent_by), at: at(model.consent_at) })}</span>
            {/if}
          {/snippet}
          {#snippet empty()}<EmptyState kind="unused" title={t('app.agents.models_empty')} />{/snippet}
        </DataTable>
      {/if}
    </TableSection>
  {:else}
    <TableSection title={t('app.agents.tab_metering')} description={t('app.agents.metering_caption')}>
      <DataTable
        label={t('app.agents.metering_caption')}
        columns={meteringColumns}
        rows={totals}
        key={(row: Total) => row.agentId}
        rowLabel={(row: Total) => nameOfAgent(row.agentId)}
      >
        {#snippet cell(row: Total, column: DataColumn)}
          {#if column.id === 'agent'}
            {nameOfAgent(row.agentId)}
          {:else if column.id === 'allowed'}
            <span class="number">{row.allowed}</span>
          {:else if column.id === 'held'}
            <span class="number">{row.held}</span>
          {:else if column.id === 'refused'}
            <span class="number">{row.refused}</span>
          {:else}
            <span class="number">{grouped(row.tokens)}</span>
          {/if}
        {/snippet}
        {#snippet empty()}<EmptyState kind="settled" title={t('app.agents.metering_empty')} />{/snippet}
      </DataTable>
    </TableSection>
  {/if}
</SettingsLayout>

<ViewSectionsAsFile {...pagePart('/agents/settings')} bind:isOpen={isFileOpen} isTriggerHidden />
<ModelDialog opened={modelOpened} onclose={() => (modelOpened = undefined)} />

<style>
  .name { display: block; font-weight: var(--fw-medium); }
  .sub { display: block; color: var(--text-subtle); font-size: var(--fs-075); overflow-wrap: anywhere; }
  .number { font-variant-numeric: tabular-nums; }
</style>
