// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The seam every component reads through, and the subscription API a framework binds to.
//
// F1's engine is **online-only and deliberately so**: a call goes straight to the Transport, and
// there is no queue, no local store and no hybrid logical clock. Those implement `:pull` and
// `:push`, which do not exist yet, and a client written against a protocol with no server is a
// client written twice (`offline-sync.md` §9 arrives in F6 with `0.8.5`).
//
// What has to be right *today* is the shape, because it is what everything else is built on: a
// component subscribes to a resource and is told about every state it can be in, and it never
// learns that a Transport exists. When F6 puts a queue behind this, no component changes.
//
// No Svelte, and nothing framework-shaped. The subscription is a function that takes a listener
// and returns an unsubscribe - the smallest thing runes, signals or a React hook can all wrap, and
// the reason this package can be exercised headlessly at all (ADR-0033 §2).

import { TransportError } from './errors.ts';
import type { Clock, RequestOptions, Transport } from './ports.ts';
import { systemClock } from './ports.ts';

/** How long a read may take before it is abandoned. A number, because "no deadline" is not one. */
export const DEFAULT_TIMEOUT_MS = 10_000;

/**
 * Every state a resource can be in, as one union rather than three booleans.
 *
 * Three booleans have eight combinations and four of them are nonsense - loading *and* failed,
 * neither loading nor loaded nor failed. A union has exactly the states that exist, and a caller
 * that forgets one does not compile.
 */
export type ResourceState<T> =
  | { readonly status: 'idle' }
  | { readonly status: 'loading' }
  | {
      readonly status: 'ready';
      readonly data: T;
      readonly at: number;
      /**
       * The `ETag` the server sent with this read, where it sent one.
       *
       * It is handed to the caller rather than applied behind their back: a write goes to a path,
       * a read came from a path, and the two are usually but not always the same one. The engine
       * attaching the tag it happens to hold would be right until the first operation where they
       * differ, and wrong silently.
       */
      readonly etag?: string;
    }
  | { readonly status: 'failed'; readonly error: TransportError };

export type Listener<T> = (state: ResourceState<T>) => void;
export type Unsubscribe = () => void;

/** What a resource is: where to read it, and how long that may take. */
export interface ResourceRequest {
  /** The path under the API base, query already encoded. */
  readonly path: string;
  readonly timeoutMs?: number;
  /**
   * The request document, for the two reads of this API that are `POST`s.
   *
   * `/items:query` and `/search` read - nothing is written and the same request may be repeated -
   * and they are `POST` deliberately: a query is a document rather than a set of parameters, and
   * what somebody is searching for is their content, which a query string would carry through
   * access logs, proxies and browser history (`security.md` §9, ADR-0018).
   *
   * Present means the read is a `POST`; absent means it is a `GET`. It changes nothing else: such
   * a resource is subscribed to, refreshed, paged and cached exactly like any other, and it
   * invalidates nothing, because it wrote nothing.
   */
  readonly body?: unknown;
}

/**
 * The envelope every paged read of this API answers with (`api-guidelines.md` §5).
 *
 * The engine knows this shape and only this one. That is not a merge rule - merging is the
 * server's (ADR-0021) - it is the contract's pagination envelope, and concatenating a page onto
 * the page before it is what "load more" means rather than a resolution of anything.
 */
interface Paged {
  readonly data: readonly unknown[];
  readonly page?: { readonly next_cursor?: string | null; readonly has_more?: boolean };
}

/**
 * stableKey turns a request into the string two callers asking the same question share.
 *
 * Keys are sorted, because `{a, b}` and `{b, a}` are one query and two `JSON.stringify` results -
 * and a board that subscribed twice to one column would load it twice and show whichever answer
 * arrived last.
 */
function stableKey(value: unknown): string {
  if (value === null || typeof value !== 'object') return JSON.stringify(value) ?? 'null';
  if (Array.isArray(value)) return `[${value.map(stableKey).join(',')}]`;
  const entries = Object.entries(value as Record<string, unknown>)
    .filter(([, v]) => v !== undefined)
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0));
  return `{${entries.map(([k, v]) => `${JSON.stringify(k)}:${stableKey(v)}`).join(',')}}`;
}

