// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * How long the workspace keeps what is in its trash — read **only where the actor may read it**.
 *
 * The same shape `health.svelte.ts` has, for the same reason: `/retention-policies` needs
 * `retention:read`, which an ordinary member does not have, and a `403` is *silence* rather than a
 * message. A screen that reported "you may not see the retention policy" would be telling somebody
 * about a permission they were never going to use.
 *
 * What this exists for is the sentence on the trash screen. "It will be gone anyway in a week"
 * changes a decision, so the window is stated where a person decides — and where the client cannot
 * read it, nothing is stated rather than the documented default being asserted as this
 * installation's. 30 days is `data-retention.md`'s default with a seven-day floor; the tenant may
 * have set something else, and this client has no way to know which.
 */

import type { RetentionPolicy } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/retention-policies';

/** The catalogue's kind for what sits in the trash (`data-retention.md` §3). */
const TRASH_KIND = 'TRASH';

class Retention {
  #days = $state<number | undefined>(undefined);
  #hasAsked = false;

  /** The tenant's trash retention in days, or `undefined` while unknown — including "may not read". */
  get trashDays(): number | undefined {
    return this.#days;
  }

  /**
   * Asks once, and stays quiet about a refusal.
   *
   * Once because the policy does not change while a screen is open, and quietly because a refusal
   * is the ordinary answer for most actors: the screen simply says less.
   */
  async start(): Promise<void> {
    if (this.#hasAsked) return;
    this.#hasAsked = true;
    try {
      const answer = await engine.refresh<readonly RetentionPolicy[]>({ path: PATH });
      if (answer.status !== 'ready') return;
      const policy = (answer.data ?? []).find(
        (each) => each.data_kind === TRASH_KIND && each.enabled !== false,
      );
      this.#days = policy?.retain_days;
    } catch {
      // Silence. A refusal is not a failure the reader has anything to do about.
    }
  }
}

export const retention = new Retention();
