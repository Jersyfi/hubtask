// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The slice of IndexedDB `IndexedDbStorage` uses, in memory, so that the shared storage test can
// hold the browser store to the same promises as the memory store where the tests run - Node has
// no IndexedDB. What it fakes is the request-and-event shape: open with an upgrade, a transaction
// over one object store with a compound key path, get/put/delete/getAll, an index with a unique
// key cursor, and deleteDatabase. What a real engine does with the same calls is the browser
// job's question (apps/webapp/e2e), which opens the real store in three engines.

type Listener = (() => void) | null;

class FakeRequest<T> {
  result!: T;
  error: Error | null = null;
  onsuccess: Listener = null;
  onerror: Listener = null;
  onupgradeneeded: Listener = null;
  onblocked: Listener = null;
  readonly #transaction: FakeTransaction | undefined;

  constructor(transaction?: FakeTransaction) {
    this.#transaction = transaction;
  }

  succeed(result: T): void {
    this.result = result;
    this.#transaction?.issued();
    queueMicrotask(() => {
      this.#transaction?.delivering();
      this.onsuccess?.();
      this.#transaction?.delivered();
    });
  }
}

/**
 * The part of a transaction's life the store depends on. A transaction is active in the task
 * that created it and in each request's success continuation - microtasks included, which is how
 * one `await` chains a thousand puts on one transaction - and inactive once such a task has
 * ended. When it goes inactive with no request outstanding it is finished: a request issued on it
 * throws `TransactionInactiveError`, `objectStore()` still answers, and `oncomplete` fires a
 * task later. That last window is what the store must survive (issue 776).
 */
class FakeTransaction {
  #active = true;
  #outstanding = 0;
  #finished = false;
  oncomplete: Listener = null;
  onabort: Listener = null;
  onerror: Listener = null;
  readonly #database: FakeDatabase;

  constructor(database: FakeDatabase) {
    this.#database = database;
    this.#deactivateLater();
  }

  #deactivateLater(): void {
    setTimeout(() => {
      this.#active = false;
      if (this.#outstanding === 0 && !this.#finished) {
        this.#finished = true;
        setTimeout(() => this.oncomplete?.(), 0);
      }
    }, 0);
  }

  issued(): void {
    if (!this.#active) {
      throw new DOMException('the transaction is not active', 'TransactionInactiveError');
    }
    this.#outstanding += 1;
  }

  delivering(): void {
    this.#outstanding -= 1;
    this.#active = true;
  }

  delivered(): void {
    this.#deactivateLater();
  }

  objectStore(name: string): FakeObjectStore {
    const held = this.#database.stores.get(name);
    if (!held) throw new Error(`no store ${name}`);
    return new FakeObjectStore(held.rows, held.keyPath, held.indexes, this);
  }
}

/** A compound key as a string, so a Map can hold it. */
const keyOf = (key: unknown): string => JSON.stringify(key);

class FakeIndex {
  readonly #rows: Map<string, Record<string, unknown>>;
  readonly #field: string;
  readonly #transaction: FakeTransaction | undefined;

  constructor(rows: Map<string, Record<string, unknown>>, field: string, transaction: FakeTransaction | undefined) {
    this.#rows = rows;
    this.#field = field;
    this.#transaction = transaction;
  }

