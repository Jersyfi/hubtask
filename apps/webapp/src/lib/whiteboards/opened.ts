// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The boards this device opened last, for the whiteboards' home's *recently opened* (scene whome):
 * by identifier alone, per account, sent nowhere and ended with the session, as the hub visits are
 * (`offlinehubs.ts`). The home resolves them against the boards it reads.
 */

import type { VisitStore } from '../data/offlinehubs.ts';

/** At most this many boards are remembered; the home shows the first few it can still read. */
export const MAX_OPENED = 24;

const PREFIX = 'hubtask.boards-opened.';

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

/** The boards opened on this device, the last opened first. */
export function boardsOpened(accountId: string | undefined, store = defaultStore()): readonly string[] {
  if (!accountId) return [];
  return Object.entries(read(store, accountId)).sort((a, b) => b[1] - a[1]).map(([id]) => id);
}

/** Notes that a board was opened now; past the cap the oldest is let go. */
export function noteBoardOpened(accountId: string | undefined, boardId: string, now: number, store = defaultStore()): void {
  if (!accountId || !store) return;
  const kept = Object.entries({ ...read(store, accountId), [boardId]: now }).sort((a, b) => b[1] - a[1]).slice(0, MAX_OPENED);
  try {
    store.setItem(PREFIX + accountId, JSON.stringify(Object.fromEntries(kept)));
  } catch {
    // A browser that refuses storage falls back to the recently changed; nothing is lost.
  }
}

/** Ends them with the session. */
export function forgetBoardsOpened(accountId: string | undefined, store = defaultStore()): void {
  if (!accountId) return;
  try {
    store?.removeItem(PREFIX + accountId);
  } catch {
    // Nothing to remove from.
  }
}

/**
 * The boards the home draws first: those opened here, in that order, as far as they are still
 * readable; when none is, the last changed (`recent`). `opened` says which of the two it is.
 */
export function boardsToContinue<B extends { id: string }>(openedIds: readonly string[], all: readonly B[], recent: readonly B[], count: number): { boards: B[]; opened: boolean } {
  const byId = new Map(all.map((board) => [board.id, board]));
  const boards = openedIds.map((id) => byId.get(id)).filter((board): board is B => board !== undefined).slice(0, count);
  return boards.length > 0 ? { boards, opened: true } : { boards: recent.slice(0, count), opened: false };
}
