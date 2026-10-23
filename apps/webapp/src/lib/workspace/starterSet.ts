// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The starter sets of a workspace's first start (UC-WRK-30 check 5): for each answer to "what do
 * you use Hubtask for", a hub and its collections on matching presets. The client's, not the
 * server's: their names are words in the reader's language (`app.starter.name.*`), which rule 8
 * keeps out of the backend. A collection on its hub's preset names none and follows the hub.
 */

import type { BuiltInKey } from '../data/presetChoice.ts';

export const PURPOSES = ['MYSELF', 'HOUSEHOLD', 'CLIENTS', 'TEAM'] as const;
export type Purpose = (typeof PURPOSES)[number];

export interface StarterPlace {
  /** The catalogue code of its name. */
  readonly name: string;
  /** Its preset; absent on a collection that follows its hub. */
  readonly preset?: BuiltInKey;
}

export interface StarterSet {
  readonly hub: StarterPlace & { readonly preset: BuiltInKey };
  readonly collections: readonly StarterPlace[];
}

const name = (key: string): string => `app.starter.name.${key}`;

export const STARTER_SETS: Readonly<Record<Purpose, StarterSet>> = {
  MYSELF: {
    hub: { name: name('personal'), preset: 'TASKS' },
    collections: [{ name: name('to_do') }, { name: name('shopping'), preset: 'LIST' }, { name: name('ideas'), preset: 'LIST' }],
  },
  HOUSEHOLD: {
    hub: { name: name('home'), preset: 'TASKS' },
    collections: [{ name: name('shopping'), preset: 'LIST' }, { name: name('chores') }, { name: name('projects'), preset: 'PROJECT' }],
  },
  CLIENTS: {
    hub: { name: name('clients'), preset: 'PROJECT' },
    collections: [{ name: name('first_client') }, { name: name('pipeline'), preset: 'FLOW' }],
  },
  TEAM: {
    hub: { name: name('team'), preset: 'PROJECT' },
    collections: [{ name: name('projects') }, { name: name('requests'), preset: 'FLOW' }, { name: name('meetings'), preset: 'TASKS' }],
  },
};
