// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A live document's local changes the server has not had yet, kept on the device so a tab closed
// or reloaded offline loses no word (offline-sync.md §13). They sit in the account's own store,
// under a collection of their own, so sign-out deletes them with the copy (§9.6); the browser
// store is not encrypted, by decision (ADR-0033 §4). And while any open document holds such
// changes, leaving the page asks first.

import type { UpdateKeeper } from '@hubtask/editor';
import { LIVE_UNSENT as COLLECTION, type Storage } from '@hubtask/sync-engine';

// keeperIn keeps one document's unsent updates in the store; an empty list removes the record.
export function keeperIn(storage: Storage, kind: string, id: string): UpdateKeeper {
  const key = `${kind}:${id}`;
  return {
    load: async () => (await storage.get<{ updates: Uint8Array[] }>(COLLECTION, key))?.updates ?? [],
    save: (updates) =>
      updates.length === 0 ? storage.delete(COLLECTION, key) : storage.put(COLLECTION, key, { id: key, updates: [...updates] }),
  };
}

// UnsentDocuments counts the changes waiting across the open documents.
export class UnsentDocuments {
  readonly #counts = new Map<string, number>();

  get waiting(): number {
    let total = 0;
    for (const count of this.#counts.values()) total += count;
    return total;
  }

  set(key: string, count: number): void {
    this.#counts.set(key, count);
  }

  forget(key: string): void {
    this.#counts.delete(key);
  }
}

export const unsentDocuments = new UnsentDocuments();

if (typeof window !== 'undefined') {
  window.addEventListener('beforeunload', (event) => {
    if (unsentDocuments.waiting === 0) return;
    // The browser asks in its own words; a page may no longer choose them.
    event.preventDefault();
  });
}
