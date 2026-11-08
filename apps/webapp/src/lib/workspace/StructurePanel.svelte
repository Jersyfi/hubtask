<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's or a collection's settings page (UC-WRK-23 checks 1-3, 7; page-system §9.6): a
  // `SettingsLayout` whose section list says each section's state, one section at its address -
  // the shape of the administration, so setting something up looks the same everywhere. Its *Structure* section is a preset to start
  // from and the nine settings, each with where its value comes from, its lock and a reset; the
  // entry types only where the workflows module is on (UC-FLW-01 check 8).
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

  import { Button, Checkbox, DataTable, Input, SaveBar, Select, SettingRow, SettingsLayout, SettingsSection, SourceChip } from '@hubtask/design-system/components';
  import type { DataColumn, PageHead, SectionLink } from '@hubtask/design-system/components';
  import type { Container, CustomFieldDefinition, EntryType, StructureChange, StructurePreview, StructureSettings, Workflow } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { customFields } from '../data/customfields.svelte.ts';
  import { fieldName, fieldsAtHub } from '../data/customfields.ts';
  import { copyKeyFor, entryTypes, offeredTypes as typesOfLevel, typesAtHub, typesShown } from '../data/entrytypes.svelte.ts';
  import { countsChange, figureOffer, isComplete, type FigureKind } from '../data/figureOffer.ts';
  import { levelWords } from '../data/levels.ts';
  import { presetName, presetOrigin, presets, presetValue } from '../data/presets.svelte.ts';
  import { shippedCopy, workflowCopyKey, workflowFor, workflows, workflowsAtHub } from '../data/workflows.svelte.ts';
  import CategoriesDialog from '../entries/CategoriesDialog.svelte';
  import NarrowingDialog from './NarrowingDialog.svelte';
  import { reachOf } from '../settings/reach.ts';
  import SavePresetDialog from './SavePresetDialog.svelte';
  import { GROUP_CODES, STRUCTURE_GROUPS, groupOf, isGroup, orderSections, sectionOf, type StructureGroup } from './placeSections.ts';
  import { DRAWABLE } from '../entries/layouts.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import ViewSectionsAsFile from '../settings/ViewSectionsAsFile.svelte';
  import { placePart } from '../settings/asFile.ts';
  import { viewAsFileAction } from '../settings/viewasfile.svelte.ts';
  import { leaveGuard } from '../frame/leave.svelte.ts';
  import TableSection from '../page/TableSection.svelte';
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
    chooseEntryTypesOf,
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
    /** The section's state in the list: a count, *On*. */
    readonly status?: string;
  }

  interface Props {
    isOpen: boolean;
    /** The page's head: its title and the trail to the place. */
    head: PageHead;
    container: Container;
    /** Whether the reader may change the workspace's shape: *Save as preset* then offers the workspace. */
    mayShapeWorkspace?: boolean;
    /** Whether the reader holds STRUCTURE here: a hub's *Make a copy here* is offered only then. */
    mayShapeHere?: boolean;
    /** The sections before the structure (its general fields) and after it, in the list's order. */
    before?: readonly StructureTab[];
    tabs?: readonly StructureTab[];
    /** The open section's id; none is the list (a phone's first page). */
    tab?: string;
    /** Where each section lives, and going there: every section has its own address. */
    hrefOf: (section: string) => string;
    onsection: (section: string) => void;
    /** Back from a section to the list, below `expanded`. */
    onlist: () => void;
  }

  const { isOpen, head, container, mayShapeWorkspace = false, mayShapeHere = false, before = [], tabs = [], tab, hrefOf, onsection, onlist }: Props = $props();
  // One section list for every place (user-experience.md §5.7): the structure's settings spread over
  // the sections they belong to, in the level's fixed order; one draft and one save bar hold them.
  const groups = $derived((Object.keys(STRUCTURE_GROUPS) as StructureGroup[]).map((id) => ({ id, label: t(GROUP_CODES[id]) })));
  const sections = $derived(orderSections(container.type === 'HUB' ? 'HUB' : 'COLLECTION', [...before, ...groups, ...tabs]));
  const openSection = $derived(sectionOf(tab || (sections[0]?.id ?? 'general')));
  let isSavingPreset = $state(false);

  const structure = $derived(container.structure);
  const isHub = $derived(container.type === 'HUB');
  // A10: the hub's document holds the hub's own blocks and, by handle, its collections'; a
  // collection without a handle has no blocks there to show.
  const fileHub = $derived(isHub ? container.id : container.handle ? (container.parent_id ?? undefined) : undefined);
  let isFileOpen = $state(false);
  const fileAction = $derived(fileHub ? viewAsFileAction(() => (isFileOpen = true)) : undefined);
  /** The hub's file holds its module switches and calendars beside the structure (configuration/ui.md A10). */
  const fileSections = $derived(isHub ? ['modules', ...placePart(true).sections, 'business_calendars'] : placePart(false, container.handle ?? undefined).sections);

  // The presets, the accounts that set a lock, and the number fields a sum may name.
  // From `untrack`: subscribing delivers at once and writes the store this effect would read.
  $effect(() => {
    if (!isOpen) return;
    const id = container.id;
    const hub = isHub;
    const flows = typesShown(manifest.value);
    return untrack(() => {
      const stops = [presets.open(), customFields.open(), entryTypes.open(), ...(hub ? [workflows.open(), customFields.openHub(id)] : [customFields.open(id), ...(flows ? [workflows.open()] : [])])];
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
  $effect(() => leaveGuard.hold(() => changeCount));

  /**
   * What the change in the draft reaches, asked once the person pauses (configuration/ui.md A3):
   * the save bar says it before the save, and *Show plan* opens it in full. A preview that fails
   * leaves the bar without the line; the save asks again and reports it.
   */
  const PREVIEW_PAUSE_MS = 400;
  let reach = $state<{ change: StructureChange; preview: StructurePreview } | undefined>();
  $effect(() => {
    const asked = change;
    const id = container.id;
    reach = undefined;
    if (!asked) return;
    let isCurrent = true;
    const timer = setTimeout(() => {
      containers.previewStructure(id, asked).then(
        (preview) => {
          if (isCurrent) reach = { change: asked, preview };
        },
        () => {},
      );
    }, PREVIEW_PAUSE_MS);
    return () => {
      isCurrent = false;
      clearTimeout(timer);
    };
  });
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
      case 'entry_types': {
        const narrowed = Object.keys(settings.entry_types ?? {});
        return narrowed.length === 0
          ? t('app.structure.entry_types_all')
          : narrowed.map((type) => levelWords(settings, type, t).label).join(', ');
      }
    }
  }

  /** The settings this reader is shown: the entry types only where types are shown at all. */
  const workflowInForce = $derived(
    !isHub && typesShown(manifest.value) ? workflowFor(workflows.list, container.id, container.parent_id ?? undefined, 'task') : undefined,
  );
  const shownSettings = $derived(SETTINGS.filter((setting) => setting !== 'entry_types' || typesShown(manifest.value)));

  /** The key of the type being copied here, while the copy is on its way. */
  let copying = $state<string | undefined>(undefined);

  /** A shared type made the hub's own (UC-FLW-01 check 10), named as its source and keyed freely. */
  async function copyHere(type: { key: string; level: string; name?: string | null }) {
    if (copying !== undefined) return;
    copying = type.key;
    failure = undefined;
    try {
      await entryTypes.copyHere(type.key, container.id, copyKeyFor(entryTypes.all, type.key), type.name ?? levelName(type.level));
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      copying = undefined;
    }
  }

  /** A shared field made the hub's own under its key (UC-WRK-12 check 9). */
  async function copyFieldHere(definition: CustomFieldDefinition) {
    if (copying !== undefined) return;
    copying = `field:${definition.id}`;
    failure = undefined;
    try {
      await customFields.copyHere(definition.id, container.id);
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      copying = undefined;
    }
  }

  /** A shared workflow made the hub's own (UC-FLW-02 check 9); the shipped one with the client's words. */
  async function copyWorkflowHere(workflow: Workflow) {
    if (copying !== undefined) return;
    copying = `workflow:${workflow.key}`;
    failure = undefined;
    const key = workflowCopyKey(workflows.list?.data ?? [], workflow.key);
    try {
      if (workflow.system) {
        await workflows.create(key, workflow.name ?? t('workflows.default_name'), shippedCopy(workflow.published, t), container.id);
      } else {
        await workflows.copyHere(workflow.key, container.id, key);
      }
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      copying = undefined;
    }
  }

  function chooseEntryTypes(type: string, selected: readonly string[]) {
    if (!draft) return;
    const inUse = typesOfLevel(entryTypes.all, type, undefined, fileHub).map((each) => each.key);
    draft = setValue(draft, 'entry_types', chooseEntryTypesOf(values.entry_types, type, inUse, selected));
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
  /** What a collection may apply by itself; the server keeps it (UC-WRK-31 check 2, user-experience.md §7.5). */
  const AUTOMATION = ['REQUIRED_BEYOND_FIRST_COLUMN', 'NEW_INTO_FIRST_COLUMN'] as const;

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

  /** A structure section's state in the list: locked, set here, or the default (PS-9). */
  function groupStatus(group: StructureGroup): Pick<SectionLink, 'status' | 'statusTone'> {
    if (!structure) return {};
    if (group === 'template') return startedFrom ? { status: startedFrom } : {};
    const settings = shownSettings.filter((setting) => groupOf(setting) === group);
    if (settings.length === 0) return {};
    if (settings.some((setting) => lockerOf(setting) !== undefined)) return { status: t('app.settings.status_locked'), statusTone: 'locked' };
    const set = settings.filter((setting) => isOwn(structure, setting, container.type)).length;
    return set > 0 ? { status: t('app.settings.status_set', { count: set }), statusTone: 'set' } : { status: t('app.settings.status_default') };
  }
  const links = $derived<SectionLink[]>(
    sections.map((each) => ({
      id: each.id,
      label: each.label,
      href: hrefOf(each.id),
      ...(isGroup(each.id) ? groupStatus(each.id) : 'status' in each && each.status ? { status: each.status } : {}),
    })),
  );

  // The types a new entry here may take, as one table (page-system §9.6): a tick per row says
  // whether it is offered here, *Make a copy here* sits in the row's menu.
  interface TypeRow {
    readonly type: EntryType;
    readonly name: string;
    readonly level: string;
    readonly source: string;
  }
  const levelRank = (level: string): number => {
    const at = types.indexOf(level);
    return at === -1 ? types.length : at;
  };
  function typeSource(type: EntryType): string {
    if (!type.hub_id) return t('app.structure.hub_types_shared');
    return type.copied_from ? t('app.structure.hub_types_copied', { from: type.copied_from }) : t('app.structure.hub_types_own');
  }
  const typeRows = $derived.by<TypeRow[]>(() => {
    const atHub = isHub ? typesAtHub(entryTypes.all, container.id) : undefined;
    const shown = atHub
      ? [...atHub.shared.filter((each) => !each.archived), ...atHub.own]
      : offeredTypes.flatMap((level) => typesOfLevel(entryTypes.all, level, undefined, fileHub));
    return [...shown]
      .sort((a, b) => levelRank(a.level) - levelRank(b.level))
      .map((type) => ({ type, name: type.name ?? levelName(type.level), level: levelName(type.level), source: typeSource(type) }));
  });
  const offeredKeys = (level: string): string[] => typesOfLevel(entryTypes.all, level, values.entry_types?.[level], fileHub).map((each) => each.key);
  function offerType(row: TypeRow, isOffered: boolean) {
    const keys = offeredKeys(row.type.level);
    chooseEntryTypes(row.type.level, isOffered ? [...keys, row.type.key] : keys.filter((key) => key !== row.type.key));
  }
  const typeColumns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.settings.type_name'), priority: 1 },
    { id: 'level', label: t('app.settings.type_level'), priority: 2 },
    { id: 'source', label: t('app.settings.type_source'), priority: 3 },
    { id: 'offered', label: t('app.settings.type_offered'), priority: 2, isStatus: true },
  ]);
  const copyMenu = (isShared: boolean, name: string) =>
    isHub && mayShapeHere && isShared ? { label: t('app.page.more_for', { name }), items: [{ id: 'copy', label: t('app.structure.hub_types_copy') }] } : undefined;

  interface FieldRow {
    readonly definition: CustomFieldDefinition;
    readonly isShared: boolean;
  }
  const fieldRows = $derived.by<FieldRow[]>(() => {
    if (!isHub) return [];
    const here = fieldsAtHub(customFields.atHub(container.id), container.id);
    return [...here.shared.map((definition) => ({ definition, isShared: true })), ...here.own.map((definition) => ({ definition, isShared: false }))];
  });
  const sourceColumns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.settings.type_name'), priority: 1 },
    { id: 'source', label: t('app.settings.type_source'), priority: 2, isStatus: true },
  ]);
