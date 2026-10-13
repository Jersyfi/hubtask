// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the invitation card says before anything is typed: the workspace and the invited address
 * (UC-ID-07 check 1).
 *
 * The token travels in the body of a `POST`, never in an address, and the server answers only for a
 * token that would redeem - every other is the redemption's one refusal, which this module keeps as
 * a code for the card to say. Nothing is spent by asking.
 */

import { TransportError } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const INSPECT = '/auth/invitations:inspect';

/** What `:inspect` answers. */
export interface InvitationSummary {
  readonly workspace_name: string;
  readonly email: string;
}

class Invitation {
  #summary = $state<InvitationSummary | undefined>(undefined);
  #failure = $state<string | undefined>(undefined);

  /** The workspace and the address, once read. */
  get summary(): InvitationSummary | undefined {
    return this.#summary;
  }

  /** The code of a refusal, where the invitation would not redeem. */
  get failure(): string | undefined {
    return this.#failure;
  }

  /** Reads the invitation a token names. */
  async inspect(token: string): Promise<void> {
    this.#summary = undefined;
    this.#failure = undefined;
    try {
      this.#summary = await engine.mutate<InvitationSummary>('POST', INSPECT, { token });
    } catch (cause) {
      this.#failure = cause instanceof TransportError
        ? (cause.detailCode ?? cause.code ?? 'errors.internal')
        : 'errors.internal';
    }
  }
}

export const invitation = new Invitation();
