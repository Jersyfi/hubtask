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
  // the catalogue (`STRUCTURE`). A template is not sent with the create: the hub is made as from the
  // workspace's default and opens on its *Products* section with that template's plan (§8.11), so
  // the plan is read in words before anything else is applied.

  import { untrack } from 'svelte';

  import { Button, Dialog, Input, Radio, Stack, Switch, TemplateCard, TemplateGallery, Textarea, VisuallyHidden } from '@hubtask/design-system/components';
  import type { ConfigTemplate, ConfigTemplateList } from '@hubtask/sync-engine';

  import TemplatePreview from '../settings/TemplatePreview.svelte';
  import { config } from '../data/config.svelte.ts';
  import { holdsHere } from '../data/capability.svelte.ts';
  import { hubFamilies, markOf, prefilledName, switchedOn } from './hubGallery.ts';

  import { containers } from '../data/containers.svelte.ts';
  import { presetChoice, presetName, presetOrigin, presets, presetValue } from '../data/presets.svelte.ts';
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
     * Ran after the write succeeded, with the container the server answered and, for a hub, the
     * catalogue template chosen for it to take next.
     */
    oncreated?: (id: string, template?: string) => void;
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

  // Reopening starts a new answer rather than resuming the last refused one.
  $effect(() => {
    if (isOpen) return;
    name = '';
    description = '';
    isPrivate = false;
    preset = '';
    fromTemplate = '';
    filled = '';
    symbol = undefined;
    colour = undefined;
    failure = undefined;
    isNameFailure = false;
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

  // Without presets answered, the *Simple* family is the workspace's default alone, so a template
  // chosen can always be unchosen.
  const simple = $derived(presetOptions.length > 0 ? presetOptions
    : [{ value: '', label: t('app.structure.preset_workspace_default'), hint: t('app.structure.preset_workspace_default_about') }]);

  $effect(() => {
    if (!isOpen || type !== 'HUB' || isPrivateOnly) return;
    return untrack(() => workspace.open());
  });
  const isPrivateHubOnly = $derived(isHub && isPrivateOnly);
  const isPrivacyOffered = $derived(isHub && !isPrivateOnly && workspace.workspace?.private_hubs_offered === true);
  const title = $derived(t(isPrivateHubOnly ? 'app.workspace.create_private_hub' : isHub ? 'app.workspace.create_hub' : 'app.workspace.create_collection'));

  /** *Create* was pressed without a name: the field says so (design-system.md §4, case 3). */
  let isNameMissing = $state(false);
  let body = $state<HTMLElement | null>(null);

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
      const created = await containers.create(
        {
          type,
          parent_id: isHub ? null : parentId,
          name: name.trim(),
          description: description.trim() === '' ? null : description.trim(),
          ...lookBody(symbol, colour),
          ...(isPrivateHubOnly || (isPrivacyOffered && isPrivate) ? { private: true } : {}),
          ...presetChoice(preset),
        },
        crypto.randomUUID(),
      );
      // It appears because the write invalidated the level and the engine re-read it. Nothing is
      // inserted into the sidebar or the list by hand.
      const chosenTemplate = isHub && fromTemplate ? fromTemplate : undefined;
      isOpen = false;
      oncreated?.(created.id, chosenTemplate);
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
  dismissLabel={t('app.workspace.cancel')}
>
  <div bind:this={body}>
  <Stack gap="150">
    <Input
      label={t(isHub ? 'app.workspace.hub_name' : 'app.workspace.collection_name')}
      bind:value={name}
      oninput={() => (isNameMissing = false)}
      error={isNameMissing ? t('app.workspace.name_required') : isNameFailure ? failure?.message : undefined}
    />
    <Textarea label={t('app.workspace.description')} bind:value={description} />
    <LookFields {type} bind:symbol bind:colour />
    {#if isHub && (presetOptions.length > 0 || families.length > 0)}
      <div class="gallery" data-hub-gallery>
        <p class="lead">{t('app.workspace.hub_gallery_about')}</p>
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
        {#each families as group (group.family)}
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
        {#if fromTemplate}<p class="lead" data-hub-template-next>{t('app.workspace.hub_template_next')}</p>{/if}
      </div>
    {:else if presetOptions.length > 0}
      <Radio label={t('app.structure.preset_at_creation')} options={presetOptions} bind:value={preset} />
    {/if}
    {#if isPrivateHubOnly}
      <p class="private">{t('app.workspace.private_hint')}</p>
    {:else if isPrivacyOffered}
      <Switch label={t('app.workspace.private')} hint={t('app.workspace.private_hint')} bind:checked={isPrivate} />
    {/if}
    <!-- Everything that is not about the name is a sentence above the buttons rather than a field
         error, because there is no field for it to land on. -->
    {#if failure && !isNameFailure}
      <p class="failure" role="alert">{failure.message}</p>
    {/if}
  </Stack>
  </div>

  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.workspace.cancel')}</Button>
    <Button
      isBusy={isSaving}
      busyLabel={t('app.workspace.creating')}
      onclick={create}
    >
      {title}
    </Button>
  {/snippet}
</Dialog>

<style>
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
  .gallery { display: flex; flex-direction: column; gap: var(--sp-150); }
  .lead { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); max-width: 64ch; }
  .private { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); max-width: 64ch; }
</style>
