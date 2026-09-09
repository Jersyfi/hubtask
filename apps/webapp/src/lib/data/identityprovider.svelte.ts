// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace's identity provider, as the administration screen reads and writes it (H-04).
 *
 * **The client secret goes one way.** It is a member of the configuration and of no answer: sealed
 * on the way in (E-02) and read only by the token exchange. So there is nothing here that holds
 * one, nothing that reads one back, and a screen that showed a row of dots where a secret used to
 * be would be inventing a fact the server does not offer.
 *
 * **A provider is set whole.** `PUT` rather than a patch, which is the contract's shape and the
 * safe one: discovery runs against the issuer before anything is stored, and a half-changed
 * configuration is a workspace whose sign-in is broken in a way nobody asked for.
 *
 * **Not configured is not a failure.** The read answers `404` where a workspace has never set one,
 * and that is an empty screen with a form on it rather than an error — which is why the state is
 * handed out whole and the screen tells the two apart.
 */

import type { IdentityProvider, IdentityProviderConfiguration, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/identity-provider';

class IdentityProviderStore {
  #state = $state<ResourceState<IdentityProvider>>({ status: 'idle' });

  /** What the last read answered: loading, the configuration, or the refusal. */
  get state(): ResourceState<IdentityProvider> {
    return this.#state;
  }

  /** The configuration, where there is one. */
  get provider(): IdentityProvider | undefined {
    return this.#state.status === 'ready' ? this.#state.data : undefined;
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<IdentityProvider>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }

  /** Sets it, whole. The read is invalidated, so the screen sees what the server stored. */
  async configure(body: IdentityProviderConfiguration): Promise<IdentityProvider> {
    return engine.mutate<IdentityProvider>('PUT', PATH, body, { invalidates: [PATH] });
  }

  /**
   * Takes it away.
   *
   * The accounts it provisioned keep their rows and their live sessions; what they lose is the way
   * to sign in again. The screen says that before it offers the button — the server audits the
   * answer, and a record of something nobody understood is a record of a surprise.
   */
  async remove(): Promise<void> {
    await engine.mutate('DELETE', PATH, undefined, { invalidates: [PATH] });
  }
}

export const identityProvider = new IdentityProviderStore();
export { PATH as identityProviderPath };
