// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A query line handed from one screen to a collection's list in memory (*Als Liste öffnen*,
 * work-surface §6.5): never in the address, because a line can hold names and words the reader
 * typed (security.md §9). It is taken once, by the collection it was meant for, and goes stale
 * after a few seconds, so a later visit opens the collection as it is.
 */

const FRESH_MS = 10_000;

let held: { readonly collectionId: string; readonly line: string; readonly at: number } | undefined;

/** Leaves the line for the collection about to be opened. */
export function handOver(collectionId: string, line: string, now = Date.now()): void {
  held = { collectionId, line, at: now };
}

/** The line left for this collection, once; nothing for another one or after it went stale. */
export function takeOver(collectionId: string, now = Date.now()): string | undefined {
  const left = held;
  if (!left || left.collectionId !== collectionId) return undefined;
  held = undefined;
  return now - left.at <= FRESH_MS ? left.line : undefined;
}
