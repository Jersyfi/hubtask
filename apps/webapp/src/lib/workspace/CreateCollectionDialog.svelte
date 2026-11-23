<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Creating a collection in a hub. A hub is a page of its own (ADR-0111); a collection's name,
  // kind and look are two to six fields of one group, which is *Dialog M*.
  //
  // A dialog rather than an inline field, and for the reason the rename beside it gives: a name
  // collision arrives as a 409 with `containers.name_taken`, the reader's next action is to type a
  // different name, and a sentence somewhere else on the screen is one they would have to carry
  // back to the field.
  //
  // The kinds are the hub page's tiles (`ChoiceCards` with the tick, UC-WRK-01 check 12): one
  // radio group, *Like its hub* first and chosen at opening - no preset, so the collection follows
  // its hub now and later (UC-WRK-23 check 1) -, then every preset the installation answers,
  // *Scrum* where *Project* is offered, and *Desk*. A choice changes by choosing another.
  //
  // A colour and a symbol are chosen here as on the hub's page, from the same fields (UC-WRK-33):
  // a place is told apart from the moment it exists. Neither is required - a place nobody chose for
  // keeps its type's mark.

  import { ChoiceCards, Dialog, Disclosure, Icon, Stack, Textarea, VisuallyHidden, type ChoiceCard } from '@hubtask/design-system/components';

  import { containers } from '../data/containers.svelte.ts';
  import { presetMark, presetName, presetOrigin, presets, presetValue } from '../data/presets.svelte.ts';
  import { entryTypes } from '../data/entrytypes.svelte.ts';
  import { portalTypesOf } from '../portal/portal.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import ContainerMark from './ContainerMark.svelte';
  import LookFields from './LookFields.svelte';
  import { DESK, LIKE_HUB, SCRUM, collectionRequest } from './createCollection.ts';
  import type { PlaceSymbol } from './look.ts';
  import { renderProblem } from '../problem.ts';

  import type { LabelToken } from '@hubtask/design-system';
  import type { TransportError } from '@hubtask/sync-engine';
  import { tick, untrack } from 'svelte';

  interface Props {
    isOpen: boolean;
    /** The hub the collection goes into. */
    parentId?: string;
    /** Ran after the write succeeded, with the collection the server answered and where it opens. */
    oncreated?: (id: string, landing?: string) => void;
  }

  let { isOpen = $bindable(), parentId, oncreated }: Props = $props();

  let name = $state('');
  let description = $state('');
  let kind = $state(LIKE_HUB);
  let symbol = $state<PlaceSymbol | undefined>(undefined);
  let colour = $state<LabelToken | undefined>(undefined);
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isNameFailure = $state(false);
  /** *Create* was pressed without a name: the field says so (design-system.md §4, case 3). */
  let isNameMissing = $state(false);

  // Reopening starts a new answer rather than resuming the last refused one.
  $effect(() => {
    if (isOpen) return;
    name = '';
    description = '';
    kind = LIKE_HUB;
    symbol = undefined;
    colour = undefined;
    failure = undefined;
    isNameFailure = false;
    isNameMissing = false;
  });

  $effect(() => {
    if (!isOpen) return;
    return untrack(() => presets.open());
  });

  // *Desk* (the navigation concept §5.1): a collection that offers the hub's portal types by name.
  // Without a portal type in the hub its tile is refused and says why.
  $effect(() => {
    if (!isOpen) return;
    return untrack(() => entryTypes.open());
  });
  const deskTypes = $derived(!parentId ? [] : portalTypesOf(entryTypes.all, parentId));

  const project = $derived(presets.all.find((each) => each.key === 'PROJECT'));
  const kinds = $derived<ChoiceCard[]>([
    { value: LIKE_HUB, label: t('app.workspace.kind_like_hub'), summary: t('app.workspace.kind_like_hub_about'), icon: 'hub' },
    ...presets.all.map((each) => ({
      value: presetValue(each),
      label: presetName(each),
      summary: each.key ? t('app.structure.preset_marked', { origin: presetOrigin(each), about: t(`app.structure.preset_about.${each.key}`) }) : presetOrigin(each),
      icon: presetMark(each),
    })),
    ...(project
      ? [{ value: SCRUM, label: t('app.workspace.kind_scrum'), summary: t('app.structure.preset_marked', { origin: presetOrigin(project), about: t('app.workspace.kind_about.SCRUM') }), icon: 'repeat' as const }]
      : []),
    deskTypes.length > 0
      ? { value: DESK, label: t('app.workspace.kind_desk'), summary: t('app.workspace.kind_desk_about'), icon: 'inbox' }
      : { value: DESK, label: t('app.workspace.kind_desk'), icon: 'inbox', disabledReason: t('app.workspace.kind_desk_needs') },
  ]);

  const hubName = $derived(!parentId ? undefined : containers.find(parentId)?.name);
  const title = $derived(hubName ? t('app.workspace.new_collection_in', { hub: hubName }) : t('app.workspace.create_collection'));

  let nameInput = $state<HTMLInputElement | null>(null);
  const uid = $props.id();
  const nameErrorId = `${uid}-name-error`;
  const nameError = $derived(isNameMissing ? t('app.workspace.name_required') : isNameFailure ? failure?.message : undefined);

  // As on the hub's page: a missing or taken name marks the field and takes the focus there. A
  // refusal that names no field is the form's: the focus moves to it, which scrolls it into view
  // above the buttons and reads it out (design-system.md §13, G5).
  const focusName = () => queueMicrotask(() => nameInput?.focus());
  let failureNote = $state<HTMLElement | null>(null);
  const focusFailure = () => void tick().then(() => failureNote?.focus());

  async function create() {
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
      const request = collectionRequest({ parentId, name, description, symbol, colour, kind });
      // The `Idempotency-Key` is the caller's, as it is at every other write in this application:
      // pressing Create twice is one intent, not two collections.
      const created = await containers.create(request.body, crypto.randomUUID());
      if (request.isDesk && deskTypes.length > 0) {
        await containers.setStructure(created.id, { settings: { entry_types: { TASK: deskTypes } } }, created.version);
      }
      // It appears because the write invalidated the level and the engine re-read it. Nothing is
      // inserted into the sidebar or the list by hand.
      isOpen = false;
      oncreated?.(created.id, request.landing(created.id));
    } catch (error) {
      const problem = error as TransportError;
      failure = renderProblem(problem, messages);
      isNameFailure =
        problem.detailCode === 'containers.name_taken' || failure.fields.has('/name');
      if (isNameFailure) focusName();
      else focusFailure();
    } finally {
      isSaving = false;
    }
  }
