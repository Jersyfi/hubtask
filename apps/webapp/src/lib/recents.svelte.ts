// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What this device opened last (ADR-0063 decision 1).
 *
 * **It is the device's, and it is sent nowhere.** There is no server record of what somebody
 * looked at and this does not invent one: "what I was working on" is a property of the screen in
 * front of them, the way the fold and the theme are (ADR-0043), and a second person on a second
 * device is not owed a list from the first.
 *
 * **It holds titles, so it is the account's and it ends with the session.** A title is user
 * content, and `localStorage` outlives a sign-out where the replica does not
 * (`offline-sync.md` §9.6). Two things follow, and both are load-bearing: the key carries the
 * account's id, so two people sharing a browser never read each other's list, and `forget()` runs
 * on the way out with the rest of what the session leaves behind. Storing only ids and resolving
 * the titles later would have been the tidier answer and a useless one — nothing in this client
 * holds an entry it is not currently showing, so every row would resolve to nothing.
 *
 * Nothing is remembered about an entry beyond what a row draws: an id to navigate to, a kind to
 * pick the address, and the title as it read when it was opened. The rules about what the list may
 * hold are in `recents.ts`, pure and tested; this half holds the state and the storage.
 */

import { isUnchanged, keyFor, parse, promote, type Recent } from './recents.ts';

export type { Recent, RecentKind } from './recents.ts';

/** `localStorage` where it can be reached; nothing where it cannot (a sandbox, a test). */
function storage(): Storage | undefined {
  try {
    return globalThis.localStorage;
  } catch {
    return undefined;
  }
}

class Recents {
  #rows = $state<readonly Recent[]>([]);
  /** Whose list this is. Nothing is read or written before the account is known. */
  #accountId: string | undefined;

  get rows(): readonly Recent[] {
    return this.#rows;
  }

  /**
   * Adopts one account's list, and drops the one before it.
   *
   * Called with the account rather than with the session, because the id is the key: a frame that
   * started this at sign-in would have to guess whose list it was reading.
   */
  adopt(accountId: string | undefined): void {
    if (accountId === this.#accountId) return;
    this.#accountId = accountId;
    this.#rows = accountId ? parse(storage()?.getItem(keyFor(accountId)) ?? null) : [];
  }

  /** Notes one as the most recently opened. Nothing happens before an account has been adopted. */
  note(row: Recent): void {
    const accountId = this.#accountId;
    // Unchanged is not written: a screen notes what it is showing on every render of it, and
    // rewriting the same list each time would be a write per keystroke for no reader.
    if (!accountId || isUnchanged(this.#rows, row)) return;
    this.#rows = promote(this.#rows, row);
    try {
      storage()?.setItem(keyFor(accountId), JSON.stringify(this.#rows));
    } catch {
      // A browser that refuses storage still gets the list for as long as the page is open.
    }
  }

  /** Ends it, with the session. Called from the one place that discards what a session left. */
  forget(): void {
    const accountId = this.#accountId;
    this.#rows = [];
    this.#accountId = undefined;
    if (!accountId) return;
    try {
      storage()?.removeItem(keyFor(accountId));
    } catch {
      // Nothing to remove from, which is the same outcome.
    }
  }
}

export const recents = new Recents();
