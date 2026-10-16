// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The passkeys and security keys of the reader's own account (identity.md §23, UC-ID-22): the list,
 * and adding one.
 *
 * **Adding is proof, then device, then name kept.** `POST /accounts/me/passkeys:start` asks for the
 * step-up - `stepUp.around` answers it - and returns the options; the browser asks the device; the
 * answer goes to `:complete` with the name the person typed. A prompt the person closes is their
 * choice and says nothing (P-11); the server's challenge simply lapses.
 *
 * **The recovery codes of a first factor are shown once and held only here**, for exactly as long as
 * the screen shows them - the second factor's reasoning (`mfa.svelte.ts`).
 */

import { TransportError, type PasskeyList, type PasskeyRegistered, type PasskeyRegistrationOptions, type ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { stepUp } from './stepup.svelte.ts';
import { ceremonyEndOf, creationOptionsOf, registrationOf, type CreatedCredential } from './passkeys.ts';
import { messages } from '../i18n/i18n.svelte.ts';
import { renderProblem, type RenderedProblem } from '../problem.ts';

const PATH = '/accounts/me/passkeys';
const START = '/accounts/me/passkeys:start';
const COMPLETE = '/accounts/me/passkeys:complete';

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
        { timeoutMs: TIMEOUT_MS, invalidates: [PATH] },
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
