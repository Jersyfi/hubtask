// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The saved views along a container's path, the export, and the calendar feeds.
 *
 * **The export is a read that answers a file.** It goes through the seam's `document`, which is a
 * `POST` for the reason `/search` is one — a view's query is the caller's content, and a query
 * string travels through access logs, proxies and browser history. Nothing of the query reaches a
 * URL this client composes.
 *
 * **A feed's token exists once.** It is in the answer that minted it and nowhere afterwards: not in
 * this store, not in a log, not in a URL this client builds. The list carries no token at all, and
 * a feed whose token was lost is revoked and made again.
 */

import type {
  CalendarFeed,
  CalendarFeedCreate,
  CalendarFeedSecret,
  ResourceState,
  SavedView,
  SavedViewCreate,
  SavedViewUpdate,
  TransportDocument,
  ViewExport,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';

/**
 * Where a path's views live.
 *
 * A **bare array**, not a `{data, page}` envelope — the contract declares one, and it is worth
 * saying out loud because most lists in this client are paged. The set a person picks from in one
 * collection is bounded by what people will make and read at a glance.
 */
export const viewsPath = (containerId?: string) =>
  containerId ? `/views?container_id=${containerId}` : '/views';

export const FEEDS_PATH = '/integrations/calendar-feeds';

const TOUCHES = ['/views'];

class Views {
  #paths = $state<Record<string, ResourceState<readonly SavedView[]>>>({});

  of(containerId?: string): readonly SavedView[] {
    const state = this.#paths[viewsPath(containerId)];
    return state?.status === 'ready' ? state.data : [];
  }

  stateOf(containerId?: string): ResourceState<readonly SavedView[]> | undefined {
    return this.#paths[viewsPath(containerId)];
  }

  /** Starts one path's list. **From `untrack`**, for the reason every other store records. */
  open(containerId?: string): () => void {
    const path = viewsPath(containerId);
    return engine.subscribe<readonly SavedView[]>({ path }, (next) => {
      this.#paths = { ...this.#paths, [path]: next };
    });
  }

  async create(body: SavedViewCreate, idempotencyKey: string): Promise<SavedView> {
    return engine.mutate<SavedView>('POST', '/views', body, {
      idempotencyKey,
      invalidates: TOUCHES,
    });
  }

  /** Changes its own fields. The scope and the sharing are not among them, by the contract's design. */
  async update(id: string, body: SavedViewUpdate, version: number): Promise<SavedView> {
    return engine.mutate<SavedView>('PATCH', `/views/${id}`, body, {
      ifMatch: etagFor(version),
      invalidates: TOUCHES,
    });
  }

  async remove(id: string, version: number): Promise<void> {
    await engine.mutate<void>('DELETE', `/views/${id}`, undefined, {
      ifMatch: etagFor(version),
      invalidates: TOUCHES,
    });
  }

  /** Decides who sees it. `STRUCTURE` on the scope's path is what this takes; the caller gates. */
  async share(
    id: string,
    sharing: 'PRIVATE' | 'SCOPE' | 'PUBLIC_LINK',
    version: number,
    idempotencyKey: string,
  ): Promise<SavedView> {
    return engine.mutate<SavedView>('POST', `/views/${id}:share`, { sharing }, {
      idempotencyKey,
      ifMatch: etagFor(version),
      invalidates: TOUCHES,
    });
  }

  /**
   * The view's rows, rendered whole, as a file.
   *
   * A generous deadline: this is one synchronous render of up to `max_export_rows`, which is a
   * different kind of wait from a document read — and the API's own deadline would cut off a large
   * one that was working perfectly well.
   */
  async export(id: string, body: ViewExport): Promise<TransportDocument> {
    return engine.document(`/views/${id}:export`, body, { timeoutMs: 120_000 });
  }
}

class Feeds {
  #state = $state<ResourceState<readonly CalendarFeed[]>>({ status: 'idle' });

  get all(): readonly CalendarFeed[] {
    return this.#state.status === 'ready' ? this.#state.data : [];
  }

  get state(): ResourceState<readonly CalendarFeed[]> {
    return this.#state;
  }

  open(): () => void {
    return engine.subscribe<readonly CalendarFeed[]>({ path: FEEDS_PATH }, (next) => {
      this.#state = next;
    });
  }

  /**
   * Mints one, and answers the token — for the only time.
   *
   * What comes back is handed straight to the caller and held nowhere: a store that kept it would
   * be a second place it exists, and the whole point of "shown once" is that there is not one.
   */
  async create(body: CalendarFeedCreate, idempotencyKey: string): Promise<CalendarFeedSecret> {
    return engine.mutate<CalendarFeedSecret>('POST', FEEDS_PATH, body, {
      idempotencyKey,
      invalidates: [FEEDS_PATH],
    });
  }

  /** Revokes it. The row stays with the moment, so "revoked on Tuesday" is answerable. */
  async revoke(id: string): Promise<void> {
    await engine.mutate<void>('DELETE', `${FEEDS_PATH}/${id}`, undefined, {
      invalidates: [FEEDS_PATH],
    });
  }
}

export const views = new Views();
export const feeds = new Feeds();
