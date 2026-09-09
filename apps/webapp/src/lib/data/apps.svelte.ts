// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace's registered third-party apps (H-05).
 *
 * **A confidential client's secret exists in one answer.** Registration carries it; nothing after
 * that does. So this module hands it to its caller and keeps no copy, exactly as the token store
 * does — and a public client gets none at all, because it cannot keep one and brings PKCE instead.
 *
 * **Removing an app removes every grant that pointed at it**, and the sessions those grants
 * leashed refuse on their next request. That is the server's doing; the screen's job is to say it
 * before the button rather than after.
 */

import type { ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/oauth/clients';
/** Removing an app withdraws its grants, so a person's grant list is stale afterwards. */
const TOUCHES = [PATH, '/oauth/grants'];

/** A registered app, as `OauthClient` answers it. */
export interface App {
  readonly id: string;
  readonly name: string;
  readonly redirect_uris: readonly string[];
  readonly confidential: boolean;
  readonly created_at: string;
}

/** What registration answers: the app, and the secret where there is one. */
export interface RegisteredApp extends App {
  readonly client_secret?: string | null;
}

class Apps {
  #state = $state<ResourceState<readonly App[]>>({ status: 'idle' });

  get state(): ResourceState<readonly App[]> {
    return this.#state;
  }

  get all(): readonly App[] {
    return this.#state.status === 'ready' ? this.#state.data : [];
  }

  /** Starts the listing. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<readonly App[]>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }

  /**
   * Registers one and answers the secret, once.
   *
   * The redirect URIs are matched byte for byte at authorization and at exchange, so they are sent
   * exactly as typed — trimmed of surrounding space and nothing else. A client that normalised a
   * trailing slash would register an address the app then fails to match.
   */
  async register(
    name: string,
    redirectUris: readonly string[],
    confidential: boolean,
  ): Promise<RegisteredApp> {
    return engine.mutate<RegisteredApp>(
      'POST',
      PATH,
      { name, redirect_uris: redirectUris, confidential },
      { idempotencyKey: crypto.randomUUID(), invalidates: TOUCHES },
    );
  }

  /** Removes it, and with it every grant that pointed at it. */
  async remove(clientId: string): Promise<void> {
    await engine.mutate('DELETE', `${PATH}/${clientId}`, undefined, { invalidates: TOUCHES });
  }
}

export const apps = new Apps();
