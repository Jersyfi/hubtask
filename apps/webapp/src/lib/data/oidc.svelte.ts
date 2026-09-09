// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Signing in through the workspace's own provider (H-04): the two calls, and what is held between
 * them, which is nothing.
 *
 * **The flow's memory is the server's.** `:start` answers a `state`, and the verifier and the nonce
 * that answer it never leave the server (ADR-0036). So this module keeps no handle, writes no
 * storage and survives no redirect — the browser leaves for the provider and comes back to a fresh
 * document, and everything needed to finish is in the address the provider put it in.
 *
 * **The sign-in screen asks and lets the server refuse.** Whether a provider is configured is
 * behind a permission a signed-out visitor does not hold, so there is no honest way to know before
 * asking. `identity_provider.not_configured` and `identity_provider.disabled` are catalogue codes
 * the server already answers, and rendering one of those is cheaper and truer than a second
 * unauthenticated endpoint that would exist only to hide a button.
 *
 * **Local sign-in never goes away.** `observability-reliability.md` §7's degradation is exactly
 * that: when discovery cannot be reached, local accounts keep working — which they only do if the
 * password form is still on the screen.
 */

import { TransportError } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { navigableUrl, type Handoff } from './oidc.ts';
import { session } from '../session.svelte.ts';

const START = '/auth/oidc:start';
const CALLBACK = '/auth/oidc:callback';

/** The one refusal every spent, unknown or unverifiable flow gets. It is the server's own code. */
const REFUSED = 'auth.oidc_failed';

/** Where to send the browser, as `OidcAuthorization` answers it. */
interface Authorization {
  readonly authorization_url: string;
}

/** The pair, as `SessionTokens` answers it — the same pair a password sign-in mints. */
interface SessionTokens {
  readonly access_token: string;
  readonly refresh_token: string;
}

class Oidc {
  #failure = $state<string | undefined>(undefined);
  #working = $state(false);

  /** Why the last attempt did not go through, as the server's own code. */
  get failure(): string | undefined {
    return this.#failure;
  }

  get isWorking(): boolean {
    return this.#working;
  }

  /**
   * Begins the flow and answers where to send the browser.
   *
   * `undefined` means it did not begin, and `failure` says why — no provider, one switched off, or
   * discovery unreachable. The caller stays on the sign-in screen with the password form intact.
   */
  async begin(loginHint?: string): Promise<string | undefined> {
    this.#working = true;
    this.#failure = undefined;
    try {
      const hint = loginHint?.trim();
      const answer = await engine.mutate<Authorization>(
        'POST',
        START,
        hint ? { login_hint: hint } : {},
      );
      const url = navigableUrl(answer.authorization_url);
      // An answer that is not a navigation is this installation's own defect rather than a
      // refusal, and `errors.internal` is what a reader is owed for one.
      if (url === undefined) this.#failure = 'errors.internal';
      return url;
    } catch (cause) {
      this.#failure = codeOf(cause);
      return undefined;
    } finally {
      this.#working = false;
    }
  }

  /**
   * Exchanges what the provider handed back, and holds the session it answers.
   *
   * The pair goes through `session.hold`, which is the same door the enforcement enrolment uses:
   * how somebody proved themselves is an attribute of the session, not a class of it, so there is
   * no second kind of session here to keep.
   */
  async complete(handoff: Handoff): Promise<boolean> {
    this.#working = true;
    this.#failure = undefined;
    try {
      const answer = await engine.mutate<SessionTokens>('POST', CALLBACK, {
        code: handoff.code,
        state: handoff.state,
      });
      session.hold({ access: answer.access_token, refresh: answer.refresh_token });
      return true;
    } catch (cause) {
      this.#failure = codeOf(cause);
      return false;
    } finally {
      this.#working = false;
    }
  }

  /**
   * The flow ended before there was anything to exchange: the provider refused, or the address
   * carries no flow at all.
   *
   * The same sentence a spent `state` gets, and deliberately so — "start again" is the whole of
   * what a reader can do about either, and telling the two apart would be telling somebody
   * standing at the callback whether a handle they do not hold was ever real.
   */
  refuse(): void {
    this.#failure = REFUSED;
  }

  /** Drops the refusal. Called when the screen that shows it leaves. */
  forget(): void {
    this.#failure = undefined;
  }
}

function codeOf(cause: unknown): string {
  return cause instanceof TransportError
    ? (cause.detailCode ?? cause.code ?? 'errors.internal')
    : 'errors.internal';
}

export const oidc = new Oidc();
