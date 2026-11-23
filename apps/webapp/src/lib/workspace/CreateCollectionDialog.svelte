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
  // A colour and a symbol are chosen here as on the edit dialog, from the same fields (UC-WRK-33):
  // a place is told apart from the moment it exists. Neither is required - a place nobody chose for
  // keeps its type's mark.
  //
  // *How much structure* (UC-WRK-23 check 1): one of the presets, or nothing chosen - the collection
  // then follows its hub, now and later. Offered where the installation answers presets; without
  // them the server decides as before.

  import { Dialog, Disclosure, Icon, Stack, Textarea } from '@hubtask/design-system/components';

  import { containers } from '../data/containers.svelte.ts';
  import { presetChoice, presetName, presetOrigin, presets, presetValue } from '../data/presets.svelte.ts';
  import { entryTypes } from '../data/entrytypes.svelte.ts';
  import { portalTypesOf } from '../portal/portal.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { focusFirst } from '../forms/missing.ts';
  import LookFields from './LookFields.svelte';
  import { lookBody, type PlaceSymbol } from './look.ts';
  import { renderProblem } from '../problem.ts';

  import type { LabelToken } from '@hubtask/design-system';
  import type { TransportError } from '@hubtask/sync-engine';
  import { untrack } from 'svelte';

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
  /** The preset chosen, or empty for the hub's. */
  let preset = $state('');
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
    preset = '';
    symbol = undefined;
    colour = undefined;
    failure = undefined;
    isNameFailure = false;
  });

  $effect(() => {
    if (!isOpen) return;
    return untrack(() => presets.open());
  });

  // *Desk* (the navigation concept §5.1): a collection that offers the hub's portal types by name,
  // which is what makes it a desk; it starts from *Tasks*. Without a portal type in the hub the card
  // says so instead of offering a desk nobody could ask through.
  const DESK = 'desk';
  $effect(() => {
    if (!isOpen) return;
    return untrack(() => entryTypes.open());
  });
  const deskTypes = $derived(!parentId ? [] : portalTypesOf(entryTypes.all, parentId));

  // The kinds of a new collection as the navigation concept's *New collection* draws them: small
  // toggle cards, each a mark, a name and a few words. None pressed is the hub's own structure
  // (UC-WRK-23 check 1); pressing the chosen one again returns to that. *Scrum* is the *Project*
  // preset opened on its sprint view, where the first sprint starts (§5.1) - offered where the
  // installation answers *Project*.
  const SCRUM = 'scrum';
  const KIND_MARKS: Readonly<Record<string, 'square-check' | 'columns-3'>> = { LIST: 'square-check', TASKS: 'square-check', PROJECT: 'columns-3', FLOW: 'columns-3' };
  const kinds = $derived([
    ...presets.all.map((each) => ({
      value: presetValue(each),
      label: presetName(each),
      icon: (each.key ? KIND_MARKS[each.key] : undefined) ?? ('square-check' as const),
      hint: each.key ? t('app.structure.preset_marked', { origin: presetOrigin(each), about: t(`app.workspace.kind_about.${each.key}`) }) : presetOrigin(each),
    })),
    ...(presets.all.some((each) => each.key === 'PROJECT')
      ? [{ value: SCRUM, label: t('app.workspace.kind_scrum'), icon: 'repeat' as const, hint: t('app.structure.preset_marked', { origin: presetOrigin(presets.all.find((each) => each.key === 'PROJECT')!), about: t('app.workspace.kind_about.SCRUM') }) }]
      : []),
  ]);

  const hubName = $derived(!parentId ? undefined : containers.find(parentId)?.name);
  const title = $derived(hubName ? t('app.workspace.new_collection_in', { hub: hubName }) : t('app.workspace.create_collection'));

  /** *Create* was pressed without a name: the field says so (design-system.md §4, case 3). */
  let isNameMissing = $state(false);
  let body = $state<HTMLElement | null>(null);
  const uid = $props.id();
  const nameErrorId = `${uid}-name-error`;
  const kindsId = `${uid}-kind`;
  const nameError = $derived(isNameMissing ? t('app.workspace.name_required') : isNameFailure ? failure?.message : undefined);

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
      // pressing Create twice is one intent, not two collections.
      const created = await containers.create(
        {
          type: 'COLLECTION',
          parent_id: parentId,
          name: name.trim(),
          description: description.trim() === '' ? null : description.trim(),
          ...lookBody(symbol, colour),
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
  size="m"
  dismissLabel={t('app.workspace.cancel')}
  primary={{
    label: t('app.workspace.create'),
    isBusy: isSaving,
    busyLabel: t('app.workspace.creating'),
    onclick: create,
  }}
>
  <div bind:this={body}>
  <Stack gap="150">
    <!-- The name's label inside its box, as the concept's *New collection* draws it. -->
    <div class="name-field">
      <label class="name" data-invalid={nameError ? '' : undefined}>
        <span class="name-label">{t('app.workspace.name_inline')}</span>
        <input type="text" bind:value={name} oninput={() => (isNameMissing = false)}
          aria-invalid={nameError ? 'true' : undefined} aria-describedby={nameError ? nameErrorId : undefined} />
      </label>
      {#if nameError}<p class="name-error" id={nameErrorId}>{nameError}</p>{/if}
    </div>
    {#if kinds.length > 0}
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
        <div data-look-fields><LookFields bind:symbol bind:colour /></div>
      </Stack>
    </Disclosure>
    <!-- Everything that is not about the name is a sentence above the buttons rather than a field
         error, because there is no field for it to land on. -->
    {#if failure && !isNameFailure}
      <p class="failure" role="alert">{failure.message}</p>
    {/if}
  </Stack>
  </div>

</Dialog>

<style>
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
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
</style>
