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
import type { ByteTransfer, Clock, RequestOptions, Transport } from './ports.ts';
import { systemClock } from './ports.ts';
import type { ChangeRecord } from './schema.ts';

/** How long a read may take before it is abandoned. A number, because "no deadline" is not one. */
export const DEFAULT_TIMEOUT_MS = 10_000;

/** How long the stream may take to answer with its headers. The body is meant never to end. */
export const DEFAULT_CONNECT_TIMEOUT_MS = 15_000;
/**
 * How long the stream may stay silent before the connection is treated as dead.
 *
 * Comfortably longer than the server's heartbeat, so an idle workspace is not a reconnect loop,
 * and short enough that a proxy which dropped the connection without saying so is noticed in under
 * a minute rather than never.
 */
export const DEFAULT_IDLE_TIMEOUT_MS = 45_000;
/** The first wait after a failed connection, doubling up to the ceiling. */
export const RECONNECT_BASE_MS = 1_000;
/** The longest the engine waits between attempts. A tab left open overnight still comes back. */
export const RECONNECT_MAX_MS = 30_000;

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
   * The proof a privileged action demanded (H-03), consumed by the one action it is presented to.
   *
   * Per call and never held: a second privileged action needs a second proof, and an engine that
   * kept one would be an engine quietly answering the second demand with the first answer.
   */
  readonly stepUpToken?: string;
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

/**
 * How the change stream is listened to, and what a record means to this application.
 *
 * `pathsFor` is the whole reason this is a parameter rather than a table inside the engine: the
 * engine must not learn what a hub is (ADR-0033 §2). It is handed a record and answers with the
 * path prefixes that record makes stale — `/items/{id}` for an entry, the container's subtree for
 * a container, nothing at all for an entity this client does not read.
 */
export interface ListenOptions {
  /** What a record makes stale, as path prefixes. Empty means "this record changes nothing here". */
  readonly pathsFor: (record: ChangeRecord) => readonly string[];
  /** The stream's path. The contract's is `/stream`, and there is no reason to name another. */
  readonly path?: string;
  readonly connectTimeoutMs?: number;
  readonly idleTimeoutMs?: number;
  /**
   * Every record, before it is acted on. For a client that wants to show "somebody changed this"
   * rather than only re-read it. It is a notification and not a hook: what it returns is ignored,
   * because a listener that could veto an invalidation would be a merge rule in another shape.
   */
  readonly onRecord?: (record: ChangeRecord) => void;
  /**
   * How the engine waits between attempts, injected so a test does not spend the wait.
   *
   * The default is a timer that ends early when the listener is stopped — a tab closing must not
   * be held open by a thirty-second backoff nobody is waiting for any more.
   */
  readonly wait?: (ms: number) => Promise<void>;
}

/**
 * A refused credential, as this package recognises one: `401`, and nothing else. A `403` is a
 * permission and not a session - refreshing on one would be a client asking for a better answer
 * to a question it already got right.
 */
