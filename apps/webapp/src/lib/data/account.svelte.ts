// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Who is signed in, and what they prefer.
 *
 * `GET /accounts/me` is what F1-08 added for exactly this, and this is the client task it named as
 * its consumer. The account carries `locale`, `time_zone` and `week_start`, and a binding client
 * requirement says those are what the application speaks and shows (`roadmap.md` phase 5,
 * `i18n-l10n.md` §2).
 *
 * Read only when there is a bearer, and quiet when refused - the same shape as the health report
 * and for a smaller reason: an anonymous client asking who it is gets a `401` it already knew
 * about. Until F1-11 puts a token behind the platform seam, `platform.bearer()` answers
 * `undefined` and nothing is read at all, which is the honest state of an application nobody has
 * signed into.
 *
 * **There is no theme here.** `Account` in the contract carries locale, time zone and week start
 * and no appearance preference, so the theme keeps following the system (`lib/theme.ts`). Giving
 * the client one would mean adding a field to `api/openapi.yaml` first (ADR-0004), which is a
 * change to the contract rather than a frame that consumes it.
 */

import type { Account, ResourceState } from '@hubtask/sync-engine';

import { platform } from '../platform/index.ts';
import { engine } from './engine.ts';

const PATH = '/accounts/me';

class Actor {
  #state = $state<ResourceState<Account>>({ status: 'idle' });

  get state(): ResourceState<Account> {
    return this.#state;
  }

  get account(): Account | undefined {
    return this.#state.status === 'ready' ? this.#state.data : undefined;
  }

  /** The account's locale, when there is an account and it has one. */
  get locale(): string | undefined {
    return this.account?.locale ?? undefined;
  }

  start(): () => void {
    if (platform.bearer() === undefined) {
      // Nobody is signed in, so nothing is known - including whatever a previous session read.
      // `engine.reset()` drops the engine's copy on sign-out; this drops this module's.
      this.#state = { status: 'idle' };
      return () => {};
    }

    return engine.subscribe<Account>({ path: PATH }, (next) => {
      if (next.status === 'failed' && (next.error.status === 401 || next.error.status === 403)) {
        // Not signed in after all. F1-11 takes this as its cue to ask for a token again; here it
        // is simply nobody, which is what `idle` means.
        this.#state = { status: 'idle' };
        return;
      }
      this.#state = next;
    });
  }
}

export const actor = new Actor();
