// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The one Transport that talks to a real server.
//
// It is the whole network surface of the client: no component, and nothing else in this package,
// ever calls `fetch`. That is what makes three promises checkable in one file rather than in fifty
// - every request carries its bearer, every request that takes an idempotency key sends one, and
// every request has a deadline.

import { TransportError, type FieldProblem } from './errors.ts';
import type {
  ByteTransfer,
  TransportDocument,
  RequestOptions,
  Response,
  StreamConnection,
  StreamEvent,
  StreamOptions,
  Transport,
} from './ports.ts';

export interface FetchTransportOptions {
  /** Where the API is. `/api/v1` against the origin that served the bundle (ADR-0028). */
  readonly baseUrl: string;
  /** Injected so a test can supply its own, and so no module-level global is captured. */
  readonly fetch?: typeof globalThis.fetch;
}

/** The problem document shape this reads. Only the fields the client acts on. */
interface ProblemBody {
  readonly code?: string;
  readonly detail_code?: string;
  readonly params?: Record<string, string>;
  readonly field_errors?: readonly FieldProblem[];
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

  /**
   * `GET /stream`, read from the body as it arrives - never through `EventSource`, which cannot
   * carry a bearer, and a token in a URL is forbidden (`security.md`, `apps/webapp/CLAUDE.md`).
   * The cursor goes in `Last-Event-ID`, which the contract says a client that manages its own
   * connection should send explicitly.
   */
  async stream(path: string, options: StreamOptions): Promise<StreamConnection> {
    for (const [name, value] of [['connectTimeoutMs', options.connectTimeoutMs], ['idleTimeoutMs', options.idleTimeoutMs]] as const) {
      if (!Number.isFinite(value) || value <= 0) {
        throw new TypeError(`a stream needs a positive ${name}; there is no default of "forever"`);
      }
    }

    // One controller for the whole connection. The connect deadline arms it until the headers
    // arrive; the idle deadline arms it between chunks; the caller's signal arms it whenever.
    const controller = new AbortController();
    const abort = () => controller.abort();
    if (options.signal?.aborted) controller.abort();
    options.signal?.addEventListener('abort', abort, { once: true });
    const connecting = setTimeout(abort, options.connectTimeoutMs);

    const headers = new Headers({ Accept: 'text/event-stream' });
    if (options.token) headers.set('Authorization', `Bearer ${options.token}`);
    if (options.lastEventId) headers.set('Last-Event-ID', options.lastEventId);

    let answer: globalThis.Response;
    try {
      answer = await this.#fetch(`${this.#baseUrl}${path}`, {
        method: 'GET',
        headers,
        signal: controller.signal,
        credentials: 'same-origin',
      });
    } catch (cause) {
      clearTimeout(connecting);
      options.signal?.removeEventListener('abort', abort);
      const aborted = cause instanceof Error && (cause.name === 'TimeoutError' || cause.name === 'AbortError');
      throw new TransportError(aborted ? 'timeout' : 'offline', { cause });
    }
    clearTimeout(connecting);

    if (!answer.ok) {
      options.signal?.removeEventListener('abort', abort);
      // A refusal is an ordinary problem document, and `Retry-After` rides with a 503: the server
      // is saying when, and a client that reconnected sooner would be hammering it.
      await this.#read(answer).catch((error: unknown) => {
        if (error instanceof TransportError) {
          throw new TransportError(error.kind, {
            status: error.status, code: error.code, detailCode: error.detailCode,
            params: error.params, fieldErrors: error.fieldErrors, requestId: error.requestId,
            retryAfterMs: retryAfterOf(answer.headers), cause: error,
          });
        }
        throw error;
      });
      throw new TransportError('malformed', { status: answer.status });
    }
    if (!answer.body) {
      options.signal?.removeEventListener('abort', abort);
      throw new TransportError('malformed', { status: answer.status });
    }

    const body = answer.body;
    const release = () => options.signal?.removeEventListener('abort', abort);
    return { events: readEvents(body, controller, options.idleTimeoutMs, release) };
  }

  /**
   * The bytes of an upload, to the absolute URL the server handed over.
   *
   * No bearer, ever: the URL is its own credential - a presigned bucket URL or this server's
   * token-protected content route - and a bearer sent to a bucket would be a bearer leaked to a
   * third party. `credentials: 'omit'` says the same about cookies. The deadline is the caller's,
   * sized by the bytes; the abort is the caller's; the progress is reported as bytes leave where
   * the runtime can stream a request body, and once at the end where it cannot.
   */
  async transfer(transfer: ByteTransfer): Promise<void> {
    if (!Number.isFinite(transfer.timeoutMs) || transfer.timeoutMs <= 0) {
      throw new TypeError('a transfer needs a positive timeoutMs; there is no default of "forever"');
    }

    const deadline = AbortSignal.timeout(transfer.timeoutMs);
    const signal = transfer.signal ? AbortSignal.any([deadline, transfer.signal]) : deadline;
    const headers = new Headers();
    if (transfer.contentType) headers.set('Content-Type', transfer.contentType);

    const total = sizeOf(transfer.body);
    const report = transfer.onProgress ?? (() => {});
    const streaming = supportsStreamingUploads();

    let answer: globalThis.Response;
    try {
      answer = await this.#fetch(transfer.url, {
        method: transfer.method,
        headers,
        body: streaming ? counted(transfer.body, total, report) : transfer.body,
        signal,
        credentials: 'omit',
        // `duplex` is what a streamed request body requires, and TypeScript's `RequestInit`
        // has not caught up with the specification.
        ...(streaming ? { duplex: 'half' } : {}),
      } as RequestInit);
    } catch (cause) {
      const aborted = cause instanceof Error && (cause.name === 'TimeoutError' || cause.name === 'AbortError');
      throw new TransportError(aborted ? 'timeout' : 'offline', { cause });
    }

    if (!answer.ok) {
      // A bucket answers XML and this server answers a problem document; neither is required to
      // be readable, and the status is what a caller acts on either way.
      let problem: ProblemBody = {};
      try {
        problem = (JSON.parse(await answer.text()) ?? {}) as ProblemBody;
      } catch {
        problem = {};
      }
      throw new TransportError('problem', {
        status: answer.status, code: problem.code, detailCode: problem.detail_code,
        params: problem.params, requestId: problem.request_id ?? undefined,
      });
    }
    report(total, total);
  }

  /**
   * A `POST` whose answer is a file.
   *
   * The same request every other call makes — bearer, deadline, same-origin credentials — with two
   * differences: `Accept` names the three things an export can be, and the answer is read as bytes
   * rather than parsed. A refusal still arrives as a problem document, so it is read as one: a
   * caller that handed a person a file containing `{"code":"forbidden"}` would be a caller that
   * checked nothing.
   */
  async document(path: string, body: unknown, options: RequestOptions): Promise<TransportDocument> {
    if (!Number.isFinite(options.timeoutMs) || options.timeoutMs <= 0) {
      throw new TypeError('a request needs a positive timeoutMs; there is no default of "forever"');
    }

    const deadline = AbortSignal.timeout(options.timeoutMs);
    const signal = options.signal ? AbortSignal.any([deadline, options.signal]) : deadline;

    const headers = new Headers({ Accept: 'text/csv, application/json, text/calendar' });
    if (options.token) headers.set('Authorization', `Bearer ${options.token}`);
    if (options.idempotencyKey) headers.set('Idempotency-Key', options.idempotencyKey);
    headers.set('Content-Type', 'application/json');

    let answer: globalThis.Response;
    try {
      answer = await this.#fetch(`${this.#baseUrl}${path}`, {
        method: 'POST',
        headers,
        body: JSON.stringify(body ?? {}),
        signal,
        credentials: 'same-origin',
      });
    } catch (cause) {
      const aborted = cause instanceof Error && (cause.name === 'TimeoutError' || cause.name === 'AbortError');
      throw new TransportError(aborted ? 'timeout' : 'offline', { cause });
    }

    if (!answer.ok) {
      let problem: ProblemBody = {};
      try {
        problem = (JSON.parse(await answer.text()) ?? {}) as ProblemBody;
      } catch {
        problem = {};
      }
      throw new TransportError('problem', {
        status: answer.status, code: problem.code, detailCode: problem.detail_code,
        params: problem.params, fieldErrors: problem.field_errors,
        requestId: problem.request_id ?? undefined,
      });
    }

    const collected = new Map<string, string>();
    answer.headers.forEach((value, name) => collected.set(name.toLowerCase(), value));

    return {
      body: await answer.blob(),
      contentType: answer.headers.get('Content-Type') ?? undefined,
      fileName: fileNameOf(answer.headers.get('Content-Disposition')),
      headers: collected,
    };
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
    if (options.ifMatch) headers.set('If-Match', options.ifMatch);
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
      detailCode: problem.detail_code,
      params: problem.params,
      // Passed on rather than interpreted: which field a message belongs under is the frame's
      // question, and it needs the server's `path` to answer it (ADR-0025).
      fieldErrors: Array.isArray(problem.field_errors) ? problem.field_errors : undefined,
      requestId: problem.request_id ?? answer.headers.get('X-Request-Id') ?? undefined,
    });
  }
}

