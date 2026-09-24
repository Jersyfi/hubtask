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
 * **No language is chosen here, and that is deliberate** (ADR-0066 decision 1). An entry used to be
 * indexed under the language it was written in while the query was read under the caller's, which
 * is why a workspace written in one language and read in another could answer "nothing matches"
 * about an entry plainly there — and why this screen grew a picker, a widening and a badge to work
 * around it. The fix is where the asymmetry was: the stored document carries the entry's own
 * configuration *and* `simple`, so every word form is found whoever asks. `ItemSearchQuery.language`
 * is still in the contract and is simply not sent.
 */

import type { TransportError, WorkItem, WorkItemPage } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { keyFor, keysIn } from './searchhandle.ts';

/** How many pages one search walks at most, so a slow installation cannot be asked forever. */
const MAX_PAGES = 10;

const PAGE_SIZE = 50;

/** How much of the search to use (J-10): words and, where the installation has it, meaning; or words only. */
export type SearchMode = 'AUTO' | 'LEXICAL';

export interface SearchAsked {
  readonly q: string;
  /**
   * `AUTO` by default, which is the contract's own. Sent only when the reader chose, so that an
   * installation without meaning is never asked for a mode it could not promise - there is
   * deliberately no `SEMANTIC`.
   */
  readonly mode?: SearchMode;
  /** A hub or a collection to look in. Omitted searches everything the caller may see. */
  readonly containerId?: string;
  /**
   * What narrows the hits, in the grammar the item query uses (ADR-0064).
   *
   * It travels as it was built — `searchquery.ts` compiles it and the domain reads it — and it is
   * what makes a search askable without words at all: a filter with no term is a work list,
   * ordered by when it is due rather than ranked.
   */
  readonly filter?: unknown;
  /**
   * The order of a search that has no words to rank, and how far it reaches.
   *
   * All three are the contract's own (`ItemSearchQuery`, ADR-0064) and none of them had a control
   * until the filter language: a sort is refused beside words by name, and the two flags are what
   * `is:archived` and `is:trashed` mean.
   */
  readonly sort?: readonly { readonly field: string; readonly direction: 'ASC' | 'DESC' }[];
  readonly includeArchived?: boolean;
  readonly includeTrashed?: boolean;
}

class Search {
  #hits = $state<readonly WorkItem[]>([]);
  #status = $state<'idle' | 'searching' | 'done' | 'failed'>('idle');
  #error = $state<TransportError | undefined>(undefined);
  /** Whether the walk stopped at `MAX_PAGES` rather than at the end of the results. */
  #isPartial = $state(false);
  /** Which search the answers on screen belong to, so a slower earlier one cannot overwrite them. */
  #generation = 0;
  /**
   * The words the app bar handed over, waiting for the screen they were handed to.
   *
   * In memory rather than in the address: the address carries the *narrowing* and never the words
   * (ADR-0063 decision 4 as corrected, issue 997). What makes them survive a reload is not this —
   * it is the handle below.
   */
  #handedOver = $state<string | undefined>(undefined);

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


  /** Whether the bar has words waiting for the search screen. */
  get handedOver(): string | undefined {
    return this.#handedOver;
  }

  /** The bar's own verb: hand the words to the screen it is about to navigate to. */
  handOver(term: string): void {
    this.#handedOver = term;
  }

  /**
   * The screen's own verb: take them, and leave nothing behind.
   *
   * Taken rather than read, because the same words handed over twice are two searches — somebody
   * who searches for the same term again from the bar has asked again, and a value that stayed
   * would make the second press do nothing.
   */
  takeHandover(): string | undefined {
    const term = this.#handedOver;
    this.#handedOver = undefined;
    return term;
  }

  /**
   * Keeps the words under the handle the address carries, so that a reload finds them again.
   *
   * `sessionStorage` for what a search is — the thing somebody is doing now: it survives a reload
   * and the back button, and dies with the tab, exactly as the credential does. A browser that
   * refuses storage still searches; it just forgets across a reload, which is where this started.
   */
  remember(handle: string, term: string): void {
    try {
      if (term === '') globalThis.sessionStorage?.removeItem(keyFor(handle));
      else globalThis.sessionStorage?.setItem(keyFor(handle), term);
    } catch {
      // No storage. The search still works; it does not survive a reload.
    }
  }

  /** What was typed under this handle, or nothing — another tab's handle, or a cleared one. */
  recall(handle: string): string | undefined {
    try {
      return globalThis.sessionStorage?.getItem(keyFor(handle)) ?? undefined;
    } catch {
      return undefined;
    }
  }

