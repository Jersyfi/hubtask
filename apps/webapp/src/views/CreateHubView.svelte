<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Create hub* (user-experience.md §5.2, ADR-0111): a create page at `/hubs/new`, because a name,
  // *Private*, a look and thirty templates in eight families are past a dialog - the dialog buried
  // *Private* three screens below the name. The hub's details stand in one column - its mark and
  // name, *Private* right under the name, colour, symbol, description, and a summary of the start -
  // beside the choice of how it starts: a search, the families as a filter, and the tiles as one
  // radio group, *Empty hub* first, pinned and chosen when the page opens. The head and its primary
  // stay in view; from `expanded` each column scrolls on its own.
  //
  // Who sees what is the dialog's rule, unchanged: the catalogue for a shared hub and a reader with
  // `STRUCTURE`; *Private* where the workspace offers it; a reader who may make only a private hub
  // of their own gets no switch and no template (configuration.md §6), and the page says
  // why. A template chosen with *Private* on is sent private (UC-ID-16 check 9).
  //
  // Choosing by the arrow keys chooses at each press. What a template sets up is read from the
  // catalogue at once; the plan is asked once the choice has rested, and always before the hub is
  // made, so walking the tiles asks the server once and not per tile.
  //
  // Below `medium` the page is the phone's whole screen (`CreatePage`): the details are the page,
  // the look a closed row, and the starts an optional second step - most hubs are name → create,
  // so nobody pays a press for the gallery. The step is a full-screen layer, not a route: the
  // address stays, a tile chosen there is chosen at once, and *Done*, ‹, Escape and the system's
  // back close it.

  import { tick, untrack } from 'svelte';

  import { Button, ChoiceCards, CreatePage, Dialog, Disclosure, Icon, Input, SearchField, Switch, Textarea, type ChoiceCardGroup } from '@hubtask/design-system/components';
  import type { LabelToken } from '@hubtask/design-system';
  import type { ConfigTemplate, ConfigTemplateList, TransportError } from '@hubtask/sync-engine';

  import { holdsHere, hubCreationHere } from '../lib/data/capability.svelte.ts';
  import { config } from '../lib/data/config.svelte.ts';
  import { answersOf, carriesSamples } from '../lib/data/configtemplates.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { presetMark, presetName, presetOrigin, presets, presetValue } from '../lib/data/presets.svelte.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import { leaveGuard } from '../lib/frame/leave.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { hubScreenOf } from '../lib/product/home.ts';
  import ContainerMark from '../lib/workspace/ContainerMark.svelte';
  import LookFields from '../lib/workspace/LookFields.svelte';
  import TemplateTakeFields from '../lib/workspace/TemplateTakeFields.svelte';
  import {
    EMPTY_HUB,
    familyCounts,
    hubRequest,
    landingOf,
    presetStart,
    shownStarts,
    templateStart,
    type Start,
    type StartFamily,
  } from '../lib/workspace/createHub.ts';
  import { createsOf, hubFamilies, markOf, prefilledName, switchedOn } from '../lib/workspace/hubGallery.ts';
  import type { PlaceSymbol } from '../lib/workspace/look.ts';
  import { afterStarter } from '../lib/workspace/starterSet.ts';

  interface Props {
    /** A catalogue template chosen before the page opened (`?template=`): a product's first visit. */
    template?: string;
    /** The product the page was opened from (`?product=`): a hub made without a template lands on its screen. */
    product?: string;
    /** Leaves without making anything: back where the person came from. */
    oncancel: () => void;
    /** Goes to the new hub, replacing this address. */
    onland: (path: string) => void;
    /** Holds the phone's second step in the session history: back calls `onback`; the release takes it out. */
    onstep?: (onback: () => void) => () => void;
  }

  const { template: asked, product, oncancel, onland, onstep }: Props = $props();

  // The page is one screen from `medium` up, so its head stays: what scrolls is inside it
  // (`page.fill`); on a phone, the whole screen.
  $effect(() => page.fill('medium'));
  $effect(() => page.takeScreen());

  const isPhone = $derived(viewport.isCompact);
  let isLookOpen = $state(false);
  let isStepOpen = $state(false);
  // The step is the phone's: wider, the starts stand beside or under the details.
  $effect(() => {
    if (!isPhone) isStepOpen = false;
  });
  $effect(() => {
    if (!isStepOpen || !onstep) return;
    return untrack(() => onstep(() => (isStepOpen = false)));
  });

  const creation = $derived(hubCreationHere());
  const isPrivateOnly = $derived(creation === 'private');
  const offersCatalogue = $derived(!isPrivateOnly && holdsHere('STRUCTURE') === true);

  let name = $state('');
  let description = $state('');
  let isPrivate = $state(false);
  let symbol = $state<PlaceSymbol | undefined>(undefined);
  let colour = $state<LabelToken | undefined>(undefined);
  let start = $state(EMPTY_HUB);
  /** The start the page opened with, so a choice the person made counts as unsaved. */
  let opened = $state(EMPTY_HUB);
  /** The name the last template chosen filled in, so a name the person typed is never replaced. */
  let filled = $state('');
  let family = $state<'all' | StartFamily>('all');
  let query = $state('');
  let answers = $state<Record<string, string>>({});
  let withSamples = $state(true);
  let acknowledged = $state(false);
  let isPlanning = $state(false);
  let mustAcknowledge = $state(false);
  let planFailure = $state<string | undefined>(undefined);
  let isSaving = $state(false);
  let isMade = $state(false);
  let isNameMissing = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isNameFailure = $state(false);
  let nameField = $state<HTMLElement | null>(null);

  // Read once the catalogue has answered, so a `?template=` names a template that exists or none.
  let catalogue = $state<readonly ConfigTemplate[]>([]);
  let isCatalogueRead = $state(false);
  $effect(() => {
    if (!offersCatalogue) return;
    return untrack(() => config.templates((next: { status: string; data?: ConfigTemplateList }) => {
      if (next.status === 'ready' && Array.isArray(next.data?.data)) {
        catalogue = next.data.data;
        isCatalogueRead = true;
      }
    }));
  });
  $effect(() => untrack(() => presets.open()));
  $effect(() => {
    if (isPrivateOnly) return;
    return untrack(() => workspace.open());
  });

  const isPrivacyOffered = $derived(!isPrivateOnly && workspace.workspace?.private_hubs_offered === true);
  const isPrivateHub = $derived(isPrivateOnly || (isPrivacyOffered && isPrivate));
  const title = $derived(isPrivateHub ? t('app.workspace.create_private_hub') : t('app.workspace.create_hub'));
  $effect(() => page.entitle(title));

  const templateName = (each: ConfigTemplate): string =>
    each.origin === 'HUBTASK' ? t(`app.structure.preset.${each.key.toUpperCase()}`) : each.name;
  const familyName = (each: StartFamily): string => t(`app.workspace.hub_family.${each}`);

  const families = $derived(offersCatalogue ? hubFamilies(catalogue) : []);
  const starts = $derived<Start[]>([
    ...presets.all.map((each) => ({
      value: presetStart(presetValue(each)),
      family: 'simple' as const,
      label: presetName(each),
      summary: each.key ? t(`app.structure.preset_about.${each.key}`) : presetOrigin(each),
      chips: [],
      icon: presetMark(each),
    })),
    ...families.flatMap((group) => group.templates.map((each) => ({
      value: templateStart(each.key),
      family: group.family,
      label: templateName(each),
      summary: each.origin === 'HUBTASK' ? t(`app.templates.summary.${each.key.toUpperCase()}`) : undefined,
      chips: switchedOn(each).map((module) => t(`app.modules.name_${module.toLowerCase()}`)),
      icon: markOf(each),
      note: each.taken_version == null ? undefined : t('app.workspace.hub_template_in_workspace'),
    }))),
  ]);
  const shown = $derived(shownStarts(starts, family, query, familyName, messages.locale));
  const counts = $derived(familyCounts(starts));
  const isFiltered = $derived(query.trim() !== '' || family !== 'all');
  const groups = $derived<ChoiceCardGroup[]>(shown.groups.map((group) => ({
    label: familyName(group.family),
    options: group.starts.map((each) => ({ value: each.value, label: each.label, description: each.note, summary: each.summary, chips: each.chips, icon: each.icon })),
  })));
  const empty = $derived({ value: EMPTY_HUB, label: t('app.workspace.hub_empty'), summary: t('app.workspace.hub_empty_about'), icon: 'plus' as const, isBlank: true });

  const chosenTemplate = $derived(
    offersCatalogue && start.startsWith('template:') ? catalogue.find((each) => templateStart(each.key) === start) : undefined,
  );
  const chosenStart = $derived(starts.find((each) => each.value === start));

  /** Chooses a start: a template names the hub until the person types a name of their own. */
  function choose(next: string): void {
    if (next === start) return;
    start = next;
    answers = {};
    withSamples = true;
    acknowledged = false;
    planFailure = undefined;
    const chosen = next.startsWith('template:') ? catalogue.find((each) => templateStart(each.key) === next) : undefined;
    const named = chosen ? templateName(chosen) : '';
    name = prefilledName(name, filled, named);
    filled = named;
    if (named !== '') isNameMissing = false;
  }

  // A template asked for in the address is chosen once the catalogue says it exists; one the person
  // cannot read, or none, leaves *Empty hub* chosen.
  let isAskedApplied = false;
  $effect(() => {
    if (isAskedApplied || !asked) return;
    if (!offersCatalogue) {
      if (creation !== undefined) isAskedApplied = true;
      return;
    }
    if (!isCatalogueRead) return;
    isAskedApplied = true;
    const wanted = templateStart(asked);
    untrack(() => {
      if (starts.some((each) => each.value === wanted)) {
        choose(wanted);
        opened = wanted;
      }
    });
  });

  // The choice rests before its plan is asked: an arrow press chooses, and five presses ask once.
  const REST_MS = 400;
  let restedKey = $state<string | undefined>(undefined);
  $effect(() => {
    const key = chosenTemplate?.key;
    if (key === restedKey) return;
    if (key === undefined) {
      restedKey = undefined;
      return;
    }
    const timer = setTimeout(() => (restedKey = key), REST_MS);
    return () => clearTimeout(timer);
  });
  const isRested = $derived(chosenTemplate !== undefined && restedKey === chosenTemplate.key);
  let planWaiters: (() => void)[] = [];
  $effect(() => {
    if (isPlanning) return;
    const waiting = planWaiters;
    planWaiters = [];
    for (const resume of waiting) resume();
  });

  /**
   * Rests the choice at once, so its plan is asked now, and resolves once no plan is being read. A
   * template the workspace took asks none: nothing is waited for, whenever the press came.
   */
  async function planRead(key: string): Promise<void> {
    restedKey = key;
    await tick();
    while (isPlanning) await new Promise<void>((resume) => planWaiters.push(resume));
  }

  const unsaved = $derived(
    !isMade && ((name.trim() !== '' && name !== filled) || description.trim() !== '' || symbol !== undefined || colour !== undefined || isPrivate || start !== opened),
  );
  $effect(() => leaveGuard.hold(() => (unsaved ? 1 : 0)));

  // Opening the page puts the focus in the name.
  $effect(() => {
    if (!nameField) return;
    untrack(() => nameField?.querySelector<HTMLInputElement>('input')?.focus());
  });

  const focusName = () => queueMicrotask(() => nameField?.querySelector<HTMLInputElement>('input')?.focus());
  let detailsPane = $state<HTMLElement | null>(null);
  const focusAcknowledge = () => detailsPane?.querySelector<HTMLInputElement>('[data-acknowledge]')?.focus();

  async function create(): Promise<void> {
    if (isSaving) return;
    if (name.trim() === '') {
      isNameMissing = true;
      focusName();
      return;
    }
    isSaving = true;
    failure = undefined;
    isNameFailure = false;
    try {
      const template = chosenTemplate;
      if (template) {
        // However it was pressed - the primary, Enter in the name - the plan is read before the hub
        // is made, and a plan that asks for the acknowledgement asks for it first.
        await planRead(template.key);
        await tick();
        if (mustAcknowledge && !acknowledged) {
          focusAcknowledge();
          return;
        }
      }
      const request = hubRequest({
        start,
        name,
        description,
        icon: symbol ?? null,
        colorToken: colour ?? null,
        isPrivate,
        isPrivacyOffered,
        isPrivateOnly,
        template: template ? { key: template.key, carriesSamples: carriesSamples(template) } : undefined,
        answers: template ? answersOf(template.questions, answers) : {},
        withSamples,
        acknowledged,
      });
      // The `Idempotency-Key` is the caller's: pressing *Create hub* twice is one intent, not two hubs.
      const hub = request.via === 'template'
        ? await containers.createFromTemplate(request.body, crypto.randomUUID())
        : await containers.create(request.body, crypto.randomUUID());
      isMade = true;
      onland(landingOf({
        templateLanding: template ? afterStarter(template, hub.id) : undefined,
        productScreen: product ? hubScreenOf(product, hub.id) : undefined,
        hubId: hub.id,
      }));
    } catch (error) {
      const problem = error as TransportError;
      failure = renderProblem(problem, messages);
      isNameFailure = problem.detailCode === 'containers.name_taken' || failure.fields.has('/name');
      if (isNameFailure) focusName();
    } finally {
      isSaving = false;
    }
  }

  const primaryReason = $derived(
    chosenTemplate && isRested && isPlanning ? t('app.starter.plan_reading')
      : chosenTemplate && isRested && mustAcknowledge && !acknowledged ? t('app.changes.acknowledge_first')
        : undefined,
  );
  const nameError = $derived(isNameMissing ? t('app.workspace.name_required') : isNameFailure ? failure?.message : undefined);
  const parts = $derived(chosenTemplate ? createsOf(chosenTemplate).filter((each) => each.part !== 'samples') : []);
  const products = $derived(chosenTemplate ? switchedOn(chosenTemplate).map((module) => t(`app.modules.name_${module.toLowerCase()}`)) : []);
  const kindWord = $derived(
    start === EMPTY_HUB ? t('app.workspace.hub_summary_start') : chosenTemplate ? t('app.workspace.hub_summary_template') : t('app.workspace.hub_summary_preset'),
  );
  const uid = $props.id();
