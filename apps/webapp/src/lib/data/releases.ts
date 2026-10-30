// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The release screen's choices (UC-PLN-08): which release it opens on, where a release's open
 * work may move, and which of a collection's entries may still join one.
 *
 * Pure, so that they are checked by `node --test`. The server decides what ships and writes the
 * notes; this module only picks from what it answered.
 */

import type { Release } from '@hubtask/sync-engine';

/** The planned releases a releasing one's open entries may move into: the soonest first. */
export function releaseTargets(releases: readonly Release[], releasing: string): Release[] {
  return releases
    .filter((r) => r.state === 'PLANNED' && r.id !== releasing)
    .sort((a, b) => a.release_on.localeCompare(b.release_on));
}

/** The release a screen opens on: the next planned one, else the latest released. */
export function releaseShown(releases: readonly Release[]): Release | undefined {
  return releaseTargets(releases, '')[0] ?? [...releases].sort((a, b) => b.release_on.localeCompare(a.release_on))[0];
}

/** The entries offered to join a release: those of the list not in it already. */
export function releaseCandidates<T extends { id: string }>(entries: readonly T[], inRelease: readonly string[]): T[] {
  const held = new Set(inRelease);
  return entries.filter((entry) => !held.has(entry.id));
}
