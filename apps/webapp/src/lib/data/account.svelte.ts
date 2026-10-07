// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Who is signed in, and what they prefer.
 *
 * `GET /accounts/me` is the read for exactly this. The account carries `locale`, `time_zone` and `week_start`, and a binding client
 * requirement says those are what the application speaks and shows (`i18n-l10n.md` §2).
 *
 * Read only when there is a bearer, and quiet when refused - the same shape as the health report
 * and for a smaller reason: an anonymous client asking who it is gets a `401` it already knew
 * about. Without a token behind the platform seam, `platform.bearer()` answers `undefined` and
 * nothing is read at all, which is the honest state of an application nobody has
 * signed into.
 *
 * **There is no theme here, by decision.** `Account` carries locale, time zone and week start and
 * no appearance preference, and ADR-0043 is why: the theme is a property of the device rather than
 * of the person, so it neither belongs in the contract nor resolves through this module. The theme
 * follows the system in `lib/theme.ts`, and a per-device override waits for ADR-0033's local
 * persistence port.
 */

import type { Account, ResourceState } from '@hubtask/sync-engine';

import { deviceZone } from '../i18n/zone.ts';
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

  /**
   * The zone this reader's dates are read in.
   *
   * The account's, and the device's where the account states none — which is the chain
   * `i18n-l10n.md` §2 describes, stopping one link short: the tenant's and the installation's
   * defaults are already resolved into the account document by the time it arrives here.
   */
  get zone(): string {
    return this.account?.time_zone ?? deviceZone();
  }

  /** Which day a week begins on for this reader. `null` where the account states none. */
  get weekStart(): string | null {
    return this.account?.week_start ?? null;
  }

  /**
   * Whether this account holds an armed second factor.
   *
   * `undefined` where nothing is known yet, or where the installation is older than the field -
   * and a screen must treat that as "not answered" rather than as "no". Guessing in either
   * direction is how the security screen came to offer actions the server refuses.
   */
  /** Whether this account holds a password. One that signs in only through a provider holds none. */
  get hasPassword(): boolean {
    // `false` only where the server said so: an older server answers nothing, and every account
    // had a password before providers existed (UC-ID-05 check 5).
    return this.account?.has_password !== false;
  }

  get hasSecondFactor(): boolean | undefined {
    return this.account?.has_second_factor;
  }

  /**
   * Whether the workspace's rule demands a second factor of this person - the reading turning it
   * off would meet. `true` only where the server said so: the permissive reading of an unanswered
   * question would hide a control somebody is entitled to (UC-ID-03 checks 5 and 6).
   */
  get secondFactorRequired(): boolean {
    return this.account?.second_factor_required === true;
  }

  /**
   * How many recovery codes are left, or `undefined` where there is nothing to count.
   *
   * Absent is not zero. It is answered only for an account that holds a second factor, so a screen
   * reading it knows both things from one field.
   */
  get recoveryCodesLeft(): number | undefined {
    return this.account?.recovery_codes_remaining;
  }

  /**
   * Reads the account again.
   *
   * For the two moments this document changes without a write to it: arming a second factor and
   * taking one off. Both move `has_second_factor`, and a screen still holding the old answer would
   * offer the action that was just taken.
   */
  async reread(): Promise<void> {
    if (platform.bearer() === undefined) return;
    await engine.refresh<Account>({ path: PATH });
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
        // Not signed in after all: here it is simply nobody, which is what `idle` means.
        this.#state = { status: 'idle' };
        return;
      }
      // Remembered beside the pair, so that a tab reloading while the server is away still knows
      // whose replica to open.
      if (next.status === 'ready') platform.rememberAccount(next.data.id);
      this.#state = next;
    });
  }
}

export const actor = new Actor();
