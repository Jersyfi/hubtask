// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The store in the browser: IndexedDB, one database per API origin and account (ADR-0033 §4).
//
// One object store rather than one per collection, keyed on `[collection, id]` with an index on
// the collection. A store per collection would fix the set of collections at the database's
// version, and every entity a later server sends - a reminder, a template, one this build has
// never heard of - would need a version bump and an upgrade transaction on every device before it
// could be held; rule 7 of offline-sync.md §9 says such a record is stored as it came. A compound
// key holds any collection the engine names, today's and next year's, under one version.
//
// **No encryption here, by decision.** ADR-0033 §4: "no browser-side encryption theatre" - a key
// held by the page is a key the page's origin can read, and what must not sit unencrypted on a
// shared machine does not go into browser storage at all. The promise lives in the shells, whose
// keystore is the platform's (ADR-0031). What the browser promises instead is §9.6's other half:
// `clear()` deletes the database - not its rows - so sign-out leaves nothing behind that a
// version bump or a stray transaction could bring back.
//
// Every request is wrapped once, so that a failure is a rejected promise with the engine's shape
// rather than an event nobody listened to. A store that cannot be opened - a private window that
// refuses it, a quota, a policy - rejects, and the engine treats a rejected store as no store.

import type { Storage } from '../ports.ts';

const STORE = 'records';
const BY_COLLECTION = 'collection';
const VERSION = 1;

interface Row {
  readonly collection: string;
  readonly id: string;
  readonly value: unknown;
}

/** One IndexedDB request as a promise. The whole of the API's event shape, in one place. */
function settled<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error ?? new Error('indexeddb: the request failed'));
  });
}

/**
 * A request that could not be issued because the transaction it was issued on is no longer
 * active - finished between its last request and its `complete` event (issue 776), or aborted
 * meanwhile. The remedy is the same for both: a fresh transaction.
 */
function isInactive(cause: unknown): boolean {
  const name = (cause as { name?: unknown } | null)?.name;
  return name === 'TransactionInactiveError' || name === 'InvalidStateError';
}

/** The name a database is opened under: the origin and the account, and nothing that could collide. */
export function databaseNameFor(apiOrigin: string, accountId: string): string {
  return `hubtask:${apiOrigin}:${accountId}`;
}

export class IndexedDbStorage implements Storage {
  readonly #name: string;
  readonly #factory: IDBFactory;
  #database: Promise<IDBDatabase> | undefined;
  #writing: IDBTransaction | undefined;

  constructor(name: string, factory: IDBFactory | undefined = globalThis.indexedDB) {
    if (!factory) throw new TypeError('indexeddb: this runtime has no IndexedDB');
    this.#name = name;
    this.#factory = factory;
  }