</script>

<!-- A tick per option inside a setting's row: the row's name labels the group (PS-11). -->
{#snippet ticks(nameId: string, options: readonly { value: string; label: string; disabledReason?: string }[], selected: readonly string[], onchange: (selected: readonly string[]) => void)}
  <div class="ticks" role="group" aria-labelledby={nameId}>
    {#each options as option (option.value)}
      <Checkbox
        label={option.label}
        density="sm"
        disabledReason={option.disabledReason}
        checked={selected.includes(option.value)}
        onchange={(event: Event) => {
          const on = (event.currentTarget as HTMLInputElement).checked;
          onchange(on ? [...selected, option.value] : selected.filter((each) => each !== option.value));
        }}
      />
    {/each}
  </div>
{/snippet}

<!-- Where a setting's value comes from, its lock and its reset: the row's source (PS-11). -->
{#snippet chipFor(setting: SettingName)}
  {@const chip = chipOf(setting)}
  <span class="head">
    <SourceChip
      label={chip.label}
      isLocked={chip.isLocked}
      detailsLabel={t('app.structure.where_from', { setting: titleOf(setting) })}
      value={chip.value}
      source={chip.source}
      whomToAsk={chip.whomToAsk}
    />
  </span>
{/snippet}



{#snippet fieldsChip()}{@render chipFor('fields')}{/snippet}

{#snippet resetOf(setting: SettingName)}
  {#if structure && draft && isOwn(structure, setting, container.type) && !draft.reset.includes(setting)}
    <Button size="sm" tone="subtle" aria-label={t('app.structure.reset_named', { setting: titleOf(setting) })} onclick={() => reset(setting)}>
      {t('app.structure.reset')}
    </Button>
  {/if}
{/snippet}

{#snippet saveBar()}
  <SaveBar
    label={t('app.save_bar.label')}
    changesLabel={changeCount > 0 ? t('app.save_bar.changes', { count: String(changeCount) }) : undefined}
    discardLabel={t('app.save_bar.discard')}
    saveLabel={t('app.workspace.save')}
    {isSaving}
    savingLabel={t('app.workspace.saving')}
    onDiscard={discard}
    onSave={() => void save()}
    reachLabel={reach ? reachOf(reach.preview, t) : undefined}
    planLabel={t('app.settings.show_plan')}
    onPlan={reach?.preview.narrows ? () => (narrowing = reach) : undefined}
  />
{/snippet}

{#snippet typesAdvanced()}
  {#if draft}
    {#each offeredTypes as type (type)}
      <SettingRow label={t('app.structure.level_name', { type: typeName(type) })} fieldWidth="sm">
        {#snippet control(nameId)}
          <Input
            label={t('app.structure.level_name', { type: typeName(type) })}
            isLabelHidden
            aria-labelledby={nameId}
            placeholder={typeName(type)}
            maxlength={60}
            disabledReason={lockReason('levels')}
            value={values.levels?.find((level) => level.type === type)?.name ?? ''}
            oninput={(event: Event) => nameLevel(type, (event.currentTarget as HTMLInputElement).value)}
          />
        {/snippet}
      </SettingRow>
    {/each}
    <div class="setting" data-setting="fields">
    {#each offeredTypes as type, index (type)}
      <SettingRow
        label={t('app.structure.fields_of', { level: levelName(type) })}
        groupLabel={index === 0 ? titleOf('fields') : undefined}
        source={index === 0 ? fieldsChip : undefined}
        fieldWidth="full"
      >
        {#snippet control(nameId)}
          <span class="acts">
            {@render ticks(nameId, fieldOptions(type), carried(type), (selected) => chooseFields(type, selected))}
            {#if !isHub}
              <Button size="sm" tone="secondary" disabledReason={categoriesWait} onclick={() => (categoriesOf = type)}>{t('app.structure.categories_of', { level: levelName(type) })}</Button>
            {/if}
            {#if index === 0}{@render resetOf('fields')}{/if}
          </span>
        {/snippet}
                </SettingRow>
              {/each}
      {#if draft.reset.includes('fields')}
        <p class="hint pending" data-reset-pending>{t('app.structure.reset_pending')}</p>
      {/if}
    </div>
  {/if}
{/snippet}

<!-- The structure's settings of one section, one row each, and the one save of the shared draft. -->
{#snippet settingsBody(group: StructureGroup)}
  {#if structure && draft}
    {@const rows = shownSettings.filter((setting) => groupOf(setting) === group && setting !== 'entry_types' && setting !== 'fields')}
    <form class="panel" data-structure-panel onsubmit={(event) => { event.preventDefault(); void save(); }}>
      <SettingsSection
        title={t(GROUP_CODES[group])}
        description={group === 'template' ? t(isHub ? 'app.structure.intro_hub' : 'app.structure.intro') : undefined}
        status={groupStatus(group).status}
        save={saveBar}
        advanced={group === 'types' && offeredTypes.length > 0 ? { label: t('app.settings.types_advanced'), description: t('app.settings.types_advanced_hint'), rows: typesAdvanced } : undefined}
      >
        {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}
        {#if group === 'template'}
          <div data-setting="preset">
            <SettingRow
              label={t('app.structure.preset_label')}
              help={draft.preset
                ? t('app.structure.preset_chosen', { preset: presets.nameOf(draft.preset) ?? '' })
                : startedFrom
                  ? t('app.structure.preset_from', { preset: startedFrom })
                  : t('app.structure.preset_hint')}
              fieldWidth="lg"
              isChanged={draft.preset !== undefined}
            >
              {#snippet control(nameId)}
                <span class="acts">
                  <Select
                    label={t('app.structure.preset_label')}
                    isLabelHidden
                    aria-labelledby={nameId}
                    placeholder={t('app.structure.preset_choose')}
                    options={presetOptions}
                    bind:value={() => draft?.preset ?? presetInForce, (key) => choosePresetKey(key)}
                  />
                  <!-- What is saved is the structure the server holds, so it waits for the save bar. -->
                  <Button size="sm" tone="subtle" disabledReason={changeCount > 0 ? t('app.presets.save_first') : undefined} onclick={() => (isSavingPreset = true)}>
                    {t('app.presets.save_as')}
                  </Button>
                </span>
              {/snippet}
            </SettingRow>
          </div>
        {/if}

        <!-- A collection's Workflow section names the workflow its tasks run, as its head does. -->
        {#if group === 'workflow' && !isHub && workflowInForce}
          <p class="hint" data-collection-workflow={workflowInForce.key}>
            {t('app.workflows.runs_here')}
            <a href="/administration/workflows">{workflowInForce.name ?? t('workflows.default_name')}</a>
          </p>
        {/if}

        {#each rows as setting (setting)}
          <div class="setting" data-setting={setting}>
            {#if setting === 'levels'}
              <SettingRow label={titleOf(setting)} help={t('app.structure.levels_hint')} fieldWidth="lg" lockReason={lockReason('levels')} isChanged={draft.reset.includes(setting)}>
                {#snippet source()}{@render chipFor(setting)}{/snippet}
                {#snippet control(nameId)}
                  <span class="acts">
                    {@render ticks(nameId, levelOptions, offeredTypes, chooseLevels)}
                    {@render resetOf(setting)}
                  </span>
                {/snippet}
              </SettingRow>
            {:else if setting === 'views'}
              <SettingRow label={t('app.structure.views_offered')} fieldWidth="lg" lockReason={lockReason('views')}>
                {#snippet source()}{@render chipFor(setting)}{/snippet}
                {#snippet control(nameId)}
                  <span class="acts">
                    {@render ticks(nameId, viewOptions, (values.views?.offered ?? []) as readonly string[], chooseViews)}
                    {@render resetOf(setting)}
                  </span>
                  {#if isViewMissing}<p class="failure" role="alert">{t('app.structure.views_missing')}</p>{/if}
                {/snippet}
              </SettingRow>
              <SettingRow label={t('app.structure.views_default')} fieldWidth="sm">
                {#snippet control(nameId)}
                  <Select
                    label={t('app.structure.views_default')}
                    isLabelHidden
                    aria-labelledby={nameId}
                    disabledReason={lockReason('views')}
                    options={((values.views?.offered ?? []) as readonly string[]).map((layout) => ({ value: layout, label: t(`app.view.${layout}`) }))}
                    bind:value={() => (values.views?.default as string | undefined) ?? '', (layout) => chooseOpening(layout)}
                  />
                {/snippet}
              </SettingRow>
            {:else if setting === 'counts' && figures}
              <SettingRow label={titleOf(setting)} help={t('app.figure.intro')} fieldWidth="md" lockReason={lockReason('counts')}>
                {#snippet source()}{@render chipFor(setting)}{/snippet}
                {#snippet control(nameId)}
                  <span class="acts">
                    <Select
                      label={t('app.figure.choice')}
                      isLabelHidden
                      aria-labelledby={nameId}
                      disabledReason={lockReason('counts')}
                      options={figures.figures.map((kind) => ({ value: kind, label: t(`app.figure.option.${kind.toLowerCase()}`) }))}
                      bind:value={() => (figures.figures.includes(figure) ? figure : 'NONE'), (kind) => chooseFigure(kind)}
                    />
                    {@render resetOf(setting)}
                  </span>
                {/snippet}
              </SettingRow>
              {#if figure === 'FIELD_SUM'}
                <SettingRow label={t('app.figure.field')} fieldWidth="md">
                  {#snippet control(nameId)}
                    <Select
                      label={t('app.figure.field')}
                      isLabelHidden
                      aria-labelledby={nameId}
                      placeholder={t('app.figure.field_choose')}
                      disabledReason={lockReason('counts')}
                      error={isFieldMissing ? t('app.figure.field_choose') : undefined}
                      options={figures.fields.map((option) => ({ value: option.key, label: option.name }))}
                      bind:value={() => values.counts?.field_key ?? '', (key) => chooseSumField(key)}
                    />
                  {/snippet}
                </SettingRow>
              {/if}
              {#if structure.counts_clipped?.from.figure === 'PROGRESS'}
                <p class="hint" data-figure-clipped>{t('app.figure.clipped', { source: sourceWords((structure.sources?.counts as string | undefined) ?? 'DEFAULT') })}</p>
              {/if}
            {:else if setting === 'completion'}
              <SettingRow label={titleOf(setting)} help={t('app.policies.completion_hint')} fieldWidth="md" lockReason={lockReason('completion')}>
                {#snippet source()}{@render chipFor(setting)}{/snippet}
                {#snippet control(nameId)}
                  <span class="acts">
                    <Select
                      label={t('app.policies.completion')}
                      isLabelHidden
                      aria-labelledby={nameId}
                      disabledReason={lockReason('completion')}
                      options={[
                        { value: 'MANUAL', label: t('app.policies.completion_manual') },
                        { value: 'ROLLUP', label: t('app.policies.completion_rollup') },
                      ]}
                      bind:value={() => (values.completion as string | undefined) ?? 'MANUAL', (choice) => draft && (draft = setValue(draft, 'completion', choice as StructureSettings['completion']))}
                    />
                    {@render resetOf(setting)}
                  </span>
                {/snippet}
              </SettingRow>
            {:else if setting === 'capture'}
              <SettingRow label={titleOf(setting)} help={t('app.structure.capture_label')} fieldWidth="lg" lockReason={lockReason('capture')}>
                {#snippet source()}{@render chipFor(setting)}{/snippet}
                {#snippet control(nameId)}
                  <span class="acts">
                    {@render ticks(
                      nameId,
                      CAPTURE.map((token) => ({ value: token, label: t(`app.structure.capture.${token}`), disabledReason: lockReason('capture') })),
                      (values.capture ?? []) as readonly string[],
                      (selected) => draft && (draft = setValue(draft, 'capture', CAPTURE.filter((token) => selected.includes(token)))),
                    )}
                    {@render resetOf(setting)}
                  </span>
                {/snippet}
              </SettingRow>
            {:else if setting === 'celebration'}
              <SettingRow label={titleOf(setting)} help={t('app.structure.celebration_label')} fieldWidth="sm" lockReason={lockReason('celebration')}>
                {#snippet source()}{@render chipFor(setting)}{/snippet}
                {#snippet control(nameId)}
                  <span class="acts">
                    <Select
                      label={t('app.structure.celebration_label')}
                      isLabelHidden
                      aria-labelledby={nameId}
                      disabledReason={lockReason('celebration')}
                      options={[
                        { value: 'ALL', label: t('app.structure.celebration.ALL') },
                        { value: 'SUBTLE', label: t('app.structure.celebration.SUBTLE') },
                      ]}
                      bind:value={() => (values.celebration as string | undefined) ?? 'ALL', (choice) => draft && (draft = setValue(draft, 'celebration', choice as StructureSettings['celebration']))}
                    />
                    {@render resetOf(setting)}
                  </span>
                {/snippet}
              </SettingRow>
            {:else if setting === 'automation'}
              <SettingRow label={titleOf(setting)} help={t('app.structure.automation_label')} fieldWidth="lg" lockReason={lockReason('automation')}>
                {#snippet source()}{@render chipFor(setting)}{/snippet}
                {#snippet control(nameId)}
                  <span class="acts">
                    {@render ticks(
                      nameId,
                      AUTOMATION.map((behaviour) => ({ value: behaviour, label: t(`app.structure.automation.${behaviour}`), disabledReason: lockReason('automation') })),
                      (values.automation ?? []) as readonly string[],
                      (selected) => draft && (draft = setValue(draft, 'automation', AUTOMATION.filter((behaviour) => selected.includes(behaviour)))),
                    )}
                    {@render resetOf(setting)}
                  </span>
                {/snippet}
              </SettingRow>
            {/if}
            {#if draft.reset.includes(setting)}
              <p class="hint pending" data-reset-pending>{t('app.structure.reset_pending')}</p>
            {/if}
          </div>
        {/each}
      </SettingsSection>
    </form>

    {#if group === 'types' && shownSettings.includes('entry_types')}
      <div class="setting" data-setting="entry_types">
        <TableSection title={titleOf('entry_types')} note={String(typeRows.length)} description={t(isHub ? 'app.structure.hub_types_hint' : 'app.structure.entry_types_hint')}>
          {#snippet acts()}
            {@render chipFor('entry_types')}
            {@render resetOf('entry_types')}
          {/snippet}
          <DataTable
            label={titleOf('entry_types')}
            columns={typeColumns}
            rows={typeRows}
            key={(row) => row.type.key}
            rowLabel={(row) => row.name}
            menu={(row) => copyMenu(!row.type.hub_id, row.name)}
            onmenu={(row) => void copyHere(row.type)}
            isBusy={copying !== undefined}
            actionsLabel={t('app.page.row_actions')}
            sheetDismissLabel={t('app.page.sheet_close')}
            sheetGrabLabel={t('app.page.sheet_grab')}
          >
            {#snippet cell(row, column)}
              {#if column.id === 'name'}<span data-entry-type={row.type.key}>{row.name}</span>
              {:else if column.id === 'level'}{row.level}
              {:else if column.id === 'source'}<span class="quiet">{row.source}</span>
              {:else if column.id === 'offered'}
                {#if offeredTypes.includes(row.type.level)}
                  <Checkbox
                    label={t('app.settings.type_offered_named', { name: row.name })}
                    isLabelHidden
                    density="sm"
                    disabledReason={lockReason('entry_types')}
                    checked={offeredKeys(row.type.level).includes(row.type.key)}
                    onchange={(event: Event) => offerType(row, (event.currentTarget as HTMLInputElement).checked)}
                  />
                {:else}<span class="quiet">{t('app.settings.type_level_off')}</span>{/if}
              {/if}
            {/snippet}
          </DataTable>
        </TableSection>
      </div>
      {#if fieldRows.length > 0}
        <div data-hub-fields>
          <TableSection title={t('app.structure.setting.fields')} note={String(fieldRows.length)} description={t('app.structure.hub_fields_hint')}>
            <DataTable
              label={t('app.structure.setting.fields')}
              columns={sourceColumns}
              rows={fieldRows}
              key={(row) => row.definition.id}
              rowLabel={(row) => fieldName(row.definition)}
              menu={(row) => copyMenu(row.isShared, fieldName(row.definition))}
              onmenu={(row) => void copyFieldHere(row.definition)}
              isBusy={copying !== undefined}
              actionsLabel={t('app.page.row_actions')}
              sheetDismissLabel={t('app.page.sheet_close')}
              sheetGrabLabel={t('app.page.sheet_grab')}
            >
              {#snippet cell(row, column)}
                {#if column.id === 'name'}{fieldName(row.definition)}
                {:else}<span class="quiet">{row.isShared ? t('app.structure.hub_types_shared') : row.definition.copied_from ? t('app.structure.hub_fields_copied') : t('app.structure.hub_types_own')}</span>{/if}
              {/snippet}
            </DataTable>
          </TableSection>
        </div>
      {/if}
    {/if}

    {#if group === 'workflow' && isHub && workflows.list}
      {@const flowsHere = workflowsAtHub(workflows.list.data, container.id)}
      {@const flowRows = [...flowsHere.shared.map((workflow) => ({ workflow, isShared: true })), ...flowsHere.own.map((workflow) => ({ workflow, isShared: false }))]}
      <div data-hub-workflows>
        <TableSection title={t('app.settings.workflows_here')} note={String(flowRows.length)} description={t('app.structure.hub_workflows_hint')}>
          <DataTable
            label={t('app.settings.workflows_here')}
            columns={sourceColumns}
            rows={flowRows}
            key={(row) => row.workflow.key}
            rowLabel={(row) => row.workflow.name ?? row.workflow.key}
            menu={(row) => copyMenu(row.isShared, row.workflow.name ?? row.workflow.key)}
            onmenu={(row) => void copyWorkflowHere(row.workflow)}
            isBusy={copying !== undefined}
            actionsLabel={t('app.page.row_actions')}
            sheetDismissLabel={t('app.page.sheet_close')}
            sheetGrabLabel={t('app.page.sheet_grab')}
          >
            {#snippet cell(row, column)}
              {#if column.id === 'name'}{row.isShared ? (row.workflow.name ?? t('workflows.default_name')) : (row.workflow.name ?? row.workflow.key)}
              {:else}<span class="quiet">{row.isShared ? t('app.structure.hub_types_shared') : row.workflow.copied_from ? t('app.structure.hub_types_copied', { from: row.workflow.copied_from }) : t('app.structure.hub_types_own')}</span>{/if}
            {/snippet}
          </DataTable>
        </TableSection>
      </div>
    {/if}
  {/if}
{/snippet}

<!-- The place's settings page (page-system §7.3): the sections with their state, one at its address,
     the head's quiet *View as file*, and the leave guard holding the structure's unsaved draft. -->
<div class="structure" data-structure-tabs>
  <SettingsLayout
    head={{ ...head, secondary: fileAction ? [fileAction] : [] }}
    nav={{ label: t('app.structure.tabs_label'), sections: links, current: openSection, onselect: onsection }}
    view={tab ? 'detail' : 'list'}
    back={{ label: head.title, onback: onlist }}
  >
    {#if isGroup(openSection)}
      {@render settingsBody(openSection)}
    {/if}
    {@render [...before, ...tabs].find((each) => each.id === openSection)?.body()}
  </SettingsLayout>
</div>
{#if fileHub}<ViewSectionsAsFile sections={fileSections} hubId={fileHub} handle={isHub ? undefined : (container.handle ?? undefined)} bind:isOpen={isFileOpen} isTriggerHidden />{/if}

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
  .structure { min-inline-size: 0; }
  .panel { display: contents; }
  .ticks { display: flex; flex-wrap: wrap; gap: var(--sp-050) var(--sp-200); }
  .acts { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-150); min-inline-size: 0; }
  .head { display: inline-flex; }
  .hint { margin: 0; color: var(--text-secondary); max-inline-size: 64ch; }
  .pending { padding: 0 var(--sp-250) var(--sp-150); }
  .quiet { color: var(--text-secondary); }
  .failure { margin: 0; padding: var(--sp-150) var(--sp-250) 0; color: var(--text-danger); }
</style>
