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

/** The types a new entry of the level may take - not archived, the level's own first. */
export function offeredTypes(types: readonly EntryType[], level: string): readonly EntryType[] {
  return types
    .filter((type) => type.level === level && !type.archived)
    .sort((a, b) => Number(b.system) - Number(a.system) || a.position - b.position || a.key.localeCompare(b.key));
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
