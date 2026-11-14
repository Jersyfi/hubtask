// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The arithmetic of an inventory object's page (work-surface §6.10, IN-2, IN-3, scene `ci`): where
 * the stage stands on its path, which attributes the object type recommends and nobody filled, the
 * work on the object split into open and done, the open entry a neighbour in the picture shows, and
 * the device's life as steps from the register record's moves. Pure, so the page draws and the tests
 * hold the same answers.
 */

import type { CiAsset, CiAttribute, CiGroup, CiReached, CiWorkEntry, ConfigurationItem } from '@hubtask/sync-engine';

import { ASSET_STATES, type AssetState } from '../data/cmdb.ts';
import { holdsAttributes, lineOfGroup, passes, readLine, type NarrowContext } from '../data/inventoryquery.ts';

/** The stages in the order an object walks them (the step bar's steps). */
export const STAGE_PATH = ['PLANNED', 'ORDERED', 'IN_STOCK', 'IN_USE', 'MAINTENANCE', 'RETIRED', 'DISPOSED'] as const;
export type Stage = (typeof STAGE_PATH)[number];

/** A stage other than *In use*: a neighbour in the picture says it beside its name. */
export const isQuietStage = (stage: string) => stage !== 'IN_USE';

/**
 * The attributes the object type recommends or requires that hold no value: the line
 * `+ n recommended attributes missing` counts them, and pressing it opens the edit.
 */
export function missingRecommended(attributes: readonly CiAttribute[], values: Readonly<Record<string, unknown>>): CiAttribute[] {
  return attributes.filter((each) => (each.need === 'RECOMMENDED' || each.need === 'REQUIRED') && isBlank(values[each.key]));
}

function isBlank(value: unknown): boolean {
  if (value === undefined || value === null) return true;
  if (typeof value === 'string') return value.trim() === '';
  if (Array.isArray(value)) return value.length === 0;
  return false;
}

/** The work on an object (IN-3): the open entries shown, the completed ones folded under *n done*. */
export function workGroups(entries: readonly CiWorkEntry[]): { readonly open: readonly CiWorkEntry[]; readonly done: readonly CiWorkEntry[] } {
  return { open: entries.filter((each) => !each.completed), done: entries.filter((each) => each.completed) };
}

/**
 * The key of the open entry each neighbour carries, by the neighbour's id: an entry on an object
 * beneath names the nearest one it reaches through (`via`), so the picture marks that node. The
 * first one in the read's order wins - the latest named.
 */
export function openKeysByObject(entries: readonly CiWorkEntry[]): ReadonlyMap<string, string> {
  const keys = new Map<string, string>();
  for (const each of entries) {
    if (each.completed || each.reason !== 'BENEATH' || !each.via || !each.key) continue;
    if (!keys.has(each.via.ci_id)) keys.set(each.via.ci_id, each.key);
  }
  return keys;
}

/** One step of a device's life, as the register shows it (scene `ci` and `invreg`). */
export interface LifeStep {
  readonly state: AssetState;
  /** Reached at some time, or passed over on the way to where it stands. */
  readonly isReached: boolean;
  /** The step it stands on now. */
  readonly isCurrent: boolean;
  /** When it last arrived there; absent for a record made before the steps were kept. */
  readonly at?: string;
  readonly by?: string | null;
  /** With whom, or in which stockroom, it was at that step. */
  readonly holder?: string | null;
  readonly stockroom?: string | null;
}

/**
 * The life from requested to disposed, each step with its day and detail from the moves: the
 * latest arrival in a step counts, because a device can go back to the stockroom and out again. A
 * record without moves still shows where it stands, every step before it as passed.
 */
export function lifeSteps(asset: Pick<CiAsset, 'state' | 'moves'>): LifeStep[] {
  const current = ASSET_STATES.indexOf(asset.state as AssetState);
  const moves = asset.moves ?? [];
  return ASSET_STATES.map((state, index) => {
    const last = [...moves].reverse().find((move) => move.state === state);
    return {
      state,
      isReached: last !== undefined || (current >= 0 && index <= current),
      isCurrent: index === current,
      ...(last ? { at: last.at, by: last.by, holder: last.assigned_to, stockroom: last.stockroom_id } : {}),
    };
  });
}

/** The saved choice between the relations' picture and list, per reader on this device. */
export type RelationsView = 'picture' | 'list';
export const RELATIONS_VIEW_KEY = 'hubtask.ci.relations';

const viewKey = (reader: string | undefined) => (reader ? `${RELATIONS_VIEW_KEY}.${reader}` : RELATIONS_VIEW_KEY);

export function readRelationsView(store: Pick<Storage, 'getItem'> | undefined, reader?: string): RelationsView | undefined {
  try {
    const kept = store?.getItem(viewKey(reader));
    return kept === 'picture' || kept === 'list' ? kept : undefined;
  } catch {
    return undefined;
  }
}

export function keepRelationsView(store: Pick<Storage, 'setItem'> | undefined, view: RelationsView, reader?: string): void {
  try {
    store?.setItem(viewKey(reader), view);
  } catch {
    // A store that refuses keeps nothing; the choice still holds while the page is open.
  }
}

/** The objects the hub knows, by id: who owns one, which need it has, which stage, for a row. */
export const byId = (cis: readonly ConfigurationItem[]): ReadonlyMap<string, ConfigurationItem> => new Map(cis.map((each) => [each.id, each]));

/**
 * Whom a raised need came from (*inherited from Kundenportal*): the nearest object among those that
 * suffer when this one fails whose own need in that dimension is the level it was raised to. The
 * impact reads the objects; their own needs are the hub's copy's. Undefined where none is known.
 */
export function inheritedFrom(
  dimension: 'c' | 'i' | 'a',
  level: string | null,
  reached: readonly Pick<CiReached, 'ci_id' | 'depth'>[],
  known: ReadonlyMap<string, Pick<ConfigurationItem, 'protection' | 'effective_protection'>>,
): string | undefined {
  if (!level) return undefined;
  const nearest = [...reached].sort((a, b) => a.depth - b.depth);
  return nearest.find((each) => known.get(each.ci_id)?.protection?.[dimension] === level)?.ci_id;
}

/**
 * The saved groups that hold an object (the rail's *Groups*): each group's filter read as the
 * inventory's line reads it, and its attribute values beside it - the device answers what the
 * server's member count would, so the rail needs no read per group.
 */
export function groupsHolding(ci: ConfigurationItem, groups: readonly CiGroup[], context: NarrowContext): CiGroup[] {
  return groups.filter((group) => {
    const of = lineOfGroup(group.filter);
    const read = readLine(of.line);
    return read.ok && passes(ci, read.filter, context) && holdsAttributes(ci, of.attributes);
  });
}