  getAll(value: unknown): FakeRequest<unknown[]> {
    const request = new FakeRequest<unknown[]>(this.#transaction);
    request.succeed([...this.#rows.values()].filter((row) => row[this.#field] === value).map((row) => structuredClone(row)));
    return request;
  }

  openKeyCursor(_range: null, direction: string): FakeRequest<{ key: unknown; continue(): void } | null> {
    const request = new FakeRequest<{ key: unknown; continue(): void } | null>(this.#transaction);
    const keys = [...new Set([...this.#rows.values()].map((row) => row[this.#field]))].sort();
    if (direction !== 'nextunique') throw new Error('the fake supports nextunique only');
    let at = 0;
    const step = () => {
      if (at >= keys.length) {
        request.succeed(null);
        return;
      }
      const key = keys[at++];
      request.succeed({ key, continue: step });
    };
    step();
    return request;
  }
}

class FakeObjectStore {
  readonly #rows: Map<string, Record<string, unknown>>;
  readonly #keyPath: string[];
  readonly #indexes: Map<string, string>;
  readonly #transaction: FakeTransaction | undefined;

  constructor(
    rows: Map<string, Record<string, unknown>>,
    keyPath: string[],
    indexes: Map<string, string>,
    transaction: FakeTransaction | undefined,
  ) {
    this.#rows = rows;
    this.#keyPath = keyPath;
    this.#indexes = indexes;
    this.#transaction = transaction;
  }

  createIndex(name: string, field: string): void {
    this.#indexes.set(name, field);
  }

  index(name: string): FakeIndex {
    const field = this.#indexes.get(name);
    if (!field) throw new Error(`no index ${name}`);
    return new FakeIndex(this.#rows, field, this.#transaction);
  }

  get(key: unknown): FakeRequest<unknown> {
    const request = new FakeRequest<unknown>(this.#transaction);
    // A fresh object per read, as the real store deserialises one.
    const held = this.#rows.get(keyOf(key));
    request.succeed(held === undefined ? undefined : structuredClone(held));
    return request;
  }

  put(row: Record<string, unknown>): FakeRequest<unknown> {
    const key = this.#keyPath.map((part) => row[part]);
    // Structured clone, as the real store serialises: a caller mutating what it put must not
    // mutate what is held.
    const request = new FakeRequest<unknown>(this.#transaction);
    // Issued first: on an inactive transaction the request throws, and nothing is written.
    request.succeed(key);
    this.#rows.set(keyOf(key), structuredClone(row));
    return request;
  }

  delete(key: unknown): FakeRequest<undefined> {
    const request = new FakeRequest<undefined>(this.#transaction);
    request.succeed(undefined);
    this.#rows.delete(keyOf(key));
    return request;
  }
}

class FakeDatabase {
  readonly objectStoreNames: { contains(name: string): boolean };
  onversionchange: Listener = null;
  closed = false;
  readonly stores = new Map<string, { rows: Map<string, Record<string, unknown>>; keyPath: string[]; indexes: Map<string, string> }>();

  constructor() {
    this.objectStoreNames = { contains: (name) => this.stores.has(name) };
  }

  createObjectStore(name: string, options: { keyPath: string[] }): FakeObjectStore {
    const store = { rows: new Map(), keyPath: options.keyPath, indexes: new Map<string, string>() };
    this.stores.set(name, store);
    return new FakeObjectStore(store.rows, store.keyPath, store.indexes, undefined);
  }

  transaction(_name: string, _mode: string): FakeTransaction {
    if (this.closed) throw new DOMException('the database is closed', 'InvalidStateError');
    return new FakeTransaction(this);
  }

  close(): void {
    this.closed = true;
  }
}

/** The factory: databases by name, deleted whole on request. */
export class FakeIndexedDb {
  readonly databases = new Map<string, FakeDatabase>();

  open(name: string, _version: number): FakeRequest<FakeDatabase> {
    const request = new FakeRequest<FakeDatabase>();
    let database = this.databases.get(name);
    const fresh = !database;
    if (!database) {
      database = new FakeDatabase();
      this.databases.set(name, database);
    }
    request.result = database;
    queueMicrotask(() => {
      if (fresh) request.onupgradeneeded?.();
      request.onsuccess?.();
    });
    return request;
  }

  deleteDatabase(name: string): FakeRequest<undefined> {
    this.databases.delete(name);
    const request = new FakeRequest<undefined>();
    request.succeed(undefined);
    return request;
  }
}

/** The fake as the type the store takes. */
export function fakeIndexedDb(): { factory: IDBFactory; fake: FakeIndexedDb } {
  const fake = new FakeIndexedDb();
  return { factory: fake as unknown as IDBFactory, fake };
}
