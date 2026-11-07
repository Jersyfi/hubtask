// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * One setting's control as the catalogue declares it (configuration/ui.md A1, A2): which editor
 * its kind takes, the words of each of its choices, whether the reader may change it here and, where
 * not, why. A screen hands in the value in force with its source and lock as its own read answered
 * them and writes through its own use case; what is drawn is decided here, from the declaration,
 * so a setting added to an area needs its words and nothing else.
 *
 * Pure, and tested beside itself.
 */

import type { SettingDefinition } from '@hubtask/sync-engine';

import { labelCode } from './catalogue.ts';

type Level = SettingDefinition['levels'][number];

/** The editor a kind is drawn with; `none` is a part of another control (a structure, a moment). */
export type Editor = 'count' | 'flag' | 'choice' | 'choices' | 'date' | 'link' | 'text' | 'locale' | 'zone' | 'none';

const EDITORS: Readonly<Record<SettingDefinition['kind'], Editor>> = {
  COUNT: 'count',
  FLAG: 'flag',
  CHOICE: 'choice',
  CHOICES: 'choices',
  DATE: 'date',
  LINK: 'link',
  TEXT: 'text',
  LOCALE: 'locale',
  TIME_ZONE: 'zone',
  STRUCTURE: 'none',
};

/** The editor of a setting's kind; a kind this build does not know yet is drawn as text. */
export const editorOf = (definition: Pick<SettingDefinition, 'kind'>): Editor => EDITORS[definition.kind] ?? 'text';

/** A message code with the parameters it is said with. */
export interface Words {
  readonly code: string;
  readonly params?: Readonly<Record<string, string>>;
}

/** The settings whose choices their own screen already words under a code of its own area. */
const OWN_CHOICES: Readonly<Record<string, (option: string) => Words>> = {
  'config.agent_apply': (value) => ({ code: 'app.changes.agent_apply_option', params: { value } }),
  'time.visibility': (option) => ({ code: `app.time_settings.visibility_${option}` }),
};

/** The words of one choice of a setting: the installation's code for it unless its screen has its own. */
export function choiceWords(key: string, option: string): Words {
  return OWN_CHOICES[key]?.(option) ?? { code: `app.instance.choice.${key}.${option}` };
}

/** The setting's name, as the screen that draws it says it. */
export const nameWords = (key: string): Words => ({ code: labelCode(key) });

/** The value in force at the reader's level, as the screen's own read answered it. */
export interface Cell {
  readonly value: unknown;
  readonly source?: string | null;
  readonly locked_by?: string | null;
}

/**
 * Why the reader cannot change the setting here, or undefined where they can: a level above
 * locked it, or the reader lacks the permission its writer needs at this level (P-05 - the
 * control is then drawn read-only rather than refused on save).
 */
export function lockedWords(
  definition: Pick<SettingDefinition, 'writers'>,
  cell: Cell | undefined,
  level: Level,
  holds: (permission: string) => boolean,
): Words | undefined {
  const lock = cell?.locked_by;
  if (lock === 'INSTANCE') return { code: 'app.source.locked_sentence_instance' };
  if (lock === 'PLAN') return { code: 'app.source.locked_sentence_plan' };
  const writer = definition.writers.find((each) => each.level === level);
  if (!writer || !holds(writer.permission)) return { code: 'app.setting.not_yours' };
  return undefined;
}

/**
 * Reads what was typed into a count. Empty is undecided; a whole number is a number; anything
 * else travels as typed, for the server to refuse at the field rather than for the control to
 * swallow.
 */
export function countFrom(raw: string): number | string | undefined {
  const trimmed = raw.trim();
  if (trimmed === '') return undefined;
  const number = Number(trimmed);
  return Number.isInteger(number) ? number : trimmed;
}

/** A count's bounds as the hint under its field, where the catalogue declares any. */
export function boundsWords(definition: Pick<SettingDefinition, 'minimum' | 'maximum'>): Words | undefined {
  const { minimum, maximum } = definition;
  if (minimum !== undefined && maximum !== undefined) return { code: 'app.setting.between', params: { minimum: String(minimum), maximum: String(maximum) } };
  if (minimum !== undefined) return { code: 'app.setting.at_least', params: { minimum: String(minimum) } };
  if (maximum !== undefined) return { code: 'app.setting.at_most', params: { maximum: String(maximum) } };
  return undefined;
}

/** A list of choices with one switched on or off, in the catalogue's order. */
export function toggled(options: readonly string[], chosen: readonly string[], option: string, on: boolean): string[] {
  return options.filter((each) => (each === option ? on : chosen.includes(each)));
}

/** The value in force said in words, for the source chip's opened details. */
export function shownValue(definition: Pick<SettingDefinition, 'key' | 'kind'>, value: unknown, t: (code: string, params?: Record<string, string>) => string): string {
  if (value === undefined || value === null || value === '') return t('app.setting.no_value');
  const say = (option: string) => {
    const words = choiceWords(definition.key, option);
    return t(words.code, words.params ? { ...words.params } : undefined);
  };
  switch (editorOf(definition)) {
    case 'flag':
      return t(value === true ? 'app.instance.flag_on' : 'app.instance.flag_off');
    case 'choice':
      return say(String(value));
    case 'choices':
      return Array.isArray(value) ? value.map((each) => say(String(each))).join(', ') : String(value);
    default:
      return String(value);
  }
}
