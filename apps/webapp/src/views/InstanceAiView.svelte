<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The AI models this installation offers every workspace (ADR-0072 §1, UC-AI-06 check 1).
  //
  // **Offered, not on.** A model added here is something every workspace may choose; it sends
  // nothing until a workspace switches it on, and that switch is the workspace's consent. So this
  // screen has no per-workspace control, and which workspaces may use offered models at all is the
  // `ai` area of the defaults, not a row here.
  //
  // **What a workspace sees is the name, the processor and where.** The address, the models and the
  // key stay the operator's; the form says so where they are typed.
  //
  // **The fields are the one model form** (`ModelFields`), a workspace's own model's too; the
  // installation adds the name and the processor a workspace chooses by.

  import { untrack } from 'svelte';

  import { Banner, Button, DataTable, Dialog, EmptyState, FilterBar, Input, ListPage, Spinner, Stack, type DataColumn } from '@hubtask/design-system/components';
  import type { AiJurisdiction, AiProviderKind, OfferedAiModel } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import ModelFields from '../lib/ai/ModelFields.svelte';
  import { emptyModel, modelBody, modelOf } from '../lib/ai/model.ts';
  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { isOfferedAiReachable } from '../lib/instance/values.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  let acting = $state<{ kind: 'add' | 'change' | 'remove'; model?: OfferedAiModel } | undefined>(undefined);
  let working = $state<string | undefined>(undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  let displayName = $state('');
  let processor = $state('');
  let draft = $state(emptyModel('OPENAI_COMPATIBLE', 'EEA'));

  $effect(() => {
    if (!instance.isElevated) return;
    const stop = untrack(() => instance.openOfferedAi());
    return stop;
  });

  // The defaults too: an offer *Workspaces may use* leaves out is in no workspace's choice, and
  // a screen that listed it without saying so would make the offer look done (P-05).
  $effect(() => {
    const stop = untrack(() => instance.openSettings());
    return stop;
  });

  const reading = $derived(instance.offeredAiState);
  const models = $derived(instance.offeredAi);
  const isReachable = $derived(isOfferedAiReachable(instance.settings.status === 'ready' ? instance.settings.data : undefined));
  const unreadable = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);

  const columns = $derived<DataColumn[]>([
    { id: 'model', label: t('app.instance.ai_column_model'), priority: 1 },
    { id: 'where', label: t('app.instance.ai_column_where'), priority: 2 },
    { id: 'models', label: t('app.instance.ai_column_models'), priority: 3 },
  ]);

  let search = $state('');
  const shown = $derived(models.filter((model) => matchesSearch(search, [model.display_name, model.processor])));

  /** A model that calls nothing is no model to offer. */
  const KINDS: readonly AiProviderKind[] = ['OPENAI_COMPATIBLE', 'OLLAMA'];

  /** Where a model processes content, in words - never the contract's value. */
  const where = (value: AiJurisdiction): string => t(`app.ai.jurisdiction_${value.toLowerCase()}`);

  function open(next: 'add' | 'change' | 'remove', model?: OfferedAiModel): void {
    acting = { kind: next, model };
    failure = undefined;
    if (next === 'add') {
      displayName = '';
      processor = '';
      draft = emptyModel('OPENAI_COMPATIBLE', 'EEA');
    }
    if (next === 'change' && model) {
      displayName = model.display_name;
      processor = model.processor;
      draft = modelOf(model);
    }
  }

  async function run(id: string, call: () => Promise<unknown>): Promise<void> {
    working = id;
    failure = undefined;
    try {
      await call();
      acting = undefined;
      draft.apiKey = '';
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('app.something_went_wrong'), fields: new Map(), isServerFault: true };
    } finally {
      working = undefined;
    }
  }

  function body(): Parameters<typeof instance.configureOfferedAi>[0] {
    // The address is required here, so it is always sent.
    return { display_name: displayName.trim(), processor: processor.trim(), ...modelBody(draft), base_url: draft.baseUrl.trim() };
  }

  $effect(() => page.entitle(t('app.instance.ai')));
</script>

