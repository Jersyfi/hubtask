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
  // **The key is written and never read.** Sealed on the way in; leaving it empty on a change keeps
  // the sealed one.

  import { untrack } from 'svelte';

  import { Banner, Button, Dialog, Input, PageHeader, Select, Spinner, Stack, Table } from '@hubtask/design-system/components';
  import type { AiJurisdiction, AiProviderKind, OfferedAiModel } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  let acting = $state<{ kind: 'add' | 'change' | 'remove'; model?: OfferedAiModel } | undefined>(undefined);
  let working = $state<string | undefined>(undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  let displayName = $state('');
  let processor = $state('');
  let kind = $state<AiProviderKind>('OPENAI_COMPATIBLE');
  let baseUrl = $state('');
  let completionModel = $state('');
  let embeddingModel = $state('');
  let apiKey = $state('');
  let jurisdiction = $state<AiJurisdiction>('EEA');

  $effect(() => {
    const stop = untrack(() => instance.openOfferedAi());
    return stop;
  });

  const reading = $derived(instance.offeredAiState);
  const models = $derived(instance.offeredAi);
  const unreadable = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);

  const columns = $derived([
    { id: 'model', label: t('app.instance.ai_column_model') },
    { id: 'where', label: t('app.instance.ai_column_where') },
    { id: 'models', label: t('app.instance.ai_column_models') },
    { id: 'actions', label: t('app.instance.column_actions'), isLabelHidden: true },
  ]);

  const kinds: { value: AiProviderKind; label: string }[] = [
    { value: 'OPENAI_COMPATIBLE', label: t('app.ai.kind_openai_compatible') },
    { value: 'OLLAMA', label: t('app.ai.kind_ollama') },
  ];
  const jurisdictions: { value: AiJurisdiction; label: string }[] = [
    { value: 'SELF_HOSTED', label: t('app.ai.jurisdiction_self_hosted') },
    { value: 'EEA', label: t('app.ai.jurisdiction_eea') },
    { value: 'ADEQUACY', label: t('app.ai.jurisdiction_adequacy') },
    { value: 'THIRD_COUNTRY', label: t('app.ai.jurisdiction_third_country') },
  ];

  /** Where a model processes content, in words - never the contract's value. */
  const where = (value: AiJurisdiction): string =>
    jurisdictions.find((each) => each.value === value)?.label ?? '';

  function open(next: 'add' | 'change' | 'remove', model?: OfferedAiModel): void {
    acting = { kind: next, model };
    failure = undefined;
    if (next === 'add') {
      displayName = '';
      processor = '';
      kind = 'OPENAI_COMPATIBLE';
      baseUrl = '';
      completionModel = '';
      embeddingModel = '';
      apiKey = '';
      jurisdiction = 'EEA';
    }
    if (next === 'change' && model) {
      displayName = model.display_name;
      processor = model.processor;
      kind = model.kind;
      baseUrl = model.base_url ?? '';
      completionModel = model.completion_model ?? '';
      embeddingModel = model.embedding_model ?? '';
      apiKey = '';
      jurisdiction = model.jurisdiction;
    }
  }

  async function run(id: string, call: () => Promise<unknown>): Promise<void> {
    working = id;
    failure = undefined;
    try {
      await call();
      acting = undefined;
      apiKey = '';
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
    } finally {
      working = undefined;
    }
  }

  function body(): Parameters<typeof instance.configureOfferedAi>[0] {
    return {
      display_name: displayName.trim(),
      processor: processor.trim(),
      kind,
      base_url: baseUrl.trim(),
      ...(completionModel.trim() ? { completion_model: completionModel.trim() } : {}),
      ...(embeddingModel.trim() ? { embedding_model: embeddingModel.trim() } : {}),
      // Absent keeps the sealed key; the field is never sent empty from here.
      ...(apiKey ? { api_key: apiKey } : {}),
      jurisdiction,
    };
  }

  $effect(() => page.entitle(t('app.instance.ai')));
</script>

