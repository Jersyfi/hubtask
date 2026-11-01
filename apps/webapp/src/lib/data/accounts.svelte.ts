// SPDX-License-Identifier: Apache-2.0
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

import { actor } from './account.svelte.ts';
import { ResolveBatch } from './accountbatch.ts';
import { engine } from './engine.ts';
import { media } from './media.svelte.ts';

class Accounts {
  #names = $state<Record<string, string>>({});
  /**
   * The standing beside the name, from the same answer.
   *
   * `AccountSummary` carries it, so keeping it costs a field rather than a request — and the
   * people screen needs it: an invitation that has not been redeemed is an `INVITED` row, and a
   * screen that could not tell it from an active person would say somebody is in the workspace
   * who has not arrived yet.
   */
  #statuses = $state<Record<string, string>>({});
  /**
   * The people mail does not reach: an account without an address, a managed one (ADR-0074 §5). The
   * summary says so without the address, so a reminder's screen can say it rather than letting the
   * mail fail silently.
   */
  #addressless = $state<Record<string, true>>({});
  /**
   * The managed accounts: a sign-in name and no address (ADR-0074 §4, §6). The server's full
   * condition for a new start password and an address, so a screen offers both exactly there.
   */
  #managed = $state<Record<string, true>>({});
  /**
   * The picture each person shows beside their name, by media identifier. The URL is minted per
   * read and expires, so it is the media cache that holds it, not this one.
   */
  #pictures = $state<Record<string, string>>({});
  /** Asked for and not answered yet, so a second caller does not ask again. */
  #asking = new Set<string>();
  /** Asked for and refused, so nobody asks again at all. */
  #unknown = new Set<string>();

  /** The display name, when it is known. `undefined` means "say what is true of every actor". */
  nameOf(id: string | null | undefined): string | undefined {
    return id ? this.#names[id] : undefined;
  }

  /** Whether mail is known not to reach this person. Unknown reads as reachable. */
  hasNoAddress(id: string | null | undefined): boolean {
    return id ? this.#addressless[id] === true : false;
  }

  /** Whether the account is known to be managed. Unknown reads as not: nothing is offered. */
  isManaged(id: string | null | undefined): boolean {
    return id ? this.#managed[id] === true : false;
  }

  /**
   * Where to draw the person's picture from, or `undefined` for the initials — no picture, not read
   * yet, or one this reader is not handed. Safe on every render, as `media.coverUrl` is.
   *
   * The reader's own picture is read off their own account, which a change of it re-reads: the
   * summary cached here would still show the one before.
   */
  pictureOf(id: string | null | undefined, now: number = Date.now()): string | undefined {
    if (!id) return undefined;
    const own = actor.account;
    const mediaId = own?.id === id ? (own.avatar_media_id ?? undefined) : this.#pictures[id];
    return media.coverUrl(mediaId, now);
  }

  /** The account's standing — `ACTIVE`, `INVITED`, `DISABLED` — where it is known. */
  statusOf(id: string | null | undefined): string | undefined {
    return id ? this.#statuses[id] : undefined;
  }

  /**
   * Asks for the names this screen needs and has not got - all of a turn's in one request.
   *
   * Safe to call on every render: an identifier already known, already being asked for, or already
   * left out of an answer is skipped, so the work happens once per identifier per session. One the
   * server could not answer (busy, down, offline) is asked again, and after the last attempt by the
   * next call that wants it.
   */
  resolve(ids: readonly (string | null | undefined)[]): void {
    const wanted: string[] = [];
    for (const id of ids) {
      if (!id || id in this.#names || this.#asking.has(id) || this.#unknown.has(id)) continue;
      this.#asking.add(id);
      wanted.push(id);
    }
    if (wanted.length > 0) this.#batch.ask(wanted);
  }

  #batchMissing = false;

  readonly #batch = new ResolveBatch<AccountSummary>({
    read: async (ids) => {
      if (!this.#batchMissing) {
        const state = await engine.refresh<{ data: AccountSummary[] }>({ path: '/accounts:resolve', body: { ids } });
        if (state.status === 'ready') return state.data.data ?? [];
        if (state.status !== 'failed' || state.error.status !== 404) throw state.status === 'failed' ? state.error : new Error('accounts not resolved');
        // A server older than the batch read, during a rolling update: one read per name, as before.
        this.#batchMissing = true;
      }
      const states = await Promise.all(ids.map((id) => engine.refresh<AccountSummary>({ path: `/accounts/${id}` })));
      return states.flatMap((state) => (state.status === 'ready' ? [state.data] : []));
    },
    idOf: (account) => account.id,
    settled: (found, unknown) => this.#settle(found, unknown),
    gaveUp: (ids) => { for (const id of ids) this.#asking.delete(id); },
  });

  #settle(found: readonly AccountSummary[], unknown: readonly string[]): void {
    const names = { ...this.#names };
    const statuses = { ...this.#statuses };
    const addressless = { ...this.#addressless };
    const managed = { ...this.#managed };
    const pictures = { ...this.#pictures };
    for (const account of found) {
      this.#asking.delete(account.id);
      if (!account.display_name) {
        this.#unknown.add(account.id);
        continue;
      }
      names[account.id] = account.display_name;
      if (account.status) statuses[account.id] = account.status;
      if (account.has_address === false) addressless[account.id] = true;
      if (account.managed === true) managed[account.id] = true;
      if (account.avatar_media_id) pictures[account.id] = account.avatar_media_id;
    }
    for (const id of unknown) {
      this.#asking.delete(id);
      this.#unknown.add(id);
    }
    this.#names = names;
    this.#statuses = statuses;
    this.#addressless = addressless;
    this.#managed = managed;
    this.#pictures = pictures;
  }
}

export const accounts = new Accounts();