</script>

{#snippet chosenLook()}
  <ContainerMark look={{ type: 'COLLECTION', icon: symbol ?? null, color_token: colour ?? null }} />
  <VisuallyHidden>{[colour ? t(`app.colour.${colour}`) : undefined, symbol ? t(`app.symbol.${symbol}`) : undefined].filter(Boolean).join(', ')}</VisuallyHidden>
{/snippet}

<Dialog
  bind:isOpen
  {title}
  size="m"
  dismissLabel={t('app.workspace.cancel')}
  primary={{
    label: t('app.workspace.create'),
    isBusy: isSaving,
    busyLabel: t('app.workspace.creating'),
    onclick: create,
  }}
>
  <Stack gap="200">
    <!-- The name's label inside its box, as the concept's *New collection* draws it. -->
    <div class="name-field">
      <label class="name" data-invalid={nameError ? '' : undefined}>
        <span class="name-label">{t('app.workspace.name_inline')}</span>
        <input type="text" bind:this={nameInput} bind:value={name} autocomplete="off"
          oninput={() => { isNameMissing = false; isNameFailure = false; }}
          onkeydown={(event) => {
            if (event.key === 'Enter' && !event.isComposing) {
              event.preventDefault();
              void create();
            }
          }}
          aria-invalid={nameError ? 'true' : undefined} aria-describedby={nameError ? nameErrorId : undefined} />
      </label>
      {#if nameError}<p class="name-error" id={nameErrorId}><Icon name="circle-x" size="sm" /><span>{nameError}</span></p>{/if}
    </div>
    <ChoiceCards label={t('app.structure.preset_at_creation')} options={kinds} mark="tick" layout="grid" bind:value={kind} />
    <!-- The look folded to one row, as on a phone's create-hub page: the one fold *Dialog M* allows.
         Closed, the row shows the place's mark as chosen and says the pair in words. -->
    <Disclosure label={t('app.workspace.hub_look')} tone="row" value={symbol || colour ? chosenLook : undefined}><LookFields bind:symbol bind:colour /></Disclosure>
    <!-- Open, because a dialog holds at most one `Disclosure` (design-system.md §11.8). -->
    <Textarea label={t('app.workspace.description')} bind:value={description} rows={2} />
    <!-- Everything that is not about the name is a sentence above the buttons rather than a field
         error, because there is no field for it to land on. -->
    {#if failure && !isNameFailure}
      <p class="failure" role="alert" tabindex="-1" bind:this={failureNote}>{failure.message}</p>
    {/if}
  </Stack>
</Dialog>

<style>
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; scroll-margin-block: var(--sp-200); }
  .failure:focus { outline: none; }
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
  /* The hub page's line under its name: the mark and the words. */
  .name-error { display: flex; align-items: center; gap: var(--sp-050); margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
