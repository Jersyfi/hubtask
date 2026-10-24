// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The places a capture may go, read from what the tree and the screens already read: the hubs and
 * their collections, the reader's role along each path, the manifest. Reactive glue over the pure
 * `target.ts`; it decides nothing of its own.
 *
 * **Who has the control** (record D13) needs, for a reader without `WRITE_ITEMS` on the workspace
 * - a child with one hub, a member of one collection - whether they may create in *some*
 * collection. That is the one case where this opens every hub's level and every visible
 * collection's memberships, once per session, as the search screen does to resolve `in:` names.
 * A reader who may write in the jumble needs none of it.
 */

import type { Container } from '@hubtask/sync-engine';
import { untrack } from 'svelte';

import { actor } from '../data/account.svelte.ts';
import { manifest } from '../data/capabilities.svelte.ts';
import { holdsHere, roleOrder } from '../data/capability.svelte.ts';
import { containers } from '../data/containers.svelte.ts';
import { archivalOf } from '../data/containers.ts';
import { people } from '../data/people.svelte.ts';
import { mayCreateIn } from '../palette/sources.ts';
import { mayCapture } from './target.ts';

/** The path a collection's role is decided along. */
export const pathOf = (collection: Container) => ({ hubId: collection.parent_id ?? undefined, collectionId: collection.id });

/** The reader's role there; `undefined` until every scope along it is read. */
export function roleIn(collection: Container): string | null | undefined {
  return people.roleOf(pathOf(collection), actor.account?.id, roleOrder());
}

/** The reader's role on a hub itself; `undefined` until it is read. */
export function roleInHub(hub: Container): string | null | undefined {
  return people.roleOf({ hubId: hub.id }, actor.account?.id, roleOrder());
}

/**
 * Whether the reader may create in the collection now: `undefined` while the role or the manifest
 * is still out, never "no" for "not yet" (record D14).
 */
export function admits(collection: Container): boolean | undefined {
  if (!manifest.value) return undefined;
  if (archivalOf(collection) !== 'active') return false;
  const role = roleIn(collection);
  if (role === undefined) return undefined;
  return mayCreateIn(collection, role, manifest.value);
}

/** `WRITE_ITEMS` on the workspace: the jumble takes a capture. */
export const jumbleWritable = (): boolean | undefined => holdsHere('WRITE_ITEMS');

/** The hubs a `#` may name: those the reader sees, not put aside. */
export function liveHubs(): readonly Container[] {
  return containers.hubs.filter((hub) => archivalOf(hub) === 'active');
}

/** Every collection read so far under a live hub, in tree order. */
export function liveCollections(): readonly Container[] {
  return liveHubs().flatMap((hub) => [...containers.collectionsOf(hub.id)].filter((collection) => archivalOf(collection) === 'active'));
}

/**
 * Opens every hub's level until the cleanup runs, so a collection is known by name wherever it is.
 * Call it from an effect; the reads are the tree's, and the engine shares them.
 */
export function openLevels(): () => void {
  const hubIds = containers.hubs.map((hub) => hub.id);
  const closes = untrack(() => hubIds.map((id) => containers.openLevel(id)));
  return () => closes.forEach((close) => close());
}

/** Starts reading the role along each collection's path; kept for the session, as every `people.open`. */
export function openRoles(collections: readonly Container[]): void {
  for (const collection of collections) untrack(() => people.open(pathOf(collection)));
}

/** Whether the capture control is drawn (D13). */
export function mayCaptureHere(): boolean {
  return mayCapture({
    isManifestRead: manifest.value !== undefined,
    jumbleWritable: jumbleWritable(),
    collections: liveCollections(),
    admits,
  });
}
