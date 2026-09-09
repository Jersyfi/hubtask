// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The proof before the irreversible (H-03, `security.md` §5).
 *
 * **A refusal, not a screen.** Any request may meet `403 auth.step_up_required` — granting an
 * `OWNER` role, minting a token with an admin scope, a destructive restore — and the recovery is
 * always the same three steps: prove yourself again, take the grant, present it on the very same
 * request. That is why this is a wrapper around a call rather than a control on a screen: a screen
 * that knew in advance which of its buttons need a proof would be a screen guessing at the
 * server's policy, and guessing wrong the day the policy moves.
 *
 * **The grant is consumed by one action.** The contract says so, and this module obeys it by
 * construction: the token is handed to exactly one retry and never stored. A second privileged
 * action meets the refusal again and asks again, which is the point rather than a friction to
 * smooth away.
 *
 * **The credential never leaves the dialog.** The password or the code goes from the field to
 * `POST /auth/step-up` and nowhere else; what comes back is a grant, which is not a credential for
 * anything but the one call.
 */

import { TransportError } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { withStepUp, type StepUpMethod } from './stepup.ts';

const STEP_UP = '/auth/step-up';

/** How long the proof itself may take. Somebody is looking at a dialog while it runs. */
const TIMEOUT_MS = 20_000;

interface Grant {
  readonly step_up_token: string;
}

/**
 * What the dialog is being asked for: the methods the refusal named, and the two answers.
 *
 * A promise-shaped handle rather than a callback: the caller that met the refusal is the one that
 * has to retry, and `await`ing the answer is what keeps the retry beside the call it retries.
 */
interface Pending {
  readonly methods: readonly StepUpMethod[];
  readonly settle: (token: string | undefined) => void;
}

class StepUp {
  #pending = $state<Pending | undefined>(undefined);
  #failure = $state<string | undefined>(undefined);
  #working = $state(false);

  /** The prompt to render, or nothing. */
  get pending(): Pending | undefined {
    return this.#pending;
  }

  /** Why the last proof was refused, as the server's own code. */
  get failure(): string | undefined {
    return this.#failure;
  }

  get isWorking(): boolean {
    return this.#working;
  }

  /**
   * Runs a call, and where it is refused for want of a proof, asks for one and runs it again.
   *
   * Once. A second `auth.step_up_required` after a fresh grant is the server saying the grant was
   * not what it wanted, and asking a third time would be a dialog somebody cannot escape.
   */
  async around<T>(call: (stepUpToken?: string) => Promise<T>): Promise<T> {
    return withStepUp(call, (methods) => this.#ask(methods));
  }

  /** Opens the prompt and waits for it. `undefined` means the reader closed it. */
  #ask(methods: readonly StepUpMethod[]): Promise<string | undefined> {
    this.#failure = undefined;
    return new Promise<string | undefined>((resolve) => {
      this.#pending = { methods, settle: resolve };
    });
  }

  /**
   * Proves it, and hands the grant to whoever is waiting.
   *
   * One of the two, never both: the contract's `StepUpRequest` says so, and sending both would be
   * asking the server to choose which credential to check.
   */
  async prove(answer: { password?: string; code?: string }): Promise<void> {
    const waiting = this.#pending;
    if (!waiting) return;

    this.#working = true;
    this.#failure = undefined;
    try {
      const grant = await engine.mutate<Grant>(
        'POST',
        STEP_UP,
        answer.code ? { code: answer.code } : { password: answer.password },
        { timeoutMs: TIMEOUT_MS },
      );
      this.#pending = undefined;
      waiting.settle(grant.step_up_token);
    } catch (cause) {
      // The dialog stays open. A wrong password here is a retry, not the end of the operation -
      // and the server's own code is what the screen renders, because "which of the two was
      // wrong" is not something this client gets to say.
      this.#failure = cause instanceof TransportError
        ? (cause.detailCode ?? cause.code ?? 'errors.internal')
        : 'errors.internal';
    } finally {
      this.#working = false;
    }
  }

  /** The reader closed the prompt. The operation they were attempting does not happen. */
  cancel(): void {
    const waiting = this.#pending;
    this.#pending = undefined;
    this.#failure = undefined;
    waiting?.settle(undefined);
  }
}

export const stepUp = new StepUp();
