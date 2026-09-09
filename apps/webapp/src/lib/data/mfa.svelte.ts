// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The second factor: enrolment, confirmation, and taking it off again (H-02).
 *
 * **The secret is shown once and this module holds it for exactly as long as the screen does.**
 * `POST /auth/mfa/totp:enroll` answers a base32 secret, a provisioning URI and ten recovery codes,
 * and there is no call that answers any of them a second time. So they live in memory here, are
 * dropped by `forget()` when the flow ends or the screen leaves, and are written to no storage
 * this module or any other owns.
 *
 * **Enrolling arms nothing.** The secret exists and sign-in is unchanged until a valid code
 * confirms it — an unconfirmed enrolment protects nobody and locks nobody out, which is what makes
 * a reader who closes the tab halfway through no worse off than before.
 *
 * **Two callers, one flow.** A signed-in person enrolling from their profile presents a bearer; an
 * administrator a tenant switch routes into enrolment presents the pending credential a `202`
 * answered instead. The routes take either, so this module takes either — and when the second one
 * confirms, the answer carries the session pair, because by then the person has proved both
 * factors.
 */

import { TransportError } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const ENROLL = '/auth/mfa/totp:enroll';
const CONFIRM = '/auth/mfa/totp:confirm';
const DISABLE = '/auth/mfa:disable';

/** The single showing, as `TotpEnrollment` answers it. */
export interface Enrollment {
  readonly secret: string;
  readonly otpauth_uri: string;
  readonly recovery_codes: readonly string[];
}

/** What `:confirm` answers. The pair is there only when a pending credential completed a sign-in. */
interface Confirmed {
  readonly armed: boolean;
  readonly tokens?: { readonly access_token: string; readonly refresh_token: string } | null;
}

class Mfa {
  #started = $state<Enrollment | undefined>(undefined);
  #failure = $state<string | undefined>(undefined);
  #working = $state(false);

  /** The secret and the codes, while the enrolment screen is showing them. */
  get enrollment(): Enrollment | undefined {
    return this.#started;
  }

  /** The last refusal, as the server's own code. */
  get failure(): string | undefined {
    return this.#failure;
  }

  get isWorking(): boolean {
    return this.#working;
  }

  /**
   * Mints the secret.
   *
   * Enrolling again before confirming replaces the unconfirmed secret, which is the server's rule
   * and is why this may be called twice without anything being cleaned up first. Enrolling while
   * armed is refused — disable first, with the password.
   */
  async start(pendingToken?: string): Promise<boolean> {
    return this.#attempt(async () => {
      this.#started = await engine.mutate<Enrollment>(
        'POST',
        ENROLL,
        pendingToken ? { pending_token: pendingToken } : {},
      );
      return true;
    });
  }

  /**
   * Arms it with one valid code, and answers the pair where the server sent one.
   *
   * `undefined` means it did not arm. A pair means this confirmation *was* the sign-in: the
   * enforcement path, where an unenrolled administrator was routed into enrolment instead of into
   * a session.
   */
  async confirm(
    code: string,
    pendingToken?: string,
  ): Promise<{ armed: boolean; tokens?: { access: string; refresh: string } } | undefined> {
    let answer: Confirmed | undefined;
    const ok = await this.#attempt(async () => {
      answer = await engine.mutate<Confirmed>('POST', CONFIRM, {
        code,
        ...(pendingToken ? { pending_token: pendingToken } : {}),
      });
      return true;
    });
    if (!ok || !answer) return undefined;

    // The secret has done its work. Held one moment longer than it is needed is one moment longer
    // than the contract's "shown for the only time" intends.
    this.#started = undefined;
    return {
      armed: answer.armed,
      tokens: answer.tokens
        ? { access: answer.tokens.access_token, refresh: answer.tokens.refresh_token }
        : undefined,
    };
  }

  /**
   * Takes it off, with the password afresh.
   *
   * The one case where "recently signed in" is not enough, because a stolen session removing the
   * second factor is exactly the attack the factor exists against (`security.md` §5). Under tenant
   * enforcement an `OWNER` or `ADMIN` cannot disable at all, and the refusal names the switch —
   * which is the sentence the screen renders rather than one this client invents.
   */
  async disable(password: string): Promise<boolean> {
    return this.#attempt(async () => {
      await engine.mutate('POST', DISABLE, { password });
      return true;
    });
  }

  /** Drops the secret and the codes. Called when the screen leaves, and after a confirmation. */
  forget(): void {
    this.#started = undefined;
    this.#failure = undefined;
  }

  async #attempt(work: () => Promise<boolean>): Promise<boolean> {
    this.#working = true;
    this.#failure = undefined;
    try {
      return await work();
    } catch (cause) {
      this.#failure = cause instanceof TransportError
        ? (cause.detailCode ?? cause.code ?? 'errors.internal')
        : 'errors.internal';
      return false;
    } finally {
      this.#working = false;
    }
  }
}

export const mfa = new Mfa();
