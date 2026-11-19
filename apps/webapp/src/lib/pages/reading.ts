// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The pages this device opened last, for the pages' home's *continue reading* (scene phome): by
 * identifier alone, per account, sent nowhere and ended with the session, as the hub visits are
 * (`offlinehubs.ts`). The home resolves the titles from the spaces it reads.
 */

import type { VisitStore } from '../data/offlinehubs.ts';

/** At most this many pages are remembered; the home shows the first few it can still read. */
export const MAX_READ = 24;

const PREFIX = 'hubtask.pages-read.';

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

/** The pages opened on this device, the last opened first. */
export function pagesRead(accountId: string | undefined, store = defaultStore()): readonly string[] {
  if (!accountId) return [];
  return Object.entries(read(store, accountId)).sort((a, b) => b[1] - a[1]).map(([id]) => id);
}

/** Notes that a page was opened now; past the cap the oldest is let go. */
export function notePageRead(accountId: string | undefined, pageId: string, now: number, store = defaultStore()): void {
  if (!accountId || !store) return;
  const kept = Object.entries({ ...read(store, accountId), [pageId]: now }).sort((a, b) => b[1] - a[1]).slice(0, MAX_READ);
  try {
    store.setItem(PREFIX + accountId, JSON.stringify(Object.fromEntries(kept)));
  } catch {
    // A browser that refuses storage offers nothing to continue; nothing is lost.
  }
}

/** Ends them with the session. */
export function forgetPagesRead(accountId: string | undefined, store = defaultStore()): void {
  if (!accountId) return;
  try {
    store?.removeItem(PREFIX + accountId);
  } catch {
    // Nothing to remove from.
  }
}
