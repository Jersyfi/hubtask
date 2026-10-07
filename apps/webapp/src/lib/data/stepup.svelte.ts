// SPDX-License-Identifier: Apache-2.0
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
 *
 * **The provider is the one method that leaves the page** (ADR-0075 §2). The browser goes to the
 * provider for a fresh sign-in and comes back to the callback, a fresh document: so where the person
 * was is written down before leaving, and the grant the return earns is held in the tab until the
 * one action it is for takes it. That is the only grant ever stored, and only for its few minutes.
 */

import { TransportError } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import type { Handoff } from './oidc.ts';
import { navigableUrl } from './oidc.ts';
import {
  forgetReturn,
  heldGrant,
  holdGrant,
  isReturning,
  rememberReturn,
  takeGrant,
  takeReturn,
  withStepUp,
  type HeldGrant,
  type StepUpMethod,
} from './stepup.ts';

const STEP_UP = '/auth/step-up';
const STEP_UP_PROVIDER = '/auth/step-up:provider';

/** The tab's own storage, or a map where a browser refuses it - private windows, embedded views. */
function tabStore(): Pick<Storage, 'getItem' | 'setItem' | 'removeItem'> {
  try {
    if (typeof sessionStorage !== 'undefined') return sessionStorage;
  } catch {
    // Refused: fall through to memory.
  }
  const held = new Map<string, string>();
  return {
    getItem: (key) => held.get(key) ?? null,
    setItem: (key, value) => void held.set(key, value),
    removeItem: (key) => void held.delete(key),
  };
}

const store = tabStore();

/** How long the proof itself may take. Somebody is looking at a dialog while it runs. */
const TIMEOUT_MS = 20_000;

interface Grant {
  readonly step_up_token: string;
  readonly expires_at: string;
}

/** Where to send the browser for a `PROVIDER` proof, as `ProviderStepUpAuthorization` answers it. */
interface ProviderStart {
  readonly authorization_url: string;
  readonly provider_name: string;
}

/**
 * What the dialog is being asked for: the methods the refusal named, and the two answers.
 *
 * A promise-shaped handle rather than a callback: the caller that met the refusal is the one that
 * has to retry, and `await`ing the answer is what keeps the retry beside the call it retries.
 */
interface Pending {
  readonly methods: readonly StepUpMethod[];
  /** The provider a `PROVIDER` proof goes to, by the name the refusal gave it. */
  readonly provider?: string;
  readonly settle: (token: string | undefined) => void;
}

class StepUp {
  #pending = $state<Pending | undefined>(undefined);
  #failure = $state<string | undefined>(undefined);
  #working = $state(false);
  /** The grant the provider's return earned, while it is held: what the frame's note says. */
  #confirmed = $state<HeldGrant | undefined>(heldGrant(store, Date.now()));

  /** The confirmation that holds, for the note that says the action can be taken now. */
  get confirmed(): HeldGrant | undefined {
    return this.#confirmed;
  }

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
    return withStepUp(call, (methods, provider) => this.#ask(methods, provider), () => this.#take());
  }

