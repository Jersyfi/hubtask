<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's AI provider, and the consent that lets it be used (ADR-0049).
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
  // **The budget is a quota per source, and it lives with the quotas** (ADR-0072 §4). The offered
  // model's (`ai_tokens_per_day`) is the installation's to set; the own model's
  // (`ai_own_tokens_per_day`) is this workspace's, set here and nowhere else. Both are read from
  // `/quotas`; the screen says each one's use and cap side by side, only for a source the rule
  // allows (P-05).
  //
  // **Which AI answers is the installation's to allow and the workspace's to choose** (ADR-0072).
  // `/ai-source` says which sources the rule in force allows, and the screen offers exactly those
  // (P-05): an offered model as one card with one switch - the switch is the consent, naming the
  // processor and where - and the own model's form only where the workspace may use its own. With
  // nothing offered and nothing decided this is the screen it always was (P-10).

  import { untrack } from 'svelte';

  import { Banner, Button, Input, PageHeader, Select, Spinner, Stack, Switch } from '@hubtask/design-system/components';
  import { TransportError } from '@hubtask/sync-engine';
  import type { AiJurisdiction, AiProviderKind, AiSourceChoice, AiSourceKind, AiSources } from '@hubtask/sync-engine';

  import { aiProvider } from '../lib/data/aiprovider.svelte.ts';
  import PersonOptOutSection from '../lib/workspace/PersonOptOutSection.svelte';
  import { quotas } from '../lib/data/quotas.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';
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
  $effect(() => untrack(() => quotas.open()));

  const source = $derived(aiProvider.source);
  const offered = $derived(source?.offered ?? []);
  /** What the rule in force allows. Before the answer arrives, the own model, as always. */
  const allowsOwn = $derived(source === undefined || source.sources === 'OWN' || source.sources === 'EITHER');
  const allowsOffered = $derived(
    source !== undefined && (source.sources === 'OFFERED' || source.sources === 'EITHER') && offered.length > 0,
  );
  /** Nothing at all is allowed here, by the installation or by the workspace's own narrowing. */
  const allowsNone = $derived(source !== undefined && source.sources === 'NONE');
  /** Whether a choice between sources is this screen's to offer: both are allowed. */
  const choosing = $derived(allowsOwn && allowsOffered);
  /** The narrowing is offered only where something is offered at all and nothing is locked. */
  const narrowable = $derived(source !== undefined && offered.length > 0 && !source.sources_lock);
  const offeredBudget = $derived(quotas.standings.find((each) => each.quota === 'ai_tokens_per_day'));
  const ownBudget = $derived(quotas.standings.find((each) => each.quota === 'ai_own_tokens_per_day'));
  let ownBudgetDraft = $state('');
  let ownBudgetWorking = $state(false);
  let ownBudgetSaved = $state(false);
  /** The draft follows the stored budget until somebody types. */
  let ownBudgetTouched = false;
  $effect(() => {
    const current = ownBudget;
    if (!current || ownBudgetTouched) return;
    ownBudgetDraft = current.configured && current.limit > 0 ? String(current.limit) : '';
  });

  /** Saves the own model's budget; an empty field is no budget. */
  async function saveOwnBudget(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    ownBudgetWorking = true;
    ownBudgetSaved = false;
    failure = undefined;
    const typed = ownBudgetDraft.trim();
    try {
      await aiProvider.setOwnBudget({ own_tokens_per_day: typed === '' ? null : Number(typed) });
      ownBudgetTouched = false;
      ownBudgetSaved = true;
    } catch (cause) {
      failure = problemOf(cause);
    } finally {
      ownBudgetWorking = false;
    }
  }
  let choosingWorking = $state<string | undefined>(undefined);

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
  const allJurisdictions: { value: AiJurisdiction; label: string }[] = [
    { value: 'SELF_HOSTED', label: t('app.ai.jurisdiction_self_hosted') },
    { value: 'EEA', label: t('app.ai.jurisdiction_eea') },
    { value: 'ADEQUACY', label: t('app.ai.jurisdiction_adequacy') },
    { value: 'THIRD_COUNTRY', label: t('app.ai.jurisdiction_third_country') },
  ];

  /** Who set a rule, in words: the installation or the plan. */
  const setBy = (origin: string | undefined): string =>
    origin === 'PLAN' ? t('app.ai.set_by_plan') : t('app.ai.set_by_installation');

  /** The narrowings the level above allows, as the options of one control, in words. */
  const narrowings = $derived.by(() => {
    const above = source?.installation_sources;
    const within: AiSources[] =
      above === 'EITHER' ? ['EITHER', 'OFFERED', 'OWN', 'NONE'] : above === 'OFFERED' ? ['OFFERED', 'NONE'] : above === 'OWN' ? ['OWN', 'NONE'] : ['NONE'];
    return [
      { value: 'INHERIT', label: t('app.ai.narrow_inherit') },
      ...within.map((value) => ({ value, label: t(`app.ai.narrow_${value.toLowerCase()}`) })),
    ];
  });

  /** Chooses which AI answers, and says a refusal where it was made. */
  async function choose(id: string, body: AiSourceChoice): Promise<void> {
    choosingWorking = id;
    failure = undefined;
    saved = false;
    try {
      await aiProvider.choose(body);
    } catch (cause) {
      failure = problemOf(cause);
    } finally {
      choosingWorking = undefined;
    }
  }

  /** A narrowing that leaves out the source in use chooses no AI with it, rather than being refused. */
  function narrow(value: string): void {
    const current: AiSourceKind = source?.source ?? 'OWN';
    const keeps =
      value === 'INHERIT' ||
      current === 'NONE' ||
      (current === 'OWN' && (value === 'OWN' || value === 'EITHER')) ||
      (current === 'OFFERED' && (value === 'OFFERED' || value === 'EITHER'));
    void choose('narrow', {
      source: keeps ? current : 'NONE',
      ...(keeps && current === 'OFFERED' && source?.offered_model_id ? { offered_model_id: source.offered_model_id } : {}),
      sources: value as AiSourceChoice['sources'],
    });
  }

  /** The host the content would be sent to, as the address field holds it; empty while it is not an address. */
  function hostOf(address: string): string {
    try {
      return new URL(address.trim()).host;
    } catch {
      return '';
    }
  }

  // UC-AI-03 check 2: the consent names who receives the content and where, in words, from what the
  // form holds now - the sentence changes with the address and the jurisdiction it is about.
  const consentLabel = $derived.by(() => {
    const host = hostOf(baseUrl);
    return host && jurisdiction
      ? t('app.ai.consent_label_named', { host, where: jurisdiction })
      : t('app.ai.consent_label');
  });

  /**
   * The declarations the installation's minimum admits (UC-AI-06 check 4). A looser one is not
   * offered, because the server refuses it (P-05); the sentence beside the field says why the list
   * is shorter. A stored declaration below the minimum stays in the list, so the form can show it.
   */
  const RANK: Record<AiJurisdiction, number> = { THIRD_COUNTRY: 0, ADEQUACY: 1, EEA: 2, SELF_HOSTED: 3 };
  const MINIMUM_RANK: Record<string, number> = { ANY: 0, ADEQUACY: 1, EEA: 2, SELF_HOSTED: 3 };
  const jurisdictions = $derived(
    allJurisdictions.filter(
      (each) =>
        RANK[each.value] >= (MINIMUM_RANK[source?.min_jurisdiction ?? 'ANY'] ?? 0) || each.value === provider?.jurisdiction,
    ),
  );

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
      <WorkspaceGate permission="STRUCTURE">
      {#if source?.search_rebuilding}
        <Banner tone="info">{t('app.ai.rebuilding')}</Banner>
      {/if}

      {#if allowsNone}
        <Banner tone="info">{t('app.ai.none_allowed', { by: setBy(source?.sources_origin) })}</Banner>
      {/if}

      {#if allowsOffered}
        <!-- One card per offered model, one switch each: the switch is the consent, and the card
             says who receives the content and where before anybody flips it (UC-AI-05). -->
        <Stack gap="150">
          <h2 class="section">{t('app.ai.offered_title')}</h2>
          {#each offered as model (model.id)}
            {@const isChosen = source?.source === 'OFFERED' && source.offered_model_id === model.id}
            <div class="panel">
              <Stack gap="100">
                <p class="name">{t('app.ai.offered_name', { name: model.display_name })}</p>
                <p class="quiet">{t('app.ai.offered_sent_to', { processor: model.processor, where: model.jurisdiction })}</p>
                <Switch
                  label={t('app.ai.offered_use', { name: model.display_name })}
                  checked={isChosen}
                  disabledReason={choosingWorking ? t('app.ai.saving') : undefined}
                  onchange={(event) =>
                    void choose(model.id, (event.currentTarget as HTMLInputElement).checked
                      ? { source: 'OFFERED', offered_model_id: model.id }
                      : { source: 'NONE' })}
                />
              </Stack>
            </div>
          {/each}
        </Stack>
      {/if}

      {#if choosing}
        <Switch
          label={t('app.ai.use_own')}
          hint={t('app.ai.use_own_hint')}
          checked={source?.source === 'OWN'}
          disabledReason={choosingWorking ? t('app.ai.saving') : undefined}
          onchange={(event) => void choose('own', { source: (event.currentTarget as HTMLInputElement).checked ? 'OWN' : 'NONE' })}
        />
      {/if}

      {#if !allowsOwn && !allowsNone}
        <p class="quiet">{t('app.ai.own_not_allowed', { by: setBy(source?.sources_origin) })}</p>
      {/if}

      {#if allowsOwn && unconfigured && !allowsOffered}
        <Banner tone="info">{t('app.ai.none')}</Banner>
      {/if}

      {#if failure}
        <Banner tone="danger" title={failure.message}>
          {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
        </Banner>
      {:else if saved}
        <Banner tone="success">{t('app.ai.saved')}</Banner>
      {/if}

      {#if allowsOwn}
      {#if choosing}<h2 class="section">{t('app.ai.own_title')}</h2>{/if}
      <form class="panel" onsubmit={save}>
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
          {#if source && source.min_jurisdiction !== 'ANY'}
            <p class="quiet">{t('app.ai.minimum', { minimum: source.min_jurisdiction, by: setBy(source.min_jurisdiction_origin) })}</p>
          {/if}
          <!-- The consent, as its own switch, naming who receives the content and where. -->
          <Switch label={consentLabel} hint={t('app.ai.consent_hint')} bind:checked={processingAllowed} />
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

      {/if}

      {#if narrowable}
        <Select
          label={t('app.ai.narrow_label')}
          hint={t('app.ai.narrow_hint')}
          value={source?.narrowing ?? 'INHERIT'}
          options={narrowings}
          onchange={(event) => narrow((event.currentTarget as HTMLSelectElement).value)}
        />
      {:else if source?.sources_lock && offered.length > 0}
        <p class="quiet">{t('app.ai.sources_locked', { by: setBy(source.sources_lock) })}</p>
      {/if}

      {#if source}
        <!-- Whether each person may keep their own content out of AI, or AI is part of
             everybody's work on a basis named here (data-protection.md §4.1). -->
        <PersonOptOutSection {source} />
      {/if}

      <Stack gap="100">
        <h2 class="section">{t('app.ai.budget_title')}</h2>
        {#if offeredBudget && allowsOffered}
          <p>
            {offeredBudget.limit === 0
              ? t('app.ai.budget_offered_unlimited', { used: offeredBudget.used ?? 0 })
              : t('app.ai.budget_offered_use', { used: offeredBudget.used ?? 0, limit: offeredBudget.limit })}
          </p>
        {/if}
        {#if ownBudget && allowsOwn}
          <p>
            {ownBudget.limit === 0
              ? t('app.ai.budget_own_unlimited', { used: ownBudget.used ?? 0 })
              : t('app.ai.budget_own_use', { used: ownBudget.used ?? 0, limit: ownBudget.limit })}
          </p>
        {/if}
        <p class="quiet">{t('app.ai.budget_day')}</p>
        {#if allowsOwn}
          <form class="budget" onsubmit={saveOwnBudget}>
            <Stack gap="100">
              <Input
                label={t('app.ai.budget_own_label')}
                hint={t('app.ai.budget_own_hint')}
                type="number"
                inputmode="numeric"
                min="1"
                value={ownBudgetDraft}
                oninput={(event) => {
                  ownBudgetTouched = true;
                  ownBudgetSaved = false;
                  ownBudgetDraft = (event.currentTarget as HTMLInputElement).value;
                }}
              />
              <div>
                <Button type="submit" tone="secondary" isBusy={ownBudgetWorking} busyLabel={t('app.ai.saving')}>
                  {t('app.ai.budget_own_save')}
                </Button>
              </div>
              {#if ownBudgetSaved}<p class="quiet" role="status">{t('app.ai.budget_own_saved')}</p>{/if}
            </Stack>
          </form>
        {/if}
        {#if allowsOffered}
          <!-- Linked, not drawn twice: the offered model's budget is the installation's quota row. -->
          <p class="quiet">{t('app.ai.budget_hint')} <a href="/administration/quotas">{t('app.ai.budget_link')}</a></p>
        {/if}
      </Stack>

      {#if provider && allowsOwn}
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
      </WorkspaceGate>
    {/if}
    </Stack>
</Stack>

<style>
  /* A form is neither prose nor a table, and it is the third case of ADR-0065 decision 2: an
     input as wide as the region is a target nobody aims at, so the fields carry a measure of
     their own while the lists and tables beside them take the width. */
  form { margin: 0; max-inline-size: 52ch; }

  /* The surface a form stands on, as the other screens of the section draw one: a standalone
     element in the sense of design-system.md rule 1, on the frame's canvas. */
  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  /* The screen takes the region it is given, and what needs a measure carries one: prose has the
     one `app.css` gives every paragraph, fields have `.fields`, and a table or a list has none
     (ADR-0065 decision 2). A 60ch column around the whole of it would be a document's measure
     around a screen that is not a document. */
  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  .quiet { margin: 0; color: var(--text-secondary); max-width: 64ch; }
  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
  .name { margin: 0; font-weight: var(--fw-semibold); }
  .budget { margin: 0; max-inline-size: 52ch; }
</style>
