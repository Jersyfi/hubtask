// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The addresses of the people a list offers, for a reader who manages members - the line the
 * people list shows beside a name. `AccountSummary` leaves the address out on purpose; it is read
 * through `POST /accounts:sign-in-details`, which asks `MANAGE_MEMBERS` at the workspace, so a
 * reader without it is never sent the read the server would refuse and record: their list goes by
 * name alone.
 */

import { holdsHere } from '../data/capability.svelte.ts';
import { people } from '../data/people.svelte.ts';

/** The most identifiers one read names (the operation's `maxItems`). */
const PER_READ = 200;

export class Addresses {
  #of = $state<Readonly<Record<string, string>>>({});
  #asked = new Set<string>();

  /** The address, once read; `undefined` while unread, refused, or for nobody's address. */
  of(id: string): string | undefined {
    return this.#of[id];
  }

  /** Reads the addresses of `ids` not asked for yet, in reads of at most two hundred. */
  read(ids: readonly string[]): void {
    if (holdsHere('MANAGE_MEMBERS') !== true) return;
    const wanted = [...new Set(ids)].filter((id) => !this.#asked.has(id));
    for (let at = 0; at < wanted.length; at += PER_READ) {
      const batch = wanted.slice(at, at + PER_READ);
      batch.forEach((id) => this.#asked.add(id));
      void people.signInDetails(batch).then(
        (answer) => {
          const found = Object.fromEntries(answer.filter((detail) => detail.email).map((detail) => [detail.account_id, detail.email as string]));
          this.#of = { ...this.#of, ...found };
        },
        // A refused or failed read leaves the list by name; nothing on screen depends on it.
        () => undefined,
      );
    }
  }
}
