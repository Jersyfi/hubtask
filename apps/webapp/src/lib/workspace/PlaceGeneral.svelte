<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A place's *General* section (page-system §9.6, PS-10, PS-11): its name, description and look;
  // a hub's default preset and whether its pages wait in drafts; a collection's key prefix and
  // mail handle (user-experience.md §5.6, ADR-0094). One row per setting and one save for all of
  // them - the prefix and the handle are no longer buttons of their own.

  import { untrack } from 'svelte';

  import { IconButton, Input, Radio, SaveBar, SettingRow, SettingsSection, Switch, Textarea } from '@hubtask/design-system/components';
  import type { LabelToken } from '@hubtask/design-system';
  import type { Container, PageSpaceDrafts, TransportError } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { copyHandle, readMailSettings, routesTo, type MailSettings } from '../data/mailintake.svelte.ts';
  import { pages, spaceDraftsPath } from '../data/pages.svelte.ts';
  import { isBuiltIn, presetChoice, presetName, presetOrigin, presets, presetValue } from '../data/presets.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { ticketKeyOf } from '../data/ticket.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { leaveGuard } from '../frame/leave.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import LookFields from './LookFields.svelte';
  import { chosenColour, chosenSymbol, lookBody, type PlaceSymbol } from './look.ts';

  const { container }: { container: Container } = $props();

  const isHub = $derived(container.type === 'HUB');

  interface Values {
    name: string;
    description: string;
    symbol?: PlaceSymbol;
    colour?: LabelToken;
    preset: string;
    prefix: string;
    handle: string;
    drafts?: boolean;
  }

  // svelte-ignore state_referenced_locally
  const space = container.type === 'HUB' ? resource<PageSpaceDrafts>(spaceDraftsPath(container.id)) : undefined;
  const draftsHeld = $derived(space?.state.status === 'ready' ? space.state.data.drafts === true : undefined);

  // What a save wrote stands while the place read back still holds what it held before: a place
  // or a structure read back need not echo every value (a preset is not named in the structure
  // it was set from). Once a value moves, the place's own stands.
  let written = $state<{ values: Partial<Values>; before: Values }>();
  const held = $derived<Values>({
    name: container.name,
    description: container.description ?? '',
    symbol: chosenSymbol(container),
    colour: chosenColour(container),
    preset: container.structure?.preset ?? container.structure?.preset_id ?? '',
    prefix: container.key_prefix ?? '',
    handle: container.handle ?? '',
    drafts: draftsHeld,
  });
  const saved = $derived.by<Values>(() => {
    if (!written) return held;
    const { values: wrote, before } = written;
    const kept = (Object.keys(wrote) as (keyof Values)[]).filter((key) => held[key] === before[key]);
    return { ...held, ...Object.fromEntries(kept.map((key) => [key, wrote[key]])) };
  });

  let values = $state<Values>({ name: '', description: '', preset: '', prefix: '', handle: '' });
  let isSaving = $state(false);
  let isNameMissing = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  /** Starts again from what the place holds; what was typed and not saved is gone. */
  function begin() {
    values = { ...saved };
    failure = undefined;
    isNameMissing = false;
  }
  $effect(() => untrack(begin));
  // The drafts' state arrives after the place: it joins the values once it answers.
  $effect(() => {
    const held = draftsHeld;
    untrack(() => {
      if (values.drafts === undefined) values.drafts = held;
    });
  });

  const wantedPrefix = $derived(values.prefix.trim().toUpperCase());
  const wantedHandle = $derived(values.handle.trim().toLowerCase());
  const changed = $derived({
    name: values.name !== saved.name,
    description: values.description !== saved.description,
    look: values.symbol !== saved.symbol || values.colour !== saved.colour,
    preset: values.preset !== saved.preset,
    prefix: !isHub && wantedPrefix !== saved.prefix,
    handle: !isHub && wantedHandle !== saved.handle,
    drafts: values.drafts !== undefined && values.drafts !== saved.drafts,
  });
  const changeCount = $derived(Object.values(changed).filter(Boolean).length);
  $effect(() => leaveGuard.hold(() => changeCount));

  $effect(() => {
    if (!isHub || !container.structure) return;
    return untrack(() => presets.open());
  });
  /** A structure is changed by choosing a preset, never by unchoosing one (UC-WRK-30). */
  const presetOptions = $derived(
    !isHub || !container.structure || presets.all.length === 0
      ? []
      : [
          ...(saved.preset === '' ? [{ value: '', label: t('app.structure.preset_workspace_default'), hint: t('app.structure.preset_workspace_default_about') }] : []),
          ...presets.all.map((each) => ({
            value: presetValue(each),
            label: t('app.structure.preset_option', { name: presetName(each), origin: presetOrigin(each) }),
            hint: isBuiltIn(each.key) ? t(`app.structure.preset_about.${each.key}`) : undefined,
          })),
        ],
  );

  let mail = $state<MailSettings | undefined>(undefined);
  $effect(() => {
    if (isHub) return;
    void readMailSettings().then((read) => (mail = read), () => undefined);
  });
  const ways = $derived(!isHub && saved.handle ? routesTo(saved.handle, mail) : undefined);
  const example = $derived(isHub ? undefined : ticketKeyOf({ handle: wantedHandle || container.handle, key_prefix: wantedPrefix || null }, 42));

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      announcer.say(t('app.mail.copied_announced'));
    } catch {
      announcer.say(t('app.mail.copy_failed', { address: text }));
    }
  }

  async function save() {
    if (isSaving || changeCount === 0) return;
    if (values.name.trim() === '') {
      isNameMissing = true;
      return;
    }
    isSaving = true;
    failure = undefined;
    const before = { ...held };
    try {
      let version = container.version;
      if (changed.handle) {
        // The handle first: a prefix left empty reads the keys with it.
        await containers.setHandle(container.id, container.parent_id, wantedHandle);
        version = containers.find(container.id)?.version ?? version;
      }
      if (changed.name || changed.description || changed.look || changed.prefix) {
        // A patch carries only what changed: an untouched value is never written back over a newer one.
        const body = {
          ...(changed.name ? { name: values.name.trim() } : {}),
          ...(changed.description ? { description: values.description.trim() === '' ? null : values.description.trim() } : {}),
          ...(changed.look ? lookBody(values.symbol, values.colour) : {}),
          ...(changed.prefix ? { key_prefix: wantedPrefix || null } : {}),
        };
        version = (await containers.update(container.id, body, version)).version;
      }
      if (changed.preset && values.preset !== '') {
        await containers.setStructure(container.id, presetChoice(values.preset), version);
      }
      if (changed.drafts && values.drafts !== undefined) await pages.setDrafts(container.id, values.drafts);
      const { drafts, ...rest } = values;
      written = { values: { ...rest, ...(drafts === undefined ? {} : { drafts }), name: values.name.trim(), description: values.description.trim(), prefix: wantedPrefix, handle: wantedHandle }, before };
      announcer.say(t('app.workspace.edited_announced', { name: values.name.trim() }));
    } catch (error) {
      failure = renderProblem(error as TransportError, messages);
    } finally {
      isSaving = false;
    }
  }

  // A save answers with the place: the values start again from it once nothing is unsaved.
  $effect(() => {
    void saved;
    if (!isSaving) untrack(() => {
      if (changeCount > 0) return;
      values = { ...saved };
    });
  });
