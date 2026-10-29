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

/** Every key of every area the server answered, area by area, each in its declared position. */
export function rowsOf(settings: InstanceSettings | undefined): Row[] {
  if (!settings) return [];
  return AREAS.flatMap((area) =>
    Object.entries(settings[area] ?? {})
      .map(([key, setting]) => ({ area, key, setting }))
      // The domain's order where the server says it; a JSON object's own order is not one.
      .sort((a, b) => (a.setting.position ?? 0) - (b.setting.position ?? 0)),
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
 * One key as the draft holds it, or undefined where it is undecided. The terms' version has no lock
 * of its own: it is locked exactly when the terms link is, so while the form is open it reads the
 * link's lock in the draft - not the one the server last answered.
 */
export function draftEntry(draft: InstanceSettings | undefined, area: Area, key: string): InstanceSetting | undefined {
  const held = draft?.[area]?.[key];
  if (!held?.set) return undefined;
  if (area === 'legal' && key === 'terms_version') {
    return { ...held, locked: draft?.legal?.terms_url?.locked === true };
  }
  return held;
}

/**
 * Whether a count is a number of bytes, which is said as a size rather than as its digits. The
 * attachment storage is the one; the contract declares no unit, so it is named here as the limits
 * screen names it.
 */
export const isBytes = (area: Area, key: string): boolean => area === 'quotas' && key === 'media_bytes';

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
  // The installation-only values beside the areas travel as they were read: a `PUT` that left one
  // out would switch it off.
  const document: InstanceSettings = {
    blocklist_file: draft.blocklist_file,
    metering: draft.metering,
    reserved_short_names: draft.reserved_short_names,
    owners_change_address: draft.owners_change_address,
    address_change_cooldown_days: draft.address_change_cooldown_days,
    owners_own_domains: draft.owners_own_domains,
  };
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

/**
 * How many values a draft changes against what is in force: each key whose decision, value or
 * lock differs, and the blocklist's path - the save bar's count (UC-WRK-32 check 3).
 */
export function changesBetween(draft: InstanceSettings, read: InstanceSettings): number {
  const after = documentOf(draft);
  const before = documentOf(read);
  let changes = (after.blocklist_file ?? '') === (before.blocklist_file ?? '') ? 0 : 1;
  for (const area of AREAS) {
    const keys = new Set([...Object.keys(after[area] ?? {}), ...Object.keys(before[area] ?? {})]);
    for (const key of keys) {
      if (JSON.stringify(after[area]?.[key] ?? null) !== JSON.stringify(before[area]?.[key] ?? null)) changes += 1;
    }
  }
  return changes;
}

/** A refusal's field, as the server names it: the area and the key. */
export const fieldOf = (area: Area, key: string): string => `/${area}/${key}`;

/** The areas a plan decides, in the screen's order: the installation's, without localisation. */
export const PLAN_AREAS = ['sign_in', 'legal', 'quotas', 'ai'] as const satisfies readonly Area[];

/** A plan's document in the installation level's shape: the areas it shares with it. */
export type PlanAreas = Partial<Record<Area, Record<string, InstanceSetting>>>;

/**
 * A plan's body to send: its name, the features it leaves out and every decided key of its areas -
 * a `PUT` replaces what the plan decides, so a key left out is one it no longer decides.
 */
export function planBodyOf(
  displayName: string,
  leftOut: readonly string[],
  areas: PlanAreas,
): Record<string, unknown> {
  const body: Record<string, unknown> = { display_name: displayName, left_out: [...leftOut] };
  for (const area of PLAN_AREAS) {
    const entries: Record<string, InstanceSetting> = {};
    for (const [key, setting] of Object.entries(areas[area] ?? {})) {
      if (!setting.set) continue;
      entries[key] = { set: true, value: setting.value, locked: setting.locked };
    }
    body[area] = entries;
  }
  return body;
}

/**
 * Whether a workspace may choose a model this installation offers: `ai.sources` decided as the
 * offered model or either. Undecided, the product's default holds - their own model only
 * (`EffectiveAi`) - and a plan or a workspace only narrows what the installation leaves, so no
 * level below can open it. `undefined` while the level is unread.
 */
export function isOfferedAiReachable(settings: InstanceSettings | undefined): boolean | undefined {
  if (!settings) return undefined;
  const sources = settings.ai?.['sources'];
  return sources?.set === true && (sources.value === 'OFFERED' || sources.value === 'EITHER');
}
