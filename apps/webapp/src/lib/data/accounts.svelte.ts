// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The names behind the identifiers other records carry.
 *
 * `ActivityEntry.actor` carries `{type, id}` and no label, and the contract says why: "the account
 * is one request away and this record is deleted with its entry, so there is nothing for a copy of
 * somebody's name to outlive". `GET /accounts/{accountId}` is that request, and this is the client
 * that makes it.
 *
 * **A cache rather than a resource.** Every other store here subscribes to a level and re-reads it
 * when a write invalidates it; this one holds names, and a name is not something a screen watches
 * for changes. A feed of twenty entries by three people is three reads, once, and then nothing.
 *
 * **A failed resolve is remembered as failed.** An identifier that does not resolve - a member of
 * another tenant whose id leaked into a record, an account since purged - must not be asked for
 * again on every render. It falls back to the sentence that is true of every actor, permanently,
 * which is what `actorCodes` already answers with.
 */

import type { AccountSummary } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

class Accounts {
  #names = $state<Record<string, string>>({});
  /** Asked for and not answered yet, so a second caller does not ask again. */
  #asking = new Set<string>();
  /** Asked for and refused, so nobody asks again at all. */
  #unknown = new Set<string>();

  /** The display name, when it is known. `undefined` means "say what is true of every actor". */
  nameOf(id: string | null | undefined): string | undefined {
    return id ? this.#names[id] : undefined;
  }

  /**
   * Asks for the names this screen needs and has not got.
   *
   * Safe to call on every render: an identifier already known, already being asked for, or already
   * refused is skipped, so the work happens once per identifier per session.
   */
  resolve(ids: readonly (string | null | undefined)[]): void {
    for (const id of ids) {
      if (!id || id in this.#names || this.#asking.has(id) || this.#unknown.has(id)) continue;
      this.#asking.add(id);
      void this.#read(id);
    }
  }

  async #read(id: string): Promise<void> {
    try {
      const state = await engine.refresh<AccountSummary>({ path: `/accounts/${id}` });
      if (state.status === 'ready' && state.data.display_name) {
        this.#names = { ...this.#names, [id]: state.data.display_name };
      } else {
        this.#unknown.add(id);
      }
    } catch {
      // A refusal is an answer: this reader may not have that name, and the feed says "Somebody".
      // Rethrowing would put a sentence about an account on a screen about an entry.
      this.#unknown.add(id);
    } finally {
      this.#asking.delete(id);
    }
  }
}

export const accounts = new Accounts();
