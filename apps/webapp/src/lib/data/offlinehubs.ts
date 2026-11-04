// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which hubs the browser's copy holds in a workspace too large to hold whole (offline-sync.md §1):
 * those this device opened in the last thirty days - a hub itself, or a collection in it - most
 * recent first. The device's recents are the record of what was opened, and they are sent nowhere.
 */

import type { Recent } from '../recents.ts';

export const OFFLINE_DAYS = 30;

const DAY_MS = 24 * 60 * 60 * 1000;

export function offlineHubs(recents: readonly Recent[], now: number): readonly string[] {
  const since = now - OFFLINE_DAYS * DAY_MS;
  const hubs: string[] = [];
  for (const row of [...recents].sort((a, b) => (b.at ?? 0) - (a.at ?? 0))) {
    if (row.at === undefined || row.at < since) continue;
    const hub = row.kind === 'hub' ? row.id : row.kind === 'collection' ? row.placeId : undefined;
    if (hub && !hubs.includes(hub)) hubs.push(hub);
  }
  return hubs;
}
