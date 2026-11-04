// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which hubs the browser's copy holds in a workspace too large to hold whole (offline-sync.md §1):
 * those this device opened in the last thirty days - the hub itself or a collection in it - most
 * recent first. The device keeps when it last opened each hub, by identifier alone, per account,
 * and sends it nowhere; it ends with the session like the recents.
 */

export const OFFLINE_DAYS = 30;
/** At most this many hubs are remembered; a copy holding more would pass the bound anyway. */
export const MAX_REMEMBERED = 200;

const DAY_MS = 24 * 60 * 60 * 1000;
const PREFIX = 'hubtask.hub-visits.';

/** Where the visits are kept: `localStorage`, or a test's map. */
export interface VisitStore {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

function defaultStore(): VisitStore | undefined {
  try {
    return globalThis.localStorage;
  } catch {
    return undefined;
  }
}

function read(store: VisitStore | undefined, accountId: string): Record<string, number> {
  try {
    const held = JSON.parse(store?.getItem(PREFIX + accountId) ?? '{}') as unknown;
    if (typeof held !== 'object' || held === null || Array.isArray(held)) return {};
    return Object.fromEntries(Object.entries(held).filter(([, at]) => typeof at === 'number')) as Record<string, number>;
  } catch {
    return {};
  }
}

/** The hubs opened within the thirty days before `now`, most recent first. */
export function offlineHubs(accountId: string | undefined, now: number, store = defaultStore()): readonly string[] {
  if (!accountId) return [];
  const since = now - OFFLINE_DAYS * DAY_MS;
  return Object.entries(read(store, accountId))
    .filter(([, at]) => at >= since)
    .sort((a, b) => b[1] - a[1])
    .map(([hub]) => hub);
}

/** Notes that a hub was opened now; what is older than thirty days or past the cap is let go. */
export function noteHubVisit(accountId: string | undefined, hubId: string, now: number, store = defaultStore()): void {
  if (!accountId || !store) return;
  const since = now - OFFLINE_DAYS * DAY_MS;
  const kept = Object.entries({ ...read(store, accountId), [hubId]: now })
    .filter(([, at]) => at >= since)
    .sort((a, b) => b[1] - a[1])
    .slice(0, MAX_REMEMBERED);
  try {
    store.setItem(PREFIX + accountId, JSON.stringify(Object.fromEntries(kept)));
  } catch {
    // A browser that refuses storage holds the whole copy or none; nothing is lost.
  }
}

/** Ends them with the session. */
export function forgetHubVisits(accountId: string | undefined, store = defaultStore()): void {
  if (!accountId) return;
  try {
    store?.removeItem(PREFIX + accountId);
  } catch {
    // Nothing to remove from.
  }
}
