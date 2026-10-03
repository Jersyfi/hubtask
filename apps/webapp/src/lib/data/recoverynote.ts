// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The note a recovery code leaves (UC-ID-02 check 6, SC-18), as it is kept between pages.
 *
 * In the tab's own storage: the number arrived with this one sign-in and nothing after it answers it
 * again, so a reload must not lose it - and a tab is where this sign-in lives. Read rather than
 * taken: it stays until the person closes it or replaces the authenticator, which is when it stops
 * being true.
 */

type Store = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;

const KEY = 'hubtask.recovery_note';

export interface RecoveryNote {
  /** How many recovery codes are left after the one this sign-in spent. */
  readonly left: number;
}

export function holdNote(store: Store, left: number): void {
  store.setItem(KEY, JSON.stringify({ left }));
}

export function readNote(store: Store): RecoveryNote | undefined {
  const raw = store.getItem(KEY);
  if (raw === null) return undefined;
  try {
    const left = (JSON.parse(raw) as { left?: unknown }).left;
    if (typeof left === 'number' && Number.isInteger(left) && left >= 0) return { left };
  } catch {
    // Unreadable is no note.
  }
  return undefined;
}

export function dropNote(store: Store): void {
  store.removeItem(KEY);
}