  #open(): Promise<IDBDatabase> {
    this.#database ??= new Promise<IDBDatabase>((resolve, reject) => {
      const request = this.#factory.open(this.#name, VERSION);
      request.onupgradeneeded = () => {
        const database = request.result;
        if (!database.objectStoreNames.contains(STORE)) {
          const store = database.createObjectStore(STORE, { keyPath: ['collection', 'id'] });
          store.createIndex(BY_COLLECTION, 'collection', { unique: false });
        }
      };
      request.onsuccess = () => {
        const database = request.result;
        // Another tab deleting the database - a sign-out there - closes this one's handle, so the
        // next call opens again rather than failing on a closed connection.
        database.onversionchange = () => {
          database.close();
          this.#database = undefined;
        };
        resolve(database);
      };
      request.onerror = () => reject(request.error ?? new Error('indexeddb: open failed'));
      request.onblocked = () => reject(new Error('indexeddb: open blocked by another connection'));
    }).catch((cause) => {
      this.#database = undefined;
      throw cause;
    });
    return this.#database;
  }

  async #store(mode: IDBTransactionMode): Promise<IDBObjectStore> {
    const database = await this.#open();
    return database.transaction(STORE, mode).objectStore(STORE);
  }

  /**
   * The one writing transaction, reused while it is alive. A snapshot is tens of thousands of
   * records applied one after another, and a transaction per record - each committed to disk on
   * its own - is what made a 67,000-entry workspace take half a minute in Firefox. A transaction
   * stays alive as long as a request is issued from a request's own completion, which is how
   * the replica writes: one put resolves, the next is issued in its continuation. When nothing
   * follows it, the transaction commits on its own and the next write opens a fresh one.
   *
   * The request is issued here, not by the caller, because a finished transaction is only found
   * out at that moment: `objectStore()` still answers between the last request's completion and
   * the `complete` event, and it is the request that throws (issue 776). Then the held transaction is
   * dropped and the request goes on a fresh one - the reuse is only ever for a request issued
   * from a continuation, and that one never lands in the window.
   */
  async #write<T>(issue: (store: IDBObjectStore) => IDBRequest<T>): Promise<T> {
    const reused = this.#reuse(issue);
    if (reused) return reused;
    const database = await this.#open();
    const transaction = database.transaction(STORE, 'readwrite');
    const done = () => {
      if (this.#writing === transaction) this.#writing = undefined;
    };
    transaction.oncomplete = done;
    transaction.onabort = done;
    transaction.onerror = done;
    this.#writing = transaction;
    return settled(issue(transaction.objectStore(STORE)));
  }

  /**
   * A read: on the writing transaction while one is alive - a read on the same transaction keeps
   * it alive for the write that follows it, which is the replica's whole pattern (read the copy,
   * write it back) - and on a fresh readonly one otherwise.
   */
  async #read<T>(issue: (store: IDBObjectStore) => IDBRequest<T>): Promise<T> {
    const reused = this.#reuse(issue);
    if (reused) return reused;
    return settled(issue(await this.#store('readonly')));
  }

  /** Issues the request on the held writing transaction; nothing when there is none or it is gone. */
  #reuse<T>(issue: (store: IDBObjectStore) => IDBRequest<T>): Promise<T> | undefined {
    if (!this.#writing) return undefined;
    try {
      return settled(issue(this.#writing.objectStore(STORE)));
    } catch (cause) {
      if (!isInactive(cause)) throw cause;
      this.#writing = undefined;
      return undefined;
    }
  }

  async get<T>(collection: string, id: string): Promise<T | undefined> {
    const row = await this.#read((store) => store.get([collection, id]));
    return (row as Row | undefined)?.value as T | undefined;
  }

  async put<T>(collection: string, id: string, value: T): Promise<void> {
    const row: Row = { collection, id, value };
    await this.#write((store) => store.put(row));
  }

  async delete(collection: string, id: string): Promise<void> {
    await this.#write((store) => store.delete([collection, id]));
  }

  async all<T>(collection: string): Promise<readonly T[]> {
    const rows = await this.#read((store) => store.index(BY_COLLECTION).getAll(collection));
    return (rows as Row[]).map((row) => row.value as T);
  }

  async collections(): Promise<readonly string[]> {
    const store = await this.#store('readonly');
    const names: string[] = [];
    await new Promise<void>((resolve, reject) => {
      const request = store.index(BY_COLLECTION).openKeyCursor(null, 'nextunique');
      request.onsuccess = () => {
        const cursor = request.result;
        if (!cursor) return resolve();
        names.push(String(cursor.key));
        cursor.continue();
      };
      request.onerror = () => reject(request.error ?? new Error('indexeddb: the cursor failed'));
    });
    return names;
  }

  /** Deletes the database, not its rows (offline-sync.md §9.6). The next call opens a fresh one. */
  async clear(): Promise<void> {
    const open = this.#database;
    this.#database = undefined;
    this.#writing = undefined;
    if (open) {
      try {
        (await open).close();
      } catch {
        // A database that never opened has nothing to close.
      }
    }
    await new Promise<void>((resolve, reject) => {
      const request = this.#factory.deleteDatabase(this.#name);
      request.onsuccess = () => resolve();
      request.onerror = () => reject(request.error ?? new Error('indexeddb: delete failed'));
      // Blocked means another connection is still open; the deletion happens once it closes,
      // and this promise need not wait for a tab that may be gone.
      request.onblocked = () => resolve();
    });
  }
}
