// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * An installed app's passkey step-up, as the dialog draws it (ADR-0096 decision 5): the wait of
 * `keyWait.ts` over this app's shell, the workspace it is signed in to and the reader's language.
 */

import type { PasskeyAssertionOptions } from '@hubtask/sync-engine';

import type { KeyAnswer } from '../data/passkeys.ts';
import { messages, t } from '../i18n/i18n.svelte.ts';
import { platform } from '../platform/index.ts';
import { newState } from './appSignIn.ts';
import { KeyWait } from './keyWait.ts';
import { loopbackLook } from './loopbackPage.ts';

class BrowserKey {
  #waiting = $state(false);
  #wait: KeyWait | undefined;

  /** Whether this app's passkey answers in the system browser, rather than in the page. */
  get isAvailable(): boolean {
    return platform.browserSignIn !== undefined;
  }

  /** Whether the browser is open and the dialog waits for it. */
  get isWaiting(): boolean {
    return this.#waiting;
  }

  /** The workspace's host, where the passkey belongs and the browser opens. */
  get host(): string {
    return new URL(platform.apiBase).host;
  }

  answer(options: PasskeyAssertionOptions): Promise<KeyAnswer | null> {
    return this.#waitFor()?.answer(options) ?? Promise.resolve(null);
  }

  again(): void {
    this.#wait?.again();
  }

  stop(): void {
    this.#wait?.stop();
  }

  #waitFor(): KeyWait | undefined {
    const signIn = platform.browserSignIn;
    if (signIn === undefined) return undefined;
    this.#wait ??= new KeyWait({
      signIn,
      origin: new URL(platform.apiBase).origin,
      page: () => ({ title: t('app.step_up.back_title'), body: t('app.step_up.back_body'), lang: messages.locale, ...loopbackLook() }),
      newState: () => newState(),
      onWaiting: (isWaiting) => {
        this.#waiting = isWaiting;
      },
    });
    return this.#wait;
  }
}

export const browserKey = new BrowserKey();
