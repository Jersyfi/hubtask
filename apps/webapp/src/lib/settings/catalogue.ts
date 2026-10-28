// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The settings catalogue as the screens read it (configuration/ui.md A7, A10): what a setting is
 * called, which screen draws it at which level, and the rows the palette offers for it.
 *
 * **The list is the server's** (`GET /settings/catalogue`); nothing here declares a setting. What
 * is the client's is only where each area is drawn: one screen per area and level, so a setting
 * added to an area is found on that area's screen without a change here.
 *
 * Pure, over a translator handed in, and tested beside itself.
 */

import type { SettingDefinition } from '@hubtask/sync-engine';

import type { Offer, Reach, Translate } from '../palette/sources.ts';

type Level = SettingDefinition['levels'][number];

/** A setting's area: the first part of its key - `sign_in`, `module`. */
export const areaOf = (key: string): string => key.slice(0, Math.max(0, key.indexOf('.'))) || key;

/** A setting's name inside its area: `min_length`, `planning`. */
export const nameOf = (key: string): string => key.slice(key.indexOf('.') + 1);

/** The message code of a setting's name, from the codes its own screen already draws it with. */
export function labelCode(key: string): string {
  const area = areaOf(key);
  if (area === 'structure') return `app.structure.setting.${nameOf(key)}`;
  if (area === 'module') return `app.modules.name_${nameOf(key)}`;
  return `app.instance.setting.${key}`;
}

/** Where one area is drawn at one level: the screen's address and its name's code. */
interface Screen {
  readonly path: string;
  readonly code: string;
}

const INSTANCE: Screen = { path: '/instance/settings', code: 'app.instance.settings' };
const PLAN: Screen = { path: '/instance/plans', code: 'app.instance.plans' };

/** The workspace's screen of each area; an area without one is not decided by the workspace. */
const WORKSPACE: Readonly<Record<string, Screen>> = {
  sign_in: { path: '/administration/sign-in', code: 'app.admin.sign_in' },
  legal: { path: '/administration/legal', code: 'app.signin_settings.legal' },
  localisation: { path: '/administration/workspace', code: 'app.admin.workspace' },
  ai: { path: '/administration/ai', code: 'app.admin.ai' },
  structure: { path: '/administration/presets', code: 'app.presets.title' },
  module: { path: '/administration/modules', code: 'app.modules.title' },
};

/** The screen that draws a setting at a level, or none where no screen of the reader's does. */
export function screenOf(definition: SettingDefinition, level: Level): Screen | undefined {
  if (!definition.levels.includes(level)) return undefined;
  if (level === 'INSTANCE') return INSTANCE;
  if (level === 'PLAN') return PLAN;
  if (level === 'WORKSPACE') return WORKSPACE[areaOf(definition.key)];
  return undefined;
}

/**
 * Whether a setting is one a person sets by name. A value of kind `STRUCTURE` outside the
 * structure area is a part of another control (the rotation's start) and has no row of its own.
 */
const isNamed = (definition: SettingDefinition): boolean =>
  definition.kind !== 'STRUCTURE' || areaOf(definition.key) === 'structure';

/**
 * The palette's rows for the catalogue (A7): every setting at every level the reader reaches, each
 * opening the screen that draws it there, at its row. The installation and plans are reached by an
 * operator, the workspace's screens by its administrators; a hub's and a collection's settings are
 * reached through the place itself, which the palette lists already.
 */
export function catalogueOffers(
  definitions: readonly SettingDefinition[],
  reach: Pick<Reach, 'isAdministrationReachable' | 'isInstanceReachable'>,
  t: Translate,
): readonly Offer[] {
  const levels: Level[] = [];
  if (reach.isInstanceReachable) levels.push('INSTANCE', 'PLAN');
  if (reach.isAdministrationReachable) levels.push('WORKSPACE');
  const offers: Offer[] = [];
  for (const definition of definitions) {
    if (!isNamed(definition)) continue;
    const label = t(labelCode(definition.key));
    for (const level of levels) {
      const screen = screenOf(definition, level);
      if (!screen) continue;
      offers.push({
        kind: 'setting',
        id: `${level}:${definition.key}`,
        label,
        // The key finds it too: the words `hubctl` and the configuration file use.
        keywords: definition.key,
        context: t(screen.code),
        area: level === 'WORKSPACE' ? 'administration' : 'instance',
        target: { kind: 'setting', path: screen.path, row: definition.key },
      });
    }
  }
  return offers;
}
