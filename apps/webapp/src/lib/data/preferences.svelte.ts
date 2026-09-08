// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Somebody's own preferences: how the product speaks to them, and what it tells them about.
 *
 * **Writing the account's preferences invalidates the account.** The frame reads the locale, the
 * zone and the week start from `/accounts/me`, and applies the language in one place — so the write
 * naming that path is what makes the page speak the new language without a reload. Nothing here
 * applies anything itself; a second module that set an attribute would be the second answer to
 * "which locale", which `apps/webapp/CLAUDE.md` rules out by name.
 *
 * **A notification row is written whole.** `enabled` and `include_title` travel together, because
 * "a row is a statement about a category rather than two switches that could drift".
 */

import type {
  AccountPreferences,
  Account,
  NotificationPreference,
  NotificationPreferenceList,
  ResourceState,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const ME = '/accounts/me';

export const notificationsPath = (accountId: string) =>
  `/accounts/${accountId}/notification-preferences`;

class Preferences {
  #rows = $state<Record<string, ResourceState<NotificationPreferenceList>>>({});

  /** One account's rows, empty until they have been read. */
  of(accountId: string): readonly NotificationPreference[] {
    const state = this.#rows[notificationsPath(accountId)];
    return state?.status === 'ready' ? (state.data.data ?? []) : [];
  }

  stateOf(accountId: string): ResourceState<NotificationPreferenceList> | undefined {
    return this.#rows[notificationsPath(accountId)];
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(accountId: string): () => void {
    const path = notificationsPath(accountId);
    return engine.subscribe<NotificationPreferenceList>({ path }, (next) => {
      this.#rows = { ...this.#rows, [path]: next };
    });
  }

  /**
   * Sets the locale, the zone and the week start — or clears them.
   *
   * `/accounts` is invalidated, which is how the frame learns: it reads `/accounts/me`, and the
   * engine re-reads a watched entry when a write names its prefix. That is the whole mechanism by
   * which the page changes language, and it needed no new one.
   */
  async setAccount(accountId: string, body: AccountPreferences): Promise<Account> {
    return engine.mutate<Account>('PATCH', `/accounts/${accountId}/preferences`, body, {
      invalidates: ['/accounts'],
    });
  }

  /** Writes one category-and-channel pair, both switches together. */
  async setNotification(
    accountId: string,
    category: string,
    channel: string,
    body: { enabled: boolean; include_title: boolean },
  ): Promise<NotificationPreference> {
    return engine.mutate<NotificationPreference>(
      'PUT',
      `${notificationsPath(accountId)}/${encodeURIComponent(category)}/${encodeURIComponent(channel)}`,
      body,
      { invalidates: [notificationsPath(accountId)] },
    );
  }
}

export const preferences = new Preferences();
export { ME };
