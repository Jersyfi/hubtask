// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the *Structure* panel holds between opening and saving (UC-WRK-23 checks 1-3, 7): the
 * values shown, the preset chosen, the settings sent back to what is set above - and the one
 * `PATCH …/structure` they make.
 *
 * The server resolves; this computes nothing it would decide. A preset's values are the ones
 * `/presets` answered; a reset sends the setting's name and shows its value until the answer
 * brings what is set above; a locked setting is never sent - the server refuses it, and the
 * panel shows it as the lock keeps it (`structure.setting_locked`).
 */

import type { Capabilities, EffectiveStructure, Preset, StructureChange, StructureSettings } from '@hubtask/sync-engine';

import { allowedChildTypes, rootTypes } from '../data/capability.ts';
import { presetChoice, presetValue } from '../data/presetChoice.ts';

/**
 * The nine settings, in the order the panel shows them (UC-WRK-23 check 2). `entry_types` is drawn
 * only where the workflows module is on (UC-FLW-01 checks 3, 8).
 */
export const SETTINGS = ['levels', 'fields', 'views', 'counts', 'completion', 'capture', 'celebration', 'automation', 'entry_types'] as const;
export type SettingName = (typeof SETTINGS)[number];

export interface Draft {
  /**
   * The preset chosen in this sitting, if any - Hubtask's key or a saved preset's id
   * (`presetChoice.ts`): its values replace the container's own.
   */
  readonly preset: string | undefined;
  /** What each setting shows now. */
  readonly values: StructureSettings;
  /** The settings sent back to what is set above. */
  readonly reset: readonly SettingName[];
}

/** The source of a value the container sets itself: a collection's own, or a hub's. */
export const ownSource = (type: string): 'HUB' | 'COLLECTION' => (type === 'HUB' ? 'HUB' : 'COLLECTION');

/** The lock on a whole setting, or on one field of one level (`fields.TASK.DUE_DATE`). */
export function lockOf(structure: EffectiveStructure, key: string): EffectiveStructure['locks'][string] | undefined {
  return structure.locks?.[key];
}

/** Whether a setting is set by this container and may be sent back to what is set above. */
export function isOwn(structure: EffectiveStructure, setting: SettingName, containerType: string): boolean {
  return structure.sources?.[setting] === ownSource(containerType) && lockOf(structure, setting) === undefined;
}

const cloneSettings = (settings: StructureSettings): StructureSettings => JSON.parse(JSON.stringify(settings)) as StructureSettings;

export function startDraft(structure: EffectiveStructure): Draft {
  return { preset: undefined, values: cloneSettings(structure.settings), reset: [] };
}

/**
 * A preset chosen: its values replace every setting the workspace has not locked; a locked one
 * keeps the value in force, as the server keeps it (`kept_by_lock`). Resets chosen before are
 * dropped - the preset replaces them as it replaces the rest.
 */
export function choosePreset(draft: Draft, structure: EffectiveStructure, preset: Preset): Draft {
  const values = cloneSettings(structure.settings) as Record<string, unknown>;
  const from = cloneSettings(preset.settings) as Record<string, unknown>;
  for (const setting of SETTINGS) {
    if (lockOf(structure, setting) !== undefined) continue;
    if (from[setting] !== undefined) values[setting] = from[setting];
  }
  return { ...draft, preset: presetValue(preset), values: values as StructureSettings, reset: [] };
}

/** One setting changed in the panel. */
export function setValue<K extends SettingName>(draft: Draft, setting: K, value: StructureSettings[K]): Draft {
  return { ...draft, values: { ...draft.values, [setting]: value }, reset: draft.reset.filter((each) => each !== setting) };
}

/** One setting sent back to what is set above; its value shows as it was until the answer arrives. */
export function resetSetting(draft: Draft, structure: EffectiveStructure, setting: SettingName): Draft {
  const values = { ...draft.values, [setting]: cloneSettings(structure.settings)[setting] } as StructureSettings;
  return { ...draft, values, reset: draft.reset.includes(setting) ? draft.reset : [...draft.reset, setting] };
}

/**
 * A comparable form of a value: arrays of words sorted, since the order of a set of capabilities,
 * views or capture words carries nothing - except the levels, whose order is the top-down one.
 */
function canonical(value: unknown, key = ''): unknown {
  if (Array.isArray(value)) {
    const items = value.map((each) => canonical(each));
    return key !== 'levels' && items.every((each) => typeof each === 'string') ? [...items].sort() : items;
  }
  if (value && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>)
        // An absent value and a null one say the same: no field named, no name given.
        .filter(([, each]) => each !== undefined && each !== null)
        .map(([name, each]) => [name, canonical(each, name)] as const)
        .sort(([a], [b]) => (a < b ? -1 : 1)),
    );
  }
  return value ?? null;
}
const same = (a: unknown, b: unknown, key: string): boolean => JSON.stringify(canonical(a, key)) === JSON.stringify(canonical(b, key));

