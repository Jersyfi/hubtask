<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Creating a hub or a collection.
  //
  // One component for both, for the reason `ContainerView` is one view for both: they differ in
  // what they hold, not in what it takes to make one. A hub is created at the top level and a
  // collection under a named hub, and that is the whole difference in this form.
  //
  // A dialog rather than an inline field, and for the reason the rename beside it gives: a name
  // collision arrives as a 409 with `containers.name_taken`, the reader's next action is to type a
  // different name, and a sentence somewhere else on the screen is one they would have to carry
  // back to the field.
  //
  // A colour and a symbol are chosen here as on the edit dialog, from the same fields (UC-WRK-33):
  // a place is told apart from the moment it exists. Neither is required - a place nobody chose for
  // keeps its type's mark.
  //
  // *Private* is offered for a hub where the workspace has more than one person
  // (`private_hubs_offered`, UC-ID-16 check 8): a workspace of one has nobody to keep a hub from.
  // Every person may make one, whatever their role, so the switch carries no gate of its own.
  // Somebody without the right to shape the workspace may create *only* a private hub of their own
  // (UC-ID-16 check 1): for them the dialog is that and nothing else - no switch, the hub is private.
  // Whoever opens it decides which, from the manifest (`hubCreation`).
  //
  // *How much structure* (UC-WRK-23 check 1): one of the presets, or nothing chosen - a collection
  // then follows its hub, now and later, and a hub starts from the workspace's default preset.
  // Offered where the installation answers presets; without them the server decides as before.
  //
  // A hub is chosen from the template gallery instead (user-experience.md §5.2): the presets as its
  // *Simple* family, then the catalogue's templates with a hub part by family, for whoever may read
  // the catalogue (`STRUCTURE`). A template chosen asks its questions and says what it sets up
  // before the press, and `POST /containers:from-template` makes the hub with its parts in one
  // transaction: a refused part leaves no hub behind.

  import { untrack } from 'svelte';

  import { Button, Dialog, Disclosure, Icon, Inline, Input, Stack, Switch, TemplateCard, TemplateGallery, Textarea, VisuallyHidden } from '@hubtask/design-system/components';
  import type { ConfigTemplate, ConfigTemplateList } from '@hubtask/sync-engine';

  import TemplatePreview from '../settings/TemplatePreview.svelte';
  import { config } from '../data/config.svelte.ts';
  import { holdsHere } from '../data/capability.svelte.ts';
  import { answersOf, carriesSamples } from '../data/configtemplates.ts';
  import { afterStarter } from './starterSet.ts';
  import { hubFamilies, markOf, prefilledName, switchedOn } from './hubGallery.ts';
  import TemplateTakeFields from './TemplateTakeFields.svelte';

  import { containers } from '../data/containers.svelte.ts';
  import { presetChoice, presetName, presetOrigin, presets, presetValue } from '../data/presets.svelte.ts';
  import { entryTypes } from '../data/entrytypes.svelte.ts';
  import { portalTypesOf } from '../portal/portal.ts';
  import { workspace } from '../data/workspace.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { focusFirst } from '../forms/missing.ts';
  import LookFields from './LookFields.svelte';
  import { lookBody, type PlaceSymbol } from './look.ts';
  import { renderProblem } from '../problem.ts';

  import type { LabelToken } from '@hubtask/design-system';
  import type { TransportError } from '@hubtask/sync-engine';

  interface Props {
    isOpen: boolean;
    /** `HUB` creates at the top level; `COLLECTION` creates under `parentId`. */
    type: 'HUB' | 'COLLECTION';
    /** The hub a collection goes into. Ignored for a hub, which sits in nothing. */
    parentId?: string;
    /**
     * A hub that can only be private: the reader may not create a shared one. Ignored for a
     * collection.
     */
    isPrivateOnly?: boolean;
    /** A catalogue template chosen before the dialog opened (a product's first visit). */
    template?: string;
    /**
     * Ran after the write succeeded, with the container the server answered and, for a hub made
     * from a template, where that template's main product opens in it.
     */
    oncreated?: (id: string, landing?: string) => void;
  }

  let { isOpen = $bindable(), type, parentId, isPrivateOnly = false, template: preselected, oncreated }: Props = $props();

  let name = $state('');
  let description = $state('');
  let isPrivate = $state(false);
  /** The preset chosen, or empty for the hub's (a collection) or the workspace's default (a hub). */
  let preset = $state('');
  /** The catalogue template chosen for a hub, or empty where a preset (or the default) is. */
  let fromTemplate = $state('');
  /** The name the last template chosen filled in, so a name the reader typed is never replaced. */
  let filled = $state('');
  let symbol = $state<PlaceSymbol | undefined>(undefined);
  let colour = $state<LabelToken | undefined>(undefined);
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isNameFailure = $state(false);
  let answers = $state<Record<string, string>>({});
  let withSamples = $state(true);
  let acknowledged = $state(false);
  let isPlanning = $state(false);
  let mustAcknowledge = $state(false);
  let planFailure = $state<string | undefined>(undefined);

  // Reopening starts a new answer rather than resuming the last refused one.
  $effect(() => {
    if (isOpen) return;
    name = '';
    description = '';
    isPrivate = false;
    preset = '';
    fromTemplate = '';
    shownFamily = 'all';
    filled = '';
    symbol = undefined;
    colour = undefined;
    failure = undefined;
    isNameFailure = false;
    answers = {};
    withSamples = true;
    acknowledged = false;
    planFailure = undefined;
  });

  const isHub = $derived(type === 'HUB');

  $effect(() => {
    if (isOpen && preselected) untrack(() => { fromTemplate = preselected; preset = ''; });
  });

  /** The catalogue, for a shared hub and a reader who may read it; else only the presets. */
  const offersCatalogue = $derived(isHub && !isPrivateOnly && holdsHere('STRUCTURE') === true);
  let catalogue = $state<readonly ConfigTemplate[]>([]);
  $effect(() => {
    if (!isOpen || !offersCatalogue) return;
    return untrack(() => config.templates((next: { status: string; data?: ConfigTemplateList }) => {
      if (next.status === 'ready' && Array.isArray(next.data?.data)) catalogue = next.data.data;
    }));
  });
  const families = $derived(offersCatalogue ? hubFamilies(catalogue) : []);
  /** The family the gallery is narrowed to (§5.2's family filter), or every family. */
  let shownFamily = $state('all');
  const isShown = (family: string): boolean => shownFamily === 'all' || shownFamily === family;
  const templateName = (each: ConfigTemplate): string =>
    each.origin === 'HUBTASK' ? t(`app.structure.preset.${each.key.toUpperCase()}`) : each.name;
  const productsOf = (each: ConfigTemplate): string[] =>
    switchedOn(each).map((module) => t(`app.modules.name_${module.toLowerCase()}`));
  const summaryOf = (each: ConfigTemplate): string | undefined =>
    each.origin === 'HUBTASK' ? t(`app.templates.summary.${each.key.toUpperCase()}`) : undefined;
  function fillName(next: string): void {
    name = prefilledName(name, filled, next);
    filled = next;
  }
  function pickPreset(value: string): void {
    preset = value;
    if (fromTemplate) fillName('');
    fromTemplate = '';
  }
  function pickTemplate(key: string): void {
    if (fromTemplate !== key) { answers = {}; withSamples = true; acknowledged = false; }
    fromTemplate = key;
    preset = '';
    const chosen = catalogue.find((each) => each.key === key);
    if (chosen) fillName(templateName(chosen));
  }
  // A template chosen before the dialog opened names the hub once the catalogue has answered.
  $effect(() => {
    if (!isOpen || !preselected || fromTemplate !== preselected) return;
    const chosen = catalogue.find((each) => each.key === preselected);
    if (chosen) untrack(() => { if (name.trim() === '') fillName(templateName(chosen)); });
  });

  $effect(() => {
    if (!isOpen) return;
    return untrack(() => presets.open());
  });
  const presetOptions = $derived(
    presets.all.length === 0
      ? []
      : [
          isHub
            ? { value: '', label: t('app.structure.preset_workspace_default'), hint: t('app.structure.preset_workspace_default_about') }
            : { value: '', label: t('app.structure.preset_follow_hub'), hint: t('app.structure.preset_follow_hub_about') },
          // Each marked with where it comes from (UC-WRK-30 check 2): Hubtask's with what it is,
          // a saved one with whose it is.
          ...presets.all.map((each) => ({
            value: presetValue(each),
            label: presetName(each),
            hint: each.key
              ? t('app.structure.preset_marked', { origin: presetOrigin(each), about: t(`app.structure.preset_about.${each.key}`) })
              : presetOrigin(each),
          })),
        ],
  );

  // *Desk* (the navigation concept §5.1): a collection that offers the hub's portal types by name,
  // which is what makes it a desk; it starts from *Tasks*. Without a portal type in the hub the card
  // says so instead of offering a desk nobody could ask through.
  const DESK = 'desk';
  $effect(() => {
    if (!isOpen || isHub) return;
    return untrack(() => entryTypes.open());
  });
  const deskTypes = $derived(isHub || !parentId ? [] : portalTypesOf(entryTypes.all, parentId));

  // The kinds of a new collection as the navigation concept's *New collection* draws them: small
  // toggle cards, each a mark, a name and a few words. None pressed is the hub's own structure
  // (UC-WRK-23 check 1); pressing the chosen one again returns to that. *Scrum* is the *Project*
  // preset opened on its sprint view, where the first sprint starts (§5.1) - offered where the
  // installation answers *Project*.
  const SCRUM = 'scrum';
  const KIND_MARKS: Readonly<Record<string, 'square-check' | 'columns-3'>> = { LIST: 'square-check', TASKS: 'square-check', PROJECT: 'columns-3', FLOW: 'columns-3' };
  const kinds = $derived(
    isHub
      ? []
      : [
          ...presets.all.map((each) => ({
            value: presetValue(each),
            label: presetName(each),
            icon: (each.key ? KIND_MARKS[each.key] : undefined) ?? ('square-check' as const),
            hint: each.key ? t('app.structure.preset_marked', { origin: presetOrigin(each), about: t(`app.workspace.kind_about.${each.key}`) }) : presetOrigin(each),
          })),
          ...(presets.all.some((each) => each.key === 'PROJECT')
            ? [{ value: SCRUM, label: t('app.workspace.kind_scrum'), icon: 'repeat' as const, hint: t('app.structure.preset_marked', { origin: presetOrigin(presets.all.find((each) => each.key === 'PROJECT')!), about: t('app.workspace.kind_about.SCRUM') }) }]
            : []),
        ],
  );

  // Without presets answered, the *Simple* family is the workspace's default alone, so a template
  // chosen can always be unchosen.
  const simple = $derived(presetOptions.length > 0 ? presetOptions
    : [{ value: '', label: t('app.structure.preset_workspace_default'), hint: t('app.structure.preset_workspace_default_about') }]);

  $effect(() => {
    if (!isOpen || type !== 'HUB' || isPrivateOnly) return;
    return untrack(() => workspace.open());
  });
  const isPrivateHubOnly = $derived(isHub && isPrivateOnly);
  const chosenTemplate = $derived(isHub && fromTemplate ? catalogue.find((each) => each.key === fromTemplate) : undefined);
  // The workspace part is planned where the workspace has not taken the template: the hub's make
  // takes it then; one it took stays at its version.
  const plansWorkspace = $derived(chosenTemplate?.taken_version == null);
  const isPrivacyOffered = $derived(isHub && !isPrivateOnly && workspace.workspace?.private_hubs_offered === true);
  const hubName = $derived(isHub || !parentId ? undefined : containers.find(parentId)?.name);
  const title = $derived(
    isPrivateHubOnly ? t('app.workspace.create_private_hub')
      : isHub ? t('app.workspace.create_hub')
        : hubName ? t('app.workspace.new_collection_in', { hub: hubName }) : t('app.workspace.create_collection'),
  );
  const createLabel = $derived(isHub ? title : t('app.workspace.create'));

  /** *Create* was pressed without a name: the field says so (design-system.md §4, case 3). */
  let isNameMissing = $state(false);
  let body = $state<HTMLElement | null>(null);
  const uid = $props.id();
  const nameErrorId = `${uid}-name-error`;
  const kindsId = `${uid}-kind`;

  async function create() {
    if (isSaving) return;
    if (name.trim() === '') {
      isNameMissing = true;
      queueMicrotask(() => focusFirst(body));
      return;
    }
    isSaving = true;
    failure = undefined;
    isNameFailure = false;
    try {
      // The `Idempotency-Key` is the caller's, as it is at every other write in this application:
      // pressing Create twice is one intent, not two hubs.
      if (chosenTemplate) {
        const given = answersOf(chosenTemplate.questions, answers);
        const hub = await containers.createFromTemplate(
          {
            name: name.trim(),
            description: description.trim() === '' ? null : description.trim(),
            ...lookBody(symbol, colour),
            template: chosenTemplate.key,
            ...(Object.keys(given).length > 0 ? { answers: given } : {}),
            ...(carriesSamples(chosenTemplate) && withSamples ? { samples: true } : {}),
            ...(acknowledged ? { acknowledge: true } : {}),
          },
          crypto.randomUUID(),
        );
        isOpen = false;
        oncreated?.(hub.id, afterStarter(chosenTemplate, hub.id));
        return;
      }
      const created = await containers.create(
        {
          type,
          parent_id: isHub ? null : parentId,
          name: name.trim(),
          description: description.trim() === '' ? null : description.trim(),
          ...lookBody(symbol, colour),
          ...(isPrivateHubOnly || (isPrivacyOffered && isPrivate) ? { private: true } : {}),
          ...(preset === DESK ? { preset: 'TASKS' as const } : preset === SCRUM ? { preset: 'PROJECT' as const } : presetChoice(preset)),
        },
        crypto.randomUUID(),
      );
      if (preset === DESK && deskTypes.length > 0) {
        await containers.setStructure(created.id, { settings: { entry_types: { TASK: deskTypes } } }, created.version);
      }
      // It appears because the write invalidated the level and the engine re-read it. Nothing is
      // inserted into the sidebar or the list by hand.
      isOpen = false;
      oncreated?.(created.id, preset === SCRUM ? `/collections/${created.id}/sprint` : undefined);
    } catch (error) {
      const problem = error as TransportError;
      failure = renderProblem(problem, messages);
      isNameFailure =
        problem.detailCode === 'containers.name_taken' || failure.fields.has('/name');
    } finally {
      isSaving = false;
    }
  }