/**
 * The `Retry-After` of a refusal, in milliseconds. Delta-seconds only: the HTTP-date form would
 * need the machine's clock, and this package reads it in exactly one place (`systemClock`).
 */
function retryAfterOf(headers: Headers): number | undefined {
  const value = headers.get('Retry-After');
  if (value === null) return undefined;
  const seconds = Number(value);
  return Number.isFinite(seconds) && seconds >= 0 ? seconds * 1000 : undefined;
}

/**
 * The event-stream framing (WHATWG "Server-sent events" §9.2), read from the body as it arrives.
 *
 * A line is a field and a value; a blank line dispatches what was collected; a line starting with
 * a colon is a comment, which is what the heartbeat is. `id` is remembered per event so the engine
 * can resume from it; `retry` is passed on as the server's suggestion. The idle deadline is armed
 * between chunks: a heartbeat arrives well inside it, and silence for longer is a proxy that
 * dropped the connection without saying so - the iteration ends, and the engine reconnects.
 */
async function* readEvents(
  body: ReadableStream<Uint8Array>, controller: AbortController, idleTimeoutMs: number,
  release: () => void,
): AsyncGenerator<StreamEvent> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  // Aborting the fetch ends a network body, but not a body the runtime handed over already - and
  // never a body a test fed by hand. Cancelling the reader is what actually ends `read()`, so the
  // abort the idle deadline and the caller's signal share is wired to it rather than trusted to
  // travel on its own.
  const cancel = () => { void reader.cancel().catch(() => {}); };
  if (controller.signal.aborted) cancel();
  controller.signal.addEventListener('abort', cancel, { once: true });
  let idle = setTimeout(() => controller.abort(), idleTimeoutMs);
  let buffered = '';
  let pending: { id?: string; event?: string; data: string[]; retryMs?: number } = { data: [] };

  try {
    for (;;) {
      let chunk: ReadableStreamReadResult<Uint8Array>;
      try {
        chunk = await reader.read();
      } catch {
        // The server closed, the network dropped, or the controller fired: all three end the
        // stream, and none is an error to a caller that reconnects with its cursor.
        return;
      }
      if (chunk.done) return;
      clearTimeout(idle);
      idle = setTimeout(() => controller.abort(), idleTimeoutMs);

      buffered += decoder.decode(chunk.value, { stream: true });
      let newline = buffered.indexOf('\n');
      while (newline >= 0) {
        const line = buffered.slice(0, newline).replace(/\r$/, '');
        buffered = buffered.slice(newline + 1);
        newline = buffered.indexOf('\n');

        if (line === '') {
          if (pending.data.length > 0) {
            yield { id: pending.id, event: pending.event, data: pending.data.join('\n'), retryMs: pending.retryMs };
          }
          pending = { data: [], retryMs: pending.retryMs };
          continue;
        }
        if (line.startsWith(':')) continue;

        const colon = line.indexOf(':');
        const field = colon < 0 ? line : line.slice(0, colon);
        let value = colon < 0 ? '' : line.slice(colon + 1);
        if (value.startsWith(' ')) value = value.slice(1);
        switch (field) {
          case 'id': pending.id = value; break;
          case 'event': pending.event = value; break;
          case 'data': pending.data.push(value); break;
          case 'retry': {
            const ms = Number(value);
            if (Number.isInteger(ms) && ms >= 0) pending.retryMs = ms;
            break;
          }
          default: break;
        }
      }
    }
  } finally {
    clearTimeout(idle);
    release();
    controller.signal.removeEventListener('abort', cancel);
    controller.abort();
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}

