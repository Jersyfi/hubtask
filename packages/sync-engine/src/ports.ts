// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The three ports of ADR-0033 §2, and nothing else.
//
// They are narrow on purpose. A port is what the engine needs, not what an implementation offers:
// `Storage` says "put, get, delete, clear" rather than exposing IndexedDB's cursors, because the
// SQLite implementation the shells bring (ADR-0031) has none, and a port shaped by its first
// implementation is a port that has to be rewritten by its second.
//
// What is *not* here is as decided as what is. There is no `merge`, and there never will be:
// merging is the server's (ADR-0021, offline-sync.md §4), and a merge rule in this package is a
// bug against that decision rather than a feature. The engine queues, pushes, applies what the
// server answers, and surfaces the conflict for the UI to render.

/** How a request identifies itself to the server, and how long it may take. */
export interface RequestOptions {
  /**
   * The bearer the platform seam holds (F1-11). It is passed per call rather than held here,
   * because a token that lives in the transport is a token that outlives a sign-out.
   */
  readonly token?: string;
  /**
   * The `Idempotency-Key` for an operation that takes one. Absent where the operation does not:
   * sending one to an endpoint that ignores it teaches a client a habit the server does not keep.
   */
  readonly idempotencyKey?: string;
  /**
   * How long the call may take, in milliseconds. **Required, with no default of "forever":** a
   * client call without a deadline is the same defect as a server one, and the failure is the same
   * - a request nobody is waiting for any more, still holding a connection.
   */
  readonly timeoutMs: number;
  /** Lets a caller abandon the call for its own reasons, on top of the deadline. */
  readonly signal?: AbortSignal;
}

/** What came back: the status, the parsed body, and the entity tag if the server sent one. */
export interface Response<T> {
  readonly status: number;
  readonly body: T;
  readonly etag?: string;
}

/**
 * Transport is the seam between the engine and a server. `@hubtask/api-client` supplies the types
 * it is parameterised with; an in-memory fake supplies it in tests, which is what makes the engine
 * exercisable headlessly - the hard requirement ADR-0033 places on it, and the first-party
 * counterpart to `hubctl sync-conformance`.
 */
export interface Transport {
  /** A read. The path is relative to the API base; the query is already encoded into it. */
  get<T>(path: string, options: RequestOptions): Promise<Response<T>>;
  /** A write. `method` is the verb the contract declares for the operation. */
  send<T>(
    method: 'POST' | 'PATCH' | 'PUT' | 'DELETE',
    path: string,
    body: unknown,
    options: RequestOptions,
  ): Promise<Response<T>>;
}

/**
 * Storage is the local store. Typed as a key-value store over the record shapes in `schema.ts`,
 * because that is the intersection of IndexedDB and SQLite - and the intersection is the honest
 * port when two implementations are already known.
 *
 * F1 ships no implementation. The engine is online-only until F6 brings the protocol
 * (`offline-sync.md` §9), and a store with nothing to put in it would be a guess about a shape the
 * `:pull` contract has not yet had to satisfy.
 */
export interface Storage {
  get<T>(collection: string, id: string): Promise<T | undefined>;
  put<T>(collection: string, id: string, value: T): Promise<void>;
  delete(collection: string, id: string): Promise<void>;
  /** Everything in one collection. The engine reads whole collections, never ranges. */
  all<T>(collection: string): Promise<readonly T[]>;
  /**
   * Removes everything. Sign-out deletes the store completely on every platform
   * (`offline-sync.md` §9.6, ADR-0033 §4), so this is a promise rather than a convenience.
   */
  clear(): Promise<void>;
}

/**
 * Clock is the port rule 4 exists for, on this side of the wire too. A test that cannot fix the
 * time is a test that will flake, and a deadline is exactly the kind of thing that flakes.
 */
export interface Clock {
  /** Milliseconds since the epoch. */
  now(): number;
}

/** The clock production uses. The only place in this package that reads the machine's time. */
export const systemClock: Clock = { now: () => Date.now() };
