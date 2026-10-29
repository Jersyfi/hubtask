// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace's entry types (UC-FLW-01): which a new entry of a level may take, what an entry's
 * type is, and how it is drawn. Pure, for the reason `levels.ts` is; `entrytypes.svelte.ts` binds it
 * to the read.
 *
 * **Without the workflows module nothing here is shown** (check 8): `typesShown` is the one place
 * that asks, so a picker and a row mark cannot disagree about it. A system type - the level's own,
 * keyed after it - is never marked on a row: it is what an entry is without a type of its own.
 */

import { ICON_NAMES, type IconName } from '@hubtask/design-system/icons';
import type { Capabilities, EntryType } from '@hubtask/sync-engine';

/** The module whose screens these are, as `modules_on` names it. */
export const WORKFLOWS = 'WORKFLOWS';

/** Whether the workspace shows types at all: the workflows module is on. */
export function typesShown(manifest: Capabilities | undefined): boolean {
  const on = (manifest as { modules_on?: readonly string[] } | undefined)?.modules_on ?? [];
  return on.includes(WORKFLOWS);
}

/** The key of a level's own type: the level in lower case. */
export const systemKeyOf = (level: string): string => level.toLowerCase();

/**
 * The types a new entry of the level may take - not archived, the level's own first. `offered` is
 * the collection's `entry_types` for the level: only those, in its order, the level's own still
 * first where it is among them. A key that names no type in use is passed over, and a list of which
 * none is left offers every type - the server's rule (`EntryTypes.ChooseAmong`), so the first one
 * here is the one an entry sent without a type gets.
 */
export function offeredTypes(types: readonly EntryType[], level: string, offered?: readonly string[]): readonly EntryType[] {
  const inUse = types
    .filter((type) => type.level === level && !type.archived)
    .sort((a, b) => Number(b.system) - Number(a.system) || a.position - b.position || a.key.localeCompare(b.key));
  if (!offered || offered.length === 0) return inUse;
  const narrowed = offered.flatMap((key) => inUse.filter((type) => type.key === key));
  if (narrowed.length === 0) return inUse;
  return [...narrowed].sort((a, b) => Number(b.system) - Number(a.system));
}

/** The type keys a collection's settings offer on a level, or none where it offers every type. */
export function entryTypesOffered(settings: { entry_types?: Record<string, readonly string[]> } | undefined, level: string): readonly string[] | undefined {
  return settings?.entry_types?.[level];
}

/** An entry's type: its own key, or its level's own where it names none or one not read yet. */
export function typeOf(types: readonly EntryType[], level: string, key: string | undefined): EntryType | undefined {
  const own = key ? types.find((type) => type.key === key) : undefined;
  return own ?? types.find((type) => type.key === systemKeyOf(level));
}

/** The mark a row draws for its entry's type; none for the level's own type. */
export function rowMarkOf(
  types: readonly EntryType[],
  level: string,
  key: string | undefined,
): { readonly icon: IconName; readonly colorToken: string; readonly name: string } | undefined {
  const type = typeOf(types, level, key);
  if (!type || type.system) return undefined;
  return { icon: iconOf(type), colorToken: type.color_token, name: type.name ?? type.key };
}

/** The icon a type names, where the set has it; a tag otherwise, never a broken image. */
export function iconOf(type: Pick<EntryType, 'icon'>): IconName {
  return (ICON_NAMES as readonly string[]).includes(type.icon) ? (type.icon as IconName) : 'tag';
}

/** The levels a type sits on, in the order the administration lists them. */
export const TYPE_LEVELS = ['TASK', 'WORK_PACKAGE', 'ACTIVITY'] as const;

/** The icons the administration offers a type, from the design system's set. */
export const TYPE_ICONS: readonly IconName[] = [
  'tag', 'task', 'work-package', 'activity', 'square-check', 'star', 'flag', 'zap', 'lightbulb',
  'rocket', 'wrench', 'hammer', 'code', 'git-branch', 'shield', 'lock', 'key', 'circle-alert',
  'triangle-alert', 'file-text', 'book-open', 'briefcase', 'calendar', 'mail', 'inbox', 'user',
  'users', 'globe', 'house', 'heart', 'gift', 'shopping-cart', 'wallet', 'sparkles',
];

/** Every type of a level as the administration lists it - archived ones included, last. */
export function levelTypes(types: readonly EntryType[], level: string): readonly EntryType[] {
  return types
    .filter((type) => type.level === level)
    .sort(
      (a, b) =>
        Number(a.archived) - Number(b.archived) ||
        Number(b.system) - Number(a.system) ||
        a.position - b.position ||
        a.key.localeCompare(b.key),
    );
}

/**
 * The positions that move a type one place up or down among its level's types that are not
 * archived: every type of the level numbered anew from 1, and only those whose number changes
 * returned, so a move writes as few types as it can. The level's own type stays first.
 */
export function reorder(
  types: readonly EntryType[],
  key: string,
  by: -1 | 1,
): readonly { readonly type: EntryType; readonly position: number }[] {
  const moved = types.find((type) => type.key === key);
  if (!moved || moved.system || moved.archived) return [];
  const order = offeredTypes(types, moved.level).filter((type) => !type.system);
  const at = order.findIndex((type) => type.key === key);
  const to = at + by;
  if (to < 0 || to >= order.length) return [];
  const next = [...order];
  [next[at], next[to]] = [next[to] as EntryType, next[at] as EntryType];
  return next
    .map((type, index) => ({ type, position: index + 1 }))
    .filter(({ type, position }) => type.position !== position);
}

/** A key from a name, as the administration proposes it: lower case, words joined by `_`. */
export function keyFrom(name: string): string {
  return name
    .normalize('NFKD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
    .slice(0, 50);
}

/**
 * Every type in the order a list groups by and a filter offers them (S-03): by level as the
 * administration lists them, each level's own type first, archived ones last - an archived type
 * still marks the entries that keep it, so it is still a value to group or filter by.
 */
export function typeOrder(types: readonly EntryType[]): readonly EntryType[] {
  return TYPE_LEVELS.flatMap((level) => levelTypes(types, level));
}

/**
 * A type's name by its key: its own, a level's own type named after its level (`levelName`, the
 * caller's translation), a key no read type carries as itself - a list and a board head a group so.
 */
export function typeNameOf(types: readonly EntryType[], key: string, levelName: (level: string) => string): string {
  const type = types.find((each) => each.key === key);
  if (!type) return key;
  return type.name ?? levelName(type.level);
}

/** The values a change of type clears once confirmed; a completion is never among them. */
export const DROPPABLE = ['due_at', 'recurrence'] as const;

/**
 * The values a change of type would drop, by field, from the server's refusal
 * (`entry_types.values_dropped`, UC-FLW-01 check 4); none for any other answer. `isConfirmable` is
 * false where one of them is a completion, which is never dropped: the entry is reopened first.
 */
export function droppedBy(error: unknown): { readonly fields: readonly string[]; readonly isConfirmable: boolean } | undefined {
  const refusal = error as { detailCode?: string; fieldErrors?: readonly { path?: string }[] } | undefined;
  if (refusal?.detailCode !== 'entry_types.values_dropped') return undefined;
  const fields = (refusal.fieldErrors ?? []).flatMap((each) => (each.path ? [each.path.replace(/^\//, '')] : []));
  if (fields.length === 0) return undefined;
  return { fields, isConfirmable: fields.every((field) => (DROPPABLE as readonly string[]).includes(field)) };
}
