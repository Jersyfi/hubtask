// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A level's words in a collection: the name a person gave it in the structure in force, else the
 * catalogue's word for its type (UC-WRK-23 check 6, `domain-model.md` §2 "Levels and presets").
 *
 * **A given name never runs through a grammar written for a type.** "Add {type, select, TASK
 * {a task} …}" knows the article of "task"; it knows nothing of "item", "Zutat" or "Kapitel". So a
 * named level takes its own message codes (`app.level.*`), which put the name as it was written,
 * and an unnamed one keeps the type's codes and their grammar. Which of the two applies is decided
 * here and nowhere else.
 *
 * The name is the person's content (rule 10): it is shown, never logged or sent anywhere else.
 * Pure, for the reason `offer.ts` is; `capability.svelte.ts` binds it to the catalogue.
 */

import type { StructureSettings } from '@hubtask/sync-engine';

/** The catalogue, as `t` is: a code and its parameters in, a sentence out. */
export type Translate = (code: string, params?: Record<string, string>) => string;

/** The name a person gave the level of this type, as written; `undefined` where none was given. */
export function levelName(settings: StructureSettings | undefined, type: string): string | undefined {
  const name = settings?.levels?.find((level) => level.type === type)?.name;
  return typeof name === 'string' && name.trim() !== '' ? name.trim() : undefined;
}

/** What a screen says about one level of a collection. */
export interface LevelWords {
  readonly type: string;
  /** The given name, or `undefined` where the level carries the catalogue's word. */
  readonly name: string | undefined;
  /** The level on its own - an entry's badge, a heading, a choice: "Task", or the given name. */
  readonly label: string;
  /** The control that adds one: "Add a task", or "Add item". */
  readonly add: string;
  /** The control that adds one inside an entry. */
  addInside(title: string): string;
  /** The command that creates one in a container: "New task in Kitchen". */
  newIn(container: string): string;
}

export function levelWords(settings: StructureSettings | undefined, type: string, translate: Translate): LevelWords {
  const name = levelName(settings, type);
  if (name === undefined) {
    return {
      type,
      name,
      label: translate('app.item.type_name', { type }),
      add: translate('app.entries.add_typed', { type }),
      addInside: (title) => translate('app.entries.add_typed_inside', { type, title }),
      newIn: (container) => translate('app.palette.new_in', { type, name: container }),
    };
  }
  return {
    type,
    name,
    label: name,
    add: translate('app.level.add', { level: name }),
    addInside: (title) => translate('app.level.add_inside', { level: name, title }),
    newIn: (container) => translate('app.level.new_in', { level: name, name: container }),
  };
}

/**
 * The refusals that name a level, and their counterparts that take a name. A refusal outside this
 * list names no level and reads as it is.
 */
const NAMED: Readonly<Record<string, string>> = {
  'items.capability_not_supported': 'app.level.capability_not_supported',
  'items.capability_hidden': 'app.level.capability_hidden',
  'items.level_hidden': 'app.level.level_hidden',
  'items.parent_item_required': 'app.level.parent_item_required',
  'items.parent_type_invalid': 'app.level.parent_type_invalid',
  'items.depth_exceeded': 'app.level.depth_exceeded',
  'items.type_unsupported': 'app.level.type_unsupported',
};

/**
 * A refusal in words, its levels by the names this collection gave them. Where no level it names
 * carries a name, the type's own sentence stands; where one does, the named sentence takes every
 * level it mentions by its label, so "“item” cannot sit in “Task”" never mixes two grammars.
 */
export function levelRefusal(
  settings: StructureSettings | undefined,
  refusal: { readonly code: string; readonly params?: Record<string, string> },
  translate: Translate,
): string {
  const params = refusal.params ?? {};
  const named = NAMED[refusal.code];
  const types = [params.item_type, params.parent_type].filter((type): type is string => typeof type === 'string' && type !== '');
  if (!named || !types.some((type) => levelName(settings, type) !== undefined)) return translate(refusal.code, params);
  const label = (type: string) => levelWords(settings, type, translate).label;
  return translate(named, {
    ...params,
    level: label(params.item_type ?? ''),
    ...(params.parent_type ? { parent: label(params.parent_type) } : {}),
  });
}
