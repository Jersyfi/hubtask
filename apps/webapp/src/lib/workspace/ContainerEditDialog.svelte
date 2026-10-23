<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Edit* for a hub or a collection (ux-inventory.md §E4): its name, description, colour and
  // symbol (UC-WRK-33), and for a hub the preset its collections follow (UC-WRK-23 check 1) - the
  // fields the create dialog offers, in one place instead of a rename and a separate look.
  //
  // A dialog with a save rather than a write per field, for the reason the create dialog gives: a
  // taken name arrives as a 409 with `containers.name_taken` and lands on the name field. The
  // version is the one the dialog opened with, so a change that lost a race is refused rather than
  // winning by being second (ADR-0025). The preset is its own write (`PATCH …/structure`), sent
  // only when it changed, after the rest and on the version that answered; the server keeps what
  // the workspace locked.
  //
  // A collection's preset is not here: it is its *Structure* panel's first setting, beside what it
  // replaces.

  import { untrack } from 'svelte';

  import { Button, Dialog, Input, Radio, Stack, Textarea } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { presetChoice, presetName, presetOrigin, presets, presetValue } from '../data/presets.svelte.ts';
  import { focusFirst } from '../forms/missing.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import LookFields from './LookFields.svelte';
  import { chosenColour, chosenSymbol, lookBody, type PlaceSymbol } from './look.ts';

  import type { LabelToken } from '@hubtask/design-system';
  import type { Container, TransportError } from '@hubtask/sync-engine';

  interface Props {
    isOpen: boolean;
    container: Container;
  }

  let { isOpen = $bindable(), container }: Props = $props();

  let name = $state('');
  let description = $state('');
  let symbol = $state<PlaceSymbol | undefined>(undefined);
  let colour = $state<LabelToken | undefined>(undefined);
  /** The hub's preset as it opened, and as chosen: empty where the hub follows the workspace's default. */
  let presetBefore = $state('');
  let preset = $state('');
  let version = $state(0);
  let isSaving = $state(false);
  let isNameMissing = $state(false);
  let isNameFailure = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let body = $state<HTMLElement | null>(null);

  const isHub = $derived(container.type === 'HUB');

  // Opening starts from what the place has now; what was tried and not saved is gone.
  $effect(() => {
    if (!isOpen) return;
    untrack(() => {
      name = container.name;
      description = container.description ?? '';
      symbol = chosenSymbol(container);
      colour = chosenColour(container);
      presetBefore = container.structure?.preset ?? container.structure?.preset_id ?? '';
      preset = presetBefore;
      version = container.version;
      failure = undefined;
      isNameMissing = false;
      isNameFailure = false;
    });
  });

  $effect(() => {
    if (!isOpen || !isHub || !container.structure) return;
    return untrack(() => presets.open());
  });

  /**
   * The presets a hub may start its collections from. "The workspace's default" only while the hub
   * has none of its own: a structure is changed by choosing a preset, never by unchoosing one.
   */
  const presetOptions = $derived(
    !isHub || !container.structure || presets.all.length === 0
      ? []
      : [
          ...(presetBefore === ''
            ? [{ value: '', label: t('app.structure.preset_workspace_default'), hint: t('app.structure.preset_workspace_default_about') }]
            : []),
          // Saved presets too, each marked with where it comes from, as at creation (UC-WRK-30
          // check 2): a saved one has no key, and keying the list by one drew them all as one.
          ...presets.all.map((each) => ({
            value: presetValue(each),
            label: presetName(each),
            hint: each.key
              ? t('app.structure.preset_marked', { origin: presetOrigin(each), about: t(`app.structure.preset_about.${each.key}`) })
              : presetOrigin(each),
          })),
        ],
  );

  async function save() {
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
      const saved = await containers.update(
        container.id,
        { name: name.trim(), description: description.trim() === '' ? null : description.trim(), ...lookBody(symbol, colour) },
        version,
      );
      version = saved.version;
      if (preset !== '' && preset !== presetBefore) {
        await containers.setStructure(container.id, presetChoice(preset), saved.version);
      }
      announcer.say(t('app.workspace.edited_announced', { name: name.trim() }));
      isOpen = false;
    } catch (error) {
      const problem = error as TransportError;
      failure = renderProblem(problem, messages);
      isNameFailure = problem.detailCode === 'containers.name_taken' || failure.fields.has('/name');
    } finally {
      isSaving = false;
    }
  }
</script>

<Dialog bind:isOpen title={t(isHub ? 'app.workspace.edit_hub' : 'app.workspace.edit_collection')} dismissLabel={t('app.workspace.cancel')}>
  <div bind:this={body}>
    <Stack gap="150">
      <Input
        label={t(isHub ? 'app.workspace.hub_name' : 'app.workspace.collection_name')}
        bind:value={name}
        oninput={() => (isNameMissing = false)}
        error={isNameMissing ? t('app.workspace.name_required') : isNameFailure ? failure?.message : undefined}
      />
      <Textarea label={t('app.workspace.description')} bind:value={description} />
      <LookFields type={container.type} bind:symbol bind:colour />
      {#if presetOptions.length > 0}
        <Radio label={t('app.workspace.edit_preset')} hint={t('app.workspace.edit_preset_hint')} options={presetOptions} bind:value={preset} />
      {/if}
      {#if failure && !isNameFailure}<p class="failure" role="alert">{failure.message}</p>{/if}
    </Stack>
  </div>

  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.workspace.cancel')}</Button>
    <Button isBusy={isSaving} busyLabel={t('app.workspace.saving')} onclick={save}>{t('app.workspace.save')}</Button>
  {/snippet}
</Dialog>

<style>
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
</style>
