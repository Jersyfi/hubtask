// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The providers a workspace signs its people in through, as the administration screen reads and
 * writes them (H-04, SI-10).
 *
 * **Plural, and two levels deep.** The listing carries the workspace's own rows and the ones its
 * installation offers every workspace. `scope` says which: an `installation` row is offered here and
 * is not this workspace's to change, so the screen draws no controls for it at all rather than
 * controls that answer a refusal.
 *
 * **The client secret goes one way.** It is a member of the configuration and of no answer: sealed
 * on the way in (E-02) and read only by the token exchange. So there is nothing here that holds one,
 * nothing that reads one back, and a screen that showed a row of dots where a secret used to be
 * would be inventing a fact the server does not offer. Omitting it on a replace keeps the one that
 * is sealed — which is the contract's promise and not a rule this module invents.
 *
 * **A provider is set whole.** `PUT` rather than a patch, which is the contract's shape and the safe
 * one: discovery runs against the issuer before anything is stored, and a half-changed configuration
 * is a workspace whose sign-in is broken in a way nobody asked for.
 *
 * **The presets are read once and are not a provider.** They are three rows and this installation's
 * own callback address — what a registration form asks for — so they live beside the listing rather
 * than inside it.
 */

import type {
  IdentityProvider,
  IdentityProviderConfiguration,
  IdentityProviderPreset,
  ResourceState,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/identity-providers';
const PRESETS = '/identity-provider-presets';

class IdentityProviderStore {
  #state = $state<ResourceState<readonly IdentityProvider[]>>({ status: 'idle' });
  #presets = $state<ResourceState<readonly IdentityProviderPreset[]>>({ status: 'idle' });

  /** What the last read answered: loading, the providers, or the refusal. */
  get state(): ResourceState<readonly IdentityProvider[]> {
    return this.#state;
  }

  /** The providers in force, both levels, in the order their buttons are drawn. */
  get all(): readonly IdentityProvider[] {
    return this.#state.status === 'ready' ? this.#state.data : [];
  }

  /** The workspace's own — the ones it may change. */
  get own(): readonly IdentityProvider[] {
    return this.all.filter((one) => one.scope === 'workspace');
  }

  /** What the installation offers every workspace on it, this one included. */
  get inherited(): readonly IdentityProvider[] {
    return this.all.filter((one) => one.scope === 'installation');
  }

  get presets(): readonly IdentityProviderPreset[] {
    return this.#presets.status === 'ready' ? this.#presets.data : [];
  }

  /** The preset of one kind, where it has been read. */
  presetOf(kind: IdentityProvider['kind']): IdentityProviderPreset | undefined {
    return this.presets.find((one) => one.kind === kind);
  }

  /** Starts both reads. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    const stopProviders = engine.subscribe<readonly IdentityProvider[]>({ path: PATH }, (next) => {
      this.#state = next;
    });
    const stopPresets = engine.subscribe<readonly IdentityProviderPreset[]>({ path: PRESETS }, (next) => {
      this.#presets = next;
    });
    return () => {
      stopProviders();
      stopPresets();
    };
  }

  /** Adds one. The listing is invalidated, so the screen sees what the server stored. */
  async add(body: IdentityProviderConfiguration): Promise<IdentityProvider> {
    return engine.mutate<IdentityProvider>('POST', PATH, body, { invalidates: [PATH] });
  }

  /** Replaces one, whole. */
  async configure(id: string, body: IdentityProviderConfiguration): Promise<IdentityProvider> {
    return engine.mutate<IdentityProvider>('PUT', `${PATH}/${id}`, body, { invalidates: [PATH] });
  }

  /**
   * Takes one away.
   *
   * The accounts it provisioned keep their rows and their live sessions; what they lose is the way
   * to sign in again. The screen says that before it offers the button — the server audits the
   * answer, and a record of something nobody understood is a record of a surprise.
   */
  async remove(id: string): Promise<void> {
    await engine.mutate('DELETE', `${PATH}/${id}`, undefined, { invalidates: [PATH] });
  }
}

export const identityProvider = new IdentityProviderStore();
export { PATH as identityProviderPath, PRESETS as identityProviderPresetPath };
