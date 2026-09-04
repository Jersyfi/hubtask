// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The trash, and the one irreversible thing in this milestone.
 *
 * **One row per deletion, not one per deleted row.** The contract is explicit: "a hub with two
 * hundred entries under it went into the trash as one act and comes back as one act, so what is
 * listed is the root of each deletion". A screen that offered to restore one entry out of a batch
 * would break the invariant the batch exists for (I-C2), and there is nothing here that could —
 * the list has no rows below the root to offer.
 *
 * **A short page is not the end.** The list "spans hubs, so it is narrowed to what the caller may
 * see rather than refused: an entry deleted out of a hub the caller has no access to is not in the
 * page. Pages can therefore be shorter than the requested size; walk on until `has_more` is
 * false." That is the same walk `/search` needs and for the same reason, and it is why this is a
 * store rather than a `resource()`.
 *
 * **Emptying is the one thing that cannot be taken back**, so nothing here does it on its own. The
 * store performs it; the screen asks first, says how much will go, and never makes it a default.
 */

import type { PurgeSummary, TrashEntry, TrashPage, TransportError } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/trash';
const PAGE_SIZE = 50;
/** How many pages one listing walks, so a very large trash cannot be asked for forever. */
const MAX_PAGES = 20;

/** What a write to the trash makes stale: the trash, and wherever a restored thing came back to. */
const TOUCHES = ['/trash', '/items', '/containers'];

class Trash {
  #rows = $state<readonly TrashEntry[]>([]);
  #status = $state<'idle' | 'loading' | 'ready' | 'failed'>('idle');
  #error = $state<TransportError | undefined>(undefined);
  #isPartial = $state(false);

  get rows(): readonly TrashEntry[] {
    return this.#rows;
  }

  get status(): 'idle' | 'loading' | 'ready' | 'failed' {
    return this.#status;
  }

  get error(): TransportError | undefined {
    return this.#error;
  }

  /** Whether the walk stopped at its own bound rather than at the end of the trash. */
  get isPartial(): boolean {
    return this.#isPartial;
  }

  /** Reads the whole trash, walking past the short pages the contract warns about. */
  async load(): Promise<void> {
    this.#status = 'loading';
    this.#error = undefined;
    this.#isPartial = false;

    const found: TrashEntry[] = [];
    let cursor: string | null | undefined;

    try {
      for (let page = 0; page < MAX_PAGES; page += 1) {
        const query = new URLSearchParams({ page_size: String(PAGE_SIZE) });
        if (cursor) query.set('cursor', cursor);
        const answer = await engine.refresh<TrashPage>({ path: `${PATH}?${query}` });
        if (answer.status !== 'ready') {
          this.#error = answer.status === 'failed' ? answer.error : undefined;
          this.#status = 'failed';
          return;
        }

        found.push(...(answer.data.data ?? []));
        this.#rows = [...found];
        cursor = answer.data.page?.next_cursor;
        // `has_more`, not the length of the page. A short page here means "some of what is in the
        // trash is not yours to see", which is the opposite of "that was the last of it".
        if (!answer.data.page?.has_more || !cursor) {
          this.#status = 'ready';
          return;
        }
      }
      this.#isPartial = true;
      this.#status = 'ready';
    } catch (error) {
      this.#error = error as TransportError;
      this.#status = 'failed';
    }
  }

  /**
   * Empties the trash, in one pass.
   *
   * "A large trash takes more than one pass: `matched` says how many rows this pass considered,
   * and calling again continues where it left off" — so the answer is handed back rather than
   * swallowed, and the screen says what happened including what was **kept**: a legal hold stays
   * and is counted in the answer rather than failing the call.
   */
  async empty(idempotencyKey: string): Promise<PurgeSummary> {
    const summary = await engine.mutate<PurgeSummary>('POST', '/trash:empty', undefined, {
      idempotencyKey,
      invalidates: TOUCHES,
    });
    await this.load();
    return summary;
  }
}

export const trash = new Trash();