function isRefusal(cause: unknown): boolean {
  return cause instanceof TransportError && cause.status === 401;
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
  /**
   * Called when a request meets a `401`, to exchange the refresh token for the next pair (F4-03).
   *
   * `true` means a new credential is held and the request is retried **once**; `false` means the
   * session is over and `onUnauthorized` follows. The exchange itself is the application's - this
   * package knows there is one and never what it presents - and it happens here rather than in
   * every store for a reason that is not tidiness: presenting a retired refresh token is theft as
   * far as the server is concerned and costs the whole family (`security.md` §5, T-01), so ten
   * concurrent requests meeting one expired access token have to share **one** exchange. That is
   * what the single-flight below is for.
   */
  readonly onRefresh?: () => Promise<boolean>;
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
  readonly #onRefresh: () => Promise<boolean>;
  /** The exchange in flight, so that concurrent refusals share one rather than racing. */
  #renewal: Promise<boolean> | undefined;

  constructor(options: SyncEngineOptions) {
    this.#transport = options.transport;
    this.#clock = options.clock ?? systemClock;
    this.#token = options.token ?? (() => undefined);
    this.#onUnauthorized = options.onUnauthorized ?? (() => {});
    // No refresher is the shape F1 shipped: a `401` ends the session at once, which is what a
    // client holding a credential somebody typed can honestly do.
    this.#onRefresh = options.onRefresh ?? (async () => false);
  }

  /**
   * Runs a call, and on a refused credential exchanges the pair once and runs it again.
   *
   * Once, and only once. A second `401` after a fresh access token is not an expiry - it is the
   * account, the scope or the session itself - and retrying past it would be a client arguing with
   * a server that has already answered twice.
   */
  async #attempt<T>(call: () => Promise<T>): Promise<T> {
    try {
      return await call();
    } catch (cause) {
      if (!isRefusal(cause)) throw cause;
      if (!(await this.#renew())) {
        this.#onUnauthorized();
        throw cause;
      }
      try {
        return await call();
      } catch (again) {
        this.#noticeRefusal(again);
        throw again;
      }
    }
  }

  /**
   * The exchange, at most one at a time.
   *
   * The promise is published before it is awaited and cleared when it settles, so every caller
   * that arrives while one is running joins it. Without this, a screen making six reads on a
   * stale access token would present the same refresh token six times - and the server would read
   * the second presentation as a stolen one and end every session the account has.
   */
  #renew(): Promise<boolean> {
    this.#renewal ??= this.#onRefresh().finally(() => {
      this.#renewal = undefined;
    });
    return this.#renewal;
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
    const answer = await this.#attempt(
      () => this.#transport.send<T>(method, path, body, this.#options(options)),
    );
    this.#invalidate(options.invalidates);
    return answer.body;
  }

  /**
   * transfer sends bytes to a URL the server handed over, and invalidates what they changed.
   *
   * It is a pass-through and deliberately little else. The engine has nothing to add to a byte
   * transfer - there is no cursor in it, no entity to cache, no version to carry - and it is here
   * only so that an application holds one seam rather than two: `apps/webapp` constructs the
   * transport once, in one file, and everything after that goes through the engine.
   *
   * **No bearer, and no `invalidates` by default.** The URL is its own credential (`ByteTransfer`),
   * and putting bytes in a bucket changes nothing the client is holding - the object becomes usable
   * at confirmation, which is an ordinary `mutate`. A caller that does want a re-read after the
   * bytes may name the prefixes.
   */
  async transfer(bytes: ByteTransfer, options: { invalidates?: readonly string[] } = {}): Promise<void> {
    await this.#transport.transfer(bytes);
    // Named prefixes only. `#invalidate(undefined)` means "everything", which is the right default
    // for a write and the wrong one here: the bytes changed nothing the client is holding.
    if (options.invalidates) this.#invalidate(options.invalidates);
  }

  /**
   * document performs a read whose answer is a file rather than data.
   *
   * A pass-through like `transfer`, and it invalidates **nothing**: an export is a read, whatever
   * its verb. It is a `POST` because what a view selects is the caller's content and a query string
   * travels through access logs — the same reason `/search` is one — and the engine already knows
   * that a `POST` can be a read.
   */
  async document(path: string, body: unknown, options: { timeoutMs?: number; idempotencyKey?: string } = {}) {
    return this.#attempt(() => this.#transport.document(path, body, this.#options(options)));
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

  /**
   * listen opens the change stream and keeps it open, re-reading what a record names.
   *
   * One connection per tab, and one for every resource the tab holds: the stream carries
   * everything the caller may read and has no subscription filter, because what a client wants to
   * see is a question about its own screen and the authorisation already answers who may see what.
   *
   * A record is **a signal to re-read, never data to apply**. Applying `payload` to local state
   * would be a merge, and merging is the server's (ADR-0021). So what a record does is exactly
   * what a write does: it invalidates prefixes, watched entries are read again and unwatched ones
   * are forgotten.
   *
   * The stream is an accelerator and not a second source of truth. Every way it can end - the
   * server closing it, a proxy dropping it, a `503`, a cursor the server will not resume from - is
   * recovered by reconnecting or by re-reading, and never by replaying anything from memory.
   *
   * Returns the stop. Calling it ends the connection and the loop; the engine keeps everything it
   * has read, because stopping the stream is not signing out.
   */
  listen(options: ListenOptions): Unsubscribe {
    const controller = new AbortController();
    void this.#listen(options, controller.signal);
    return () => controller.abort();
  }

  /**
   * The connection loop: open, read until it ends, decide what ending it was, come back.
   *
   * The four refusals are four different recoveries and that is why they are told apart here
   * rather than by a component:
   *
   * * `401` ends it. The credential is dead, the hook is told, and a loop that kept reconnecting
   *   with it would hammer a server that has already said no.
   * * `sync.cursor_too_old` means the gap is wider than the tombstone window, so a delta would be
   *   silently wrong (`offline-sync.md` §7). Everything held is dropped and the stream restarts
   *   with no cursor - a full resynchronisation is the only safe answer.
   * * `sync.cursor_invalid` is a cursor this installation never minted. Start again without one;
   *   nothing held is known to be wrong.
   * * `503` carries `Retry-After`, and it is a number the server chose. Waiting less would be
   *   hammering a server that is already shedding load.
   */
  async #listen(options: ListenOptions, signal: AbortSignal): Promise<void> {
    const wait = options.wait ?? sleeper(signal);
    const path = options.path ?? '/stream';
    /** The last `id` seen. In memory for the tab's lifetime - the store that would keep it is F6's. */
    let cursor: string | undefined;
    /** The server's own reconnect suggestion, from the `retry:` field it sends on connect. */
    let suggested: number | undefined;
    let attempt = 0;

    while (!signal.aborted) {
      let pause: number;
      try {
        const connection = await this.#transport.stream(path, {
          token: this.#token(),
          lastEventId: cursor,
          connectTimeoutMs: options.connectTimeoutMs ?? DEFAULT_CONNECT_TIMEOUT_MS,
          idleTimeoutMs: options.idleTimeoutMs ?? DEFAULT_IDLE_TIMEOUT_MS,
          signal,
        });
        // The connection was accepted, so whatever went wrong before is over.
        attempt = 0;

        for await (const event of connection.events) {
          if (event.retryMs !== undefined) suggested = event.retryMs;
          // The cursor advances on the frame rather than on the record: an event this client has
          // no mapping for still moves the position, and a reconnect that asked for it again
          // would be asking for a record it has already been given.
          if (event.id) cursor = event.id;
          const record = recordOf(event.data);
          if (!record) continue;
          options.onRecord?.(record);
          // The empty list is not the absent one: `#invalidate(undefined)` means everything, and
          // an application that maps a record to no path means the opposite. The list travels as
          // it is, and a record that names nothing here invalidates nothing.
          this.#invalidate(options.pathsFor(record));
        }
        // The server closed the stream: a deployment, a drain, an idle proxy. Come back when it
        // asked to be come back to.
        pause = suggested ?? RECONNECT_BASE_MS;
      } catch (cause) {
        const error = cause instanceof TransportError ? cause : new TransportError('offline', { cause });
        if (error.status === 401) {
          // A stream outlives an access token by design - it is open for as long as the tab is.
          // So a refused connection is an expiry until the exchange says otherwise, and only then
          // is it the end of the session.
          if (await this.#renew()) continue;
          this.#onUnauthorized();
          return;
        }
        if (error.isCursorTooOld) this.#invalidate(undefined);
        if ((error.isCursorTooOld || error.isCursorInvalid) && cursor !== undefined) {
          // A refused cursor is not a busy server. Drop it and reconnect at once - and only once,
          // because the branch needs a cursor to drop and there is now none.
          cursor = undefined;
          continue;
        }
        attempt += 1;
        pause = error.retryAfterMs ?? backoff(attempt - 1);
      }

      if (signal.aborted) return;
      if (pause > 0) await wait(pause);
    }
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
      entry = {
        key,
        path: request.path,
        request,
        state: { status: 'idle' },
        listeners: new Set(),
      };
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
      const answer = await this.#attempt(() => (request.body === undefined
        ? this.#transport.get<T>(request.path, options)
        : this.#transport.send<T>('POST', request.path, request.body, options)));
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
      // The refusal itself was already reported by `#attempt`, which is the one place that sees
      // a 401 - here it only becomes a state a screen can render.
      this.#publish(entry, { status: 'failed', error });
    }
  }

  /** A refused credential, told once to whoever asked to be told. */
  #noticeRefusal(cause: unknown): void {
    if (isRefusal(cause)) this.#onUnauthorized();
  }

  #publish<T>(entry: ResourceEntry<T>, state: ResourceState<T>): void {
    entry.state = state;
    for (const listener of entry.listeners) listener(state);
  }

  #options(given: {
    idempotencyKey?: string;
    timeoutMs?: number;
    ifMatch?: string;
    stepUpToken?: string;
  }): RequestOptions {
    return {
      token: this.#token(),
      idempotencyKey: given.idempotencyKey,
      ifMatch: given.ifMatch,
      stepUpToken: given.stepUpToken,
      timeoutMs: given.timeoutMs ?? DEFAULT_TIMEOUT_MS,
    };
  }

  /**
   * Drops what a write made stale. Everything, when the write did not say.
   *
   * The two halves of that are not the same, and treating them alike was a defect: an entry with
   * **listeners** is a screen somebody is looking at, and dropping it takes the listeners with it —
   * so the component that made the write is never told, and the change it just performed does not
   * appear. An entry with **no** listeners is a cache nobody is watching, and removing it is right:
   * reloading it on the spot would turn one write into a burst of requests for screens nobody has
   * open.
   *
   * So: watched entries are read again, unwatched ones are forgotten. The reload is not awaited —
   * a write returns what the server answered and does not wait for the screens around it to catch
   * up — and a reload that fails reaches its subscribers as a `failed` state like any other.
   */
  #invalidate(prefixes: readonly string[] | undefined): void {
    const stale = [...this.#resources].filter(
      ([, entry]) => prefixes === undefined || prefixes.some((prefix) => entry.path.startsWith(prefix)),
    );

    for (const [key, entry] of stale) {
      if (entry.listeners.size === 0) {
        this.#resources.delete(key);
        continue;
      }
      // Somebody asked to be told about this one, so tell them. The request is the entry's own, so
      // a query keeps its document and a page keeps its cursor.
      void this.#load(entry.request, entry);
    }
  }
}

