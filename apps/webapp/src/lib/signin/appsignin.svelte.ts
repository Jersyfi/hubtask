// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The installed apps' sign-in through the system browser (ADR-0096; UC-SYN-07 check 3), as one
 * store: the address asked for, judged before the browser opens, the wait, and the code taken back.
 *
 * **The verifier stays in this page's memory**, never `$state` and never storage: it is the half of
 * the exchange the browser never sees, and whoever holds it with the code holds the session.
 */

import { engine, exchangeAppCode, readServerSelf } from '../data/engine.ts';
import { toast } from '../frame/toasts.svelte.ts';
import { messages, t } from '../i18n/i18n.svelte.ts';
import { platform } from '../platform/index.ts';
import { loopbackLook } from './loopbackPage.ts';
import type { ReturnedCode } from '../platform/index.ts';
import { API_PATH } from '../platform/server.ts';
import {
  addressOf,
  consentUrl,
  judge,
  newPkce,
  newState,
  type Refusal,
  type ServerSelf,
  type WorkspaceAddress,
} from './appSignIn.ts';

export type Phase = 'ask' | 'checking' | 'waiting';

interface Pending {
  readonly address: WorkspaceAddress;
  /** The installation's own host, which the app's list groups by; none in single mode. */
  readonly installation: string | undefined;
  readonly client: string;
  readonly verifier: string;
  readonly state: string;
  readonly redirect: string;
  readonly url: string;
}

class AppSignIn {
  #phase = $state<Phase>('ask');
  #refusal = $state<Refusal | undefined>(undefined);
  #invalid = $state(false);
  #address = $state<WorkspaceAddress | undefined>(undefined);
  #unfinished = $state(false);
  #asking = $state(false);
  #pending: Pending | undefined;

  get phase(): Phase {
    return this.#phase;
  }

  /** Why the address was refused at the field, after *Continue*. */
  get refusal(): Refusal | undefined {
    return this.#refusal;
  }

  /** Whether what was typed is not an address at all. */
  get isInvalid(): boolean {
    return this.#invalid;
  }

  get address(): WorkspaceAddress | undefined {
    return this.#address;
  }

  /** A sign-in came back refused or expired: the wait's banner. */
  get isUnfinished(): boolean {
    return this.#unfinished;
  }

  /** Whether the person chose to enter an address rather than sign in again to the last workspace. */
  get isAsking(): boolean {
    return this.#asking;
  }