</script>

{#snippet creates()}
  <dl class="sets-up" data-hub-sets-up>
    <dt>{t('app.workspace.hub_sets_up_collection')}</dt><dd>{t('app.workspace.hub_sets_up_collection_named')}</dd>
    {#if products.length > 0}<dt>{t('app.nav.products')}</dt><dd>{products.join(', ')}</dd>{/if}
    {#each parts as each (each.part)}<dt>{t(`app.templates.creates.${each.part}`)}</dt><dd>{each.names.length}</dd>{/each}
  </dl>
{/snippet}

{#snippet startTools(hasHeading: boolean)}
  <div class="tools">
    <div class="tools-top">
      {#if hasHeading}<h2 id="{uid}-start">{t('app.workspace.hub_start_title')}</h2>{/if}
      {#if families.length > 0}
        <div class="search">
          <SearchField label={t('app.workspace.hub_search')} isLabelHidden clearLabel={t('app.workspace.hub_search_clear')} placeholder={t('app.workspace.hub_search')} bind:value={query} autocomplete="off" />
        </div>
      {/if}
    </div>
    {#if families.length > 0}
      <div class="families" role="radiogroup" aria-label={t('app.workspace.hub_families')} data-family-filter>
        {#each [{ family: 'all' as const, count: starts.length }, ...counts] as each (each.family)}
          <label class="family">
            <input type="radio" name="{uid}-family" value={each.family} checked={family === each.family} onchange={() => (family = each.family)} />
            {each.family === 'all' ? t('app.workspace.hub_family_all') : familyName(each.family)}
            <small>{each.count}</small>
          </label>
        {/each}
      </div>
      <p class="count" hidden={isFiltered}>{t('app.workspace.hub_count_all', { count: starts.length, families: counts.length })}</p>
      <p class="count" role="status" hidden={!isFiltered}>{isFiltered ? t('app.workspace.hub_count_found', { count: shown.count }) : ''}</p>
    {:else if isPrivateOnly}
      <p class="count">{t('app.workspace.hub_templates_need_structure')}</p>
    {/if}
  </div>
{/snippet}

{#snippet startTiles()}
  <div class="scroll">
    <ChoiceCards label={t('app.workspace.hub_start_title')} isLabelHidden options={[empty]} {groups} mark="tick" layout="grid"
      bind:value={() => start, (next) => choose(next)} />
    {#if isFiltered && shown.count === 0}
      <p class="no-match">
        {query.trim() === '' ? t('app.workspace.hub_no_match_family') : t('app.workspace.hub_no_match', { query: query.trim() })}
        <Button size="sm" tone="secondary" onclick={() => { query = ''; family = 'all'; }}>{t('app.workspace.hub_reset_search')}</Button>
      </p>
    {/if}
  </div>
{/snippet}

<CreatePage
  isFilled
  head={{
    title,
    breadcrumb: {
      label: t('app.workspace.hub_new_trail'),
      expandLabel: t('app.workspace.hub_new_trail_expand'),
      trail: [{ id: 'hubs', label: t('app.nav.hubs') }, { id: 'new', label: t('app.workspace.create_hub') }],
    },
    primary: { label: t('app.workspace.create_hub'), opener: 'create-hub', onclick: create, disabledReason: primaryReason ?? (isSaving ? t('app.workspace.creating') : undefined) },
    secondary: [{ label: t('app.workspace.cancel'), isQuiet: true, onclick: oncancel }],
  }}
  close={{ label: t('app.workspace.cancel'), onclose: oncancel }}
>
  <div class="panes" data-create-hub>
    <section class="details" aria-label={t('app.workspace.hub_details')} data-private={isPrivateHub ? '' : undefined} bind:this={detailsPane}>
      {#if failure && !isNameFailure}
        <p class="failure" role="alert">{failure.message}</p>
      {:else if planFailure}
        <p class="failure" role="alert">{planFailure}</p>
      {/if}
      <div class="name a-name" bind:this={nameField}>
      <!-- The label stands over the mark and the field together, as the field names the pair. -->
      <label class="name-label" for="{uid}-name">{t('app.workspace.hub_name')}</label>
      <div class="name-row">
        <span class="mark">
          <ContainerMark look={{ type: 'HUB', icon: symbol ?? null, color_token: colour ?? null }} size="lg" hasDefaultTile />
          {#if isPrivateHub}<span class="lock" aria-hidden="true"><Icon name="lock" size="sm" /></span>{/if}
        </span>
        <Input
          id="{uid}-name"
          label={t('app.workspace.hub_name')}
          isLabelHidden
          bind:value={name}
          maxlength={200}
          autocomplete="off"
          isInvalid={nameError !== undefined}
          describedBy={nameError ? `${uid}-name-error` : undefined}
          oninput={() => { isNameMissing = false; isNameFailure = false; }}
          onkeydown={(event: KeyboardEvent) => {
            if (event.key === 'Enter' && !event.isComposing) {
              event.preventDefault();
              void create();
            }
          }}
        />
      </div>
      <!-- Under the row, not under the field alone, so the mark keeps its foot on the field's. -->
      {#if nameError}<p class="name-error" id="{uid}-name-error"><Icon name="circle-x" size="sm" /><span>{nameError}</span></p>{/if}
      </div>

      {#if isPrivacyOffered}
        <div class="private a-priv" data-on={isPrivate ? '' : undefined}>
          <span class="private-word" aria-hidden="true"><Icon name="lock" size="sm" />{t('app.workspace.private')}</span>
          <Switch label={t('app.workspace.private')} isLabelHidden bind:checked={isPrivate} aria-describedby="{uid}-private" />
          <p class="private-hint" id="{uid}-private">{t('app.workspace.private_hint')}</p>
        </div>
      {:else if isPrivateOnly}
        <p class="private-only a-priv">{t('app.workspace.private_hint')}</p>
      {/if}

      {#if isPhone}
        <!-- Closed until opened: the mark beside the name keeps the choice in view meanwhile. -->
        <div class="a-look"><Disclosure label={t('app.workspace.hub_look')} tone="row" bind:isOpen={isLookOpen}><LookFields bind:symbol bind:colour /></Disclosure></div>
      {:else}
        <div class="look a-look"><LookFields bind:symbol bind:colour /></div>
      {/if}

      <div class="a-desc"><Textarea label={t('app.workspace.description')} bind:value={description} rows={2} /></div>

      <div class="summary a-sum" data-hub-summary>
        <div class="summary-head">
          <span class="summary-mark" aria-hidden="true"><Icon name={start === EMPTY_HUB ? 'plus' : (chosenStart?.icon ?? 'layout-template')} /></span>
          <span class="summary-name"><small>{kindWord}</small><strong>{start === EMPTY_HUB ? t('app.workspace.hub_empty') : chosenStart?.label}</strong></span>
          {#if isPhone}
            <!-- One element whatever it says, so the focus given back on closing the step finds it. -->
            <Button size="sm" tone="secondary" aria-haspopup="dialog" onclick={() => (isStepOpen = true)} data-hub-step-open>
              {start === EMPTY_HUB ? t('app.workspace.hub_template_open') : t('app.workspace.hub_template_change')}
            </Button>
          {/if}
        </div>
        <p class="summary-line">{start === EMPTY_HUB ? t('app.workspace.hub_empty_long') : chosenStart?.summary}</p>
        {#if chosenTemplate}
          <TemplateTakeFields template={chosenTemplate} plansWorkspace={chosenTemplate.taken_version == null && isRested} samplesOffered={carriesSamples(chosenTemplate)}
            bind:answers bind:withSamples bind:acknowledged bind:isPlanning bind:mustAcknowledge bind:failure={planFailure} {creates} />
        {/if}
      </div>
    </section>

    {#if isPhone}
      <Dialog bind:isOpen={isStepOpen} size="l" isStep title={t('app.workspace.hub_template_open')} dismissLabel={t('app.workspace.hub_template_back')}
        primary={{ label: t('app.workspace.hub_template_done'), onclick: () => (isStepOpen = false) }}>
        <div class="start" data-hub-gallery data-hub-step>
          {@render startTools(false)}
          {@render startTiles()}
        </div>
      </Dialog>
    {:else}
      <section class="start" aria-labelledby="{uid}-start" data-hub-gallery>
        {@render startTools(true)}
        {@render startTiles()}
      </section>
    {/if}
  </div>
</CreatePage>

<style>
  /* From `expanded`: the details at the pane's width beside the start, each scrolling on its own. */
  .panes {
    display: grid;
    grid-template-columns: var(--layout-pane-width) minmax(0, 1fr);
    grid-template-rows: minmax(0, 1fr);
    block-size: 100%;
    min-block-size: 0;
  }

  .details {
    display: flex;
    flex-direction: column;
    gap: var(--sp-200);
    padding: var(--sp-300);
    overflow: auto;
    background: var(--bg-surface);
    border-inline-end: var(--bw-hairline) solid var(--border-subtle);
  }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }

  /* The mark stands beside the field, its foot on the field's foot; the label above both. */
  .name-row { display: flex; align-items: flex-end; gap: var(--sp-150); }
  .name-row > :global(:last-child) { flex: 1; min-inline-size: 0; }
  .name-row > .mark { position: relative; display: inline-flex; flex: none; }
  /* One column no wider than the details: a field's own width never pushes the row past them. */
  .name { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-050); }
  .name-label { color: var(--text-secondary); font-size: var(--fs-090); font-weight: var(--fw-medium); }
  .name-error { display: flex; align-items: center; gap: var(--sp-050); margin: 0; color: var(--text-danger); font-size: var(--fs-075); }

  .lock {
    position: absolute;
    inset-block-end: calc(var(--sp-050) * -1);
    inset-inline-end: calc(var(--sp-050) * -1);
    display: grid;
    place-items: center;
    inline-size: var(--sp-250);
    block-size: var(--sp-250);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-full);
    background: var(--bg-surface);
    color: var(--text-primary);
  }

  .lock :global(svg) { inline-size: var(--fs-050); block-size: var(--fs-050); }

  /* *Private* in a bordered box: the word and the switch on one line, the sentence under both. */
  .private {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--sp-050) var(--sp-150);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
  }

  .private[data-on] { border-color: var(--accent-primary); background: var(--accent-primary-subtle); }
  .private-word { display: flex; align-items: center; gap: var(--sp-100); font-weight: var(--fw-semibold); }
  .private-hint { grid-column: 1 / -1; margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
  .private[data-on] .private-hint { color: var(--text-secondary); }
  .private-only { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }

  .look { display: grid; gap: var(--sp-150); }

  .summary {
    display: grid;
    gap: var(--sp-100);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-canvas);
  }

  .summary-head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .summary-mark {
    display: grid;
    flex: none;
    place-items: center;
    inline-size: var(--density-control-sm-min);
    block-size: var(--density-control-sm-min);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
  }

  .summary-mark :global(svg) { inline-size: var(--fs-200); block-size: var(--fs-200); }
  .summary-name { flex: 1; min-inline-size: 0; }
  .summary-name small { display: block; color: var(--text-subtle); font-size: var(--fs-075); }
  .summary-name strong { font-weight: var(--fw-semibold); }
  .summary-line { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }

  .sets-up { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: var(--sp-025) var(--sp-150); margin: 0; font-size: var(--fs-075); }
  .sets-up dt { color: var(--text-subtle); }
  .sets-up dd { margin: 0; overflow-wrap: anywhere; }

  .start { display: grid; grid-template-rows: auto minmax(0, 1fr); min-block-size: 0; min-inline-size: 0; }

  .tools { display: grid; gap: var(--sp-150); padding: var(--sp-200) var(--sp-300) var(--sp-150); background: var(--bg-canvas); }
  .tools-top { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-150); }
  .tools-top h2 { flex: 1; min-inline-size: 8ch; margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .search { flex: 0 1 var(--layout-field-md); min-inline-size: var(--layout-field-sm); }

  .families { display: flex; flex-wrap: wrap; gap: var(--sp-050); }

  .family {
    box-sizing: border-box;
    position: relative;
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    min-block-size: var(--density-control-sm-min);
    padding: 0 var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-full);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font-size: var(--fs-090);
    white-space: nowrap;
    cursor: pointer;
  }

  .family input { position: absolute; inset: 0; margin: 0; opacity: 0; cursor: inherit; }
  .family small { color: var(--text-subtle); font-size: var(--fs-050); }
  .family:hover { border-color: var(--border-strong); }
  .family:has(input:checked) { border-color: var(--accent-primary); background: var(--accent-primary-subtle); color: var(--text-brand); font-weight: var(--fw-medium); }
  .family:has(input:focus-visible) { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .count { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
  .count[hidden] { display: none; }

  .scroll { overflow: auto; padding: 0 var(--sp-300) var(--sp-300); }
  .scroll :global([role='radiogroup'] > .group) { margin-block-start: var(--sp-200); }

  .no-match { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); margin: 0; padding: var(--sp-300) 0; color: var(--text-secondary); font-size: var(--fs-090); }

  /* 600 to 904: one scroll under the head; the details a block of two columns on top, the start's
     tools sticking at its top. */
  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width < 905px) {
    .panes { display: block; overflow: auto; }

    .details {
      display: grid;
      grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
      grid-template-areas: 'name look' 'priv look' 'desc sum';
      align-items: start;
      padding: var(--sp-200) var(--sp-300);
      overflow: visible;
      border-inline-end: 0;
      border-block-end: var(--bw-hairline) solid var(--border-subtle);
    }

    .failure { grid-column: 1 / -1; }
    .a-name { grid-area: name; }
    .a-priv { grid-area: priv; }
    .a-look { grid-area: look; }
    .a-desc { grid-area: desc; }
    .a-sum { grid-area: sum; }

    .start { display: block; }
    .tools { position: sticky; inset-block-start: 0; z-index: var(--z-sticky); }
    .scroll { overflow: visible; }
  }

  /* Below 600 the details are the page, one column; the starts are the second step, a full-screen
     layer whose tools stay while its tiles scroll, the families one row that scrolls sideways. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    /* The details fill the screen down to the foot's bar, one surface as far as the primary. */
    .panes { display: flex; flex-direction: column; block-size: auto; overflow: visible; }
    .details { display: flex; flex: 1 0 auto; align-items: stretch; padding: var(--sp-200); border-block-end: 0; }

    .start { display: grid; flex: 1 1 auto; grid-template-columns: minmax(0, 1fr); grid-template-rows: auto minmax(0, 1fr); gap: var(--sp-200); min-block-size: 0; }
    .tools { position: static; padding: 0; background: none; }
    .search { flex: 1 1 100%; min-inline-size: 0; }

    .families {
      flex-wrap: nowrap;
      overflow-x: auto;
      margin-inline: calc(var(--sp-200) * -1);
      padding: var(--sp-025) var(--sp-200);
    }

    /* The tiles scroll here, their focus rings inside the edge. */
    .scroll { overflow: auto; margin-inline: calc(var(--sp-050) * -1); padding: var(--sp-050) var(--sp-050) var(--sp-300); }
  }
</style>
