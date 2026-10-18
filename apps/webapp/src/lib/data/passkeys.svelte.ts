// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The passkeys and security keys of the reader's own account (identity.md §23, UC-ID-22): the list,
 * adding one, renaming one, removing one.
 *
 * **Adding is proof, then device, then name kept.** `POST /accounts/me/passkeys:start` asks for the
 * step-up - `stepUp.around` answers it - and returns the options; the browser asks the device; the
 * answer goes to `:complete` with the name the person typed. A prompt the person closes is their
 * choice and says nothing (P-11); the server's challenge simply lapses.
 *
 * **Every write re-reads the account and the list** (`/accounts/me` covers both): a passkey is a
 * second factor, so adding or removing one moves the account's `has_second_factor`, its codes and
 * whether the authenticator app may go - and a screen still holding the old answer would offer a
 * removal the server now refuses (P-05).
 *
 * **The recovery codes of a first factor are shown once and held only here**, for exactly as long as
 * the screen shows them - the second factor's reasoning (`mfa.svelte.ts`).
 */

import { TransportError, type PasskeyEnrolled, type PasskeyList, type PasskeyRegistered, type PasskeyRegistrationOptions, type ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { stepUp } from './stepup.svelte.ts';
import { ceremonyEndOf, creationOptionsOf, registrationOf, type CreatedCredential } from './passkeys.ts';
import { messages } from '../i18n/i18n.svelte.ts';
import { renderProblem, type RenderedProblem } from '../problem.ts';

const PATH = '/accounts/me/passkeys';
const START = '/accounts/me/passkeys:start';
const COMPLETE = '/accounts/me/passkeys:complete';
/** The setup during sign-in, with the ENROLL step's credential instead of a session (identity.md §23.2). */
const ENROLL_OPTIONS = '/auth/mfa/passkey:enroll-options';
const ENROLL = '/auth/mfa/passkey:enroll';
/** What a passkey's write makes stale: the account and its passkeys, both under this prefix. */
const ACCOUNT = '/accounts/me';

/** How long a step may take. Somebody is waiting on it; the device's own wait is the browser's. */
const TIMEOUT_MS = 20_000;

/** How an attempt to add one ended. */
export type Added = 'added' | 'cancelled' | 'refused';

class Passkeys {
  #state = $state<ResourceState<PasskeyList>>({ status: 'idle' });
  #failure = $state<RenderedProblem | undefined>(undefined);
  #working = $state(false);
  #fresh = $state<readonly string[] | undefined>(undefined);

  /** The list as answered, or nothing while unread or refused. */
  get list(): PasskeyList | undefined {
    return this.#state.status === 'ready' ? this.#state.data : undefined;
  }

  /** Why the last step was refused, in words; nothing after a cancelled prompt. */
  get failure(): RenderedProblem | undefined {
    return this.#failure;
  }

  get isWorking(): boolean {
    return this.#working;
  }

  /** The ten recovery codes a first factor answered, while the screen shows them. */
  get fresh(): readonly string[] | undefined {
    return this.#fresh;
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<PasskeyList>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }

  /**
   * Adds one: the proof, the device, the name kept. `create` is the browser's
   * `navigator.credentials.create`, handed in so a test can stand in for the device.
   */
  async add(
    name: string,
    create: (options: CredentialCreationOptions) => Promise<Credential | null>,
  ): Promise<Added> {
    this.#working = true;
    this.#failure = undefined;
    try {
      const options = await stepUp.around((stepUpToken) =>
        engine.mutate<PasskeyRegistrationOptions>('POST', START, {}, {
          stepUpToken,
          timeoutMs: TIMEOUT_MS,
          invalidates: [],
        }),
      );
      let created: Credential | null;
      try {
        created = await create({ publicKey: creationOptionsOf(options) });
      } catch (cause) {
        const end = ceremonyEndOf(cause);
        if (end === 'cancelled') return 'cancelled';
        this.#failure = ownCode(end === 'held' ? 'app.passkeys.already_held' : 'app.passkeys.device_failed');
        return 'refused';
      }
      if (created === null) return 'cancelled';
      const answer = await engine.mutate<PasskeyRegistered>(
        'POST',
        COMPLETE,
        registrationOf(created as unknown as CreatedCredential, name),
        { timeoutMs: TIMEOUT_MS, invalidates: [ACCOUNT] },
      );
      if (answer.recovery_codes && answer.recovery_codes.length > 0) this.#fresh = answer.recovery_codes;
      return 'added';
    } catch (cause) {
      this.#failure = cause instanceof TransportError ? renderProblem(cause, messages) : ownCode('app.something_went_wrong', true);
      return 'refused';
    } finally {
      this.#working = false;
    }
  }

  /**
   * Sets one up during sign-in: the options with the step's credential, the device, the key kept.
   * The answer carries the codes where the account held none, and the pair or the step still owed -
   * all handed back, because the codes are shown before the session opens.
   */
  async enroll(
    pendingToken: string,
    name: string,
    create: (options: CredentialCreationOptions) => Promise<Credential | null>,
  ): Promise<{ readonly end: Added; readonly answer?: PasskeyEnrolled }> {
    this.#working = true;
    this.#failure = undefined;
    try {
      const options = await engine.mutate<PasskeyRegistrationOptions>(
        'POST', ENROLL_OPTIONS, { pending_token: pendingToken }, { timeoutMs: TIMEOUT_MS, invalidates: [] },
      );
      let created: Credential | null;
      try {
        created = await create({ publicKey: creationOptionsOf(options) });
      } catch (cause) {
        const end = ceremonyEndOf(cause);
        if (end === 'cancelled') return { end: 'cancelled' };
        this.#failure = ownCode(end === 'held' ? 'app.passkeys.already_held' : 'app.passkeys.device_failed');
        return { end: 'refused' };
      }
      if (created === null) return { end: 'cancelled' };
      const answer = await engine.mutate<PasskeyEnrolled>(
        'POST',
        ENROLL,
        { pending_token: pendingToken, ...registrationOf(created as unknown as CreatedCredential, name) },
        { timeoutMs: TIMEOUT_MS, invalidates: [] },
      );
      return { end: 'added', answer };
    } catch (cause) {
      this.#failure = cause instanceof TransportError ? renderProblem(cause, messages) : ownCode('app.something_went_wrong', true);
      return { end: 'refused' };
    } finally {
      this.#working = false;
    }
  }

  /** Renames one with the session alone; true where it was. */
  async rename(id: string, name: string): Promise<boolean> {
    return this.#write(() =>
      engine.mutate('PATCH', `${PATH}/${encodeURIComponent(id)}`, { name }, { timeoutMs: TIMEOUT_MS, invalidates: [PATH] }),
    );
  }

  /** Removes one behind a fresh proof; true where it was. */
  async remove(id: string): Promise<boolean> {
    return this.#write(() =>
      stepUp.around((stepUpToken) =>
        engine.mutate('DELETE', `${PATH}/${encodeURIComponent(id)}`, undefined, {
          stepUpToken,
          timeoutMs: TIMEOUT_MS,
          invalidates: [ACCOUNT],
        }),
      ),
    );
  }

  /** Reads the list again, for a write elsewhere on the screen that invalidated nothing. */
  async reread(): Promise<void> {
    await engine.refresh<PasskeyList>({ path: PATH });
  }

  async #write(run: () => Promise<unknown>): Promise<boolean> {
    this.#working = true;
    this.#failure = undefined;
    try {
      await run();
      return true;
    } catch (cause) {
      // A proof the person declined to give is their choice and says nothing (P-11).
      if (cause instanceof TransportError && cause.needsStepUp) return false;
      this.#failure = cause instanceof TransportError ? renderProblem(cause, messages) : ownCode('app.something_went_wrong', true);
      return false;
    } finally {
      this.#working = false;
    }
  }

  /** Drops the codes and the sentence: the screen moved on. */
  forget(): void {
    this.#fresh = undefined;
    this.#failure = undefined;
  }
}

/** A sentence this module says itself, in the shape the server's are drawn in. */
function ownCode(code: string, isServerFault = false): RenderedProblem {
  return { message: messages.t(code, {}), fields: new Map(), isServerFault };
}

export const passkeys = new Passkeys();
