// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The fakes ADR-0033 requires the engine to be exercisable against.
//
// They live in the package rather than in a test file because they are the first-party counterpart
// to `hubctl sync-conformance` (offline-sync.md §9): the same fakes will drive the conformance run
// when F6 brings the protocol, and a fake that only one test file can reach is a fake that gets
// rewritten.

import type { Clock, RequestOptions, Response, Transport } from '../src/ports.ts';

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
export class FakeTransport implements Transport {
  readonly calls: Call[] = [];
  #answers = new Map<string, unknown>();
  #failures = new Map<string, Error>();

  /** Sets what a path answers with. */
  answer(path: string, body: unknown): this {
    this.#answers.set(path, body);
    return this;
  }

  /** Sets what a path fails with. */
  fail(path: string, error: Error): this {
    this.#failures.set(path, error);
    return this;
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
    return { status: 200, body: this.#answers.get(path) as T };
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
