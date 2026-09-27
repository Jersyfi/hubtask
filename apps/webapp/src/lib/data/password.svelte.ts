// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The three moments a password moves: asked for, reset, changed.
 *
 * **One answer, whatever is true.** `password:forgot` answers `202` for an address that holds an
 * account and for one that does not, and this module renders the same sentence for both: which
 * addresses have accounts is exactly what a probe is trying to learn (T-02). There is no error
 * state here for "no such account", because there is no such answer.
 *
 * **A reset is not automatically a session.** Where the account has a second factor, the server
 * answers `202` rather than `201` and the sign-in screen becomes the second step: control of a
 * mailbox is one proof, and it does not replace the one the account already demanded.
 *
 * **Changing a password ends the other sessions.** That is the server's doing and this module only
 * says so; the sentence is the one thing a person needs, because they are about to find their
 * other browser signed out.
 */

import { TransportError } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { messages } from '../i18n/i18n.svelte.ts';
import { renderProblem, type RenderedProblem } from '../problem.ts';
import { session } from '../session.svelte.ts';

const FORGOT = '/auth/password:forgot';
const RESET = '/auth/password:reset';
const CHANGE = '/auth/password';

/** The pair, or the second step a reset still owes. Narrowed to what this module uses. */
interface ResetAnswer {
  readonly access_token?: string;
  readonly refresh_token?: string;
  readonly pending_token?: string;
  readonly methods?: readonly string[];
}

class Password {
  #working = $state(false);
  #sent = $state(false);
  #changed = $state(false);
  #problem = $state<RenderedProblem | undefined>(undefined);

  get isWorking(): boolean {
    return this.#working;
  }

  /** Whether the "if an account exists" sentence is on the screen. */
  get wasSent(): boolean {
    return this.#sent;
  }

  get wasChanged(): boolean {
    return this.#changed;
  }

  get problem(): RenderedProblem | undefined {
    return this.#problem;
  }

  clear(): void {
    this.#sent = false;
    this.#changed = false;
    this.#problem = undefined;
  }

  /**
   * Asks for a link.
   *
   * A transport failure is the one thing worth saying, because it means the request did not
   * arrive; everything else is the same `202` and the same sentence.
   */
  async forget(email: string): Promise<void> {
    this.#working = true;
    this.#problem = undefined;
    try {
      await engine.mutate<void>('POST', FORGOT, { email });
      this.#sent = true;
    } catch (cause) {
      this.#problem = this.#render(cause);
    } finally {
      this.#working = false;
    }
  }

  /**
   * Spends the token from the mail and sets the password.
   *
   * `true` means there is a session. `false` means either a refusal, or a second factor the
   * session store now carries - the same shape a password sign-in has, because it is the same
   * machine.
   */
  async reset(token: string, newPassword: string): Promise<boolean> {
    this.#working = true;
    this.#problem = undefined;
    try {
      const answer = await engine.mutate<ResetAnswer>('POST', RESET, { token, password: newPassword });
      if (answer.access_token && answer.refresh_token) {
        session.hold({ access: answer.access_token, refresh: answer.refresh_token });
        return true;
      }
      if (answer.pending_token) {
        session.owe(answer.pending_token, answer.methods ?? []);
        return false;
      }
      this.#problem = { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
      return false;
    } catch (cause) {
      this.#problem = this.#render(cause);
      return false;
    } finally {
      this.#working = false;
    }
  }

  /**
   * Changes the password of the signed-in account, behind the step-up the server demands.
   *
   * The step-up is the proof of the old password, which is why there is no "current password"
   * field: asking for it beside a step-up would be asking twice for one thing (3.3.7).
   */
  async change(newPassword: string, stepUpToken?: string): Promise<boolean> {
    this.#working = true;
    this.#problem = undefined;
    this.#changed = false;
    try {
      await engine.mutate<void>('POST', CHANGE, {
        password: newPassword,
        ...(stepUpToken ? { step_up_token: stepUpToken } : {}),
      });
      this.#changed = true;
      return true;
    } catch (cause) {
      this.#problem = this.#render(cause);
      return false;
    } finally {
      this.#working = false;
    }
  }

  #render(cause: unknown): RenderedProblem {
    return cause instanceof TransportError
      ? renderProblem(cause, messages)
      : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
  }
}

export const password = new Password();
