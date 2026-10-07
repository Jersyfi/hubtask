// SPDX-License-Identifier: Apache-2.0
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
 * **One switch is a workspace's even on a row that is not.** An installation's provider is offered
 * everywhere and on nowhere; `:offer` is the one verb that answers "is this a way in here", over
 * the row's own `enabled` for a workspace's provider and over the workspace's own list for an
 * inherited one (ADR-0070 §2). That is why an inherited row carries exactly one control and no
 * others.
 *
 * **The presets are read once and are not a provider.** They are three rows and this installation's
 * own callback address — what a registration form asks for — so they live beside the listing rather
 * than inside it.
 *
 * **So is the number the password switch says** (ADR-0078 §1): how many people here no provider
 * switched on here signs in. It follows the providers - switching one on or off, or removing it,
 * reads it again - and it is a number, never a list.
 */

import type {
  AccountsWithoutProvider,
  IdentityProvider,
  IdentityProviderConfiguration,
  IdentityProviderPreset,
  ResourceState,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { stepUp } from './stepup.svelte.ts';

const PATH = '/identity-providers';
const PRESETS = '/identity-provider-presets';
const WITHOUT = '/tenant/accounts-without-provider';

class IdentityProviderStore {
  #state = $state<ResourceState<readonly IdentityProvider[]>>({ status: 'idle' });
  #presets = $state<ResourceState<readonly IdentityProviderPreset[]>>({ status: 'idle' });
  #without = $state<ResourceState<AccountsWithoutProvider>>({ status: 'idle' });

  /**
   * How many active people here no provider switched on here signs in, once read. `undefined`
   * before the read and where it was refused - the screen then says nothing rather than a guess.
   */
  get withoutProvider(): number | undefined {
    return this.#without.status === 'ready' ? this.#without.data.count : undefined;
  }

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
    const stopWithout = engine.subscribe<AccountsWithoutProvider>({ path: WITHOUT }, (next) => {
      this.#without = next;
    });
    return () => {
      stopProviders();
      stopPresets();
      stopWithout();
    };
  }

  /** Adds one. The listing is invalidated, so the screen sees what the server stored. */
  async add(body: IdentityProviderConfiguration): Promise<IdentityProvider> {
    // Every change to a way in asks for a fresh proof (ADR-0071's addendum): which provider may
    // vouch for this workspace's people is a sign-in rule like any other.
    return stepUp.around((stepUpToken) =>
      engine.mutate<IdentityProvider>('POST', PATH, body, { stepUpToken, invalidates: [PATH] }),
    );
  }

  /**
   * Switches a provider on or off as a way into this workspace.
   *
   * The same call for both levels, because it is the same question. The server refuses switching
   * off the last way in — a workspace nobody can reach is not a state a click may produce — and the
   * screen shows that refusal rather than predicting it: whether another way exists is the server's
   * count, and a client that kept its own would disagree with it eventually.
   */
  async offer(id: string, offered: boolean): Promise<IdentityProvider> {
    return stepUp.around((stepUpToken) =>
      engine.mutate<IdentityProvider>('POST', `${PATH}/${id}:offer`, { offered }, {
        stepUpToken,
        invalidates: [PATH, WITHOUT],
      }),
    );
  }

  /** Replaces one, whole. */
  async configure(id: string, body: IdentityProviderConfiguration): Promise<IdentityProvider> {
    return stepUp.around((stepUpToken) =>
      engine.mutate<IdentityProvider>('PUT', `${PATH}/${id}`, body, { stepUpToken, invalidates: [PATH] }),
    );
  }

  /**
   * Takes one away.
   *
   * The accounts it provisioned keep their rows and their live sessions; what they lose is the way
   * to sign in again. The screen says that before it offers the button — the server audits the
   * answer, and a record of something nobody understood is a record of a surprise.
   */
  async remove(id: string): Promise<void> {
    await stepUp.around((stepUpToken) =>
      engine.mutate('DELETE', `${PATH}/${id}`, undefined, { stepUpToken, invalidates: [PATH, WITHOUT] }),
    );
  }
}

export const identityProvider = new IdentityProviderStore();
export { PATH as identityProviderPath, PRESETS as identityProviderPresetPath };
