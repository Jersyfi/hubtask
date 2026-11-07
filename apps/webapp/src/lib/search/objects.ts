// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The named objects of the modules, found by name (user-experience.md §5.3): one source per kind,
 * with the module that must be on, the reads it makes - the workspace's list, each hub's or each
 * collection's - and the row each listed object becomes. The search and the palette read the same
 * sources, so one finds what the other does. Every read is the reader's own, so what they may not
 * read is not found (P-05). Pages are searched by the server and stay outside (`pages.search`).
 */

import type { IconName } from '@hubtask/design-system/components';
import type {
  ConfigurationItemList, DashboardList, GoalList, PortfolioList, ReleaseList, SprintList, WhiteboardList,
} from '@hubtask/sync-engine';

import { cisPath } from '../data/cmdb.ts';
import { nameMatches, type SearchKind } from './kinds.ts';

/** One object a source found, as a row of the search and the palette. */
export interface ObjectRow {
  readonly kind: ObjectKind;
  readonly id: string;
  readonly title: string;
  readonly href: string;
  /** The hub or collection it stands in, named beside it; none for a workspace's object. */
  readonly placeId?: string;
}

export type ObjectKind = Extract<SearchKind, 'boards' | 'goals' | 'portfolios' | 'dashboards' | 'assets' | 'releases' | 'sprints'>;

export interface ObjectSource {
  readonly kind: ObjectKind;
  readonly module: string;
  readonly icon: IconName;
  /** Whose list is read: the workspace's once, each hub's, or each collection's. */
  readonly scope: 'workspace' | 'hub' | 'collection';
  readonly path: (placeId: string) => string;
  readonly rows: (list: unknown) => readonly ObjectRow[];
}

/** A hub's inventory is read in its largest page: a name search over a first page of 100 missed the rest. */
const CI_PAGE = 500;

const dataOf = <T>(list: unknown): readonly T[] => ((list as { data?: readonly T[] } | undefined)?.data ?? []);

export const OBJECT_SOURCES: readonly ObjectSource[] = [
  {
    kind: 'boards', module: 'WHITEBOARD', icon: 'layout-template', scope: 'hub', path: (hub) => `/hubs/${hub}/whiteboards`,
    rows: (list) => dataOf<WhiteboardList['data'][number]>(list).map((board) => ({ kind: 'boards', id: board.id, title: board.title, href: `/whiteboards/${board.id}`, placeId: board.hub_id })),
  },
  {
    kind: 'goals', module: 'GOALS', icon: 'flag', scope: 'workspace', path: () => '/goals',
    rows: (list) => dataOf<GoalList['data'][number]>(list).map((goal) => ({ kind: 'goals', id: goal.id, title: goal.title, href: `/goals/${goal.id}`, placeId: goal.hub_id ?? undefined })),
  },
  {
    kind: 'portfolios', module: 'GOALS', icon: 'briefcase', scope: 'workspace', path: () => '/portfolios',
    rows: (list) => dataOf<PortfolioList['data'][number]>(list).map((portfolio) => ({ kind: 'portfolios', id: portfolio.id, title: portfolio.name, href: `/portfolios/${portfolio.id}` })),
  },
  {
    kind: 'dashboards', module: 'GOALS', icon: 'gauge', scope: 'workspace', path: () => '/dashboards',
    rows: (list) => dataOf<DashboardList['data'][number]>(list).map((dashboard) => ({ kind: 'dashboards', id: dashboard.id, title: dashboard.name, href: `/dashboards/${dashboard.id}`, placeId: dashboard.hub_id ?? undefined })),
  },
  {
    kind: 'assets', module: 'ASSETS', icon: 'shield', scope: 'hub', path: (hub) => cisPath(hub, { limit: CI_PAGE }),
    rows: (list) => dataOf<ConfigurationItemList['data'][number]>(list)
      .filter((ci) => !ci.merged_into)
      .map((ci) => ({ kind: 'assets', id: ci.id, title: ci.name, href: `/cis/${ci.id}`, placeId: ci.hub_id })),
  },
  {
    // A release has no screen of its own: it opens on its hub's releases.
    kind: 'releases', module: 'PLANNING', icon: 'rocket', scope: 'hub', path: (hub) => `/hubs/${hub}/releases`,
    rows: (list) => dataOf<ReleaseList['data'][number]>(list).map((release) => ({ kind: 'releases', id: release.id, title: release.name, href: `/hubs/${release.hub_id}/releases`, placeId: release.hub_id })),
  },
  {
    kind: 'sprints', module: 'PLANNING', icon: 'repeat', scope: 'collection', path: (collection) => `/collections/${collection}/sprints`,
    rows: (list) => dataOf<SprintList['data'][number]>(list).map((sprint) => ({ kind: 'sprints', id: sprint.id, title: sprint.name, href: `/collections/${sprint.collection_id}/sprints`, placeId: sprint.collection_id })),
  },
];

/** The sources whose module is on and whose kind is wanted. */
export function sourcesWanted(modulesOn: readonly string[] | undefined, wants: (kind: ObjectKind) => boolean): readonly ObjectSource[] {
  return OBJECT_SOURCES.filter((source) => (modulesOn?.includes(source.module) ?? false) && wants(source.kind));
}

/** Every read the sources make over the places the reader sees, each once, keyed by its path. */
export function readsOf(
  sources: readonly ObjectSource[],
  places: { readonly hubs: readonly string[]; readonly collections: readonly string[] },
): readonly { readonly path: string; readonly source: ObjectSource }[] {
  return sources.flatMap((source) => {
    const ids = source.scope === 'workspace' ? [''] : source.scope === 'hub' ? places.hubs : places.collections;
    return ids.map((id) => ({ path: source.path(id), source }));
  });
}

/**
 * The rows read that hold every word, per kind at most `limit`, a name that starts with the words
 * before one that only holds them.
 */
export function objectHits(read: Readonly<Record<string, readonly ObjectRow[]>>, words: string, limit: number): readonly ObjectRow[] {
  const wanted = words.trim().toLocaleLowerCase();
  const all = Object.values(read).flat().filter((row) => nameMatches(row.title, words));
  const starts = (row: ObjectRow) => (row.title.toLocaleLowerCase().startsWith(wanted) ? 0 : 1);
  return OBJECT_SOURCES.flatMap((source) =>
    all.filter((row) => row.kind === source.kind).sort((a, b) => starts(a) - starts(b)).slice(0, limit),
  );
}
