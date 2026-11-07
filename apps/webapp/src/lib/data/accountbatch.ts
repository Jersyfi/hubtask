// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * How the names behind account identifiers are asked for: every identifier asked in one turn goes
 * in one request (`POST /accounts:resolve`, up to 200 each), so a screen naming 120 people costs one
 * request and not 120 a rate limit would refuse in part. A request the server could not answer - a
 * `429`, a `5xx`, no network - is asked again after the server's `Retry-After` or a doubling wait;
 * only an answer that leaves an identifier out makes it unknown for the session.
 */

import { TransportError } from '@hubtask/sync-engine';

/** The most identifiers one request may name, as the contract bounds it. */
export const RESOLVE_AT_MOST = 200;
/** How often a request the server could not answer is asked again before it is given up for now. */
export const RESOLVE_ATTEMPTS = 5;

export interface ResolveHooks<T> {
  /** One request for the identifiers; it answers the accounts it found, or throws. */
  readonly read: (ids: readonly string[]) => Promise<readonly T[]>;
  readonly idOf: (account: T) => string;
  /** The accounts that arrived, and the identifiers the answer left out. */
  readonly settled: (found: readonly T[], unknown: readonly string[]) => void;
  /** The identifiers given up for now: asked again by the next call that wants them. */
  readonly gaveUp: (ids: readonly string[]) => void;
  readonly wait?: (ms: number) => Promise<void>;
  readonly schedule?: (run: () => void) => void;
}

/** Whether a failure may be asked again: the server busy or down, or no network. */
export function retryable(error: unknown): boolean {
  if (!(error instanceof TransportError)) return false;
  if (error.kind === 'offline' || error.kind === 'timeout') return true;
  return error.status === 429 || (error.status !== undefined && error.status >= 500);
}

export class ResolveBatch<T> {
  readonly #hooks: ResolveHooks<T>;
  #queued = new Set<string>();
  #scheduled = false;

  constructor(hooks: ResolveHooks<T>) {
    this.#hooks = hooks;
  }

  /** Queues the identifiers for this turn's request. The caller skips those it knows or asks already. */
  ask(ids: readonly string[]): void {
    for (const id of ids) this.#queued.add(id);
    if (this.#scheduled || this.#queued.size === 0) return;
    this.#scheduled = true;
    (this.#hooks.schedule ?? queueMicrotask)(() => {
      this.#scheduled = false;
      const ids = [...this.#queued];
      this.#queued.clear();
      for (let at = 0; at < ids.length; at += RESOLVE_AT_MOST) void this.#send(ids.slice(at, at + RESOLVE_AT_MOST), 0);
    });
  }

  async #send(ids: readonly string[], attempt: number): Promise<void> {
    try {
      const found = await this.#hooks.read(ids);
      const arrived = new Set(found.map((account) => this.#hooks.idOf(account)));
      this.#hooks.settled(found, ids.filter((id) => !arrived.has(id)));
    } catch (error) {
      if (!retryable(error)) {
        // A refusal is an answer: this reader may not have those names.
        this.#hooks.settled([], ids);
        return;
      }
      if (attempt + 1 >= RESOLVE_ATTEMPTS) {
        this.#hooks.gaveUp(ids);
        return;
      }
      const after = (error as TransportError).retryAfterMs ?? Math.min(30_000, 1_000 * 2 ** attempt);
      await (this.#hooks.wait ?? ((ms) => new Promise((resolve) => setTimeout(resolve, ms))))(after);
      await this.#send(ids, attempt + 1);
    }
  }
}
