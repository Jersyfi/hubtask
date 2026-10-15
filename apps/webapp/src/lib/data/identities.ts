// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The tab's note that it left for a provider to connect it to the account (ADR-0078 §1).
 *
 * The provider knows one callback address, which a sign-in, a step-up and a connection all come
 * back to. What tells the callback that this return finishes a connection is this note, written just
 * before the browser leaves and taken once when it comes back. It holds no credential - the state
 * and the code arrive in the address - only that a connection is under way and when it left.
 */

/** The storage the note lives in: the tab's own, so another tab's sign-in is never mistaken for it. */
export type Store = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;

const NOTE = 'hubtask.connect-return';

/**
 * How long the note is believed: the server's flow lives ten minutes, and a note older than that is a
 * trip abandoned with Back, which must not turn the tab's next provider sign-in into a connection.
 */
const NOTE_LIFETIME_MS = 10 * 60_000;

/** Notes that the browser is leaving for the provider to connect it, and when. */
export function rememberConnect(store: Store, now: number): void {
  store.setItem(NOTE, JSON.stringify({ at: now }));
}

/** Whether this return finishes a connection. Read before completing; an old or unreadable note is dropped. */
export function isConnecting(store: Store, now: number): boolean {
  const raw = store.getItem(NOTE);
  if (raw === null) return false;
  try {
    const left = (JSON.parse(raw) as { at?: number }).at;
    if (typeof left === 'number' && now - left >= 0 && now - left < NOTE_LIFETIME_MS) return true;
  } catch {
    // Unreadable is not a trip.
  }
  store.removeItem(NOTE);
  return false;
}

/** Forgets the note: the return was taken, or the tab signed out. */
export function forgetConnect(store: Store): void {
  store.removeItem(NOTE);
}
