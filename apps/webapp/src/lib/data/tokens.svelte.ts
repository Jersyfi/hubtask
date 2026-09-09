// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Personal access tokens: one's own, and a service account's (G-01, `security.md` §5).
 *
 * **The credential exists in one answer and nowhere else.** `POST /auth/tokens` carries it; every
 * later read carries a hash's shadow and not the value. So this module hands the minted token
 * straight to its caller and keeps **no** copy — no field, no cache, no storage. What is listed
 * afterwards is metadata about a credential, never the credential.
 *
 * **A token's shape is never checked here.** The security scheme accepts three kinds of credential
 * and a client enforcing one pattern refuses the other two (`apps/webapp/CLAUDE.md`).
 *
 * **`account_id` is the one exception to "a token is its holder's".** Omitted means the caller's
 * own; naming a service account lists or mints that account's, which needs the permission that
 * manages members — because a service account is nothing but access and somebody has to answer
 * for it. Another person's is refused whatever the role, and this module does not offer it.
 *
 * **An admin scope demands a step-up**, so every mint goes through the wrapper. Not only the admin
 * ones: which scopes are privileged is the server's rule, and a client that decided when to expect
 * the prompt would be a client re-implementing that rule.
 */

import type { AccessToken, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { stepUp } from './stepup.svelte.ts';

const PATH = '/auth/tokens';

/** The mint's answer: the listing's fields, plus the value, for the only time. */
export interface MintedToken extends AccessToken {
  readonly token: string;
}

/** What a mint asks for. Every field required, because the contract requires every field. */
export interface Mint {
  readonly name: string;
  readonly scopes: readonly string[];
  /** An instant, mandatory and at most a year out. There is no default — a caller has to choose. */
  readonly expiresAt: string;
  /** A service account's, where the caller may mint for one. Omitted means their own. */
  readonly accountId?: string;
}

/** The listing's path for one holder. Built here so the store and its callers agree on one string. */
export function tokensPath(accountId?: string): string {
  return accountId ? `${PATH}?account_id=${encodeURIComponent(accountId)}` : PATH;
}

class Tokens {
  #lists = $state<Record<string, ResourceState<readonly AccessToken[]>>>({});

  /** The listing for one holder — the caller's own where no account is named. */
  stateOf(accountId?: string): ResourceState<readonly AccessToken[]> {
    return this.#lists[tokensPath(accountId)] ?? { status: 'idle' };
  }

  of(accountId?: string): readonly AccessToken[] {
    const state = this.stateOf(accountId);
    return state.status === 'ready' ? state.data : [];
  }

  /** Starts a listing. **From `untrack`**, for the reason every other store records. */
  open(accountId?: string): () => void {
    const path = tokensPath(accountId);
    return engine.subscribe<readonly AccessToken[]>({ path }, (next) => {
      this.#lists = { ...this.#lists, [path]: next };
    });
  }

  /**
   * Mints one and answers the credential, once.
   *
   * The value is returned rather than held: the caller shows it through `OneTimeSecret` and it
   * dies with that screen. A store that kept it would be a store holding a credential for as long
   * as the tab is open.
   */
  async mint(request: Mint): Promise<MintedToken> {
    return stepUp.around((stepUpToken) =>
      engine.mutate<MintedToken>(
        'POST',
        PATH,
        {
          name: request.name,
          scopes: request.scopes,
          expires_at: request.expiresAt,
          ...(request.accountId ? { account_id: request.accountId } : {}),
        },
        { idempotencyKey: crypto.randomUUID(), invalidates: [PATH], stepUpToken },
      ),
    );
  }

  /** Withdraws one. What was minted with it stops working; the row stays, marked as revoked. */
  async revoke(tokenId: string): Promise<void> {
    await stepUp.around((stepUpToken) =>
      engine.mutate('DELETE', `${PATH}/${tokenId}`, undefined, {
        invalidates: [PATH],
        stepUpToken,
      }),
    );
  }
}

export const tokens = new Tokens();
