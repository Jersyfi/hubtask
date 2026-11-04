// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The pages waiting for the reader's review across the hubs (user-experience.md §3.3): each hub's
 * own pages read, only where pages are on there - a read of a module that is off is refused -,
 * gathered as the pages' home gathers them. *Today*'s band and the *Today* row's count read the
 * same paths, so they always say the same.
 */

import type { PageList } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { modules } from './modules.svelte.ts';
import { pagesPath } from './pages.svelte.ts';
import { pagesToReview } from '../product/home.ts';

export type ReviewRow = PageList['data'][number];

/** Watches the hubs' pages to review; answers every change. **From `untrack`**. */
export function watchPageReviews(hubIds: readonly string[], viewerId: string | undefined, onNext: (rows: ReviewRow[]) => void): () => void {
  const on: Record<string, boolean> = {};
  const lists: Record<string, PageList['data']> = {};
  const reads = new Map<string, () => void>();
  const answer = () => onNext(pagesToReview(lists, hubIds.filter((id) => on[id] === true), viewerId));
  const stops = hubIds.map((hubId) => modules.here(hubId, (next) => {
    if (next.status !== 'ready') return;
    on[hubId] = Array.isArray(next.data.modules_on) && next.data.modules_on.includes('KNOWLEDGE');
    if (on[hubId] && !reads.has(hubId)) {
      reads.set(hubId, engine.subscribe<PageList>({ path: pagesPath(hubId) }, (pages) => {
        if (pages.status === 'ready') {
          lists[hubId] = pages.data.data;
          answer();
        }
      }));
    } else if (!on[hubId]) {
      reads.get(hubId)?.();
      reads.delete(hubId);
      answer();
    }
  }));
  return () => {
    stops.forEach((stop) => stop());
    reads.forEach((stop) => stop());
  };
}
