// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Who is signed in, and the three things that change it: a token typed in, a sign-out, and a
 * refusal from the server.
 *
 * **This is the honest version of "sign-in and session", and the milestone that replaces it is
 * named.** `api/openapi.yaml` declares one security scheme — a bearer that may be an OIDC access
 * token, an `hbt_pat_…`, or a service account token — and no login route, no session endpoint and
 * no redirect flow. So the application asks for a token, verifies it by reading
 * `GET /accounts/me` with it, and holds it behind the platform seam. Session management and the
 * OIDC connection are `0.6.0`, and they replace this file, the seam's `holdBearer`, and the screen
 * that calls it.
 *
 * **The token never appears anywhere but the field that accepts it.** It is not put in a URL, not
 * logged, not written into a message, and not stored anywhere this module does not own. The one
 * thing that reads it back is the engine, per request, through the seam.
 */

import type { Account } from '@hubtask/sync-engine';

import { engine, whenCredentialRefused } from './data/engine.ts';
import { messages } from './i18n/i18n.svelte.ts';
import { platform } from './platform/index.ts';
import { renderProblem, type RenderedProblem } from './problem.ts';

const ME = '/accounts/me';

export type SessionStatus = 'signed-out' | 'verifying' | 'signed-in';

class Session {
  /**
   * A token that is already held means somebody signed in and reloaded. It is believed until a
   * request says otherwise, which is what makes a reload cheap: the alternative is a verification
   * round trip in front of every first paint, for an answer the next request gives anyway.
   */
  #status = $state<SessionStatus>(platform.bearer() === undefined ? 'signed-out' : 'signed-in');
  #problem = $state<RenderedProblem | undefined>(undefined);
  #intended = $state<string | undefined>(undefined);
  /** A refusal *during* verification is the answer to the token, not the loss of a session. */
  #verifying = false;

  get status(): SessionStatus {
    return this.#status;
  }

  get isSignedIn(): boolean {
    return this.#status === 'signed-in';
  }

  /** Why the last attempt failed, as a sentence. Cleared by the next attempt. */
  get problem(): RenderedProblem | undefined {
    return this.#problem;
  }

  /** Where the reader was when the session ended, so that signing in again returns them to it. */
  get intendedPath(): string | undefined {
    return this.#intended;
  }

  /**
   * Verifies a token and keeps it if the server accepts it.
   *
   * The shape is deliberately not checked. `security.md` gives an exact pattern for a personal
   * access token, and a client that enforced it would reject the other two credentials the same
   * scheme accepts — an OIDC access token and a service account token. The server is the only
   * thing that knows, and asking it costs one request.
   */
  async signIn(token: string): Promise<boolean> {
    this.#problem = undefined;
    this.#status = 'verifying';
    this.#verifying = true;
    platform.holdBearer(token);

    try {
      const state = await engine.refresh<Account>({ path: ME });
      if (state.status === 'ready') {
        this.#status = 'signed-in';
        return true;
      }
      // Not a session: a token that was refused. Nothing is kept, including the read that failed.
      this.#problem = state.status === 'failed' ? renderProblem(state.error, messages) : undefined;
      this.#discard();
      return false;
    } finally {
      this.#verifying = false;
    }
  }

  /**
   * Ends the session on purpose.
   *
   * `offline-sync.md` §9.6 - "local storage is discarded completely on sign-out" - applies from
   * the first day there is anything to discard rather than from the day there is a lot, which is
   * why `engine.reset()` is here and not a to-do.
   */
  signOut(): void {
    this.#problem = undefined;
    this.#intended = undefined;
    this.#discard();
  }

  /**
   * The server refused the credential mid-session.
   *
   * The path is remembered before anything else happens, because it is the one thing that is lost
   * otherwise: signing in again should return the reader to what they were looking at, not to the
   * start. `errors.unauthenticated` is what they are told - the server's own code for it, rather
   * than a status code shown raw.
   */
  refused(at: string | undefined): void {
    if (this.#verifying || this.#status === 'signed-out') return;
    this.#intended = at;
    this.#problem = {
      message: messages.t('errors.unauthenticated'),
      fields: new Map(),
      isServerFault: false,
    };
    this.#discard();
  }

  /** Takes the remembered path, once. A path that navigated twice is a path that fights the reader. */
  takeIntendedPath(): string | undefined {
    const path = this.#intended;
    this.#intended = undefined;
    return path;
  }

  /** The token and everything read with it, in that order. */
  #discard(): void {
    platform.releaseBearer();
    engine.reset();
    this.#status = 'signed-out';
  }
}

export const session = new Session();

// The engine sees every refusal; this is where one becomes a decision. Registered at module load
// so that a `401` from a request nobody is watching still ends the session.
whenCredentialRefused(() => session.refused(location.pathname + location.search));
