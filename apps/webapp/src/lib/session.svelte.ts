// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Who is signed in, and the four things that change it: a sign-in, a redeemed invitation, a
 * sign-out, and a refusal the seam could not exchange away.
 *
 * **What `0.6.0` actually brought, and what it did not.** H-01 minted a session: `POST
 * /auth/sessions` takes an email and a password and answers an access token of fifteen minutes
 * beside a refresh token of thirty days. What it did not bring is a session the browser holds and
 * script cannot read — every route in this contract takes a bearer, and `/auth/oidc:callback`
 * answers the same pair. So the pair goes where the token went, and `platform/tokenStore.ts`
 * carries the reasoning for that.
 *
 * **No credential appears anywhere but the field that accepts it.** Not in a URL, not in a log,
 * not written into a message, and not stored anywhere this module does not own. The one thing that
 * reads the bearer back is the engine, per request, through the seam; the one thing that reads the
 * refresh token is the engine's exchange.
 *
 * **The one refusal is rendered as the one refusal.** A wrong password and an address nobody holds
 * produce the same answer, byte for byte, because whether an account exists is exactly what a
 * guessing client is trying to learn (`security.md` T-02). This module adds nothing to it.
 */

import { TransportError } from '@hubtask/sync-engine';

import { engine, whenCredentialRefused } from './data/engine.ts';
import { live } from './data/live.svelte.ts';
import { messages } from './i18n/i18n.svelte.ts';
import { platform } from './platform/index.ts';
import { renderProblem, type RenderedProblem } from './problem.ts';

const SESSIONS = '/auth/sessions';
const REDEEM = '/auth/invitations:redeem';

export type SessionStatus = 'signed-out' | 'verifying' | 'signed-in';

/** The pair, as `SessionTokens` answers it. Narrowed to what this module uses. */
interface SessionTokens {
  readonly access_token: string;
  readonly refresh_token: string;
}

/**
 * The second step a two-step sign-in owes (H-02), as the `202` carries it.
 *
 * F4-03 does not complete one — the enrolment and the code screens are F4-04's — so what this
 * module does with it is hold it and say so. The alternative would be rendering the `202` as a
 * failure, which is the one thing it is not: the password was right.
 */
export interface SecondFactorOwed {
  readonly methods: readonly string[];
}

class Session {
  /**
   * A pair that is already held means somebody signed in and reloaded. It is believed until a
   * request says otherwise, which is what makes a reload cheap: the alternative is a verification
   * round trip in front of every first paint, for an answer the next request gives anyway.
   */
  #status = $state<SessionStatus>(platform.bearer() === undefined ? 'signed-out' : 'signed-in');
  #problem = $state<RenderedProblem | undefined>(undefined);
  #intended = $state<string | undefined>(undefined);
  #owed = $state<SecondFactorOwed | undefined>(undefined);
  /** A refusal *during* a sign-in is the answer to the credential, not the loss of a session. */
  #signingIn = false;

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

  /** The second factor a `202` asked for, where one was asked for. */
  get secondFactorOwed(): SecondFactorOwed | undefined {
    return this.#owed;
  }

  /** Where the reader was when the session ended, so that signing in again returns them to it. */
  get intendedPath(): string | undefined {
    return this.#intended;
  }

