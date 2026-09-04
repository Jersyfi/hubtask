// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Full-text search, which is a different question from filtering and is asked differently.
 *
 * **A `POST`, and there is no `GET`.** What somebody is looking for is their content, and a query
 * string travels through access logs, proxies and browser history (`security.md` §9, ADR-0018).
 * Nothing here puts the term in a URL, and the screen that uses it does not put it in the address
 * bar either — a client that helpfully reflected it would undo the reason the operation is a
 * `POST`.
 *
 * **A short page is not the end.** This is the one read of the API that is unanchored, and the
 * contract says what that costs: it "answers what the caller may see rather than refusing what
 * they may not, so a page can be shorter than the size asked for. Walk on until `has_more` is
 * false rather than stopping at the first short page." So this walks, and the walk is the reason
 * search is a store of its own rather than a `resource()` — the engine's `loadMore` appends one
 * page per press, which is right for a list a person is paging and wrong for a read whose pages
 * are short for a reason that has nothing to do with the reader.
 *
 * **Two languages are in play.** The entry is indexed under the language it was written in; the
 * *query* is read under the caller's, which is this request's `language`. `text_languages` is what
 * a picker for it is built from — the installation's answer rather than the product's, because the
 * mapping from a tag to a text search configuration is what its PostgreSQL was built with
 * (ADR-0034).
 */

import type { TransportError, WorkItem, WorkItemPage } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

/** How many pages one search walks at most, so a slow installation cannot be asked forever. */
const MAX_PAGES = 10;

const PAGE_SIZE = 50;

export interface SearchAsked {
  readonly q: string;
  /** BCP-47, from `text_languages`. Empty means the caller's own locale, which is the default. */
  readonly language?: string;
  /** A hub or a collection to look in. Omitted searches everything the caller may see. */
  readonly containerId?: string;
}

class Search {
  #hits = $state<readonly WorkItem[]>([]);
  #status = $state<'idle' | 'searching' | 'done' | 'failed'>('idle');
  #error = $state<TransportError | undefined>(undefined);
  /** Whether the walk stopped at `MAX_PAGES` rather than at the end of the results. */
  #isPartial = $state(false);
  /** Which search the answers on screen belong to, so a slower earlier one cannot overwrite them. */
  #generation = 0;

  get hits(): readonly WorkItem[] {
    return this.#hits;
  }

  get status(): 'idle' | 'searching' | 'done' | 'failed' {
    return this.#status;
  }

  get error(): TransportError | undefined {
    return this.#error;
  }

  get isPartial(): boolean {
    return this.#isPartial;
  }

  /** Empties it. What clearing the field does, and what leaving the screen should do. */
  reset(): void {
    this.#generation += 1;
    this.#hits = [];
    this.#status = 'idle';
    this.#error = undefined;
    this.#isPartial = false;
  }

  /**
   * Runs one search, walking until the server says there is no more.
   *
   * The generation is not a nicety. Somebody typing narrows their search several times a second
   * and each keystroke is a request; without it the answer to "mil" can arrive after the answer to
   * "milk" and replace it, which looks exactly like a search that ignores what was typed last.
   */
  async run(asked: SearchAsked): Promise<void> {
    const term = asked.q.trim();
    if (term === '') {
      this.reset();
      return;
    }

    this.#generation += 1;
    const mine = this.#generation;
    this.#status = 'searching';
    this.#error = undefined;
    this.#isPartial = false;

    const found: WorkItem[] = [];
    let cursor: string | null | undefined;

    try {
      for (let page = 0; page < MAX_PAGES; page += 1) {
        const answer = await engine.mutate<WorkItemPage>(
          'POST',
          '/search',
          {
            q: term,
            ...(asked.language ? { language: asked.language } : {}),
            ...(asked.containerId ? { container_id: asked.containerId } : {}),
            page: { size: PAGE_SIZE, ...(cursor ? { cursor } : {}) },
          },
          // A search writes nothing, so it makes nothing stale. Naming an empty list of prefixes
          // is what says so: the default drops everything the client holds, which for a read would
          // reload every screen behind this one.
          { invalidates: [] },
        );

        // A later search has started. Everything from here on belongs to an answer nobody is
        // waiting for any more.
        if (mine !== this.#generation) return;

        found.push(...(answer.data ?? []));
        this.#hits = [...found];
        cursor = answer.page?.next_cursor;
        // `has_more`, not the length of the page. This is the one read where a short page means
        // "some of what is here is not yours to see" rather than "that was the last of it".
        if (!answer.page?.has_more || !cursor) {
          this.#status = 'done';
          return;
        }
      }
      // The walk hit its own bound rather than the end of the results, and the screen says so
      // instead of presenting a partial answer as a complete one.
      this.#isPartial = true;
      this.#status = 'done';
    } catch (error) {
      if (mine !== this.#generation) return;
      this.#error = error as TransportError;
      this.#status = 'failed';
    }
  }
}

export const search = new Search();
