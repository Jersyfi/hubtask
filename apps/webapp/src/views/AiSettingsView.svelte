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

  import { tick, untrack } from 'svelte';

  import { Banner, Button, Input, PageHeader, Select, Spinner, Stack, Switch, Tabs } from '@hubtask/design-system/components';
  import { TransportError } from '@hubtask/sync-engine';
  import type { AiProviderKind, AiSourceChoice, AiSourceKind, AiSources } from '@hubtask/sync-engine';

  import ModelFields from '../lib/ai/ModelFields.svelte';
  import { emptyModel, modelBody, modelOf } from '../lib/ai/model.ts';
  import { aiProvider } from '../lib/data/aiprovider.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import PersonOptOutSection from '../lib/workspace/PersonOptOutSection.svelte';
  import { quotas } from '../lib/data/quotas.svelte.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { check, focusFirst, isComplete, type Missing } from '../lib/forms/missing.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import ValueSource from '../lib/workspace/ValueSource.svelte';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';
  import WriteOnly from '../lib/workspace/WriteOnly.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  interface Props {
    /** The tab the address names (`?tab=`); the source where it names none or one not drawn. */
    tab?: string;
    /** Replaces the address when the tab changes, so a reload or a link keeps it. */
    onnavigate?: (path: string) => void;
  }

  let { tab, onnavigate }: Props = $props();

  let draft = $state(emptyModel('', ''));
  let processingAllowed = $state(false);
  let confirmingRemoval = $state(false);
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /** What a press of the own model's save found missing, marked at its fields. */
  let missing = $state<Missing>({});
  let ownForm = $state<HTMLFormElement | undefined>(undefined);
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
    failure = undefined;
    const typed = ownBudgetDraft.trim();
    try {
      await aiProvider.setOwnBudget({ own_tokens_per_day: typed === '' ? null : Number(typed) });
      ownBudgetTouched = false;
      toast.show({ text: t('app.ai.budget_own_saved'), tone: 'success' });
    } catch (cause) {
      failure = problemOf(cause);
    } finally {
      ownBudgetWorking = false;
    }
  }
  let choosingWorking = $state<string | undefined>(undefined);

  /**
   * Proposals on arrival, where there is AI to make them: absent rather than disabled where the
   * manifest announces no AI (absence is absence). Switched at once, as a switch is.
   */
  const hasAi = $derived(manifest.value?.features?.ai_suggestions === true);
  let arrivalWorking = $state(false);
  async function prepareOnArrival(prepare: boolean): Promise<void> {
    arrivalWorking = true;
    failure = undefined;
    try {
      await aiProvider.setArrivalProposals(prepare);
      announcer.say(t(prepare ? 'app.ai.arrival_on_announced' : 'app.ai.arrival_off_announced'));
    } catch (cause) {
      failure = problemOf(cause);
    } finally {
      arrivalWorking = false;
    }
  }

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
    draft = modelOf(current);
    processingAllowed = current.processing_allowed;
  });

  // The contract's own enum, which is not a hard-coded list of what the installation supports: a
  // kind the server refuses comes back as `ai.kind_unknown`, rendered like any refusal.
  const KINDS: readonly AiProviderKind[] = ['OPENAI_COMPATIBLE', 'OLLAMA', 'NOOP'];

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
    const host = hostOf(draft.baseUrl);
    return host && draft.jurisdiction
      ? t('app.ai.consent_label_named', { host, where: draft.jurisdiction })
      : t('app.ai.consent_label');
  });

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    // Never switched off for an empty choice: pressed, the missing choice is marked and focused
    // (UC-WRK-32 check 3).
    missing = check([
      { kind: 'field', key: 'kind', code: 'app.ai.kind_needed', isFilled: draft.kind !== '' },
      { kind: 'field', key: 'jurisdiction', code: 'app.ai.jurisdiction_needed', isFilled: draft.jurisdiction !== '' },
    ]);
    if (!isComplete(missing)) {
      void tick().then(() => focusFirst(ownForm));
      return;
    }
    isWorking = true;
    failure = undefined;
    try {
      // Clearing the key is the removal below, never an empty field.
      await aiProvider.configure({ ...modelBody(draft), processing_allowed: processingAllowed });
      // Out of the component's state at once: a key kept for a second submission is a key sitting
      // in a form for as long as the tab is open.
      draft.apiKey = '';
      toast.show({ text: t('app.ai.saved'), tone: 'success' });
    } catch (cause) {
      failure = problemOf(cause);
    } finally {
      isWorking = false;
    }
  }

  async function remove(): Promise<void> {
    isWorking = true;
    failure = undefined;
    try {
      await aiProvider.remove();
      confirmingRemoval = false;
      filled = false;
      draft = emptyModel('', '');
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

  // **Three tabs** (ux-inventory §E8): which AI answers - the offered models, the own model with
  // its consent, the narrowing, removing it -; what the people here are asked and what happens to
  // what they bring in; and the budget. The own model's consent stays in its form, because the
  // form saves the two together and a consent saved apart from what it consents to is not one.
  /** Whether each person here is asked for their position, as `PersonOptOutSection` decides it. */
  const peopleAsked = $derived(
    source !== undefined && (workspace.workspace?.several_people === true || source.person_opt_out === 'NOT_OFFERED'),
  );
  const arrivalOffered = $derived(source !== undefined && hasAi && !allowsNone);
  /** The consent and people tab is drawn where it would hold something, and absent otherwise. */
  const tabs = $derived([
    { id: 'source', label: t('app.ai.tab_source') },
    ...(peopleAsked || arrivalOffered ? [{ id: 'consent', label: t('app.ai.tab_consent') }] : []),
    { id: 'budget', label: t('app.ai.tab_budget') },
  ]);
  const current = $derived(tabs.some((each) => each.id === tab) ? (tab as string) : 'source');

  function select(next: string): void {
    onnavigate?.(next === 'source' ? '/administration/ai' : `/administration/ai?tab=${next}`);
  }
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
      {#if failure}
        <Banner tone="danger" title={failure.message}>
          {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
        </Banner>
      {/if}

      <!-- The tabs outside the gate: a reader who may not change anything still reads every tab. -->
      <Tabs label={t('app.ai.title')} selected={current} onselect={select} {tabs}>
        <WorkspaceGate permission="STRUCTURE">
        {#if current === 'consent'}
          <Stack gap="300">
            {#if source}
              <!-- Whether each person may keep their own content out of AI, or AI is part of
                   everybody's work on a basis named here (data-protection.md §4.1). -->
              <PersonOptOutSection {source} />
            {/if}
            {#if source && arrivalOffered}
              <Switch
                label={t('app.ai.arrival_label')}
                hint={t('app.ai.arrival_hint')}
                checked={source.prepare_on_arrival}
                disabledReason={arrivalWorking ? t('app.ai.saving') : undefined}
                onchange={(event) => void prepareOnArrival((event.currentTarget as HTMLInputElement).checked)}
                data-arrival-proposals
              />
            {/if}
          </Stack>
        {:else if current === 'budget'}
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
                      ownBudgetDraft = (event.currentTarget as HTMLInputElement).value;
                    }}
                  />
                  <WriteOnly>
                    <div>
                      <Button type="submit" tone="secondary" isBusy={ownBudgetWorking} busyLabel={t('app.ai.saving')}>
                        {t('app.ai.budget_own_save')}
                      </Button>
                    </div>
                  </WriteOnly>
                </Stack>
              </form>
            {/if}
            {#if allowsOffered}
              <!-- Linked, not drawn twice: the offered model's budget is the installation's quota row. -->
              <p class="quiet">{t('app.ai.budget_hint')} <a href="/administration/quotas">{t('app.ai.budget_link')}</a></p>
            {/if}
          </Stack>
        {:else}
          <Stack gap="300">
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
                        isBusy={choosingWorking === model.id}
                        busyLabel={t('app.ai.saving')}
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
                isBusy={choosingWorking === 'own'}
                busyLabel={t('app.ai.saving')}
                onchange={(event) => void choose('own', { source: (event.currentTarget as HTMLInputElement).checked ? 'OWN' : 'NONE' })}
              />
            {/if}

            {#if !allowsOwn && !allowsNone}
              <p class="quiet">{t('app.ai.own_not_allowed', { by: setBy(source?.sources_origin) })}</p>
            {/if}

            {#if allowsOwn && unconfigured && !allowsOffered}
              <Banner tone="info">{t('app.ai.none')}</Banner>
            {/if}

            {#if allowsOwn}
            {#if choosing}<h2 class="section">{t('app.ai.own_title')}</h2>{/if}
            <form class="panel" bind:this={ownForm} onsubmit={save}>
              <Stack gap="200">
                <ModelFields
                  bind:draft
                  kinds={KINDS}
                  minimum={source?.min_jurisdiction ?? 'ANY'}
                  stored={provider?.jurisdiction}
                  hasKey={provider?.has_api_key === true}
                  {missing}
                >
                  {#snippet more()}
                    {#if source && source.min_jurisdiction !== 'ANY'}
                      <p class="quiet">
                        {t('app.ai.minimum', { minimum: source.min_jurisdiction, by: setBy(source.min_jurisdiction_origin) })}
                        <ValueSource source={source.min_jurisdiction_origin} lock={source.min_jurisdiction_lock} value={source.min_jurisdiction} />
                      </p>
                    {/if}
                    <!-- The consent, as its own switch, naming who receives the content and where. -->
                    <Switch label={consentLabel} hint={t('app.ai.consent_hint')} bind:checked={processingAllowed} />
                  {/snippet}
                </ModelFields>
                <WriteOnly>
                  <div>
                    <Button
                      type="submit"
                      tone="primary"
                      isBusy={isWorking}
                      busyLabel={t('app.ai.saving')}
                    >
                      {t('app.ai.save')}
                    </Button>
                  </div>
                </WriteOnly>
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
              <p class="quiet">
                {t('app.ai.sources_locked', { by: setBy(source.sources_lock) })}
                <ValueSource source={source.sources_origin} lock={source.sources_lock} value={t(`app.ai.narrow_${source.sources.toLowerCase()}`)} />
              </p>
            {/if}

            <WriteOnly>
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
            </WriteOnly>
          </Stack>
        {/if}
        </WorkspaceGate>
      </Tabs>
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
    box-shadow: var(--shadow-raised);
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
