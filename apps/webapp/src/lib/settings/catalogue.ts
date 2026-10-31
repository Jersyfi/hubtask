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

/** The settings whose screen draws them under a code of its own area rather than the catalogue's. */
const OWN_CODES: Readonly<Record<string, string>> = {
  'agents.registered_only': 'app.agents.registered_only',
  'config.agent_apply': 'app.changes.agent_apply',
  'planning.time_tolerance_days': 'app.planning.tolerance',
};

/** The message code of a setting's name, from the codes its own screen already draws it with. */
export function labelCode(key: string): string {
  const own = OWN_CODES[key];
  if (own) return own;
  const area = areaOf(key);
  if (area === 'structure') return `app.structure.setting.${nameOf(key)}`;
  if (area === 'module') return `app.modules.name_${nameOf(key)}`;
  if (area === 'notifications') return `app.profile.category_${nameOf(key).toUpperCase()}`;
  return `app.instance.setting.${key}`;
}

/** Where one area is drawn at one level: the screen's address and its name's code. */
interface Screen {
  readonly path: string;
  readonly code: string;
}

const INSTANCE: Screen = { path: '/instance/settings', code: 'app.instance.settings' };
const PLAN: Screen = { path: '/instance/plans', code: 'app.instance.plans' };
/** A person's own settings, drawn on their profile (model.md §4); the palette offers them by its own rows (SETTING_ROWS). */
const PERSON: Screen = { path: '/profile', code: 'app.profile.title' };
/** A person's own areas drawn on a page of their own rather than the profile itself. */
const PERSON_AREAS: Readonly<Record<string, Screen>> = {
  notifications: { path: '/profile/notifications', code: 'app.profile.notifications' },
};

/** The workspace's screen of each area; an area without one is not decided by the workspace. */
const WORKSPACE: Readonly<Record<string, Screen>> = {
  sign_in: { path: '/administration/sign-in', code: 'app.admin.sign_in' },
  legal: { path: '/administration/legal', code: 'app.signin_settings.legal' },
  localisation: { path: '/administration/workspace', code: 'app.admin.workspace' },
  ai: { path: '/administration/ai', code: 'app.admin.ai' },
  structure: { path: '/administration/presets', code: 'app.presets.title' },
  module: { path: '/administration/modules', code: 'app.modules.title' },
  knowledge: { path: '/administration/workspace', code: 'app.admin.workspace' },
  config: { path: '/administration/changes', code: 'app.changes.title' },
  agents: { path: '/administration/agents', code: 'app.agents.title' },
  // The workspace's tolerance is drawn beside the modules it belongs to (WorkspaceTolerance).
  planning: { path: '/administration/modules', code: 'app.modules.title' },
};

/** Where an area is drawn on a hub's or a collection's own pages, below the place's address. */
const PLACE: Readonly<Record<string, Screen>> = {
  structure: { path: 'settings/structure', code: 'app.settings.section_structure' },
  module: { path: 'settings/modules', code: 'app.modules.title' },
  planning: { path: 'plan', code: 'app.planning.title' },
};

/** The hub or collection the reader is at and may shape, whose own settings the palette offers. */
export interface Place {
  readonly level: 'HUB' | 'COLLECTION';
  readonly id: string;
}

/**
 * The screen that draws a setting at a level, or none where no screen of the reader's does. A
 * hub's and a collection's screens are the place's own, so they are answered only for a place.
 */
export function screenOf(definition: SettingDefinition, level: Level, place?: Place): Screen | undefined {
  if (!definition.levels.includes(level)) return undefined;
  if (level === 'INSTANCE') return INSTANCE;
  if (level === 'PLAN') return PLAN;
  if (level === 'WORKSPACE') return WORKSPACE[areaOf(definition.key)];
  if (level === 'PERSON') return PERSON_AREAS[areaOf(definition.key)] ?? PERSON;
  if (place === undefined || level !== place.level) return undefined;
  const screen = PLACE[areaOf(definition.key)];
  if (!screen) return undefined;
  return { path: `/${place.level === 'HUB' ? 'hubs' : 'collections'}/${place.id}/${screen.path}`, code: screen.code };
}

/**
 * Whether a setting is one a person sets by name. A value of kind `STRUCTURE` outside the
 * structure area is a part of another control (the rotation's start) and has no row of its own.
 */
export const isNamed = (definition: Pick<SettingDefinition, 'key' | 'kind'>): boolean =>
  definition.kind !== 'STRUCTURE' || areaOf(definition.key) === 'structure';

/**
 * The palette's rows for the catalogue (A7): every setting at every level the reader reaches, each
 * opening the screen that draws it there, at its row. The installation and plans are reached by an
 * operator, the workspace's screens by its administrators, and the settings of the hub or
 * collection the reader is at by whoever may shape it there.
 */
export function catalogueOffers(
  definitions: readonly SettingDefinition[],
  reach: Pick<Reach, 'isAdministrationReachable' | 'isInstanceReachable'>,
  t: Translate,
  place?: Place,
): readonly Offer[] {
  const levels: Level[] = [];
  if (reach.isInstanceReachable) levels.push('INSTANCE', 'PLAN');
  if (reach.isAdministrationReachable) levels.push('WORKSPACE');
  if (place) levels.push(place.level);
  const offers: Offer[] = [];
  for (const definition of definitions) {
    if (!isNamed(definition)) continue;
    const label = t(labelCode(definition.key));
    for (const level of levels) {
      const screen = screenOf(definition, level, place);
      if (!screen) continue;
      offers.push({
        kind: 'setting',
        id: `${level}:${definition.key}`,
        label,
        // The key finds it too: the words `hubctl` and the configuration file use.
        keywords: definition.key,
        context: t(screen.code),
        area: level === 'INSTANCE' || level === 'PLAN' ? 'instance' : level === 'WORKSPACE' ? 'administration' : undefined,
        target: { kind: 'setting', path: screen.path, row: definition.key },
      });
    }
  }
  return offers;
}
