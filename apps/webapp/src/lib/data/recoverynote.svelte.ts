// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The note a sign-in with a recovery code leaves on the pages after it (UC-ID-02 check 6, SC-18).
 *
 * Held in the tab (`recoverynote.ts` says why), shown at the top of the content area until the
 * person closes it or replaces the authenticator, and gone at sign-out with the session it is about.
 */

import { dropNote, holdNote, readNote, type RecoveryNote } from './recoverynote.ts';

/** The tab's own storage, or a map where a browser refuses it. */
function tabStore(): Pick<Storage, 'getItem' | 'setItem' | 'removeItem'> {
  try {
    if (typeof sessionStorage !== 'undefined') return sessionStorage;
  } catch {
    // Refused: fall through to memory.
  }
  const held = new Map<string, string>();
  return {
    getItem: (key) => held.get(key) ?? null,
    setItem: (key, value) => void held.set(key, value),
    removeItem: (key) => void held.delete(key),
  };
}

const store = tabStore();

class Note {
  #note = $state<RecoveryNote | undefined>(readNote(store));

  /** The note, while it stands. */
  get note(): RecoveryNote | undefined {
    return this.#note;
  }

  /** A sign-in spent a recovery code: this many are left. */
  hold(left: number): void {
    holdNote(store, left);
    this.#note = { left };
  }

  /** Closed by the person, the authenticator replaced, or the session ended. */
  close(): void {
    dropNote(store);
    this.#note = undefined;
  }
}

export const recoveryNote = new Note();
