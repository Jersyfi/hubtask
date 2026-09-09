// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The consent an app asks for, and the grants a person has already given (H-05).
 *
 * **The code is answered once and travels nowhere but the redirect.** `POST /oauth/authorize`
 * answers a single-use code with minutes of life; this module hands it to its caller, which puts it
 * in the address the server validated and leaves. Nothing keeps it.
 *
 * **A grant is withdrawn, not deleted.** `DELETE /oauth/grants/{id}` takes effect on the app's next
 * request rather than instantly — the sessions it leashed are refused when they next act — and the
 * screen says so, because "revoked" that still works for a minute is the kind of half-truth people
 * remember.
 */

import type { ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import type { AuthorizationRequest } from './oauth.ts';

const AUTHORIZE = '/oauth/authorize';
const GRANTS = '/oauth/grants';
const CLIENTS = '/oauth/clients';

/** What the app is called, as the narrow read answers it. */
export interface AppSummary {
  readonly id: string;
  readonly name: string;
}

/** One grant, as `OauthGrant` answers it. */
export interface Grant {
  readonly id: string;
  readonly client_id: string;
  readonly client_name: string;
  readonly scopes: readonly string[];
  readonly created_at: string;
  readonly last_used_at?: string | null;
}

/** The single-use code, as `OauthCode` answers it. */
interface AuthorizedCode {
  readonly code: string;
  readonly expires_at: string;
}

class Consent {
  #grants = $state<ResourceState<readonly Grant[]>>({ status: 'idle' });

  get grantsState(): ResourceState<readonly Grant[]> {
    return this.#grants;
  }

  get grants(): readonly Grant[] {
    return this.#grants.status === 'ready' ? this.#grants.data : [];
  }

  /** Starts the grants listing. **From `untrack`**, for the reason every other store records. */
  openGrants(): () => void {
    return engine.subscribe<readonly Grant[]>({ path: GRANTS }, (next) => {
      this.#grants = next;
    });
  }

  /** What the app asking for consent is called. One field, from the narrow read. */
  async appNamed(clientId: string): Promise<AppSummary | undefined> {
    const state = await engine.refresh<AppSummary>({ path: `${CLIENTS}/${clientId}` });
    return state.status === 'ready' ? state.data : undefined;
  }

  /**
   * Consents, and answers the single-use code.
   *
   * Everything is passed through as it arrived in the address — the redirect URI byte for byte,
   * the challenge, the method however malformed. The server matches the URI against what the app
   * registered and refuses anything but `S256`, and a client that corrected either would be
   * hiding a refusal the person should see.
   */
  async approve(request: AuthorizationRequest): Promise<string> {
    const answer = await engine.mutate<AuthorizedCode>(
      'POST',
      AUTHORIZE,
      {
        client_id: request.clientId,
        redirect_uri: request.redirectUri,
        scopes: request.scopes,
        code_challenge: request.challenge,
        code_challenge_method: request.method,
        ...(request.state !== undefined ? { state: request.state } : {}),
      },
      { invalidates: [GRANTS] },
    );
    return answer.code;
  }

  /** Withdraws one. The app's next request is refused; the one it is making now is not. */
  async withdraw(grantId: string): Promise<void> {
    await engine.mutate('DELETE', `${GRANTS}/${grantId}`, undefined, { invalidates: [GRANTS] });
  }
}

export const consent = new Consent();