  /** The refusal at the field, in its own words with its fix; nothing where there is none. */
  get refusalText(): string | undefined {
    const host = this.#address?.host ?? '';
    if (this.#invalid) return t('app.app_sign_in.invalid');
    const refusal = this.#refusal;
    if (refusal === undefined) return undefined;
    switch (refusal.kind) {
      case 'unreachable':
        return t('app.app_sign_in.unreachable', { host });
      case 'not-hubtask':
        return t('app.app_sign_in.not_hubtask', { host });
      case 'older':
        return t('app.app_sign_in.older', { host, server: refusal.server, app: refusal.app });
      case 'installation':
        return t('app.app_sign_in.installation', { host });
    }
  }

  /** Back to an empty field: *Add a workspace* opened afresh. */
  reset(): void {
    this.stop();
    this.#refusal = undefined;
    this.#invalid = false;
    this.#address = undefined;
  }

  /** The wait given up without asking for another address: *Add a workspace* closed. */
  stop(): void {
    platform.browserSignIn?.retire();
    this.#pending = undefined;
    this.#unfinished = false;
    this.#phase = 'ask';
  }

  /** *Switch workspace* from the signed-out card: the address is asked for. */
  askForAddress(): void {
    this.#asking = true;
    this.#refusal = undefined;
    this.#invalid = false;
  }

  /** *Continue*: the address judged, then the browser opened at its consent. */
  async continue(typed: string): Promise<void> {
    const signIn = platform.browserSignIn;
    if (signIn === undefined || this.#phase === 'checking') return;
    this.#refusal = undefined;
    this.#unfinished = false;
    const address = addressOf(typed);
    this.#invalid = address === undefined;
    if (address === undefined) return;
    this.#address = address;
    this.#phase = 'checking';
    const self = await readServerSelf<ServerSelf>(`${address.origin}${API_PATH}`);
    const verdict = judge(address, self, signIn.appVersion);
    if ('refused' in verdict) {
      this.#refusal = verdict.refused;
      this.#phase = 'ask';
      return;
    }
    await this.#open(address, verdict.client, installationOf(self));
  }

  /** *Open the browser again*: a fresh request, so a page that expired meanwhile is not reused. */
  async again(): Promise<void> {
    const pending = this.#pending;
    if (pending === undefined) return;
    this.#unfinished = false;
    await this.#open(pending.address, pending.client, pending.installation);
  }

  /** *Use another address*: the wait ends, the field returns. */
  cancel(): void {
    this.stop();
    this.#asking = true;
  }

  /**
   * Once signed in: the workspace's name for the cards that name it later, and on the run a
   * sign-in started, the toast that the copy is being made (the approved preview's choice).
   */
  async welcome(): Promise<void> {
    const signIn = platform.browserSignIn;
    if (signIn === undefined) return;
    const arrived = signIn.takeArrival();
    const read = await engine.refresh<{ readonly display_name?: string }>({ path: '/tenant' }).catch(() => undefined);
    const name = read?.status === 'ready' ? read.data.display_name : undefined;
    if (name !== undefined && name !== '') signIn.nameWorkspace(name);
    if (arrived) {
      toast.show({ text: t('app.app_sign_in.signed_in', { name: name ?? signIn.workspace?.host ?? '' }) });
    }
  }

  async #open(address: WorkspaceAddress, client: string, installation: string | undefined): Promise<void> {
    const signIn = platform.browserSignIn;
    if (signIn === undefined) return;
    const pkce = await newPkce();
    const state = newState();
    let redirect: string;
    try {
      redirect = await signIn.listenForSignIn(
        { title: t('app.app_sign_in.back_title'), body: t('app.app_sign_in.back_body', { host: address.host }), lang: messages.locale, ...loopbackLook() },
        (returned) => void this.#returned(returned),
      );
    } catch {
      this.#unfinished = true;
      this.#phase = 'waiting';
      return;
    }
    const url = consentUrl(address, {
      client, redirect, challenge: pkce.challenge, state,
      device: platform.deviceName(), platform: signIn.appPlatform,
    });
    this.#pending = { address, installation, client, verifier: pkce.verifier, state, redirect, url };
    this.#phase = 'waiting';
    try {
      await signIn.openInBrowser(url);
    } catch {
      this.#unfinished = true;
    }
  }

  async #returned(returned: ReturnedCode): Promise<void> {
    const pending = this.#pending;
    // Only the request this window made: a code with another state is somebody else's redirect.
    if (pending === undefined || returned.state !== pending.state || returned.code === undefined) {
      this.#unfinished = true;
      return;
    }
    this.#pending = { ...pending, url: '' };
    try {
      const pair = await exchangeAppCode(`${pending.address.origin}${API_PATH}`, {
        code: returned.code, redirectUri: pending.redirect, clientId: pending.client, verifier: pending.verifier,
      });
      await platform.browserSignIn?.enter({
        server: pending.address.origin,
        host: pending.address.host,
        ...(pending.installation !== undefined ? { installation: pending.installation } : {}),
      }, pair);
    } catch {
      this.#pending = pending;
      this.#unfinished = true;
    }
  }
}

/** The installation's own host, as the server names it; nothing in single mode. */
function installationOf(self: ServerSelf | null | undefined): string | undefined {
  try {
    return self?.installation_url ? new URL(self.installation_url).host : undefined;
  } catch {
    return undefined;
  }
}

export const appSignIn = new AppSignIn();