/** The change the draft makes, or `undefined` where it makes none. */
export function changeOf(draft: Draft, structure: EffectiveStructure, chosen: Preset | undefined): StructureChange | undefined {
  const base = (draft.preset && chosen ? choosePreset(startDraft(structure), structure, chosen).values : structure.settings) as Record<string, unknown>;
  const values = draft.values as Record<string, unknown>;
  const settings: Record<string, unknown> = {};
  for (const setting of SETTINGS) {
    if (draft.reset.includes(setting) || lockOf(structure, setting) !== undefined) continue;
    if (values[setting] === undefined || same(values[setting], base[setting], setting)) continue;
    settings[setting] = values[setting];
  }
  const change: StructureChange = {
    ...presetChoice(draft.preset),
    ...(Object.keys(settings).length > 0 ? { settings: settings as StructureSettings } : {}),
    ...(draft.reset.length > 0 ? { reset: [...draft.reset] } : {}),
  };
  return Object.keys(change).length > 0 ? change : undefined;
}

/** How many changes the save bar counts: the preset, each setting set, each reset. */
export function countOf(change: StructureChange | undefined): number {
  if (!change) return 0;
  return (change.preset || change.preset_id ? 1 : 0) + Object.keys(change.settings ?? {}).length + (change.reset?.length ?? 0);
}

// -------------------------------------------------------------------------------------------------
// The levels
// -------------------------------------------------------------------------------------------------

/** The installation's types, top-down: each root, then what it holds, breadth first. */
export function typesTopDown(manifest: Capabilities | undefined): readonly string[] {
  const order: string[] = [...rootTypes(manifest)];
  for (let index = 0; index < order.length; index += 1) {
    for (const child of allowedChildTypes(manifest, order[index] ?? '')) if (!order.includes(child)) order.push(child);
  }
  return order;
}

/** The parent types the installation gives a type: an activity has two. */
const parentsOf = (manifest: Capabilities | undefined, type: string): readonly string[] =>
  typesTopDown(manifest).filter((each) => allowedChildTypes(manifest, each).includes(type));

/**
 * Why a level cannot be switched as asked, or nothing: the top level is always offered, and a
 * level is offered only beneath a level it belongs under (`structure.levels_not_closed`).
 */
export function levelRefusal(manifest: Capabilities | undefined, offered: readonly string[], type: string): 'top' | 'parent' | undefined {
  if (rootTypes(manifest).includes(type)) return 'top';
  if (offered.includes(type)) return undefined;
  return parentsOf(manifest, type).some((parent) => offered.includes(parent)) ? undefined : 'parent';
}

/**
 * The levels after switching one: in the installation's order, names kept; a level that loses
 * every level it sits beneath goes with it, as the server would refuse it otherwise.
 */
export function toggleLevel(manifest: Capabilities | undefined, levels: StructureSettings['levels'], type: string): NonNullable<StructureSettings['levels']> {
  const current = levels ?? [];
  const names = new Map(current.map((level) => [level.type as string, level.name ?? null]));
  const wanted = new Set(current.map((level) => level.type as string));
  if (wanted.has(type)) wanted.delete(type);
  else wanted.add(type);
  const kept: string[] = [];
  for (const each of typesTopDown(manifest)) {
    if (!wanted.has(each)) continue;
    if (rootTypes(manifest).includes(each) || parentsOf(manifest, each).some((parent) => kept.includes(parent))) kept.push(each);
  }
  return kept.map((each) => ({ type: each, name: names.get(each) ?? null })) as NonNullable<StructureSettings['levels']>;
}

/** The levels with one level's own name set; an empty one is no name, and the translated default applies. */
export function renameLevel(levels: StructureSettings['levels'], type: string, name: string): NonNullable<StructureSettings['levels']> {
  const trimmed = name.trim();
  return (levels ?? []).map((level) => (level.type === type ? { ...level, name: trimmed === '' ? null : name } : level)) as NonNullable<StructureSettings['levels']>;
}

/**
 * The fields after the panel's checkboxes for one level changed. The checkboxes speak only for the
 * capabilities they draw: one the installation does not offer here stays as it is, and so does the
 * rest of the level's entry - its `categories` (UX-12's grouping, chosen in its own dialog) above
 * all. *Done or open* is always carried.
 */
/**
 * The collection's entry types after the reader ticked `selected` of a level's `inUse` keys
 * (UC-FLW-01 check 3): every one ticked leaves the level out - it offers every type, including one
 * added later - and none ticked is not a choice, so the value stays as it was.
 */
export function chooseEntryTypesOf(
  offered: StructureSettings['entry_types'],
  type: string,
  inUse: readonly string[],
  selected: readonly string[],
): NonNullable<StructureSettings['entry_types']> {
  const current = { ...(offered ?? {}) } as Record<string, string[]>;
  const kept = inUse.filter((key) => selected.includes(key));
  if (kept.length === 0) return current as NonNullable<StructureSettings['entry_types']>;
  if (kept.length === inUse.length) delete current[type];
  else current[type] = kept;
  return current as NonNullable<StructureSettings['entry_types']>;
}

export function chooseFieldsOf(
  fields: StructureSettings['fields'],
  type: string,
  offered: readonly string[],
  selected: readonly string[],
): NonNullable<StructureSettings['fields']> {
  const own = fields?.[type];
  const beyond = (own?.capabilities ?? []).filter((capability) => !offered.includes(capability));
  const capabilities = [...offered.filter((capability) => capability === 'COMPLETION' || selected.includes(capability)), ...beyond];
  return { ...(fields ?? {}), [type]: { ...(own ?? {}), capabilities } } as NonNullable<StructureSettings['fields']>;
}
