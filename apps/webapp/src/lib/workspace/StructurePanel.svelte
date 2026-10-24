<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's or a collection's settings page (UC-WRK-23 checks 1-3, 7; design-system.md §11.8):
  // the sections as a list at the start, the open one beside it - the shape of the administration,
  // so setting something up looks the same everywhere. Its *Structure* section is a preset to start
  // from and the eight settings, each with where its value comes from, its lock and a reset.
  //
  // Drawn only for a reader who may change the shape (check 9, P-05): the caller leaves the
  // way in out for anybody else. One save per sitting, through the save bar - every setting is
  // a field of one form (design-system.md §4, case 4) - and one `PATCH …/structure` that says
  // which preset, which settings and which resets. A lock is a `SourceChip` with the lock beside
  // the setting's name and the control still there, readable and unavailable with its reason
  // (cases 2 and 6); a value set above names where it comes from and whom to ask.
  //
  // The sections are a list, so the panel's further parts - switching a level with its preview -
  // join it without a second panel. *Save as preset* (UC-WRK-30 check 1) sits beside the preset
  // picker and saves what is in force, not the draft.

  import { untrack, type Snippet } from 'svelte';

  import { Button, CheckboxGroup, Input, Radio, SaveBar, Select, SourceChip, Stack } from '@hubtask/design-system/components';
  import type { Container, StructureChange, StructurePreview, StructureSettings } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { customFields } from '../data/customfields.svelte.ts';
  import { countsChange, figureOffer, isComplete, type FigureKind } from '../data/figureOffer.ts';
  import { levelWords } from '../data/levels.ts';
  import { presetName, presetOrigin, presets, presetValue } from '../data/presets.svelte.ts';
  import CategoriesDialog from '../entries/CategoriesDialog.svelte';
  import NarrowingDialog from './NarrowingDialog.svelte';
  import SavePresetDialog from './SavePresetDialog.svelte';
  import { DRAWABLE } from '../entries/layouts.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import {
    SETTINGS,
    changeOf,
    choosePreset,
    countOf,
    isOwn,
    levelRefusal,
    lockOf,
    renameLevel,
    resetSetting,
    chooseFieldsOf,
    setValue,
    startDraft,
    toggleLevel,
    typesTopDown,
    type Draft,
    type SettingName,
  } from './structureDraft.ts';

  /** A section beside the structure: what it is called, and what it draws while it is the open one. */
  export interface StructureTab {
    readonly id: string;
    readonly label: string;
    readonly body: Snippet;
  }

  interface Props {
    isOpen: boolean;
    container: Container;
    /** Whether the reader may change the workspace's shape: *Save as preset* then offers the workspace. */
    mayShapeWorkspace?: boolean;
    /** The sections before the structure (its general fields) and after it, in the list's order. */
    before?: readonly StructureTab[];
    tabs?: readonly StructureTab[];
    /** The open section's id; `structure` is the eight settings. */
    tab?: string;
    /** Where each section lives, and going there: every section has its own address. */
    hrefOf: (section: string) => string;
    onsection: (section: string) => void;
  }

  const { isOpen, container, mayShapeWorkspace = false, before = [], tabs = [], tab = 'structure', hrefOf, onsection }: Props = $props();
  const sections = $derived([...before, { id: 'structure', label: t('app.settings.section_structure') }, ...tabs]);
  let isSavingPreset = $state(false);

  const structure = $derived(container.structure);
  const isHub = $derived(container.type === 'HUB');

  // The presets, the accounts that set a lock, and the number fields a sum may name.
  // From `untrack`: subscribing delivers at once and writes the store this effect would read.
  $effect(() => {
    if (!isOpen) return;
    const id = container.id;
    const hub = isHub;
    return untrack(() => {
      const stops = [presets.open(), customFields.open(), ...(hub ? [] : [customFields.open(id)])];
      return () => {
        for (const stop of stops) stop();
      };
    });
  });
  $effect(() => {
    if (!isOpen || !structure) return;
    accounts.resolve(Object.values(structure.locks ?? {}).map((lock) => lock.locked_by));
  });

  let draft = $state<Draft | undefined>(undefined);
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /** A sum without its field, or no view offered: marked when the reader saves (case 3). */
  let isFieldMissing = $state(false);
  let isViewMissing = $state(false);

  // Opening starts from what is in force; a save starts again from what the server answered.
  $effect(() => {
    if (!isOpen || !structure) {
      draft = undefined;
      return;
    }
    if (draft !== undefined) return;
    untrack(() => {
      draft = startDraft(structure);
      failure = undefined;
      isFieldMissing = false;
      isViewMissing = false;
    });
  });

  const chosen = $derived(draft?.preset ? presets.find(draft.preset) : undefined);
  const change = $derived(draft && structure ? changeOf(draft, structure, chosen) : undefined);
  const changeCount = $derived(countOf(change));
  const values = $derived<StructureSettings>(draft?.values ?? structure?.settings ?? {});

  // ---------------------------------------------------------------------------------------------
  // Where a value comes from
  // ---------------------------------------------------------------------------------------------

  /** The source in a sentence's words: "the hub", "this collection". */
  function sourceWords(source: string): string {
    if (source === 'HUB' && isHub) return t('app.structure.source_this_hub');
    return t(`app.figure.source.${source.toLowerCase()}`);
  }

  /** Who locked it, by name where a person did. */
  function lockerOf(key: string): string | undefined {
    const lock = structure ? lockOf(structure, key) : undefined;
    if (!lock) return undefined;
    return accounts.nameOf(lock.locked_by) ?? t('app.structure.locked_origin', { origin: lock.origin });
  }

  /** The reason a locked control gives: the lock and who set it. */
  function lockReason(key: string): string | undefined {
    const locker = lockerOf(key);
    return locker === undefined ? undefined : t('app.structure.locked_by', { name: locker });
  }

  function chipOf(setting: SettingName) {
    const source = (structure?.sources?.[setting] as string | undefined) ?? 'DEFAULT';
    const locker = lockerOf(setting);
    const label = t(`app.structure.source.${source === 'HUB' && isHub ? 'here' : source === 'COLLECTION' ? 'here' : source.toLowerCase()}`);
    return {
      label: locker === undefined ? label : t('app.structure.source_locked', { source: label }),
      isLocked: locker !== undefined,
      value: summaryOf(setting),
      source: t('app.figure.source_hint', { source: sourceWords(source) }),
      whomToAsk:
        locker !== undefined
          ? t('app.structure.ask_locker', { name: locker })
          : source === 'HUB' && !isHub
            ? t('app.structure.ask_hub')
            : source === 'WORKSPACE' || source === 'INSTANCE' || source === 'PLAN'
              ? t('app.structure.ask_admin')
              : undefined,
    };
  }

  const typeName = (type: string): string => t('app.item.type_name', { type });
  const levelName = (type: string): string => levelWords(values, type, t).label;

  /** The value in force, as a person reads it - what the chip's details say. */
  function summaryOf(setting: SettingName): string {
    const settings = structure?.settings ?? {};
    switch (setting) {
      case 'levels':
        return (settings.levels ?? []).map((level) => levelWords(settings, level.type as string, t).label).join(' › ');
      case 'fields':
        return t('app.structure.fields_summary', { count: String(new Set(Object.values(settings.fields ?? {}).flatMap((level) => level.capabilities)).size) });
      case 'views':
        return (settings.views?.offered ?? []).map((layout) => t(`app.view.${layout}`)).join(', ');
      case 'counts':
        return t(`app.figure.option.${(settings.counts?.figure ?? 'NONE').toLowerCase()}`);
      case 'completion':
        return t(settings.completion === 'ROLLUP' ? 'app.policies.completion_rollup' : 'app.policies.completion_manual');
      case 'capture':
        return (settings.capture ?? []).map((token) => t(`app.structure.capture.${token}`)).join(', ') || t('app.structure.nothing');
      case 'celebration':
        return t(`app.structure.celebration.${settings.celebration ?? 'ALL'}`);
      case 'automation':
        return (settings.automation ?? []).map((behaviour) => t(`app.structure.automation.${behaviour}`)).join(', ') || t('app.structure.nothing');
    }
  }

  // ---------------------------------------------------------------------------------------------
  // The settings
  // ---------------------------------------------------------------------------------------------

  const types = $derived(typesTopDown(manifest.value));
  const offeredTypes = $derived((values.levels ?? []).map((level) => level.type as string));

  const levelOptions = $derived(
    types.map((type) => {
      const refusal = levelRefusal(manifest.value, offeredTypes, type);
      return {
        value: type,
        label: typeName(type),
        disabledReason:
          lockReason('levels') ??
          (refusal === 'top' ? t('app.structure.level_top') : refusal === 'parent' ? t('app.structure.level_needs_parent') : undefined),
      };
    }),
  );

  function chooseLevels(selected: readonly string[]) {
    if (!draft) return;
    const toggled = types.find((type) => selected.includes(type) !== offeredTypes.includes(type));
    if (!toggled) return;
    draft = setValue(draft, 'levels', toggleLevel(manifest.value, values.levels, toggled));
  }

  function nameLevel(type: string, name: string) {
    if (!draft) return;
    draft = setValue(draft, 'levels', renameLevel(values.levels, type, name));
  }

  /** What a level carries here: its own list, or everything the installation gives it. */
  function carried(type: string): readonly string[] {
    const own = values.fields?.[type]?.capabilities;
    const offered = manifest.value?.item_types?.find((entry) => entry.type === type)?.capabilities ?? [];
    return own ?? offered;
  }

  function fieldOptions(type: string) {
    const offered = manifest.value?.item_types?.find((entry) => entry.type === type)?.capabilities ?? [];
    return offered.map((capability) => ({
      value: capability,
      label: t(`app.structure.field.${capability}`),
      disabledReason:
        capability === 'COMPLETION'
          ? t('app.structure.field_always')
          : (lockReason('fields') ?? lockReason(`fields.${type}.${capability}`)),
    }));
  }

  function chooseFields(type: string, selected: readonly string[]) {
    if (!draft) return;
    const offered = manifest.value?.item_types?.find((entry) => entry.type === type)?.capabilities ?? [];
    draft = setValue(draft, 'fields', chooseFieldsOf(values.fields, type, offered, selected));
  }

  /**
   * The level whose categories are being chosen, in UX-12's own dialog (UC-WRK-25 check 5). It
   * writes at once against the collection's version, so it waits for a change in the panel to be
   * saved or discarded, and the panel starts again from what it wrote.
   */
  let categoriesOf = $state<string | undefined>(undefined);
  const categoriesWait = $derived(
    lockReason('fields') ?? (changeCount > 0 ? t('app.structure.categories_save_first') : undefined),
  );

  function closeCategories() {
    categoriesOf = undefined;
    draft = undefined;
  }

  const viewOptions = $derived(
    ((manifest.value?.view_layouts ?? []) as readonly string[])
      .filter((layout) => DRAWABLE.includes(layout))
      .map((layout) => ({ value: layout, label: t(`app.view.${layout}`), disabledReason: lockReason('views') })),
  );

  function chooseViews(selected: readonly string[]) {
    if (!draft) return;
    isViewMissing = false;
    const offered = viewOptions.map((option) => option.value).filter((layout) => selected.includes(layout));
    const current = values.views?.default as string | undefined;
    const opening = current && offered.includes(current) ? current : (offered[0] ?? current ?? 'LIST_COLLAPSED');
    draft = setValue(draft, 'views', { offered, default: opening } as StructureSettings['views']);
  }

  function chooseOpening(layout: string) {
    if (!draft || !values.views) return;
    draft = setValue(draft, 'views', { ...values.views, default: layout } as StructureSettings['views']);
  }

  /** What counts, offered from the levels and fields as the draft has them. */
  const figures = $derived(
    structure
      ? figureOffer({ ...structure, settings: values }, [...(isHub ? [] : customFields.of(container.id)), ...customFields.of()])
      : undefined,
  );
  const figure = $derived(((values.counts?.figure as FigureKind | undefined) ?? 'NONE'));

  function chooseFigure(kind: string) {
    if (!draft) return;
    isFieldMissing = false;
    draft = setValue(draft, 'counts', countsChange(kind as FigureKind, values.counts?.field_key ?? undefined));
  }

  function chooseSumField(key: string) {
    if (!draft) return;
    isFieldMissing = false;
    draft = setValue(draft, 'counts', { figure: 'FIELD_SUM', field_key: key });
  }

  const CAPTURE = ['LINES', 'AMOUNT', 'DATE', 'PERSON', 'LABEL'] as const;
  /** What a collection may apply by itself; the server keeps it (UC-WRK-31 check 2). */
  const AUTOMATION = ['REQUIRED_BEYOND_FIRST_COLUMN'] as const;

  // ---------------------------------------------------------------------------------------------
  // The preset, the resets, the save
  // ---------------------------------------------------------------------------------------------

  // Each marked with where it comes from (UC-WRK-30 check 2).
  const presetOptions = $derived(
    presets.all.map((preset) => ({ value: presetValue(preset), label: t('app.structure.preset_option', { name: presetName(preset), origin: presetOrigin(preset) }) })),
  );
  /** The name of the preset the layer was copied from, where it is still offered. */
  const startedFrom = $derived(presets.nameOf(structure?.preset ?? structure?.preset_id));
  /** The picker shows the preset in force until another is chosen, where it is still offered. */
  const presetInForce = $derived.by(() => {
    const value = structure?.preset ?? structure?.preset_id;
    return value && presets.find(value) ? value : '';
  });

  function choosePresetKey(key: string) {
    const preset = presets.find(key);
    if (!draft || !structure || !preset) return;
    isFieldMissing = false;
    isViewMissing = false;
    draft = choosePreset(draft, structure, preset);
  }

  function reset(setting: SettingName) {
    if (!draft || !structure) return;
    draft = resetSetting(draft, structure, setting);
  }

  function discard() {
    if (!structure) return;
    draft = startDraft(structure);
    failure = undefined;
    isFieldMissing = false;
    isViewMissing = false;
  }

  /** A change that would stop showing something, waiting for the person's choice (UC-WRK-24 check 2). */
  let narrowing = $state<{ change: StructureChange; preview: StructurePreview } | undefined>();

  /**
   * Saves a change that offers as much or more at once (check 1); one that would stop showing
   * something is previewed first and waits for *Hide* or *Flatten*.
   */
  async function save() {
    if (isSaving || !change) return;
    isFieldMissing = !isComplete(figure, values.counts?.field_key ?? undefined);
    isViewMissing = (values.views?.offered ?? []).length === 0;
    if (isFieldMissing || isViewMissing) return;
    isSaving = true;
    failure = undefined;
    try {
      const preview = await containers.previewStructure(container.id, change);
      if (preview.narrows) {
        narrowing = { change, preview };
        return;
      }
      await write(change);
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isSaving = false;
    }
  }

  async function confirmNarrowing(choice: 'HIDE' | 'FLATTEN') {
    if (!narrowing || isSaving) return;
    isSaving = true;
    failure = undefined;
    try {
      await write({ ...narrowing.change, narrowing: choice });
      narrowing = undefined;
    } catch (error) {
      narrowing = undefined;
      failure = renderProblem(error as never, messages);
    } finally {
      isSaving = false;
    }
  }

  async function write(change: StructureChange) {
    const answered = await containers.setStructure(container.id, change, container.version);
    if (answered.structure) draft = startDraft(answered.structure);
    const kept = answered.kept_by_lock ?? [];
    announcer.say(t('app.structure.saved_announced'));
    if (kept.length > 0) toast.show({ text: t('app.structure.kept_by_lock', { count: String(kept.length) }) });
  }

  const titleOf = (setting: SettingName): string => t(`app.structure.setting.${setting}`);