/**
 * One change record out of the `data` of an event, or nothing.
 *
 * A frame this client cannot read is skipped rather than thrown: the stream is an accelerator, the
 * cursor has already moved past it, and a client that tore down its connection over one unreadable
 * frame would lose the ninety-nine readable ones behind it. A newer server sending an entity this
 * client has never heard of is the ordinary case of that, not a defect.
 */
function recordOf(data: string): ChangeRecord | undefined {
  let parsed: unknown;
  try {
    parsed = JSON.parse(data);
  } catch {
    return undefined;
  }
  if (parsed === null || typeof parsed !== 'object') return undefined;
  const record = parsed as Partial<ChangeRecord>;
  if (typeof record.entity !== 'string' || typeof record.entity_id !== 'string') return undefined;
  return record as ChangeRecord;
}

/** Doubling, to a ceiling. Deterministic, because a test that cannot predict the wait cannot assert it. */
function backoff(attempt: number): number {
  return Math.min(RECONNECT_BASE_MS * 2 ** attempt, RECONNECT_MAX_MS);
}

/** The default wait: a timer that ends early when the listener is stopped. */
function sleeper(signal: AbortSignal): (ms: number) => Promise<void> {
  return (ms: number) =>
    new Promise<void>((resolve) => {
      const done = () => {
        clearTimeout(timer);
        signal.removeEventListener('abort', done);
        resolve();
      };
      const timer = setTimeout(done, ms);
      signal.addEventListener('abort', done, { once: true });
    });
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
  /**
   * The request it was loaded with, so that a reload repeats the same question.
   *
   * A query's document and a paged read's cursor both live here. Without it an invalidation could
   * only re-fetch a path, which for `POST /items:query` is not a question at all.
   */
  readonly request: ResourceRequest;
  state: ResourceState<T>;
  listeners: Set<Listener<T>>;
  /** The tag the last successful read carried, so a write can state the version it saw. */
  etag?: string;
}