  /** The held grant, consumed by the call about to be made. */
  #take(): string | undefined {
    const token = takeGrant(store, Date.now());
    this.#confirmed = undefined;
    return token;
  }

  /** Opens the prompt and waits for it. `undefined` means the reader closed it. */
  #ask(methods: readonly StepUpMethod[], provider?: string): Promise<string | undefined> {
    this.#failure = undefined;
    return new Promise<string | undefined>((resolve) => {
      this.#pending = { methods, provider, settle: resolve };
    });
  }

  /**
   * Proves it, and hands the grant to whoever is waiting.
   *
   * Exactly one method: the contract's `StepUpRequest` says so, and sending two would be asking the
   * server to choose which credential to check.
   */
  async prove(answer: { password?: string; code?: string; recoveryCode?: string }): Promise<void> {
    const waiting = this.#pending;
    if (!waiting) return;

    this.#working = true;
    this.#failure = undefined;
    try {
      const body = answer.code
        ? { code: answer.code }
        : answer.recoveryCode
          ? { recovery_code: answer.recoveryCode }
          : { password: answer.password };
      const grant = await engine.mutate<Grant>('POST', STEP_UP, body, { timeoutMs: TIMEOUT_MS });
      this.#pending = undefined;
      waiting.settle(grant.step_up_token);
    } catch (cause) {
      // The dialog stays open. A wrong password here is a retry, not the end of the operation -
      // and the server's own code is what the screen renders, because "which of the two was
      // wrong" is not something this client gets to say.
      this.#failure = codeOf(cause);
    } finally {
      this.#working = false;
    }
  }

  /**
   * Sends the browser to the provider for a fresh sign-in (ADR-0075 §2).
   *
   * The operation waiting on the prompt is not retried here: the page is about to unload. The person
   * comes back to where they were, with the grant held, and does it again.
   */
  async confirmAtProvider(): Promise<void> {
    if (!this.#pending) return;
    this.#working = true;
    this.#failure = undefined;
    try {
      const started = await engine.mutate<ProviderStart>('POST', STEP_UP_PROVIDER, undefined, {
        timeoutMs: TIMEOUT_MS,
      });
      const url = navigableUrl(started.authorization_url);
      if (url === undefined) {
        this.#failure = 'errors.internal';
        return;
      }
      rememberReturn(store, location.pathname + location.search, started.provider_name, Date.now());
      location.assign(url);
    } catch (cause) {
      this.#failure = codeOf(cause);
    } finally {
      this.#working = false;
    }
  }

  /** Whether this callback is the return of a step-up rather than a sign-in. Read before completing. */
  isReturning(): boolean {
    return isReturning(store, Date.now());
  }

  /**
   * Forgets everything this tab holds of a step-up: the note of a trip to the provider and a grant
   * the return earned. Called at sign-out, because both belong to the session that is ending.
   */
  forget(): void {
    forgetReturn(store);
    takeGrant(store, Date.now());
    this.#confirmed = undefined;
  }

  /**
   * Finishes a step-up at the provider with what the callback carried, holds the grant, and answers
   * where the person was. `undefined` is a refusal, which `failure` names; the way back is answered
   * through `returnTo` either way, so the callback can offer it.
   */
  async completeAtProvider(handoff: Handoff): Promise<{ ok: boolean; returnTo: string }> {
    const back = takeReturn(store);
    const returnTo = back?.returnTo ?? '/';
    this.#working = true;
    this.#failure = undefined;
    try {
      const grant = await engine.mutate<Grant>(
        'POST',
        STEP_UP,
        { state: handoff.state, authorization_code: handoff.code },
        { timeoutMs: TIMEOUT_MS },
      );
      const held = { token: grant.step_up_token, expiresAt: grant.expires_at, provider: back?.provider ?? '' };
      holdGrant(store, held);
      this.#confirmed = held;
      return { ok: true, returnTo };
    } catch (cause) {
      this.#failure = codeOf(cause);
      return { ok: false, returnTo };
    } finally {
      this.#working = false;
    }
  }

  /**
   * The provider sent the person back without a code - they cancelled there, or it declined. The
   * step-up did not happen; the way back is answered so the callback can offer it.
   */
  abandonAtProvider(): string {
    this.#failure = 'auth.oidc_failed';
    return takeReturn(store)?.returnTo ?? '/';
  }

  /** The reader closed the prompt. The operation they were attempting does not happen. */
  cancel(): void {
    const waiting = this.#pending;
    this.#pending = undefined;
    this.#failure = undefined;
    waiting?.settle(undefined);
  }
}

function codeOf(cause: unknown): string {
  return cause instanceof TransportError
    ? (cause.detailCode ?? cause.code ?? 'errors.internal')
    : 'errors.internal';
}

export const stepUp = new StepUp();