/** What a write takes beyond its method, path and body. */
export interface MutateOptions {
  readonly idempotencyKey?: string;
  readonly timeoutMs?: number;
  /** The version the caller read. A stale one is refused rather than silently overwriting. */
  readonly ifMatch?: string;
  /**
   * Which held reads this write makes stale, as path prefixes.
   *
   * Omitted means all of them, and that is the safe default rather than the lazy one: a write
   * whose effects nobody declared is a write whose effects nobody knows, and showing a stale row
   * is worse than reloading one that did not change. Naming the prefixes turns a correct-but-broad
   * default into a precise one - which is what a board of five columns needs, because a drag that
   * reordered one row must not reload the other four.
   *
   * A prefix matches a path, so `/containers` covers `/containers?cursor=…` and
   * `/containers/{id}` alike; the document half of a query key is not matched against, because a
   * write does not know which questions it changed the answer to.
   */
  readonly invalidates?: readonly string[];
}

export interface SyncEngineOptions {
  readonly transport: Transport;
  /** Injected, so a test can fix the time a `ready` state is stamped with (rule 4). */
  readonly clock?: Clock;
  /**
   * How the bearer is obtained, asked for per call rather than held.
   *
   * A function rather than a string, and that is the whole point: the token is the platform seam's
   * (F1-11), it is refreshed behind this package's back, and a copy taken at construction is a
   * copy that keeps working after a sign-out.
   */
  readonly token?: () => string | undefined;
  /**
   * Called when the server refuses the credential - any request, any resource.
   *
   * This is the only place that sees every `401`, which is why the hook is here rather than in a
   * caller: a client that noticed a rejected token on one screen and not on another would keep
   * making requests with a credential it already knows is dead. It is a callback and not a
   * policy - what happens next (clear the token, remember the path, show the sign-in screen) is
   * the application's, and this package holds no opinion about screens.
   */
  readonly onUnauthorized?: () => void;
}

/**
 * SyncEngine is the client's only door to a server.
 *
 * It **never merges**. Merging is the server's (ADR-0021, `offline-sync.md` §4): this applies what
 * the server answers and surfaces a conflict for the UI to render. A merge rule appearing in this
 * package is a bug against that decision rather than a feature, which is why there is no seam
 * here one could be added behind.
 */
export class SyncEngine {
  readonly #transport: Transport;
  readonly #clock: Clock;
  readonly #token: () => string | undefined;
  /** One entry per path, so two components asking for the same thing share one state. */
  readonly #resources = new Map<string, ResourceEntry<unknown>>();
  readonly #onUnauthorized: () => void;

  constructor(options: SyncEngineOptions) {
    this.#transport = options.transport;
    this.#clock = options.clock ?? systemClock;
    this.#token = options.token ?? (() => undefined);
    this.#onUnauthorized = options.onUnauthorized ?? (() => {});
  }

  /**
   * subscribe registers a listener for a resource and starts loading it.
   *
   * The listener is called at once with the current state - `idle` on the first subscriber,
   * whatever is already known on the second - so a caller never has to ask separately what it
   * missed. It returns the unsubscribe, and the last unsubscriber leaves the entry in place: a
   * component that unmounts and remounts finds what it had.
   */
  subscribe<T>(request: ResourceRequest, listener: Listener<T>): Unsubscribe {
    const entry = this.#entryFor<T>(request);
    entry.listeners.add(listener as Listener<unknown>);
    listener(entry.state as ResourceState<T>);

    if (entry.state.status === 'idle') void this.#load(request, entry);

    return () => {
      entry.listeners.delete(listener as Listener<unknown>);
    };
  }

  /** Reads the resource again, whatever state it is in. What a "retry" button calls. */
  async refresh<T>(request: ResourceRequest): Promise<ResourceState<T>> {
    const entry = this.#entryFor<T>(request);
    await this.#load(request, entry);
    return entry.state as ResourceState<T>;
  }