</script>

<!-- The eight settings: the panel's first tab, or the whole panel where it has no other. -->
{#snippet settingsBody()}
  {#if structure && draft}
    <form class="panel" data-structure-panel onsubmit={(event) => { event.preventDefault(); void save(); }}>
      <Stack gap="300">
        <p class="hint">{t(isHub ? 'app.structure.intro_hub' : 'app.structure.intro')}</p>

        <section class="setting" data-setting="preset">
          <Select
            label={t('app.structure.preset_label')}
            hint={draft.preset
              ? t('app.structure.preset_chosen', { preset: presets.nameOf(draft.preset) ?? '' })
              : startedFrom
                ? t('app.structure.preset_from', { preset: startedFrom })
                : t('app.structure.preset_hint')}
            placeholder={t('app.structure.preset_choose')}
            options={presetOptions}
            bind:value={() => draft?.preset ?? presetInForce, (key) => choosePresetKey(key)}
          />
          <!-- What is saved is the structure the server holds, so it waits for the save bar. -->
          <div class="save-as">
          <Button
            size="sm"
            tone="subtle"
            disabledReason={changeCount > 0 ? t('app.presets.save_first') : undefined}
            onclick={() => (isSavingPreset = true)}
          >
            {t('app.presets.save_as')}
          </Button>
          </div>
        </section>

        {#each SETTINGS as setting (setting)}
          {@const chip = chipOf(setting)}
          <section class="setting" data-setting={setting} aria-labelledby="structure-{setting}">
            <div class="head">
              <h3 id="structure-{setting}">{titleOf(setting)}</h3>
              <SourceChip
                label={chip.label}
                isLocked={chip.isLocked}
                detailsLabel={t('app.structure.where_from', { setting: titleOf(setting) })}
                value={chip.value}
                source={chip.source}
                whomToAsk={chip.whomToAsk}
              />
              {#if isOwn(structure, setting, container.type) && !draft.reset.includes(setting)}
                <Button size="sm" tone="subtle" aria-label={t('app.structure.reset_named', { setting: titleOf(setting) })} onclick={() => reset(setting)}>
                  {t('app.structure.reset')}
                </Button>
              {/if}
            </div>
            {#if draft.reset.includes(setting)}
              <p class="hint" data-reset-pending>{t('app.structure.reset_pending')}</p>
            {/if}

            {#if setting === 'levels'}
              <CheckboxGroup
                label={t('app.structure.levels_label')}
                hint={t('app.structure.levels_hint')}
                options={levelOptions}
                selected={offeredTypes}
                onChange={chooseLevels}
              />
              <div class="names">
                {#each offeredTypes as type (type)}
                  <Input
                    label={t('app.structure.level_name', { type: typeName(type) })}
                    placeholder={typeName(type)}
                    maxlength={60}
                    disabledReason={lockReason('levels')}
                    value={values.levels?.find((level) => level.type === type)?.name ?? ''}
                    oninput={(event: Event) => nameLevel(type, (event.currentTarget as HTMLInputElement).value)}
                  />
                {/each}
              </div>
            {:else if setting === 'fields'}
              {#each offeredTypes as type (type)}
                <CheckboxGroup
                  label={t('app.structure.fields_of', { level: levelName(type) })}
                  options={fieldOptions(type)}
                  selected={carried(type)}
                  onChange={(selected) => chooseFields(type, selected)}
                />
                {#if !isHub}
                  <Button
                    size="sm"
                    tone="secondary"
                    disabledReason={categoriesWait}
                    onclick={() => (categoriesOf = type)}
                  >{t('app.structure.categories_of', { level: levelName(type) })}</Button>
                {/if}
              {/each}
            {:else if setting === 'views'}
              <CheckboxGroup
                label={t('app.structure.views_offered')}
                options={viewOptions}
                selected={(values.views?.offered ?? []) as readonly string[]}
                error={isViewMissing ? t('app.structure.views_missing') : undefined}
                onChange={chooseViews}
              />
              <Select
                label={t('app.structure.views_default')}
                disabledReason={lockReason('views')}
                options={((values.views?.offered ?? []) as readonly string[]).map((layout) => ({ value: layout, label: t(`app.view.${layout}`) }))}
                bind:value={() => (values.views?.default as string | undefined) ?? '', (layout) => chooseOpening(layout)}
              />
            {:else if setting === 'counts' && figures}
              <Select
                label={t('app.figure.choice')}
                hint={t('app.figure.intro')}
                disabledReason={lockReason('counts')}
                options={figures.figures.map((kind) => ({ value: kind, label: t(`app.figure.option.${kind.toLowerCase()}`) }))}
                bind:value={() => (figures.figures.includes(figure) ? figure : 'NONE'), (kind) => chooseFigure(kind)}
              />
              {#if figure === 'FIELD_SUM'}
                <Select
                  label={t('app.figure.field')}
                  placeholder={t('app.figure.field_choose')}
                  disabledReason={lockReason('counts')}
                  error={isFieldMissing ? t('app.figure.field_choose') : undefined}
                  options={figures.fields.map((option) => ({ value: option.key, label: option.name }))}
                  bind:value={() => values.counts?.field_key ?? '', (key) => chooseSumField(key)}
                />
              {/if}
              {#if structure.counts_clipped?.from.figure === 'PROGRESS'}
                <p class="hint" data-figure-clipped>{t('app.figure.clipped', { source: sourceWords((structure.sources?.counts as string | undefined) ?? 'DEFAULT') })}</p>
              {/if}
            {:else if setting === 'completion'}
              <Radio
                label={t('app.policies.completion')}
                hint={t('app.policies.completion_hint')}
                disabledReason={lockReason('completion')}
                options={[
                  { value: 'MANUAL', label: t('app.policies.completion_manual') },
                  { value: 'ROLLUP', label: t('app.policies.completion_rollup') },
                ]}
                bind:value={() => (values.completion as string | undefined) ?? 'MANUAL', (choice) => draft && (draft = setValue(draft, 'completion', choice as StructureSettings['completion']))}
              />
            {:else if setting === 'capture'}
              <CheckboxGroup
                label={t('app.structure.capture_label')}
                disabledReason={lockReason('capture')}
                options={CAPTURE.map((token) => ({ value: token, label: t(`app.structure.capture.${token}`) }))}
                selected={(values.capture ?? []) as readonly string[]}
                onChange={(selected) => draft && (draft = setValue(draft, 'capture', CAPTURE.filter((token) => selected.includes(token))))}
              />
            {:else if setting === 'celebration'}
              <Radio
                label={t('app.structure.celebration_label')}
                disabledReason={lockReason('celebration')}
                options={[
                  { value: 'ALL', label: t('app.structure.celebration.ALL') },
                  { value: 'SUBTLE', label: t('app.structure.celebration.SUBTLE') },
                ]}
                bind:value={() => (values.celebration as string | undefined) ?? 'ALL', (choice) => draft && (draft = setValue(draft, 'celebration', choice as StructureSettings['celebration']))}
              />
            {:else if setting === 'automation'}
              <CheckboxGroup
                label={t('app.structure.automation_label')}
                disabledReason={lockReason('automation')}
                options={AUTOMATION.map((behaviour) => ({ value: behaviour, label: t(`app.structure.automation.${behaviour}`) }))}
                selected={(values.automation ?? []) as readonly string[]}
                onChange={(selected) => draft && (draft = setValue(draft, 'automation', AUTOMATION.filter((behaviour) => selected.includes(behaviour))))}
              />
            {/if}
          </section>
        {/each}

        {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}
      </Stack>

      <SaveBar
        label={t('app.save_bar.label')}
        changesLabel={changeCount > 0 ? t('app.save_bar.changes', { count: String(changeCount) }) : undefined}
        discardLabel={t('app.save_bar.discard')}
        saveLabel={t('app.workspace.save')}
        {isSaving}
        savingLabel={t('app.workspace.saving')}
        onDiscard={discard}
        onSave={() => void save()}
      />
    </form>
  {/if}
{/snippet}

<div class="page" data-structure-tabs>
  <nav class="sections" aria-label={t('app.structure.tabs_label')}>
    <ul>
      {#each sections as each (each.id)}
        <li>
          <a
            href={hrefOf(each.id)}
            aria-current={tab === each.id ? 'page' : undefined}
            onclick={(event) => {
              event.preventDefault();
              onsection(each.id);
            }}>{each.label}</a
          >
        </li>
      {/each}
    </ul>
  </nav>
  <div class="section">
    {#if tab === 'structure'}
      {@render settingsBody()}
    {:else}
      {@render [...before, ...tabs].find((each) => each.id === tab)?.body()}
    {/if}
  </div>
</div>

{#if narrowing}
  <NarrowingDialog
    preview={narrowing.preview}
    settings={structure?.settings}
    {isSaving}
    onconfirm={(choice) => void confirmNarrowing(choice)}
    oncancel={() => (narrowing = undefined)}
  />
{/if}

<SavePresetDialog bind:isOpen={isSavingPreset} containerId={container.id} {mayShapeWorkspace} />

{#if categoriesOf && structure}
  <CategoriesDialog
    collectionId={container.id}
    version={container.version}
    {structure}
    itemType={categoriesOf}
    onclose={closeCategories}
  />
{/if}

<style>
  .panel { display: flex; flex-direction: column; gap: var(--sp-300); }
  .setting { display: flex; flex-direction: column; gap: var(--sp-150); }
  .head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .head h3 { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .names { display: grid; gap: var(--sp-150); }
  .save-as { align-self: flex-start; }
  .hint { margin: 0; color: var(--text-secondary); max-width: 64ch; }
  .failure { margin: 0; color: var(--text-danger); }

  /* The sections at the start and the open one beside them, as the administration draws its
     areas; on a phone the list wraps above the section, every one in sight. */
  .page { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-300); }
  .sections ul { display: flex; flex-wrap: wrap; gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  /* The rows of the navigation column, so a list of places reads as one everywhere (§13 G2). */
  .sections a {
    display: flex;
    align-items: center;
    padding-block: var(--density-row-block);
    padding-inline: var(--sp-150);
    border-radius: var(--r-md);
    color: var(--text-secondary);
    font-size: var(--fs-100);
    text-decoration: none;
  }
  .sections a:hover { background: var(--bg-surface-hover); color: var(--text-primary); }
  .sections a:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: calc(var(--sp-025) * -1); }
  /* Rule 3: the open section is announced by `aria-current` and marked, not by colour alone. */
  .sections a[aria-current='page'] {
    background: var(--bg-surface-pressed);
    color: var(--text-primary);
    font-weight: var(--fw-medium);
    box-shadow: inset var(--bw-thick) 0 0 0 var(--accent-primary);
  }
  :global([dir='rtl']) .sections a[aria-current='page'] { box-shadow: inset calc(var(--bw-thick) * -1) 0 0 0 var(--accent-primary); }
  .section { min-width: 0; }

  /* From `medium` the list stands at the start, as the administration's areas do. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width >= 600px) {
    .page { grid-template-columns: minmax(0, max-content) minmax(0, 1fr); }
    .sections ul { flex-direction: column; flex-wrap: nowrap; }
  }
</style>
