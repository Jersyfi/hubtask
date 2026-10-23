// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The reads *Today* makes beyond what the frame holds (UC-WRK-34, ADR-0063 decision 1).
 *
 * **What is on the reader is one read, not three.** One wordless search: the entries assigned to
 * this reader that are open and carry a date, in the order the contract already sorts them,
 * `due_at` ascending. Where the reader's day divides them is drawn on the client (`lib/today.ts`),
 * because it depends on their time zone and on whether a date is a day or an instant, and a server
 * that answered three lists would have to ask it three times. It is askable at all because of
 * ADR-0064: a filter with no words is a question to `POST /search`, deliberately unanchored -
 * *Today* is a workspace-wide question. `@me` is resolved on the server: a client that put the
 * account's own id in the filter would be a second answer to who is signed in.
 *
 * **A project's next step is one read per project**, anchored to its collection and one entry
 * long. Asked only for the collections whose figure is *progress*, so a workspace of lists costs
 * nothing here.
 *
 * Neither read is answered by the replica: the query language is the server's (`replica.ts`).
 * Ticking an entry off goes through `items.setCompleted`, which is queued offline like any other
 * completion.
 */

import type { ItemQueryResult, ResourceState, WorkItem, WorkItemPage } from '@hubtask/sync-engine';

import { nextStepQuery } from '../today.ts';
import { engine } from './engine.ts';

/**
 * How many of them to read.
 *
 * *Today* is what is on somebody, not their whole list: a panel is read in a glance and what does
 * not fit belongs in the search this screen links to. The page is asked larger than the bands
 * draw, because the split happens after the answer arrives.
 */
const SIZE = 20;

/** What the first panel draws from: mine, open, and carrying a date. */
const MINE = {
  filter: {
    op: 'AND',
    nodes: [
      { op: 'EQ', field: 'assignee_id', value: '@me' },
      { op: 'EQ', field: 'is_completed', value: false },
      // `NOT IS_NULL` rather than a comparison: "has a date" is not "is due before something", and
      // a bound would quietly drop whatever lies beyond it.
      { op: 'NOT', nodes: [{ op: 'IS_NULL', field: 'due_at' }] },
    ],
  },
  // No `sort`: the contract's own default for a search with no words is `due_at ASC NULLS LAST`,
  // and naming it here would be a second copy of a decision that already has one (ADR-0064).
  page: { size: SIZE },
};

class Today {
  #mine = $state<ResourceState<WorkItemPage>>({ status: 'idle' });
  #steps = $state<Record<string, ResourceState<ItemQueryResult>>>({});

  get state(): ResourceState<WorkItemPage> {
    return this.#mine;
  }

  /** The entries on this reader, soonest first. Empty until the read lands. */
  get mine(): readonly WorkItem[] {
    return this.#mine.status === 'ready' ? (this.#mine.data.data ?? []) : [];
  }

  /**
   * Whether the answer is short because the server had more to say than this asked for.
   *
   * A search page can be short for a reason that has nothing to do with the reader - it answers
   * what the caller may see rather than refusing what they may not - so `has_more` is the only
   * honest reading of "there is more".
   */
  get hasMore(): boolean {
    return this.#mine.status === 'ready' && this.#mine.data.page?.has_more === true;
  }

  /** Starts the read of what is on the reader. **From `untrack`**, for the reason every store records. */
  open(): () => void {
    return engine.subscribe<WorkItemPage>({ path: '/search', body: MINE }, (next) => {
      this.#mine = next;
    });
  }

  /** The state of one project's next-step read; `idle` until it is opened. */
  stepStateOf(collectionId: string): ResourceState<ItemQueryResult> {
    return this.#steps[collectionId] ?? { status: 'idle' };
  }

  /** One project's next due step, or `undefined` where none of its open entries has a date. */
  stepOf(collectionId: string): WorkItem | undefined {
    const state = this.#steps[collectionId];
    return state?.status === 'ready' ? state.data.data?.[0] : undefined;
  }

  /**
   * Starts one project's next-step read. From `untrack`. A completion makes it stale with every
   * other entry list (`touches.ts`), so the step after the one ticked off arrives by itself.
   */
  openStep(collectionId: string): () => void {
    return engine.subscribe<ItemQueryResult>({ path: '/items:query', body: nextStepQuery(collectionId) }, (next) => {
      this.#steps = { ...this.#steps, [collectionId]: next };
    });
  }
}

export const today = new Today();
