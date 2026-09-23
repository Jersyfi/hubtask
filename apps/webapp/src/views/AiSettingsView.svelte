<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's AI provider, and the consent that lets it be used (J-02, ADR-0049, F5-05).
  //
  // **Two decisions, kept two.** Configuring a provider says where a call would go; consenting
  // says the workspace's content may go there. `processing_allowed` is the second decision as a
  // switch of its own, false until somebody flips it, with the sentence beside it that says what
  // flipping it means in the catalogue's words: which fields leave the installation and to whom.
  //
  // **The key goes one way.** It is entered here and sealed by the server; nothing reads it back,
  // and `has_api_key` is the only echo (`ai-first.md` §3's guardrail). Leaving the field empty on a
  // later save keeps the sealed key - the contract's own rule - and the field's hint says so rather
  // than a row of dots standing in for a value nobody can see.
  //
  // **The budget is a quota, and it lives with the quotas.** `ai_tokens_per_day` is read from
  // `/quotas` like every other limit; this screen links to that row rather than drawing it twice.

  import { untrack } from 'svelte';

  import { Banner, Button, Input, PageHeader, Select, Spinner, Stack, Switch } from '@hubtask/design-system/components';
  import { TransportError } from '@hubtask/sync-engine';
  import type { AiJurisdiction, AiProviderKind } from '@hubtask/sync-engine';

  import { aiProvider } from '../lib/data/aiprovider.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  let kind = $state<AiProviderKind | ''>('');
  let baseUrl = $state('');
  let completionModel = $state('');
  let embeddingModel = $state('');
  let apiKey = $state('');
  let jurisdiction = $state<AiJurisdiction | ''>('');
  let processingAllowed = $state(false);
  let confirmingRemoval = $state(false);
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let saved = $state(false);
  /** Whether the fields have been filled from the last read. Once only, so typing is not overwritten. */
  let filled = false;

  $effect(() => untrack(() => aiProvider.open()));

  const reading = $derived(aiProvider.state);
  const provider = $derived(aiProvider.provider);

  /** A `404` is a workspace that has never configured one — the form, not an error. */
  const unconfigured = $derived(reading.status === 'failed' && reading.error.status === 404);
  const unreadable = $derived(
    reading.status === 'failed' && !unconfigured ? renderProblem(reading.error, messages) : undefined,
  );

  $effect(() => {
    const current = provider;
    if (!current || filled) return;
    filled = true;
    kind = current.kind;
    baseUrl = current.base_url ?? '';
    completionModel = current.completion_model ?? '';
    embeddingModel = current.embedding_model ?? '';
    jurisdiction = current.jurisdiction;
    processingAllowed = current.processing_allowed;
  });

  // The two closed sets are the contract's own enums. Written here as options for the pickers,
  // which is not a hard-coded list of what the installation supports: a kind the server refuses
  // comes back as `ai.kind_unknown`, rendered like any refusal.
  const kinds: { value: AiProviderKind; label: string }[] = [
    { value: 'OPENAI_COMPATIBLE', label: t('app.ai.kind_openai_compatible') },
    { value: 'OLLAMA', label: t('app.ai.kind_ollama') },
    { value: 'NOOP', label: t('app.ai.kind_noop') },
  ];
  const jurisdictions: { value: AiJurisdiction; label: string }[] = [
    { value: 'SELF_HOSTED', label: t('app.ai.jurisdiction_self_hosted') },
    { value: 'EEA', label: t('app.ai.jurisdiction_eea') },
    { value: 'ADEQUACY', label: t('app.ai.jurisdiction_adequacy') },
    { value: 'THIRD_COUNTRY', label: t('app.ai.jurisdiction_third_country') },
  ];

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!kind || !jurisdiction) return;
    isWorking = true;
    failure = undefined;
    saved = false;
    try {
      await aiProvider.configure({
        kind,
        ...(baseUrl.trim() ? { base_url: baseUrl.trim() } : {}),
        ...(completionModel.trim() ? { completion_model: completionModel.trim() } : {}),
        ...(embeddingModel.trim() ? { embedding_model: embeddingModel.trim() } : {}),
        // Absent keeps the sealed key; a value replaces it. The field is never sent empty from
        // here, because "clear the key" is a different intent from "I typed nothing" and this
        // screen offers the first as the removal below.
        ...(apiKey ? { api_key: apiKey } : {}),
        jurisdiction,
        processing_allowed: processingAllowed,
      });
      // Out of the component's state at once: a key kept for a second submission is a key sitting
      // in a form for as long as the tab is open.
      apiKey = '';
      saved = true;
    } catch (cause) {
      failure = problemOf(cause);
    } finally {
      isWorking = false;
    }
  }

  async function remove(): Promise<void> {
    isWorking = true;
    failure = undefined;
    saved = false;
    try {
      await aiProvider.remove();
      confirmingRemoval = false;
      filled = false;
      kind = '';
      baseUrl = '';
      completionModel = '';
      embeddingModel = '';
      apiKey = '';
      jurisdiction = '';
      processingAllowed = false;
    } catch (cause) {
      failure = problemOf(cause);
    } finally {
      isWorking = false;
    }
  }

  function problemOf(cause: unknown): ReturnType<typeof renderProblem> {
    return cause instanceof TransportError
      ? renderProblem(cause, messages)
      : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
  }
  // The bar carries the page's title on a phone (ADR-0061 decision 1's table); the head then
  // reads its heading rather than drawing it, so the screen keeps one heading.
  $effect(() => page.entitle(t('app.ai.title')));
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.ai.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'ai', label: t('app.ai.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

  <div class="screen">
    <Stack gap="300">
    <p class="quiet">{t('app.ai.intro')}</p>

    {#if reading.status === 'loading' || reading.status === 'idle'}
      <p class="quiet">
        <Spinner label={t('app.ai.reading')} />
        <span>{t('app.ai.reading')}</span>
      </p>
    {:else if unreadable}
      <Banner tone="danger" title={unreadable.message}>
        {#if unreadable.reference}{t('app.error_reference', { request_id: unreadable.reference })}{/if}
      </Banner>
    {:else}
      {#if unconfigured}
        <Banner tone="info">{t('app.ai.none')}</Banner>
      {/if}

      {#if failure}
        <Banner tone="danger" title={failure.message}>
          {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
        </Banner>
      {:else if saved}
        <Banner tone="success">{t('app.ai.saved')}</Banner>
      {/if}

      <form onsubmit={save}>
        <Stack gap="200">
          <Select label={t('app.ai.kind_label')} hint={t('app.ai.kind_hint')} bind:value={kind} placeholder={t('app.ai.kind_choose')} options={kinds} />
          <Input
            label={t('app.ai.base_url_label')}
            hint={t('app.ai.base_url_hint')}
            bind:value={baseUrl}
            type="url"
            autocomplete="off"
            spellcheck={false}
          />
          <Input label={t('app.ai.completion_model_label')} hint={t('app.ai.completion_model_hint')} bind:value={completionModel} autocomplete="off" spellcheck={false} />
          <Input label={t('app.ai.embedding_model_label')} hint={t('app.ai.embedding_model_hint')} bind:value={embeddingModel} autocomplete="off" spellcheck={false} />
          <Input
            label={t('app.ai.api_key_label')}
            hint={provider?.has_api_key ? t('app.ai.api_key_kept') : t('app.ai.api_key_hint')}
            bind:value={apiKey}
            type="password"
            autocomplete="off"
            spellcheck={false}
          />
          <Select label={t('app.ai.jurisdiction_label')} hint={t('app.ai.jurisdiction_hint')} bind:value={jurisdiction} placeholder={t('app.ai.jurisdiction_choose')} options={jurisdictions} />
          <!-- The consent, as its own switch, with what it means in words beside it. -->
          <Switch label={t('app.ai.consent_label')} hint={t('app.ai.consent_hint')} bind:checked={processingAllowed} />
          <div>
            <Button
              type="submit"
              tone="primary"
              isBusy={isWorking}
              busyLabel={t('app.ai.saving')}
              disabledReason={!kind ? t('app.ai.kind_choose') : !jurisdiction ? t('app.ai.jurisdiction_choose') : undefined}
            >
              {t('app.ai.save')}
            </Button>
          </div>
        </Stack>
      </form>

      <Stack gap="100">
        <h2 class="section">{t('app.ai.budget_title')}</h2>
        <!-- Linked, not drawn twice: the budget is the quota row it is. -->
        <p class="quiet">{t('app.ai.budget_hint')} <a href="/administration/quotas">{t('app.ai.budget_link')}</a></p>
      </Stack>

      {#if provider}
        <Stack gap="150">
          <h2 class="section">{t('app.ai.remove_title')}</h2>
          <p class="quiet">{t('app.ai.remove_cost')}</p>
          {#if confirmingRemoval}
            <Banner tone="warning">{t('app.ai.remove_confirm')}</Banner>
            <div class="row">
              <Button tone="danger" isBusy={isWorking} busyLabel={t('app.ai.removing')} onclick={() => void remove()}>
                {t('app.ai.remove_now')}
              </Button>
              <Button tone="subtle" onclick={() => (confirmingRemoval = false)}>{t('app.ai.keep')}</Button>
            </div>
          {:else}
            <div>
              <Button tone="secondary" onclick={() => (confirmingRemoval = true)}>{t('app.ai.remove')}</Button>
            </div>
          {/if}
        </Stack>
      {/if}
    {/if}
    </Stack>
  </div>
</Stack>

<style>
  /* The reading measure the section gives a document (ADR-0063 decision 7), and the **head is not
     in it**: `PageHeader` folds by the width it has rather than by the viewport, so a head inside
     a 60ch column folds like one on a phone and hides its trail behind the parent link. The trail
     is what says where in the section a screen is, so the measure belongs to the content under
     the head rather than to the screen. */
  .screen { max-width: 60ch; }
  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  .quiet { margin: 0; color: var(--text-secondary); max-width: 64ch; }
  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
</style>