</script>

<Dialog
  bind:isOpen
  {title}
  isWide={isHub && (presetOptions.length > 0 || families.length > 0)}
  dismissLabel={t('app.workspace.cancel')}
>
  <div bind:this={body}>
  <Stack gap="150">
    {#if isHub}
      <Input
        label={t('app.workspace.hub_name')}
        bind:value={name}
        oninput={() => (isNameMissing = false)}
        error={isNameMissing ? t('app.workspace.name_required') : isNameFailure ? failure?.message : undefined}
      />
    {:else}
      <!-- The name's label inside its box, as the concept's *New collection* draws it. -->
      {@const nameError = isNameMissing ? t('app.workspace.name_required') : isNameFailure ? failure?.message : undefined}
      <div class="name-field">
        <label class="name" data-invalid={nameError ? '' : undefined}>
          <span class="name-label">{t('app.workspace.name_inline')}</span>
          <input type="text" bind:value={name} oninput={() => (isNameMissing = false)}
            aria-invalid={nameError ? 'true' : undefined} aria-describedby={nameError ? nameErrorId : undefined} />
        </label>
        {#if nameError}<p class="name-error" id={nameErrorId}>{nameError}</p>{/if}
      </div>
    {/if}
    {#if isHub && (presetOptions.length > 0 || families.length > 0)}
      <div class="gallery" data-hub-gallery>
        <p class="lead">{t('app.workspace.hub_gallery_about')}</p>
        {#if families.length > 0}
          <div role="group" aria-label={t('app.workspace.hub_families')} data-family-filter>
            <Inline gap="050">
              {#each ['all', 'simple', ...families.map((group) => group.family)] as family (family)}
                <Button size="sm" tone={shownFamily === family ? 'secondary' : 'subtle'} aria-pressed={shownFamily === family} onclick={() => (shownFamily = family)}>
                  {family === 'all' ? t('app.workspace.hub_family_all') : t(`app.workspace.hub_family.${family}`)}
                </Button>
              {/each}
            </Inline>
          </div>
        {/if}
        {#if isShown('simple')}
        <div data-family="simple">
          <TemplateGallery label={t('app.workspace.hub_family.simple')}>
            {#each simple as option (option.value)}
              {@const isChosen = fromTemplate === '' && preset === option.value}
              <TemplateCard name={option.label} summary={option.hint} {isChosen} isCompact data-hub-preset={option.value || 'default'}>
                {#snippet action()}
                  <Button size="sm" tone={isChosen ? 'primary' : 'secondary'} icon={isChosen ? 'check' : undefined} aria-pressed={isChosen} onclick={() => pickPreset(option.value)}>
                    {t(isChosen ? 'app.workspace.hub_template_chosen' : 'app.workspace.hub_template_choose')}<VisuallyHidden>{` ${option.label}`}</VisuallyHidden>
                  </Button>
                {/snippet}
              </TemplateCard>
            {/each}
          </TemplateGallery>
        </div>
        {/if}
        {#each families.filter((group) => isShown(group.family)) as group (group.family)}
          <div data-family={group.family}>
            <TemplateGallery label={t(`app.workspace.hub_family.${group.family}`)}>
              {#each group.templates as each (each.key)}
                {@const isChosen = fromTemplate === each.key}
                {@const name = templateName(each)}
                <TemplateCard {name} mark={markOf(each)} summary={summaryOf(each)} products={productsOf(each)} {isChosen} isCompact data-hub-template={each.key}
                  note={each.taken_version == null ? undefined : t('app.workspace.hub_template_in_workspace')}>
                  {#snippet details()}<TemplatePreview template={each} />{/snippet}
                  {#snippet action()}
                    <Button size="sm" tone={isChosen ? 'primary' : 'secondary'} icon={isChosen ? 'check' : undefined} aria-pressed={isChosen} onclick={() => pickTemplate(each.key)}>
                      {t(isChosen ? 'app.workspace.hub_template_chosen' : 'app.workspace.hub_template_choose')}<VisuallyHidden>{` ${name}`}</VisuallyHidden>
                    </Button>
                  {/snippet}
                </TemplateCard>
              {/each}
            </TemplateGallery>
          </div>
        {/each}
        {#if chosenTemplate}
          <div class="take" data-hub-template-take>
            <TemplateTakeFields template={chosenTemplate} {plansWorkspace} samplesOffered={carriesSamples(chosenTemplate)}
              bind:answers bind:withSamples bind:acknowledged bind:isPlanning bind:mustAcknowledge bind:failure={planFailure} />
          </div>
        {/if}
      </div>
    {:else if kinds.length > 0}
      <div class="kinds" role="group" aria-label={t('app.structure.preset_at_creation')} data-collection-kinds>
        {#each kinds as kind (kind.value)}
          {@const isChosen = preset === kind.value}
          <button type="button" class="kind" aria-pressed={isChosen} aria-labelledby="{kindsId}-{kind.value}" aria-describedby="{kindsId}-{kind.value}-about"
            data-collection-kind={kind.value} onclick={() => (preset = isChosen ? '' : kind.value)}>
            <span class="kind-name" id="{kindsId}-{kind.value}"><Icon name={kind.icon} size="sm" />{kind.label}</span>
            <span class="kind-about" id="{kindsId}-{kind.value}-about">{kind.hint}</span>
          </button>
        {/each}
        <button type="button" class="kind" aria-pressed={preset === DESK} aria-labelledby="{kindsId}-desk" aria-describedby="{kindsId}-desk-about"
          aria-disabled={deskTypes.length > 0 ? undefined : 'true'} data-collection-kind={DESK}
          onclick={() => { if (deskTypes.length > 0) preset = preset === DESK ? '' : DESK; }}>
          <span class="kind-name" id="{kindsId}-desk"><Icon name="inbox" size="sm" />{t('app.workspace.kind_desk')}</span>
          <span class="kind-about" id="{kindsId}-desk-about">{deskTypes.length > 0 ? t('app.workspace.kind_desk_about') : t('app.workspace.kind_desk_needs')}</span>
        </button>
      </div>
      {#if preset === ''}<p class="follows" data-follows-hub>{t('app.workspace.kind_follows_hub')}</p>{/if}
    {/if}
    <!-- What only dresses the place waits folded below the choice of what it starts from (UX-4). -->
    <Disclosure label={t('app.workspace.look_fold')} tone="quiet">
      <Stack gap="150">
        <Textarea label={t('app.workspace.description')} bind:value={description} />
        <div data-look-fields><LookFields {type} bind:symbol bind:colour /></div>
      </Stack>
    </Disclosure>
    {#if isPrivateHubOnly}
      <p class="private">{t('app.workspace.private_hint')}</p>
    {:else if isPrivacyOffered}
      <Switch label={t('app.workspace.private')} hint={t('app.workspace.private_hint')} bind:checked={isPrivate} />
    {/if}
    <!-- Everything that is not about the name is a sentence above the buttons rather than a field
         error, because there is no field for it to land on. -->
    {#if failure && !isNameFailure}
      <p class="failure" role="alert">{failure.message}</p>
    {:else if planFailure}
      <p class="failure" role="alert">{planFailure}</p>
    {/if}
  </Stack>
  </div>

  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.workspace.cancel')}</Button>
    <Button tone="primary"
      isBusy={isSaving}
      busyLabel={t('app.workspace.creating')}
      disabledReason={chosenTemplate && isPlanning ? t('app.starter.plan_reading') : chosenTemplate && mustAcknowledge && !acknowledged ? t('app.changes.acknowledge_first') : undefined}
      data-create-container
      onclick={create}
    >
      {createLabel}
    </Button>
  {/snippet}
</Dialog>

<style>
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
  .take { display: grid; gap: var(--sp-150); }
  .gallery { display: flex; flex-direction: column; gap: var(--sp-150); }
  .lead { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); max-width: 64ch; }
  .name-field { display: grid; gap: var(--sp-050); }
  .name {
    display: flex;
    align-items: center;
    gap: var(--sp-050);
    min-block-size: var(--density-control-md-min);
    padding-inline: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-strong);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    font-size: var(--fs-100);
    cursor: text;
  }
  .name:focus-within { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .name[data-invalid] { border-color: var(--status-danger-border); background: var(--status-danger-surface); }
  .name-label { flex: none; color: var(--text-secondary); }
  .name input {
    flex: 1;
    min-inline-size: 0;
    padding: 0;
    border: 0;
    background: none;
    color: var(--text-primary);
    font: inherit;
    font-weight: var(--fw-semibold);
    outline: none;
  }
  .name-error { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  /* Three columns of small toggle cards: a mark and a name, a few words under it. */
  .kinds { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--sp-100); }
  .kind {
    display: grid;
    align-content: start;
    gap: var(--sp-025);
    padding: var(--sp-100) calc(var(--sp-100) + var(--sp-025));
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-075);
    text-align: start;
    cursor: pointer;
  }
  .kind:hover { border-color: var(--border-strong); }
  .kind:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .kind[aria-pressed='true'] { border-color: var(--accent-primary); box-shadow: 0 0 0 var(--bw-ring) var(--accent-primary-subtle); }
  .kind[aria-disabled='true'] { color: var(--text-subtle); cursor: not-allowed; }
  .kind[aria-disabled='true'] .kind-name { color: var(--text-subtle); }
  .kind[aria-disabled='true']:hover { border-color: var(--border-subtle); }
  .kind-name { display: flex; align-items: center; gap: var(--sp-050); color: var(--text-primary); font-size: var(--fs-090); font-weight: var(--fw-semibold); }
  .follows { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) { .kinds { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  .private { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); max-width: 64ch; }
</style>