<InstanceGate onleave={() => onnavigate('/')}>
  {#snippet children(door)}
    <ListPage
      head={{
        title: t('app.instance.ai'),
        breadcrumb: installationTrail({ id: 'screen', label: t('app.instance.ai') }),
        primary: { label: t('app.instance.ai_add'), icon: 'plus', onclick: () => open('add') },
      }}
      refused={door ?? (unreadable ? failed : undefined)}
    >
      {#snippet intro()}
        <Stack gap="100">
          <p class="quiet">{t('app.instance.ai_intro')}</p>
          {#if models.length > 0 && isReachable === false}
            <Banner tone="warning" title={t('app.instance.ai_unreachable_title')}>
              {t('app.instance.ai_unreachable_body', { setting: t('app.instance.setting.ai.sources') })}
              {#snippet action()}
                <Button size="sm" tone="secondary" onclick={() => onnavigate('/instance/settings/ai')}>{t('app.instance.ai_unreachable_action')}</Button>
              {/snippet}
            </Banner>
          {/if}
        </Stack>
      {/snippet}
      {#snippet filters()}
        <FilterBar
          searchLabel={t('app.instance.ai_search')}
          clearSearchLabel={t('app.page.clear_search')}
          bind:search
          countLabel={reading.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(models.length) }) : undefined}
        />
      {/snippet}

      {#if failure && !acting}<FailureNote problem={failure} />{/if}
      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet"><Spinner label={t('app.instance.reading')} /><span>{t('app.instance.reading')}</span></p>
      {:else}
        <DataTable
          label={t('app.instance.ai')}
          {columns}
          rows={shown}
          key={(model: OfferedAiModel) => model.id}
          onopen={(model: OfferedAiModel) => open('change', model)}
          rowLabel={(model: OfferedAiModel) => model.display_name}
          menu={(model: OfferedAiModel) => ({
            label: t('app.page.more_for', { name: model.display_name }),
            items: [
              { id: 'change', label: `${t('app.instance.ai_change')}…` },
              { id: 'remove', label: `${t('app.instance.ai_remove')}…`, isDestructive: true, hasSeparatorBefore: true },
            ],
          })}
          onmenu={(model: OfferedAiModel, id: string) => open(id as 'change' | 'remove', model)}
          actionsLabel={t('app.page.row_actions')}
          sheetDismissLabel={t('app.page.sheet_close')}
          sheetGrabLabel={t('app.page.sheet_grab')}
          shownLabel={models.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
        >
          {#snippet cell(model: OfferedAiModel, column: DataColumn)}
            {#if column.id === 'model'}
              <span class="name">{model.display_name}</span>
              <span class="slug">{model.processor}</span>
            {:else if column.id === 'where'}
              {where(model.jurisdiction)}
            {:else}
              {#if model.completion_model}<span class="slug">{t('app.instance.ai_completion', { model: model.completion_model })}</span>{/if}
              {#if model.embedding_model}<span class="slug">{t('app.instance.ai_embedding', { model: model.embedding_model })}</span>{/if}
              <span class="slug">{model.has_api_key ? t('app.instance.ai_key_stored') : t('app.instance.ai_key_none')}</span>
            {/if}
          {/snippet}
          {#snippet empty()}
            {#if models.length === 0}
              <EmptyState kind="unused" title={t('app.instance.ai_none')} />
            {:else}
              <EmptyState kind="filtered" title={t('app.instance.ai_none_found')}>
                {#snippet action()}<Button onclick={() => (search = '')}>{t('app.page.clear')}</Button>{/snippet}
              </EmptyState>
            {/if}
          {/snippet}
        </DataTable>
      {/if}
    </ListPage>
  {/snippet}
</InstanceGate>

{#snippet failed()}
  {#if unreadable}<FailureNote problem={unreadable} />{/if}
{/snippet}

<Dialog
  size="l"
  title={acting?.kind === 'change' ? t('app.instance.ai_change_title') : t('app.instance.ai_add_title')}
  isOpen={acting?.kind === 'add' || acting?.kind === 'change'}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (acting = undefined)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (acting = undefined)}>{t('app.instance.cancel')}</Button>
    <Button
      tone="primary"
      isBusy={working === 'configure'}
      busyLabel={t('app.instance.working')}
      onclick={() => void run('configure', () => instance.configureOfferedAi(body(), acting?.model?.id))}
    >
      {t('app.instance.ai_save')}
    </Button>
  {/snippet}
  <Stack gap="150">
    {#if failure}
      <FailureNote problem={failure} />
    {/if}
    <p class="prose">{t('app.instance.ai_form_intro')}</p>
    <ModelFields
      bind:draft
      kinds={KINDS}
      hasKey={acting?.kind === 'change' && acting.model?.has_api_key === true}
      isAddressRequired
      errors={failure?.fields}
    >
      {#snippet before()}
        <Input label={t('app.instance.ai_name_label')} hint={t('app.instance.ai_name_hint')} bind:value={displayName} error={failure?.fields.get('/display_name')} autocomplete="off" isRequired />
        <Input label={t('app.instance.ai_processor_label')} hint={t('app.instance.ai_processor_hint')} bind:value={processor} error={failure?.fields.get('/processor')} autocomplete="off" isRequired />
      {/snippet}
      {#snippet more()}
        <p class="quiet-line">{t('app.instance.ai_operator_only')}</p>
      {/snippet}
    </ModelFields>
  </Stack>
</Dialog>

<Dialog
  size="s"
  title={t('app.instance.ai_remove_title')}
  isOpen={acting?.kind === 'remove'}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (acting = undefined)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (acting = undefined)}>{t('app.instance.keep')}</Button>
    <Button
      tone="danger"
      isBusy={working === 'remove'}
      busyLabel={t('app.instance.working')}
      onclick={() => void run('remove', () => instance.removeOfferedAi(acting?.model?.id ?? ''))}
    >
      {t('app.instance.ai_remove')}
    </Button>
  {/snippet}
  <Stack gap="150">
    {#if failure}<FailureNote problem={failure} />{/if}
    <Banner tone="warning">{t('app.instance.ai_remove_cost', { name: acting?.model?.display_name ?? '' })}</Banner>
  </Stack>
</Dialog>

<style>
  .prose { margin: 0; max-inline-size: 60ch; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .quiet-line { margin: 0; max-inline-size: 60ch; color: var(--text-secondary); font-size: var(--fs-100); }
  .name { display: block; font-weight: var(--fw-medium); }
  .slug { display: block; color: var(--text-secondary); font-size: var(--fs-100); }
</style>
