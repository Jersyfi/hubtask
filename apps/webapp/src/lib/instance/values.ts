// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The installation's defaults as a reader meets them: every key of every area, drawn by the kind
 * the server declares for it (UC-INS-09 checks 1 to 3).
 *
 * **The server says the kind; this module says the words.** A key's kind, its closed set, a
 * count's bounds and what an open key leaves a workspace free to do all arrive with the key, so a
 * key added to the level needs its words in the catalogue and nothing here. Guessing the kind from
 * the current value is what sent an undecided number as text.
 */

import type { InstanceSetting, InstanceSettings } from '@hubtask/sync-engine';

/** The five areas, in the order the screen draws them - the contract's own properties. */
export const AREAS = ['sign_in', 'legal', 'localisation', 'quotas', 'ai'] as const;
export type Area = (typeof AREAS)[number];

/** One key of one area, as the screen draws it. */
export interface Row {
  readonly area: Area;
  readonly key: string;
  readonly setting: InstanceSetting;
}

/** Every key of every area the server answered, area by area, in the order it answered them. */
export function rowsOf(settings: InstanceSettings | undefined): Row[] {
  if (!settings) return [];
  return AREAS.flatMap((area) =>
    Object.entries(settings[area] ?? {}).map(([key, setting]) => ({ area, key, setting })),
  );
}

/** The code of a key's name in words. */
export const nameCode = (area: Area, key: string): string => `app.instance.setting.${area}.${key}`;

/** The code of one value of a closed set in words. */
export const choiceCode = (area: Area, key: string, value: string): string =>
  `app.instance.choice.${area}.${key}.${value}`;

/**
 * What the lock column says for a key, open or locked, worded by what an open key leaves a
 * workspace free to do. A key that is never locked - a language, a zone, a week start - says
 * nothing, because there is nothing a workspace may not do.
 */
export function lockCode(setting: InstanceSetting, locked: boolean): string | undefined {
  const may = setting.workspace_may;
  if (!may) return undefined;
  const word = may.toLowerCase();
  return locked ? `app.instance.lock_${word}_locked` : `app.instance.lock_${word}_open`;
}

/**
 * Reads what was typed into a count. Empty is undecided; a whole number is a number; anything else
 * travels as it was typed, for the server to refuse at the row rather than for this screen to
 * swallow.
 */
export function countOf(raw: string): number | string | undefined {
  const trimmed = raw.trim();
  if (trimmed === '') return undefined;
  const number = Number(trimmed);
  return Number.isInteger(number) ? number : trimmed;
}

/**
 * The document to send: every area as the draft holds it, without the read-only declarations - a
 * `PUT` replaces the level, so an undecided key is simply absent.
 */
export function documentOf(draft: InstanceSettings): InstanceSettings {
  const document: InstanceSettings = { blocklist_file: draft.blocklist_file };
  for (const area of AREAS) {
    const entries: Record<string, InstanceSetting> = {};
    for (const [key, setting] of Object.entries(draft[area] ?? {})) {
      if (!setting.set) continue;
      entries[key] = { set: true, value: setting.value, locked: setting.locked };
    }
    document[area] = entries;
  }
  return document;
}

/** A refusal's field, as the server names it: the area and the key. */
export const fieldOf = (area: Area, key: string): string => `/${area}/${key}`;