  /**
   * Signs in with an email and a password.
   *
   * `true` means there is a session. `false` means either a refusal — one sentence, the server's —
   * or a second factor owed, which `secondFactorOwed` then carries: the password was right, and
   * the difference matters to the screen even though both leave the caller signed out.
   */
  async signIn(email: string, password: string): Promise<boolean> {
    return this.#open(() =>
      engine.mutate<SessionTokens | SecondFactorAnswer>('POST', SESSIONS, { email, password }),
    );
  }

  /**
   * Redeems an invitation: the token from the mail, and the first password.
   *
   * It answers the same pair a sign-in answers, because the person has just proved control of the
   * mailbox — making them type the password again teaches nothing.
   */
  async redeem(token: string, password: string): Promise<boolean> {
    return this.#open(() => engine.mutate<SessionTokens>('POST', REDEEM, { token, password }));
  }

  /**
   * The one path both take: try, keep the pair if there is one, and say what happened.
   *
   * `engine.reset()` before the pair is held, not after: whatever the last session read is not
   * this session's to show, and a cache that survived a sign-in would show it to the wrong person.
   */
  async #open(attempt: () => Promise<SessionTokens | SecondFactorAnswer>): Promise<boolean> {
    this.#problem = undefined;
    this.#owed = undefined;
    this.#status = 'verifying';
    this.#signingIn = true;

    try {
      const answer = await attempt();
      if (isSecondFactorOwed(answer)) {
        this.#owed = { methods: answer.methods };
        this.#status = 'signed-out';
        return false;
      }

      engine.reset();
      platform.holdSession({ access: answer.access_token, refresh: answer.refresh_token });
      this.#status = 'signed-in';
      return true;
    } catch (error) {
      // Everything the engine throws is a `TransportError`; anything else would be a defect in
      // the seam rather than an answer, and `errors.internal` is what a reader is owed for one.
      this.#problem = error instanceof TransportError
        ? renderProblem(error, messages)
        : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
      this.#discard();
      return false;
    } finally {
      this.#signingIn = false;
    }
  }

  /**
   * Ends the session on purpose — here and at the server.
   *
   * The server call goes first and its failure is not reported: what a person pressed "sign out"
   * for is the credential being gone from *this* machine, and telling them it could not be ended
   * elsewhere while doing it here anyway would be a sentence they can do nothing with. The
   * session listing is where ending one elsewhere is a thing somebody can act on.
   *
   * `offline-sync.md` §9.6 — "local storage is discarded completely on sign-out" — is why
   * `engine.reset()` is in `#discard` and not a to-do.
   */
  async signOut(): Promise<void> {
    this.#problem = undefined;
    this.#intended = undefined;
    this.#owed = undefined;
    try {
      // Which session is this one is the server's answer rather than something the client keeps:
      // the pair survives a reload and a `session_id` beside it would be a third thing to store
      // and to keep in step with the exchange. The listing marks the current one, and it is one
      // read on a screen nobody is waiting on.
      const listed = await engine.refresh<readonly ListedSession[]>({ path: SESSIONS });
      const current = listed.status === 'ready' ? listed.data.find((one) => one.current) : undefined;
      if (current) await engine.mutate('DELETE', `${SESSIONS}/${current.id}`, undefined);
    } catch {
      // Already expired, already revoked, or unreachable. All three end the same way here.
    }
    this.#discard();
  }

  /**
   * The server refused the credential and the seam could not exchange it away.
   *
   * The path is remembered before anything else happens, because it is the one thing that is lost
   * otherwise: signing in again should return the reader to what they were looking at, not to the
   * start. `errors.unauthenticated` is what they are told — the server's own code for it, rather
   * than a status code shown raw.
   */
  refused(at: string | undefined): void {
    if (this.#signingIn || this.#status === 'signed-out') return;
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

  /**
   * The credentials and everything read with them, in that order.
   *
   * The stream goes first. A connection left open over a cleared cache would deliver records into
   * nothing and hold a credential the reader has just given up — and the per-credential cap counts
   * connections, not intentions.
   */
  #discard(): void {
    live.stop();
    platform.releaseBearer();
    engine.reset();
    this.#status = 'signed-out';
  }
}

/** One row of `GET /auth/sessions`, narrowed to what ending one's own needs. */
interface ListedSession {
  readonly id: string;
  readonly current: boolean;
}

/** The `202` shape, as far as telling it apart from the `201` needs. */
interface SecondFactorAnswer {
  readonly pending_token: string;
  readonly methods: readonly string[];
}

/**
 * Which of the two the server answered.
 *
 * By the field rather than by the status code: `mutate` answers a body and not a response, and
 * the two bodies are unmistakable — one carries an access token, the other a pending credential
 * that can do nothing but complete this sign-in.
 */
function isSecondFactorOwed(
  answer: SessionTokens | SecondFactorAnswer,
): answer is SecondFactorAnswer {
  return typeof (answer as SecondFactorAnswer).pending_token === 'string';
}

export const session = new Session();

// The engine sees every refusal it could not exchange away; this is where one becomes a decision.
// Registered at module load so that a `401` from a request nobody is watching still ends the
// session.
whenCredentialRefused(() => session.refused(location.pathname + location.search));
