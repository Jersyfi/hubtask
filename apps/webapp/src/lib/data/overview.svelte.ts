// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What is on the reader, for the overview (ADR-0063 decision 1).
 *
 * **One read, not four.** Everything the first panel says — what is overdue and what is due next —
 * is one wordless search: the entries assigned to this reader that are open and carry a date, in
 * the order the contract already sorts them, `due_at` ascending. Where the line between the two
 * falls is the reader's own question and is drawn on the client, because "overdue" depends on
 * their time zone and on whether a date is a day or an instant (`data/due.ts`), and a server that
 * answered two lists would have to ask it twice.
 *
 * **It is askable at all because of ADR-0064.** `POST /search` took words and nothing else until
 * F10-18; a filter with no words is now a question, and this is the screen that exists to ask it.
 * A narrowing of "mine, open, dated" is also the one read in this client that is deliberately
 * unanchored — the overview is a workspace-wide question, and anchoring it to a container would
 * make it a different one.
 *
 * `@me` is resolved on the server. A client that put the account's own id in the filter would be
 * a second answer to who is signed in, and would be wrong the moment the token belongs to somebody
 * else — which is exactly what the placeholder exists for.
 */

import type { ResourceState, WorkItem, WorkItemPage } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

/**
 * How many of them to read.
 *
 * An overview is what is on somebody, not their whole list: a panel is read in a glance and what
 * does not fit belongs in the search this screen links to. The page is asked larger than the panels
 * draw, because the split into overdue and next happens after the answer arrives.
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

class Overview {
  #mine = $state<ResourceState<WorkItemPage>>({ status: 'idle' });

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
   * A search page can be short for a reason that has nothing to do with the reader — it answers
   * what the caller may see rather than refusing what they may not — so `has_more` is the only
   * honest reading of "there is more", and the panel's link to the search is what follows it.
   */
  get hasMore(): boolean {
    return this.#mine.status === 'ready' && this.#mine.data.page?.has_more === true;
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<WorkItemPage>({ path: '/search', body: MINE }, (next) => {
      this.#mine = next;
    });
  }
}

export const overview = new Overview();