  /** The current state without subscribing. For a caller that wants one look. */
  peek<T>(request: ResourceRequest): ResourceState<T> {
    return (this.#resources.get(this.#keyFor(request))?.state ?? { status: 'idle' }) as ResourceState<T>;
  }

  /**
   * The `ETag` currently held for a plain path, for a caller that has to write against a version
   * it did not keep. A caller holding the `ready` state should read `state.etag` instead - this
   * exists so that a component which only has an id is not forced to subscribe to get a tag.
   */
  etagFor(path: string): string | undefined {
    return this.#resources.get(path)?.etag;
  }

  /**
   * mutate performs a write and returns what the server answered.
   *
   * Nothing is queued and nothing is applied optimistically: F2 is still online-only, so a write
   * either succeeds or fails in front of the person who made it. Rolling one back would be a guess
   * about a `:push` that does not exist; the queue arrives in F6 with the protocol it implements.
   *
   * `idempotencyKey` is passed through rather than minted here, because it belongs to the
   * *intent* - a retry of the same intent is the same key, and only the caller knows where one
   * intent ends. `ifMatch` is the version the caller read, and a stale one comes back as
   * `version_conflict` (ADR-0025), which `TransportError.isVersionConflict` answers.
   */
  async mutate<T>(
    method: 'POST' | 'PATCH' | 'PUT' | 'DELETE',
    path: string,
    body: unknown,
    options: MutateOptions = {},
  ): Promise<T> {
    let answer;
    try {
      answer = await this.#transport.send<T>(method, path, body, this.#options(options));
    } catch (cause) {
      this.#noticeRefusal(cause);
      throw cause;
    }
    this.#invalidate(options.invalidates);
    return answer.body;
  }

  /**
   * loadMore appends the next page of a paged resource to the one already held.
   *
   * Appending rather than replacing is the whole point: `LoadMore` is a control a person presses
   * and what they had must still be on screen afterwards. The cursor is the server's, opaque and
   * signed, and is sent back exactly as it came - never parsed, never constructed.
   *
   * A **grouped** result is not paged here, and that is the contract's design rather than a gap:
   * each group carries its own cursor and is continued by asking for that group again - its key as
   * a filter, its cursor as the cursor. That is a different question, so it is a different
   * subscription, and a board pages one column without the others noticing.
   *
   * Does nothing when there is no next page, so a caller may bind it to a button without guarding.
   */
  async loadMore<T>(request: ResourceRequest): Promise<ResourceState<T>> {
    const entry = this.#entryFor<T>(request);
    const current = entry.state;
    if (current.status !== 'ready') return current;

    const held = current.data as Paged;
    const cursor = held?.page?.next_cursor;
    if (!cursor) return current;

    try {
      const options = this.#options(request);
      // Where the cursor goes is what separates the two kinds of read: a query carries it in the
      // document it already is, a list carries it in the query string.
      const answer = request.body === undefined
        ? await this.#transport.get<T>(withCursor(request.path, cursor), options)
        : await this.#transport.send<T>('POST', request.path, nextPage(request.body, cursor), options);

      const arrived = answer.body as Paged;
      const combined = {
        ...(answer.body as object),
        data: [...(held.data ?? []), ...(arrived?.data ?? [])],
      } as T;
      entry.etag = answer.etag;
      this.#publish(entry, { status: 'ready', data: combined, at: this.#clock.now(), etag: answer.etag });
    } catch (cause) {
      // The page that failed does not take the pages that succeeded with it: the reader keeps what
      // they had and is told the next one did not arrive. Replacing the state with `failed` here
      // would empty a list because its fourth page timed out.
      const error = cause instanceof TransportError ? cause : new TransportError('malformed', { cause });
      this.#noticeRefusal(error);
      throw error;
    }
    return entry.state;
  }

  /** Forgets everything held in memory. Sign-out (`offline-sync.md` §9.6). */
  reset(): void {
    for (const entry of this.#resources.values()) entry.listeners.clear();
    this.#resources.clear();
  }

  /**
   * The key a request is held under: the path, plus the document where there is one.
   *
   * Two components asking the same question share one entry and therefore one load. Two asking
   * different questions of the same path - two columns of a board, both `POST /items:query` - do
   * not, which is the whole reason the key is not the path alone.
   */
  #keyFor(request: ResourceRequest): string {
    return request.body === undefined ? request.path : `${request.path} ${stableKey(request.body)}`;
  }

  #entryFor<T>(request: ResourceRequest): ResourceEntry<T> {
    const key = this.#keyFor(request);
    let entry = this.#resources.get(key) as ResourceEntry<T> | undefined;
    if (!entry) {
      entry = { key, path: request.path, state: { status: 'idle' }, listeners: new Set() };
      this.#resources.set(key, entry as ResourceEntry<unknown>);
    }
    return entry;
  }

