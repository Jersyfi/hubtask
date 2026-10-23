// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Where a capture goes, what it may recognise there, and whether the reader may capture at all
 * (UC-WRK-26 checks 1, 3 and 4). Pure: every answer is computed from what the screens already
 * read - the tree, the roles along a path, the manifest, the structure in force.
 *
 * **The order** (record D6): the collection the line names (`@`, within its `#` hub), else the
 * named hub's first collection that may take an entry, else the collection the reader stands in,
 * else the jumble, else nowhere. A hub has no "default collection"; its first one in the order the
 * reader sees is the one they see first.
 *
 * **Unknown is not no** (D14). A role not read yet answers `undefined` (`mayCreateIn`'s three
 * values), and a candidate whose answer is still out makes the target `pending` rather than
 * skipping to the next rule: the same keys must not land in the jumble because Enter came before a
 * membership read and in the collection after it. The field says it is still working out where the
 * entry goes, and Enter waits for that read.
 */

import type { Container, StructureSettings } from '@hubtask/sync-engine';

import { NOTHING, type Named, type Recognition } from './parse.ts';

/** Whether the reader may create in a collection now; `undefined` while the role is unread. */
export type Admits = (collection: Container) => boolean | undefined;

export type Target =
  | { readonly kind: 'collection'; readonly collection: Container; readonly hub?: Container }
  | { readonly kind: 'jumble' }
  /** A candidate's role or level is still being read. */
  | { readonly kind: 'pending' }
  /** Nothing the reader may write in: the line says so and offers `@` and `#`. */
  | { readonly kind: 'nowhere'; readonly hub?: Container };

export interface Where {
  /** What `readTargets` found. */
  readonly namedCollectionId?: string;
  readonly namedHubId?: string;
  /** The collection the reader stands in: the screen's, or the open entry's. */
  readonly currentCollectionId?: string;
  readonly find: (id: string) => Container | undefined;
  /** A hub's collections in their order; `undefined` while the hub's level is unread. */
  readonly collectionsOf: (hubId: string) => readonly Container[] | undefined;
  readonly admits: Admits;
  /** `WRITE_ITEMS` on the workspace; `undefined` before the manifest answers. */
  readonly jumbleWritable: boolean | undefined;
}

const keyOf = (container: Container) => container.order_key ?? '';
// Byte order, as the server ranks them (`order_key` is compared as bytes, COLLATE "C").
const byOrder = (a: Container, b: Container) => (keyOf(a) < keyOf(b) ? -1 : keyOf(a) > keyOf(b) ? 1 : 0);

const hubOf = (collection: Container, where: Where) => (collection.parent_id ? where.find(collection.parent_id) : undefined);

/** Where the entry goes. */
export function resolveTarget(where: Where): Target {
  // `@`, inside its `#` when both are typed - `readTargets` already chose the right one of two
  // collections sharing a name.
  if (where.namedCollectionId) {
    const collection = where.find(where.namedCollectionId);
    if (!collection) return { kind: 'pending' };
    const admitted = where.admits(collection);
    if (admitted === undefined) return { kind: 'pending' };
    if (admitted) return { kind: 'collection', collection, hub: hubOf(collection, where) };
    // `readTargets` offers only collections that are not refused; one refused since is nowhere,
    // never somewhere the line did not name.
    return { kind: 'nowhere' };
  }

  if (where.namedHubId) {
    const hub = where.find(where.namedHubId);
    const collections = where.collectionsOf(where.namedHubId);
    if (!hub || collections === undefined) return { kind: 'pending' };
    for (const collection of [...collections].sort(byOrder)) {
      const admitted = where.admits(collection);
      if (admitted === undefined) return { kind: 'pending' };
      if (admitted) return { kind: 'collection', collection, hub };
    }
    return { kind: 'nowhere', hub };
  }

  if (where.currentCollectionId) {
    const collection = where.find(where.currentCollectionId);
    if (!collection) return { kind: 'pending' };
    const admitted = where.admits(collection);
    if (admitted === undefined) return { kind: 'pending' };
    if (admitted) return { kind: 'collection', collection, hub: hubOf(collection, where) };
  }

  if (where.jumbleWritable === undefined) return { kind: 'pending' };
  return where.jumbleWritable ? { kind: 'jumble' } : { kind: 'nowhere' };
}

