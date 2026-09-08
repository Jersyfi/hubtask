// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The fakes ADR-0033 requires the engine to be exercisable against.
//
// They live in the package rather than in a test file because they are the first-party counterpart
// to `hubctl sync-conformance` (offline-sync.md §9): the same fakes will drive the conformance run
// when F6 brings the protocol, and a fake that only one test file can reach is a fake that gets
// rewritten.

import { TransportError } from '../src/errors.ts';
import type {
  ByteTransfer,
  Clock,
  RequestOptions,
  Response,
  StreamConnection,
  StreamEvent,
  StreamOptions,
  Transport,
} from '../src/ports.ts';

/** One call, as it was made. What a test asserts the headers and the deadline on. */
export interface Call {
  readonly method: string;
  readonly path: string;
  readonly body: unknown;
  readonly options: RequestOptions;
}

/**
 * FakeTransport answers from a table and records what it was asked.
 *
 * It answers *asynchronously* even though it has the answer to hand: a fake that resolves
 * synchronously hides every ordering bug a real network would expose, and the `loading` state
 * would never be observed by a test.
 */
/**
 * One scripted stream connection: either a refusal, or the events the server sends before it
 * closes the connection. `open` keeps the connection up after the last event until the engine
 * aborts it, which is what a live stream looks like between changes.
 */
export interface StreamSession {
  readonly refuse?: TransportError;
  readonly events?: readonly StreamEvent[];
  readonly open?: boolean;
}

export class FakeTransport implements Transport {
  readonly calls: Call[] = [];
  /** Every stream opened, with the cursor it was opened with - what a reconnect test asserts. */
  readonly streams: { readonly lastEventId?: string; readonly token?: string }[] = [];
  /** Every byte transfer, as it was asked for. */
  readonly transfers: ByteTransfer[] = [];
  #answers = new Map<string, unknown>();
  #etags = new Map<string, string>();
  #sequences = new Map<string, unknown[]>();
  #failures = new Map<string, Error>();
  #sessions: StreamSession[] = [];

  /** Sets what a path answers with, and the `ETag` it answers with where one matters. */
  answer(path: string, body: unknown, etag?: string): this {
    this.#answers.set(path, body);
    if (etag !== undefined) this.#etags.set(path, etag);
    return this;
  }

  /**
   * Sets a series of answers for one path, consumed one per call.
   *
   * Paging is the reason: a second page is the same path answering something else, and a fake
   * whose answer never changes cannot express that. The last entry repeats once the series is
   * exhausted, so a test that calls one time too many gets a stable answer rather than undefined.
   */
  answerEach(path: string, bodies: readonly unknown[]): this {
    this.#sequences.set(path, [...bodies]);
    return this;
  }

  /** Sets what a path fails with. */
  fail(path: string, error: Error): this {
    this.#failures.set(path, error);
    return this;
  }

  /** Scripts the stream connections, consumed one per open. The last one repeats. */
  streamSessions(...sessions: StreamSession[]): this {
    this.#sessions = [...sessions];
    return this;
  }

  async stream(_path: string, options: StreamOptions): Promise<StreamConnection> {
    this.streams.push({ lastEventId: options.lastEventId, token: options.token });
    await Promise.resolve();

    const session = this.#sessions.length > 1 ? this.#sessions.shift() : this.#sessions[0];
    if (!session) throw new TransportError('offline');
    if (session.refuse) throw session.refuse;

    const events = session.events ?? [];
    const signal = options.signal;
    return {
      events: (async function* () {
        for (const event of events) {
          if (signal?.aborted) return;
          await Promise.resolve();
          yield event;
        }
        if (session.open && signal && !signal.aborted) {
          await new Promise<void>((resolve) => signal.addEventListener('abort', () => resolve(), { once: true }));
        }
      })(),
    };
  }

  async transfer(transfer: ByteTransfer): Promise<void> {
    this.transfers.push(transfer);
    await Promise.resolve();
    const failure = this.#failures.get(transfer.url);
    if (failure) throw failure;
    const total = transfer.body instanceof Blob ? transfer.body.size : transfer.body.byteLength;
    transfer.onProgress?.(total, total);
  }

  async get<T>(path: string, options: RequestOptions): Promise<Response<T>> {
    return this.#respond<T>('GET', path, undefined, options);
  }

  async send<T>(
    method: 'POST' | 'PATCH' | 'PUT' | 'DELETE',
    path: string,
    body: unknown,
    options: RequestOptions,
  ): Promise<Response<T>> {
    return this.#respond<T>(method, path, body, options);
  }

  async #respond<T>(
    method: string,
    path: string,
    body: unknown,
    options: RequestOptions,
  ): Promise<Response<T>> {
    this.calls.push({ method, path, body, options });
    // A turn of the microtask queue, so `loading` is a state a test can observe.
    await Promise.resolve();

    const failure = this.#failures.get(path);
    if (failure) throw failure;

    const sequence = this.#sequences.get(path);
    if (sequence && sequence.length > 0) {
      const next = sequence.length > 1 ? sequence.shift() : sequence[0];
      return { status: 200, body: next as T, etag: this.#etags.get(path) };
    }
    return { status: 200, body: this.#answers.get(path) as T, etag: this.#etags.get(path) };
  }
}

/** A clock that does not move unless a test moves it (rule 4). */
export class FixedClock implements Clock {
  #at: number;

  constructor(at = 1_700_000_000_000) {
    this.#at = at;
  }

  now(): number {
    return this.#at;
  }

  advance(ms: number): void {
    this.#at += ms;
  }
}
