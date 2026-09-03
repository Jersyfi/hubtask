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
  | { readonly status: 'ready'; readonly data: T; readonly at: number }
  | { readonly status: 'failed'; readonly error: TransportError };

export type Listener<T> = (state: ResourceState<T>) => void;
export type Unsubscribe = () => void;

/** What a resource is: where to read it, and how long that may take. */
export interface ResourceRequest {
  /** The path under the API base, query already encoded. */
  readonly path: string;
  readonly timeoutMs?: number;
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
    return (this.#resources.get(request.path)?.state ?? { status: 'idle' }) as ResourceState<T>;
  }

  /**
   * mutate performs a write and returns what the server answered.
   *
   * Nothing is queued and nothing is applied optimistically: F1 is online-only, so a write either
   * succeeds or fails in front of the person who made it. `idempotencyKey` is passed through
   * rather than minted here, because it belongs to the *intent* - a retry of the same intent is
   * the same key, and only the caller knows where one intent ends.
   */
  async mutate<T>(
    method: 'POST' | 'PATCH' | 'PUT' | 'DELETE',
    path: string,
    body: unknown,
    options: { idempotencyKey?: string; timeoutMs?: number } = {},
  ): Promise<T> {
    let answer;
    try {
      answer = await this.#transport.send<T>(method, path, body, this.#options(options));
    } catch (cause) {
      this.#noticeRefusal(cause);
      throw cause;
    }
    // A write invalidates what was read: the next subscriber loads again rather than being handed
    // a document the write has just changed.
    this.#resources.clear();
    return answer.body;
  }

  /** Forgets everything held in memory. Sign-out (`offline-sync.md` §9.6). */
  reset(): void {
    for (const entry of this.#resources.values()) entry.listeners.clear();
    this.#resources.clear();
  }

  #entryFor<T>(request: ResourceRequest): ResourceEntry<T> {
    let entry = this.#resources.get(request.path) as ResourceEntry<T> | undefined;
    if (!entry) {
      entry = { state: { status: 'idle' }, listeners: new Set() };
      this.#resources.set(request.path, entry as ResourceEntry<unknown>);
    }
    return entry;
  }

  async #load<T>(request: ResourceRequest, entry: ResourceEntry<T>): Promise<void> {
    this.#publish(entry, { status: 'loading' });
    try {
      const answer = await this.#transport.get<T>(request.path, this.#options(request));
      this.#publish(entry, { status: 'ready', data: answer.body, at: this.#clock.now() });
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

  #options(given: { idempotencyKey?: string; timeoutMs?: number }): RequestOptions {
    return {
      token: this.#token(),
      idempotencyKey: given.idempotencyKey,
      timeoutMs: given.timeoutMs ?? DEFAULT_TIMEOUT_MS,
    };
  }
}

interface ResourceEntry<T> {
  state: ResourceState<T>;
  listeners: Set<Listener<T>>;
}