</script>

{#snippet saveBar()}
  <SaveBar
    label={t('app.save_bar.label')}
    changesLabel={changeCount > 0 ? t('app.save_bar.changes', { count: String(changeCount) }) : undefined}
    discardLabel={t('app.save_bar.discard')}
    saveLabel={t('app.workspace.save')}
    {isSaving}
    savingLabel={t('app.workspace.saving')}
    onDiscard={begin}
    onSave={() => void save()}
  />
{/snippet}

<form data-place-general onsubmit={(event) => { event.preventDefault(); void save(); }}>
  <SettingsSection title={t('app.settings.section_general')} save={saveBar}>
    {#if failure}<FailureNote problem={failure} />{/if}
    <SettingRow label={t(isHub ? 'app.workspace.hub_name' : 'app.workspace.collection_name')} fieldWidth="md" isRequired isChanged={changed.name}>
      {#snippet control(nameId)}
        <Input
          label={t(isHub ? 'app.workspace.hub_name' : 'app.workspace.collection_name')}
          isLabelHidden
          aria-labelledby={nameId}
          bind:value={values.name}
          oninput={() => (isNameMissing = false)}
          error={isNameMissing ? t('app.workspace.name_required') : failure?.fields.get('/name')}
        />
      {/snippet}
    </SettingRow>
    <SettingRow label={t('app.workspace.description')} fieldWidth="lg" isChanged={changed.description}>
      {#snippet control(nameId)}
        <Textarea label={t('app.workspace.description')} isLabelHidden aria-labelledby={nameId} rows={2} bind:value={values.description} />
      {/snippet}
    </SettingRow>
    <SettingRow label={t('app.settings.look')} help={t('app.settings.look_hint')} fieldWidth="full" isChanged={changed.look}>
      {#snippet control()}
        <LookFields type={container.type} bind:symbol={values.symbol} bind:colour={values.colour} />
      {/snippet}
    </SettingRow>
    {#if presetOptions.length > 0}
      <SettingRow label={t('app.workspace.edit_preset')} help={t('app.workspace.edit_preset_hint')} fieldWidth="md" isChanged={changed.preset}>
        {#snippet control()}
          <Radio label={t('app.workspace.edit_preset')} options={presetOptions} bind:value={values.preset} />
        {/snippet}
      </SettingRow>
    {/if}
    {#if isHub && values.drafts !== undefined}
      <div data-page-drafts>
        <SettingRow label={t('app.pages.drafts_switch')} help={t('app.pages.drafts_switch_hint')} fieldWidth="xs" isChanged={changed.drafts}>
          {#snippet control(nameId)}
            <Switch label={t('app.pages.drafts_switch')} isLabelHidden aria-labelledby={nameId} bind:checked={values.drafts} />
          {/snippet}
        </SettingRow>
      </div>
    {/if}
    {#if !isHub}
      <div data-key-settings data-key-prefix>
        <SettingRow
          label={t('app.keys.prefix')}
          help={`${t('app.keys.prefix_hint')}${example ? ` ${t('app.keys.example', { key: example })}` : ''}`}
          groupLabel={t('app.keys.section')}
          fieldWidth="xs"
          isChanged={changed.prefix}
        >
          {#snippet control(nameId)}
            <Input label={t('app.keys.prefix')} isLabelHidden aria-labelledby={nameId} bind:value={values.prefix} maxlength={5} />
          {/snippet}
        </SettingRow>
      </div>
      <div data-mail-settings>
        <SettingRow label={t('app.mail.handle')} help={t('app.mail.handle_hint')} groupLabel={t('app.mail.section')} fieldWidth="sm" isChanged={changed.handle}>
          {#snippet control(nameId)}
            <Input label={t('app.mail.handle')} isLabelHidden aria-labelledby={nameId} bind:value={values.handle} />
          {/snippet}
        </SettingRow>
        {#if ways}
          <SettingRow label={t('app.mail.subject_way')} help={t('app.mail.key_hint', { key: ticketKeyOf(container, 42) ?? `${saved.handle.toUpperCase()}-42` })} fieldWidth="md">
            {#snippet control()}
              <span class="way" data-mail-subject>
                <code>{ways.subject}</code>
                <IconButton icon="copy" size="sm" label={t('app.mail.copy_named', { name: ways.subject })} onclick={() => void copyHandle(saved.handle, (code, params) => t(code, params)).then((said) => said && announcer.say(said))} />
              </span>
            {/snippet}
          </SettingRow>
          {#if ways.address}
            <SettingRow label={t('app.mail.address_way')} help={t('app.mail.section_hint')} fieldWidth="md">
              {#snippet control()}
                <span class="way" data-mail-address>
                  <code>{ways.address}</code>
                  <IconButton icon="copy" size="sm" label={t('app.mail.copy_named', { name: ways.address ?? '' })} onclick={() => void copy(ways.address ?? '')} />
                </span>
              {/snippet}
            </SettingRow>
          {/if}
        {/if}
      </div>
    {/if}
  </SettingsSection>
</form>

<style>
  .way { display: inline-flex; align-items: center; gap: var(--sp-100); flex-wrap: wrap; min-inline-size: 0; }
  code { font-family: var(--font-mono); font-size: var(--fs-075); overflow-wrap: anywhere; }
</style>