/**
 * Whether the capture control is drawn at all (D13): the manifest has answered, and some write it
 * could make would be accepted - the jumble, or a collection the reader sees and may create in.
 * Absent where every write would be refused (P-05), and absent before anything is known.
 */
export function mayCapture(input: {
  readonly isManifestRead: boolean;
  readonly jumbleWritable: boolean | undefined;
  readonly collections: readonly Container[];
  readonly admits: Admits;
}): boolean {
  if (!input.isManifestRead) return false;
  if (input.jumbleWritable === true) return true;
  return input.collections.some((collection) => input.admits(collection) === true);
}

// ------------------------------------------------------------------------------------------------
// What the target recognises
// ------------------------------------------------------------------------------------------------

/** Hubtask's own value of `capture` (`StructureSettings.go`, `Default`): what an unset one means. */
const DEFAULT_CAPTURE: readonly string[] = ['DATE', 'PERSON', 'LABEL'];

/**
 * The type a capture creates there: the top level in force, else the first type the installation
 * lets stand in a collection - what the collection's own head creates.
 */
export function topLevelOf(settings: StructureSettings | undefined, roots: readonly string[]): string | undefined {
  const top = settings?.levels?.[0]?.type as string | undefined;
  return top ?? roots[0];
}

/**
 * Whether the type carries a capability there. A type the settings leave out carries everything
 * the installation offers it (`StructureLevelFields`: "A type left out carries everything").
 */
export function carries(
  settings: StructureSettings | undefined,
  type: string,
  capability: string,
  offered: (type: string) => readonly string[],
): boolean {
  const own = settings?.fields?.[type]?.capabilities as readonly string[] | undefined;
  return (own ?? offered(type)).includes(capability);
}

export interface Rules {
  readonly date: boolean;
  readonly person: boolean;
  readonly label: boolean;
  /**
   * Several lines are several entries: one level in force, or `LINES` recognised. The one rule
   * both capture fields use; UX-12 moves it into `entries/capture.ts` as `captureRulesOf`, which
   * this function then gives way to.
   */
  readonly splitsLines: boolean;
}

/** What a capture into this collection recognises (record D4): its `capture` ∩ what its top level carries. */
export function rulesOf(
  settings: StructureSettings | undefined,
  roots: readonly string[],
  offered: (type: string) => readonly string[],
): Rules {
  const type = topLevelOf(settings, roots);
  const capture = (settings?.capture as readonly string[] | undefined) ?? DEFAULT_CAPTURE;
  const has = (capability: string) => type !== undefined && carries(settings, type, capability, offered);
  return {
    date: capture.includes('DATE') && has('DUE_DATE'),
    person: capture.includes('PERSON') && has('ASSIGNMENT'),
    label: capture.includes('LABEL') && has('LABELS'),
    // No levels answered is Hubtask's default, every type the installation has - not one level.
    splitsLines: settings?.levels?.length === 1 || capture.includes('LINES'),
  };
}

/**
 * What `readLine` may read for a target. Only a collection recognises anything beyond `#` and `@`:
 * the jumble decides nothing (UC-JUM-01), and a target still being worked out recognises nothing
 * yet - a chip shown now and gone a moment later would be worse than one shown a moment late.
 */
export function recognitionOf(
  target: Target,
  rules: Rules | undefined,
  lists: { readonly people: readonly Named[]; readonly labels: readonly Named[] },
): Recognition {
  if (target.kind !== 'collection' || !rules) return NOTHING;
  return { date: rules.date, person: rules.person, label: rules.label, people: lists.people, labels: lists.labels };
}
