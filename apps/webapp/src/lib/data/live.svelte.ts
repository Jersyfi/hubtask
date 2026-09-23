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

import { platform } from '../platform/index.ts';
import { engine } from './engine.ts';
import { pathsFor, revokedContainerOf } from './live.ts';

/**
 * Where the connection stands, for the notice a reader sees.
 *
 * `off` is "no stream is running" - before the first start and after a sign-out, which is also
 * when the mark is not drawn at all. `offline` is the *device's* answer: this machine says it has
 * no network, and the three states are then not a question about the server.
 */
export type LiveState = 'off' | 'live' | 'reconnecting' | 'offline';

class Live {
  #state = $state<LiveState>('off');
  /**
   * Whether the device says it has no network.
   *
   * `navigator.onLine` is trusted **in one direction only**, which is the only direction it is
   * worth anything in: `false` means there is demonstrably no network, `true` means the machine
   * has an interface up and says nothing about whether the server is reachable. So this turns a
   * failing reconnect into *Offline* when the browser knows why, and never turns *Reconnecting…*
   * into *Connected*.
   *
   * Before this the struck cloud of ADR-0063 decision 5 was drawn by nothing: `off` is the only
   * other state and the mark is not drawn without a session, so a machine with its network pulled
   * out read *Reconnecting…* - true, and not the thing the reader needs to be told.
   */
  #isDeviceOffline = $state(false);
  /** The containers this reader has lost while the tab was open. */
  #revoked = $state<readonly string[]>([]);
  #stop: (() => void) | undefined;

  get state(): LiveState {
    // The device's answer outranks the stream's, because it explains it: attempts are failing and
    // this is why. `off` is left alone - there is nothing to be offline from.
    if (this.#state !== 'off' && this.#isDeviceOffline) return 'offline';
    return this.#state;
  }

  /** Whether the reader has lost access to this container since the tab opened. */
  hasLost(containerId: string | undefined): boolean {
    return containerId !== undefined && this.#revoked.includes(containerId);
  }

  /**
   * Opens the stream, over the account's replica. Idempotent: a second call while one is open
   * does nothing, because a tab needs one connection and the per-credential cap is real.
   *
   * The store is attached first (F6-03): one database per API origin and account, which is why
   * this waits for the account rather than for the credential alone. The engine then takes the
   * initial synchronisation or the delta before it opens the stream, and every record the stream
   * carries is written to the copy before it is acted on. Where the platform offers no store, the
   * engine listens as it did before: online-only.
   */
  start(accountId: string): void {
    if (this.#stop) return;

    this.#state = 'reconnecting';
    let stopped = false;
    let stopListening: (() => void) | undefined;

    // What the device says about its own network, followed while the stream is open. The engine
    // keeps trying either way - a tab whose network comes back reconnects without being told.
    const notice = () => {
      this.#isDeviceOffline = typeof navigator === 'undefined' ? false : navigator.onLine === false;
    };
    notice();
    globalThis.addEventListener?.('online', notice);
    globalThis.addEventListener?.('offline', notice);

    this.#stop = () => {
      stopped = true;
      globalThis.removeEventListener?.('online', notice);
      globalThis.removeEventListener?.('offline', notice);
      stopListening?.();
    };

    const attach = async () => {
      const storage = platform.storageFor(accountId);
      if (storage) {
        try {
          await engine.attach(storage, { platform: 'web', displayName: platform.deviceName() });
        } catch {
          // A store that cannot be opened is no store: the copy is a convenience in the browser,
          // and the stream and every read work without it.
        }
      }
      if (stopped) return;
      stopListening = engine.listen({
        pathsFor,
        // What the mark in the bar reads, and it is the **connection's** state rather than the
        // traffic's (issue 1017). This used to become `live` on the first record, so a stream that
        // was open and idle - a workspace nobody else is writing in - said *Reconnecting…* until
        // somebody changed something. A record proves the stream delivers; the engine is what
        // knows whether it is attached, and now it says so.
        onConnection: (isOpen) => {
          this.#state = isOpen ? 'live' : 'reconnecting';
        },
        onRecord: (record) => this.#notice(record),
      });
    };
    void attach();
  }

  /** Closes it. Sign-out calls this before `engine.reset()`, so nothing arrives into a cleared cache. */
  stop(): void {
    this.#stop?.();
    this.#stop = undefined;
    this.#state = 'off';
    this.#isDeviceOffline = false;
    this.#revoked = [];
  }

  /**
   * Every record, before the engine acts on it.
   *
   * One thing happens here and it is not a merge: a revocation is remembered so the screen it
   * concerns can say so. What the record *contains* is untouched; the engine's invalidation is
   * what makes the screen right.
   *
   * It no longer touches the notice. Whether the connection is up is `onConnection`'s answer, and
   * a record is not one - it is a change that arrived through a connection that was already up.
   */
  #notice(record: ChangeRecord): void {
    const lost = revokedContainerOf(record);
    if (lost && !this.#revoked.includes(lost)) {
      this.#revoked = [...this.#revoked, lost];
    }
  }
}

export const live = new Live();
