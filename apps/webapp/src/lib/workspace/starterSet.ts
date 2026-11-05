// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The starter sets of a workspace's first start (UC-WRK-30 check 5): for each answer to "what do
 * you use Hubtask for", a hub and its collections on matching presets. The client's, not the
 * server's: their names are words in the reader's language (`app.starter.name.*`), which rule 8
 * keeps out of the backend. A collection on its hub's preset names none and follows the hub.
 */

import type { BuiltInKey } from '../data/presetChoice.ts';
import type { StarterSetBody } from '../data/workspace.svelte.ts';
import { afterTaking } from '../product/templates.ts';
import { familiesOf, familyOf, type Family } from './hubGallery.ts';

export const PURPOSES = ['MYSELF', 'HOUSEHOLD', 'CLIENTS', 'TEAM'] as const;
export type Purpose = (typeof PURPOSES)[number];

export interface StarterPlace {
  /** The catalogue code of its name. */
  readonly name: string;
  /** Its preset; absent on a collection that follows its hub. */
  readonly preset?: BuiltInKey;
  /** The catalogue codes of a collection's sample entries' titles (user-experience.md §3.2). */
  readonly samples?: readonly string[];
}

export interface StarterSet {
  readonly hub: StarterPlace & { readonly preset: BuiltInKey };
  readonly collections: readonly StarterPlace[];
}

const name = (key: string): string => `app.starter.name.${key}`;
const sample = (...keys: string[]): readonly string[] => keys.map((key) => `app.starter.sample.${key}`);

export const STARTER_SETS: Readonly<Record<Purpose, StarterSet>> = {
  MYSELF: {
    hub: { name: name('personal'), preset: 'TASKS' },
    collections: [
      { name: name('to_do'), samples: sample('plan_week', 'dentist') },
      { name: name('shopping'), preset: 'LIST', samples: sample('coffee', 'bread') },
      { name: name('ideas'), preset: 'LIST', samples: sample('weekend_trip') },
    ],
  },
  HOUSEHOLD: {
    hub: { name: name('home'), preset: 'TASKS' },
    collections: [
      { name: name('shopping'), preset: 'LIST', samples: sample('milk', 'detergent') },
      { name: name('chores'), samples: sample('bins', 'plants') },
      { name: name('projects'), preset: 'PROJECT', samples: sample('hallway') },
    ],
  },
  CLIENTS: {
    hub: { name: name('clients'), preset: 'PROJECT' },
    collections: [
      { name: name('first_client'), samples: sample('proposal', 'kickoff') },
      { name: name('pipeline'), preset: 'FLOW', samples: sample('lead') },
    ],
  },
  TEAM: {
    hub: { name: name('team'), preset: 'PROJECT' },
    collections: [
      { name: name('projects'), samples: sample('launch_plan') },
      { name: name('requests'), preset: 'FLOW', samples: sample('drive_access') },
      { name: name('meetings'), preset: 'TASKS', samples: sample('weekly_agenda') },
    ],
  },
};

/**
 * The starter's families (user-experience.md §3.2): *Simple* first - the four purposes above, each
 * a set on presets -, then each family of the catalogue holding a template, in the gallery's
 * order. A register's template, which has no hub part, stands with them: its set is the hub on the
 * workspace's default and the register's workspace part.
 */
export function starterFamilies<T extends FamilyOffered>(templates: readonly T[]): { family: Family | 'simple'; templates: T[] }[] {
  return [
    { family: 'simple', templates: [] },
    ...familiesOf(templates).map((family) => ({ family, templates: templates.filter((each) => familyOf(each) === family) })),
  ];
}

type FamilyOffered = Parameters<typeof familyOf>[0];

/**
 * The body of a purpose's set, its words in the reader's language: the hub, its collections and
 * their sample entries, which come along unasked as a template's do (§3.2) and go in one press.
 */
export function purposeStarterBody(purpose: Purpose, words: (code: string) => string): StarterSetBody {
  const set = STARTER_SETS[purpose];
  return {
    hub_name: words(set.hub.name),
    hub_preset: set.hub.preset,
    collections: set.collections.map((each) => ({
      name: words(each.name),
      ...(each.preset ? { preset: each.preset } : {}),
      ...(each.samples && each.samples.length > 0 ? { samples: each.samples.map(words) } : {}),
    })),
  };
}

/** The body of a set built on a template: the hub named after it, its answers, samples and acknowledgement. */
export function templateStarterBody(
  hubName: string,
  key: string,
  answers: Readonly<Record<string, string>>,
  samples: boolean,
  acknowledge: boolean,
): StarterSetBody {
  return {
    hub_name: hubName, collections: [], template: key,
    ...(Object.keys(answers).length > 0 ? { answers: { ...answers } } : {}),
    ...(samples ? { samples: true } : {}),
    ...(acknowledge ? { acknowledge: true } : {}),
  };
}

/** Where the app opens after the set (§3.2): the template's main product's home in the hub, else the hub. */
export function afterStarter(template: Parameters<typeof afterTaking>[0] | undefined, hubId: string): string {
  return (template ? afterTaking(template, hubId) : undefined) ?? `/hubs/${encodeURIComponent(hubId)}`;
}