function sizeOf(body: ByteTransfer['body']): number {
  if (body instanceof Blob) return body.size;
  return body.byteLength;
}

/**
 * Whether this runtime can send a request body as a stream, which is what makes upload progress
 * observable at all. The detection is the standard one: a runtime that understands `duplex` reads
 * the getter, and one that streams the body sets no content type of its own.
 */
function supportsStreamingUploads(): boolean {
  try {
    let duplexAccessed = false;
    const request = new Request('data:,', {
      body: new ReadableStream(),
      method: 'POST',
      get duplex() {
        duplexAccessed = true;
        return 'half';
      },
    } as RequestInit);
    return duplexAccessed && !request.headers.has('Content-Type');
  } catch {
    return false;
  }
}

/** The bytes as a stream that counts what has left, so a caller can draw a progress bar. */
function counted(
  body: ByteTransfer['body'], total: number, report: (sent: number, total: number) => void,
): ReadableStream<Uint8Array> {
  const source = body instanceof Blob ? body : new Blob([body as BlobPart]);
  let sent = 0;
  return source.stream().pipeThrough(new TransformStream<Uint8Array, Uint8Array>({
    transform(chunk, controller) {
      sent += chunk.byteLength;
      report(Math.min(sent, total), total);
      controller.enqueue(chunk);
    },
  }));
}

/**
 * The name the server gave the file, out of `Content-Disposition`.
 *
 * `filename*` first, because that is the one that carries a name with characters outside ASCII in
 * it — a view called "Überfällig" exports under its own name or under a mangled one, and which of
 * the two is decided here.
 */
function fileNameOf(disposition: string | null): string | undefined {
  if (!disposition) return undefined;

  const extended = /filename\*=UTF-8''([^;]+)/i.exec(disposition);
  if (extended?.[1]) {
    try {
      return decodeURIComponent(extended[1]);
    } catch {
      // A name this cannot decode is a name the caller composes instead.
      return undefined;
    }
  }

  const plain = /filename="?([^";]+)"?/i.exec(disposition);
  return plain?.[1];
}