  /**
   * Drops every search this tab remembered. Called where the session is discarded.
   *
   * A search term is the reader's content, so it ends with their session rather than with the tab
   * — a shared browser is a real thing, and `sessionStorage` alone would keep it until the tab
   * closed.
   */
  forget(): void {
    try {
      const storage = globalThis.sessionStorage;
      if (!storage) return;
      for (const key of keysIn(storage)) storage.removeItem(key);
    } catch {
      // Nothing to forget from, which is the same outcome.
    }
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
    // Neither words nor a narrowing is not a search; it is the empty screen this started on. With
    // one of the two it is a question the contract answers (ADR-0064).
    if (term === '' && asked.filter === undefined) {
      this.reset();
      return;
    }

    this.#generation += 1;
    const mine = this.#generation;
    this.#status = 'searching';
    this.#error = undefined;
    this.#isPartial = false;

    try {
      const found = await this.#ask(term, asked, mine);
      if (found === undefined) return;
      this.#hits = found;
      this.#status = 'done';
    } catch (error) {
      if (mine !== this.#generation) return;
      this.#error = error as TransportError;
      this.#status = 'failed';
    }
  }

  /**
   * One page, for the menu in the bar.
   *
   * A separate verb rather than `run` with a bound, because the two are different reads and only
   * one of them belongs to a screen. The menu shows the first few hits while somebody is still
   * typing; it must not touch `hits`, `status` or the generation, or the search screen behind it
   * would flicker on every keystroke made in the bar.
   *
   * It does not walk. A short page is the unanchored read's own shape, so what a menu shows is
   * "some of what matches" - which is what a menu promises anyway. The screen it leads to is where
   * completeness is owed.
   */
  async peek(asked: SearchAsked, limit: number): Promise<readonly WorkItem[]> {
    const term = asked.q.trim();
    if (term === '' && asked.filter === undefined) return [];
    const answer = await engine.mutate<WorkItemPage>(
      'POST',
      '/search',
      {
        ...(term === '' ? {} : { q: term }),
        ...(asked.filter === undefined ? {} : { filter: asked.filter }),
        ...(asked.sort && asked.sort.length > 0 ? { sort: asked.sort } : {}),
        ...(asked.includeArchived ? { include_archived: true } : {}),
        ...(asked.includeTrashed ? { include_trashed: true } : {}),
        ...(asked.mode ? { mode: asked.mode } : {}),
        ...(asked.containerId ? { container_id: asked.containerId } : {}),
        page: { size: limit },
      },
      { invalidates: [] },
    );
    return (answer.data ?? []).slice(0, limit);
  }

  /**
   * One search, walked to the end or to the bound.
   *
   * Answers `undefined` when a later search has started — everything from that point belongs to an
   * answer nobody is waiting for any more, and the caller stops rather than writing it to the
   * screen.
   */
  async #ask(term: string, asked: SearchAsked, mine: number): Promise<WorkItem[] | undefined> {
    const found: WorkItem[] = [];
    let cursor: string | null | undefined;

    for (let page = 0; page < MAX_PAGES; page += 1) {
      const answer = await engine.mutate<WorkItemPage>(
        'POST',
        '/search',
        {
          ...(term === '' ? {} : { q: term }),
          ...(asked.filter === undefined ? {} : { filter: asked.filter }),
          ...(asked.sort && asked.sort.length > 0 ? { sort: asked.sort } : {}),
          ...(asked.includeArchived ? { include_archived: true } : {}),
          ...(asked.includeTrashed ? { include_trashed: true } : {}),
          // No `language`: the stored document carries the entry's own configuration and
          // `simple` both, so there is nothing left for a caller to choose. The term still never
          // reaches a URL - this is a `POST` because it is content (security.md §9).
          ...(asked.mode ? { mode: asked.mode } : {}),
          ...(asked.containerId ? { container_id: asked.containerId } : {}),
          page: { size: PAGE_SIZE, ...(cursor ? { cursor } : {}) },
        },
        // A search writes nothing, so it makes nothing stale. Naming an empty list of prefixes
        // is what says so: the default drops everything the client holds, which for a read would
        // reload every screen behind this one.
        { invalidates: [] },
      );

      if (mine !== this.#generation) return undefined;

      found.push(...(answer.data ?? []));
      cursor = answer.page?.next_cursor;
      // `has_more`, not the length of the page. This is the one read where a short page means
      // "some of what is here is not yours to see" rather than "that was the last of it".
      if (!answer.page?.has_more || !cursor) return found;
    }

    // The walk hit its own bound rather than the end of the results, and the screen says so
    // instead of presenting a partial answer as a complete one.
    this.#isPartial = true;
    return found;
  }
}

export const search = new Search();
