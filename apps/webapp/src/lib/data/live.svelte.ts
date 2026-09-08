// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The one stream this tab keeps, and what it does to the screen.
 *
 * **It opens after sign-in and closes on sign-out.** The engine reconnects with the last identifier
 * it saw, waits out a `503` for exactly the `Retry-After` the server named, and treats
 * `sync.cursor_too_old` as "forget everything and read again" — all of that is F3-04's and none of
 * it is repeated here. What this module owns is the *lifecycle* and the *mapping*: when to listen,
 * and what a record means in this application's paths.
 *
 * **The stream is an accelerator (ADR-0021).** A tab that never gets one misses nothing: every
 * write still refreshes what it touched, which is what F2 did and what this degrades to. So a
 * refusal — the per-credential cap, a proxy that will not hold a connection — is one quiet line
 * rather than an error, and nothing on any screen depends on a record having arrived.
 *
 * **A revocation is acted on rather than only received.** `offline-sync.md` §6 and §9 rule 3 bind a
 * client to delete what it holds for a container it lost. A cache is all this client holds, so
 * "delete" is "drop it and stop drawing it" — the same obligation, in the terms this client has.
 */

import type { ChangeRecord } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { pathsFor, revokedContainerOf } from './live.ts';

/** Where the connection stands, for the notice a reader sees. */
export type LiveState = 'off' | 'live' | 'reconnecting';

class Live {
  #state = $state<LiveState>('off');
  /** The containers this reader has lost while the tab was open. */
  #revoked = $state<readonly string[]>([]);
  #stop: (() => void) | undefined;

  get state(): LiveState {
    return this.#state;
  }

  /** Whether the reader has lost access to this container since the tab opened. */
  hasLost(containerId: string | undefined): boolean {
    return containerId !== undefined && this.#revoked.includes(containerId);
  }

  /**
   * Opens the stream. Idempotent: a second call while one is open does nothing, because a tab
   * needs one connection and the per-credential cap is real.
   */
  start(): void {
    if (this.#stop) return;

    this.#state = 'reconnecting';
    this.#stop = engine.listen({
      pathsFor,
      onRecord: (record) => this.#notice(record),
    });
  }

  /** Closes it. Sign-out calls this before `engine.reset()`, so nothing arrives into a cleared cache. */
  stop(): void {
    this.#stop?.();
    this.#stop = undefined;
    this.#state = 'off';
    this.#revoked = [];
  }

  /**
   * Every record, before the engine acts on it.
   *
   * Two things happen here and neither is a merge: the notice becomes `live` — a record arriving is
   * the only honest proof that the connection works — and a revocation is remembered so the screen
   * it concerns can say so. What the record *contains* is untouched; the engine's invalidation is
   * what makes the screen right.
   */
  #notice(record: ChangeRecord): void {
    if (this.#state !== 'live') this.#state = 'live';

    const lost = revokedContainerOf(record);
    if (lost && !this.#revoked.includes(lost)) {
      this.#revoked = [...this.#revoked, lost];
    }
  }
}

export const live = new Live();
