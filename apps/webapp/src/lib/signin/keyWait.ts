// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The wait for a passkey that answers in the system browser (ADR-0096 decision 5), without the
 * page: the listener opened, the browser sent to the workspace's host, the answer taken back once.
 * `browserKey.svelte.ts` draws it; this is what a test can run.
 */

import type { PasskeyAssertionOptions } from '@hubtask/sync-engine';

import type { KeyAnswer } from '../data/passkeys.ts';
import type { BrowserSignIn, LoopbackPage } from '../platform/index.ts';
import { answerOf, relayUrl } from './keyRelay.ts';

export interface KeyWaitDeps {
  readonly signIn: Pick<BrowserSignIn, 'listen' | 'retire' | 'openInBrowser'>;
  /** The workspace's origin, where the passkey belongs. */
  readonly origin: string;
  /** The loopback page's words, in the reader's language. */
  readonly page: () => LoopbackPage;
  readonly newState: () => string;
  /** Hears whether the dialog waits. */
  readonly onWaiting: (isWaiting: boolean) => void;
}

export class KeyWait {
  readonly #deps: KeyWaitDeps;
  #url: string | undefined;
  #settle: ((answer: KeyAnswer | null) => void) | undefined;

  constructor(deps: KeyWaitDeps) {
    this.#deps = deps;
  }

  /** Opens the browser with the options and waits for its answer; `null` where none came. */
  async answer(options: PasskeyAssertionOptions): Promise<KeyAnswer | null> {
    const { signIn, origin, page, newState, onWaiting } = this.#deps;
    this.#finish(null);
    const state = newState();
    const answered = new Promise<KeyAnswer | null>((resolve) => {
      this.#settle = resolve;
    });
    const port = await signIn.listen(page(), (returned) => {
      // Only the request this dialog made, and only an answer: anything else is nothing.
      const isOurs = returned.state === state && returned.error === undefined;
      this.#finish(isOurs ? (answerOf(returned.code) ?? null) : null);
    });
    this.#url = relayUrl(origin, { port, state, options });
    onWaiting(true);
    await signIn.openInBrowser(this.#url).catch(() => {
      // The dialog offers *Open the browser again*.
    });
    return answered;
  }

  /** *Open the browser again*: the same request, whose challenge still holds. */
  again(): void {
    if (this.#url !== undefined) void this.#deps.signIn.openInBrowser(this.#url).catch(() => {});
  }

  /** *Use another way*, or the dialog closed: the wait ends, the listener with it. */
  stop(): void {
    if (this.#settle === undefined) return;
    this.#deps.signIn.retire();
    this.#finish(null);
  }

  #finish(answer: KeyAnswer | null): void {
    const settle = this.#settle;
    this.#settle = undefined;
    this.#url = undefined;
    this.#deps.onWaiting(false);
    settle?.(answer);
  }
}
