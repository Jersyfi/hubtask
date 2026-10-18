// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Signing in with a passkey from the card (identity.md §23.4, UC-ID-21 checks 1, 4 and 9).
 *
 * **Two ways to ask the device, one at a time.** The address field offers this workspace's passkeys
 * once it gains focus - a conditional request, which waits until the person picks one or the page
 * moves on - and *Sign in with a passkey* asks directly. A browser runs one request at a time, so the
 * button aborts the field's and the field is armed again after it.
 *
 * **Options are fetched when they are about to be used**, never per page view: a challenge lives five
 * minutes and every fetch spends the network's share of the sign-in bucket, which people signing in
 * by password behind the same address share. One that ran out is answered `'stale'` and fetched again
 * without a word; a prompt the person closes says nothing at all (P-11).
 */

import { TransportError, type PasskeySignInOptions } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { assertionOf, canOfferInField, ceremonyEndOf, requestOptionsOf, type AssertedCredential } from './passkeys.ts';
import { session } from '../session.svelte.ts';

const OPTIONS = '/auth/sessions:passkey-options';

/** How long the options call may take. Somebody is waiting on it. */
const TIMEOUT_MS = 15_000;

/** The browser's ceremony, handed in so a test can stand in for the device. */
export type GetCredential = (options: CredentialRequestOptions) => Promise<Credential | null>;

class PasskeySignIn {
  #waiting = $state(false);
  #field: AbortController | undefined;

  /** Whether the device is being asked by the button: the card says so once. */
  get isWaiting(): boolean {
    return this.#waiting;
  }

  /** Offers this workspace's passkeys in the address field, where the browser can. Idempotent. */
  async armField(get: GetCredential, browser: unknown = globalThis): Promise<void> {
    if (this.#field || !(await canOfferInField(browser as never))) return;
    const controller = new AbortController();
    this.#field = controller;
    try {
      for (let tries = 0; tries < 2 && !controller.signal.aborted; tries++) {
        const options = await this.#options();
        const credential = await get(requestOptionsOf(options, 'conditional', controller.signal));
        if (credential === null) return;
        const ended = await session.signInWithPasskey(assertionOf(credential as unknown as AssertedCredential));
        if (ended !== 'stale') return;
      }
    } catch {
      // Aborted by the button, closed, or no options here: the field simply offers nothing now.
    } finally {
      if (this.#field === controller) this.#field = undefined;
    }
  }

  /**
   * Asks the device directly. `'cancelled'` is the person's own choice and says nothing; every other
   * end is the session's to show.
   */
  async ask(get: GetCredential): Promise<'signed-in' | 'owed' | 'cancelled' | 'refused'> {
    this.#field?.abort();
    this.#field = undefined;
    this.#waiting = true;
    try {
      for (let tries = 0; tries < 2; tries++) {
        const options = await this.#options();
        let credential: Credential | null;
        try {
          credential = await get(requestOptionsOf(options, 'optional'));
        } catch (cause) {
          if (ceremonyEndOf(cause) === 'cancelled') return 'cancelled';
          throw cause;
        }
        if (credential === null) return 'cancelled';
        const ended = await session.signInWithPasskey(assertionOf(credential as unknown as AssertedCredential));
        if (ended !== 'stale') return ended;
      }
      return 'refused';
    } catch (cause) {
      return cause instanceof TransportError ? 'refused' : 'cancelled';
    } finally {
      this.#waiting = false;
    }
  }

  async #options(): Promise<PasskeySignInOptions> {
    return engine.mutate<PasskeySignInOptions>('POST', OPTIONS, {}, { timeoutMs: TIMEOUT_MS, invalidates: [] });
  }
}

export const passkeySignIn = new PasskeySignIn();
