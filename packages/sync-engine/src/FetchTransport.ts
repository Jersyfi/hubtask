// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The one Transport that talks to a real server.
//
// It is the whole network surface of the client: no component, and nothing else in this package,
// ever calls `fetch`. That is what makes three promises checkable in one file rather than in fifty
// - every request carries its bearer, every request that takes an idempotency key sends one, and
// every request has a deadline.

import { TransportError } from './errors.ts';
import type { RequestOptions, Response, Transport } from './ports.ts';

export interface FetchTransportOptions {
  /** Where the API is. `/api/v1` against the origin that served the bundle (ADR-0028). */
  readonly baseUrl: string;
  /** Injected so a test can supply its own, and so no module-level global is captured. */
  readonly fetch?: typeof globalThis.fetch;
}

/** The problem document shape this reads. Only the fields the client acts on. */
interface ProblemBody {
  readonly code?: string;
  readonly params?: Record<string, string>;
  readonly request_id?: string;
}

export class FetchTransport implements Transport {
  readonly #baseUrl: string;
  readonly #fetch: typeof globalThis.fetch;

  constructor(options: FetchTransportOptions) {
    // Trailing slash removed once, here, so that every path below is written the same way.
    this.#baseUrl = options.baseUrl.replace(/\/+$/, '');
    this.#fetch = options.fetch ?? globalThis.fetch.bind(globalThis);
  }

  get<T>(path: string, options: RequestOptions): Promise<Response<T>> {
    return this.#call<T>('GET', path, undefined, options);
  }

  send<T>(
    method: 'POST' | 'PATCH' | 'PUT' | 'DELETE',
    path: string,
    body: unknown,
    options: RequestOptions,
  ): Promise<Response<T>> {
    return this.#call<T>(method, path, body, options);
  }

  async #call<T>(
    method: string,
    path: string,
    body: unknown,
    options: RequestOptions,
  ): Promise<Response<T>> {
    // The deadline is not optional and has no default. A call without one is a connection nobody
    // is waiting for any more, which is the same defect on this side of the wire as on the other.
    if (!Number.isFinite(options.timeoutMs) || options.timeoutMs <= 0) {
      throw new TypeError('a request needs a positive timeoutMs; there is no default of "forever"');
    }

    const deadline = AbortSignal.timeout(options.timeoutMs);
    // The caller's own reason to give up, on top of the deadline. Both, rather than either.
    const signal = options.signal ? AbortSignal.any([deadline, options.signal]) : deadline;

    const headers = new Headers({ Accept: 'application/json' });
    if (options.token) headers.set('Authorization', `Bearer ${options.token}`);
    if (options.idempotencyKey) headers.set('Idempotency-Key', options.idempotencyKey);
    if (body !== undefined) headers.set('Content-Type', 'application/json');

    let answer: globalThis.Response;
    try {
      answer = await this.#fetch(`${this.#baseUrl}${path}`, {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
        signal,
        // The bundle and the API come from one origin (ADR-0028), so there is nothing to send
        // credentials *to* cross-origin. Saying so is what keeps a later base URL change from
        // silently starting to.
        credentials: 'same-origin',
      });
    } catch (cause) {
      // An abort is the deadline or the caller; anything else is the network not answering. The
      // two are different to a caller: one is worth retrying at once, the other after a wait.
      const aborted = cause instanceof Error && (cause.name === 'TimeoutError' || cause.name === 'AbortError');
      throw new TransportError(aborted ? 'timeout' : 'offline', { cause });
    }

    return this.#read<T>(answer);
  }

  async #read<T>(answer: globalThis.Response): Promise<Response<T>> {
    const etag = answer.headers.get('ETag') ?? undefined;

    // 204 and an empty body are the same thing to a caller: nothing came back, and that is not an
    // error. Parsing it as JSON would be.
    const text = await answer.text();
    let parsed: unknown;
    if (text.length > 0) {
      try {
        parsed = JSON.parse(text);
      } catch (cause) {
        throw new TransportError('malformed', { status: answer.status, cause });
      }
    }

    if (answer.ok) {
      return { status: answer.status, body: parsed as T, etag };
    }

    // A failure is a problem document (RFC 9457): a code plus params, never a sentence. What is
    // read out of it is what the renderer needs and nothing else.
    const problem = (parsed ?? {}) as ProblemBody;
    throw new TransportError('problem', {
      status: answer.status,
      code: problem.code,
      params: problem.params,
      requestId: problem.request_id ?? answer.headers.get('X-Request-Id') ?? undefined,
    });
  }
}
