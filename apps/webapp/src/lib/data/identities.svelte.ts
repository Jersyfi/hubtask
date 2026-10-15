// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The sign-in providers connected to the reader's own account, and connecting another from here
 * (ADR-0078 §1, UC-ID-10 check 8).
 *
 * **Connecting leaves the page twice at most.** The server asks a fresh proof of the account first -
 * `stepUp.around` answers it, and an account that proves itself through its own provider leaves for
 * that one and comes back - then answers where to send the browser for a fresh sign-in at the new
 * provider. The tab notes that it left for a connection, so the callback finishes this rather than a
 * sign-in, and sends the person back to their security settings.
 */

import { TransportError, type ConnectedIdentity, type ProviderConnectAuthorization, type ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import type { Handoff } from './oidc.ts';
import { navigableUrl } from './oidc.ts';
import { stepUp } from './stepup.svelte.ts';
import { forgetConnect, isConnecting, rememberConnect, type Store } from './identities.ts';
import { messages } from '../i18n/i18n.svelte.ts';
import { renderProblem, type RenderedProblem } from '../problem.ts';

const PATH = '/accounts/me/identities';
const CONNECT = '/accounts/me/identities:connect';
const COMPLETE = '/accounts/me/identities:complete';

/** Where the person comes back to once the connection is settled. */
export const SECURITY_PATH = '/profile/security';

/** How long a step of the connection may take. Somebody is waiting on it. */
const TIMEOUT_MS = 20_000;

/** The tab's own storage, or a map where a browser refuses it. */
function tabStore(): Store {
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

class Identities {
  #state = $state<ResourceState<readonly ConnectedIdentity[]>>({ status: 'idle' });
  #failure = $state<RenderedProblem | undefined>(undefined);
  #working = $state(false);

  /** The connections, oldest first; none while unread or refused. */
  get connected(): readonly ConnectedIdentity[] {
    return this.#state.status === 'ready' && Array.isArray(this.#state.data) ? this.#state.data : [];
  }

  /** Whether the list has been answered: until then nothing is offered to connect. */
  get isRead(): boolean {
    return this.#state.status === 'ready';
  }

  /** Why the last step was refused: the server's own sentence. */
  get failure(): RenderedProblem | undefined {
    return this.#failure;
  }

  get isWorking(): boolean {
    return this.#working;
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<readonly ConnectedIdentity[]>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }

  /**
   * Proves the account, then sends the browser to the provider for a fresh sign-in. The page is
   * about to unload on success; a refusal stays on this screen with its sentence.
   */
  async connect(providerId: string): Promise<void> {
    this.#working = true;
    this.#failure = undefined;
    try {
      const started = await stepUp.around((stepUpToken) =>
        engine.mutate<ProviderConnectAuthorization>('POST', CONNECT, { provider_id: providerId }, {
          stepUpToken,
          timeoutMs: TIMEOUT_MS,
          invalidates: [],
        }),
      );
      const url = navigableUrl(started.authorization_url);
      if (url === undefined) {
        this.#failure = ownCode('app.something_went_wrong', true);
        return;
      }
      rememberConnect(store, Date.now());
      location.assign(url);
    } catch (cause) {
      this.#failure = problemOf(cause);
    } finally {
      this.#working = false;
    }
  }

  /** Whether this callback is the return of a connection. Read before completing. */
  isReturning(): boolean {
    return isConnecting(store, Date.now());
  }

  /** Forgets a trip to the provider: the tab signed out, or the return was a sign-in after all. */
  forget(): void {
    forgetConnect(store);
  }

  /** Finishes the connection with what the callback carried. False is a refusal, which `failure` names. */
  async complete(handoff: Handoff): Promise<boolean> {
    forgetConnect(store);
    this.#working = true;
    this.#failure = undefined;
    try {
      await engine.mutate<ConnectedIdentity>(
        'POST',
        COMPLETE,
        { state: handoff.state, authorization_code: handoff.code },
        { timeoutMs: TIMEOUT_MS, invalidates: [PATH] },
      );
      return true;
    } catch (cause) {
      this.#failure = problemOf(cause);
      return false;
    } finally {
      this.#working = false;
    }
  }

  /** The provider sent the person back without a code: nothing was connected. */
  abandon(): void {
    forgetConnect(store);
    this.#failure = ownCode('auth.oidc_failed');
  }
}

function problemOf(cause: unknown): RenderedProblem {
  return cause instanceof TransportError
    ? renderProblem(cause, messages)
    : ownCode('app.something_went_wrong', true);
}

/** A sentence this module says itself, in the shape the server's are drawn in. */
function ownCode(code: string, isServerFault = false): RenderedProblem {
  return { message: messages.t(code, {}), fields: new Map(), isServerFault };
}

export const identities = new Identities();