<Stack gap="300">
  <PageHeader title={t('app.instance.ai')} isTitleInBar={viewport.isCompact} />

  <InstanceGate onleave={() => onnavigate('/')}>
    <Stack gap="200">
      <p class="prose">{t('app.instance.ai_intro')}</p>

      {#if failure && !acting}
        <Banner tone="danger" title={failure.message}>
          {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
        </Banner>
      {/if}

      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet">
          <Spinner label={t('app.instance.reading')} />
          <span>{t('app.instance.reading')}</span>
        </p>
      {:else if unreadable}
        <Banner tone="danger" title={unreadable.message}>
          {#if unreadable.reference}{t('app.error_reference', { request_id: unreadable.reference })}{/if}
        </Banner>
      {:else}
        <div>
          <Button tone="primary" onclick={() => open('add')}>{t('app.instance.ai_add')}</Button>
        </div>

        {#if models.length === 0}
          <Banner tone="info">{t('app.instance.ai_none')}</Banner>
        {:else}
          <Table label={t('app.instance.ai')} isLabelHidden columns={columns}>
            {#each models as model (model.id)}
              <tr>
                <td>
                  <span class="name">{model.display_name}</span>
                  <span class="slug">{model.processor}</span>
                </td>
                <td>{where(model.jurisdiction)}</td>
                <td>
                  {#if model.completion_model}<span class="slug">{t('app.instance.ai_completion', { model: model.completion_model })}</span>{/if}
                  {#if model.embedding_model}<span class="slug">{t('app.instance.ai_embedding', { model: model.embedding_model })}</span>{/if}
                  <span class="slug">{model.has_api_key ? t('app.instance.ai_key_stored') : t('app.instance.ai_key_none')}</span>
                </td>
                <td>
                  <div class="row">
                    <Button tone="subtle" onclick={() => open('change', model)}>{t('app.instance.ai_change')}</Button>
                    <Button tone="danger" onclick={() => open('remove', model)}>{t('app.instance.ai_remove')}</Button>
                  </div>
                </td>
              </tr>
            {/each}
          </Table>
        {/if}
      {/if}
    </Stack>
  </InstanceGate>
</Stack>

<Dialog
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
      <Banner tone="danger" title={failure.message}>
        {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
      </Banner>
    {/if}
    <p class="prose">{t('app.instance.ai_form_intro')}</p>
    <Input label={t('app.instance.ai_name_label')} hint={t('app.instance.ai_name_hint')} bind:value={displayName} error={failure?.fields.get('/display_name')} autocomplete="off" isRequired />
    <Input label={t('app.instance.ai_processor_label')} hint={t('app.instance.ai_processor_hint')} bind:value={processor} error={failure?.fields.get('/processor')} autocomplete="off" isRequired />
    <Select label={t('app.instance.ai_jurisdiction_label')} hint={t('app.instance.ai_jurisdiction_hint')} bind:value={jurisdiction} options={jurisdictions} />
    <p class="quiet-line">{t('app.instance.ai_operator_only')}</p>
    <Select label={t('app.ai.kind_label')} bind:value={kind} options={kinds} />
    <Input label={t('app.ai.base_url_label')} bind:value={baseUrl} error={failure?.fields.get('/base_url')} type="url" autocomplete="off" spellcheck={false} isRequired />
    <Input label={t('app.ai.completion_model_label')} hint={t('app.ai.completion_model_hint')} bind:value={completionModel} autocomplete="off" spellcheck={false} />
    <Input label={t('app.ai.embedding_model_label')} hint={t('app.instance.ai_embedding_hint')} bind:value={embeddingModel} autocomplete="off" spellcheck={false} />
    <Input
      label={t('app.ai.api_key_label')}
      hint={acting?.kind === 'change' && acting.model?.has_api_key ? t('app.ai.api_key_kept') : t('app.ai.api_key_hint')}
      bind:value={apiKey}
      type="password"
      autocomplete="off"
      spellcheck={false}
    />
  </Stack>
</Dialog>

<Dialog
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
  <Banner tone="warning">{t('app.instance.ai_remove_cost', { name: acting?.model?.display_name ?? '' })}</Banner>
</Dialog>

<style>
  .prose { margin: 0; max-inline-size: 60ch; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .quiet-line { margin: 0; max-inline-size: 60ch; color: var(--text-secondary); font-size: var(--fs-100); }
  .name { display: block; font-weight: var(--fw-medium); }
  .slug { display: block; color: var(--text-secondary); font-size: var(--fs-100); }
  .row { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
</style>
