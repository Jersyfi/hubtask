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