  async #load<T>(request: ResourceRequest, entry: ResourceEntry<T>): Promise<void> {
    this.#publish(entry, { status: 'loading' });
    try {
      const options = this.#options(request);
      // A document makes it a `POST`, and nothing else about it changes. It still reads: no cache
      // is dropped here, because a read that invalidated would make a board reload itself.
      const answer = request.body === undefined
        ? await this.#transport.get<T>(request.path, options)
        : await this.#transport.send<T>('POST', request.path, request.body, options);
      entry.etag = answer.etag;
      this.#publish(entry, {
        status: 'ready',
        data: answer.body,
        at: this.#clock.now(),
        etag: answer.etag,
      });
    } catch (cause) {
      // Everything that reaches a listener is a TransportError, so a caller has one shape to
      // render. A failure that escaped as something else would be a failure the UI cannot name.
      const error = cause instanceof TransportError
        ? cause
        : new TransportError('malformed', { cause });
      this.#publish(entry, { status: 'failed', error });
      this.#noticeRefusal(error);
    }
  }

  /** A refused credential, told once to whoever asked to be told. */
  #noticeRefusal(cause: unknown): void {
    if (cause instanceof TransportError && cause.status === 401) this.#onUnauthorized();
  }

  #publish<T>(entry: ResourceEntry<T>, state: ResourceState<T>): void {
    entry.state = state;
    for (const listener of entry.listeners) listener(state);
  }

  #options(given: { idempotencyKey?: string; timeoutMs?: number; ifMatch?: string }): RequestOptions {
    return {
      token: this.#token(),
      idempotencyKey: given.idempotencyKey,
      ifMatch: given.ifMatch,
      timeoutMs: given.timeoutMs ?? DEFAULT_TIMEOUT_MS,
    };
  }

  /**
   * Drops what a write made stale. Everything, when the write did not say.
   *
   * The entries are removed rather than reloaded: a subscriber is told nothing here, and the next
   * one to ask loads afresh. Reloading every dropped entry on the spot would turn one write into a
   * burst of requests for screens nobody is looking at.
   */
  #invalidate(prefixes: readonly string[] | undefined): void {
    if (prefixes === undefined) {
      this.#resources.clear();
      return;
    }
    for (const [key, entry] of this.#resources) {
      if (prefixes.some((prefix) => entry.path.startsWith(prefix))) this.#resources.delete(key);
    }
  }
}

/**
 * Puts the cursor in the query string of a `GET`, replacing one that is already there.
 *
 * A `URL` needs an absolute one to parse, so a base is supplied and then discarded - the path this
 * package deals in is relative to the API root by design (ADR-0028), and building it by hand is
 * how a path with an existing query acquires a second `?`.
 */
function withCursor(path: string, cursor: string): string {
  const url = new URL(path, 'http://localhost');
  url.searchParams.set('cursor', cursor);
  return `${url.pathname}${url.search}`;
}

/** Puts the cursor in the `page` of a query document, leaving the rest of the question alone. */
function nextPage(body: unknown, cursor: string): unknown {
  const document = (body ?? {}) as Record<string, unknown>;
  const page = (document.page ?? {}) as Record<string, unknown>;
  return { ...document, page: { ...page, cursor } };
}

interface ResourceEntry<T> {
  /** What the map holds it under: the path, plus the document where there is one. */
  readonly key: string;
  /** The path alone, which is what invalidation matches against. */
  readonly path: string;
  state: ResourceState<T>;
  listeners: Set<Listener<T>>;
  /** The tag the last successful read carried, so a write can state the version it saw. */
  etag?: string;
}
