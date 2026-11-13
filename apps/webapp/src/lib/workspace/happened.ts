// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What happened in a hub (work-surface §6.1, scene hub): the entries changed last with their last
// step, and the pages and boards changed last, merged newest first into one list of sentences.

import type { ActivityEntry, Page, Whiteboard, WorkItem } from '@hubtask/sync-engine';

export interface Happening {
  readonly kind: 'entry' | 'page' | 'board';
  readonly id: string;
  readonly title: string;
  readonly href: string;
  readonly at: string;
  readonly actorId?: string;
  readonly actorKind?: string;
  /** An entry's history code, which says the verb; absent for a page or a board. */
  readonly code?: string;
  readonly collectionId?: string;
}

interface Reads {
  readonly entries: readonly WorkItem[];
  /** Each entry's last step by entry id; null where it has none, absent while it is read. */
  readonly steps: Readonly<Record<string, ActivityEntry | null>>;
  readonly pages: readonly Pick<Page, 'id' | 'title' | 'updated_at' | 'updated_by'>[];
  readonly boards: readonly Pick<Whiteboard, 'id' | 'title' | 'updated_at' | 'updated_by'>[];
}

/** The verbs a sentence has; any other step of an entry is said as a change. */
const VERBS: Record<string, string> = {
  'activity.item_created': 'created',
  'activity.item_duplicated': 'created',
  'activity.item_completed': 'completed',
  'activity.item_reopened': 'reopened',
  'activity.item_step_taken': 'moved_on',
  'activity.item_commented': 'commented',
};

/** The message suffix of a happening's sentence (`app.hub_overview.did_<verb>`). */
export function happenedVerb(each: Happening): string {
  if (each.kind === 'page') return 'page_changed';
  if (each.kind === 'board') return 'board_changed';
  return VERBS[each.code ?? ''] ?? 'changed';
}

/**
 * The newest happenings, at most `shown`. An entry waits for its step, so its sentence never names
 * a verb it does not know; one without a history is said as changed by somebody.
 */
export function latestHappenings(reads: Reads, shown: number): Happening[] {
  const all: Happening[] = [];
  for (const entry of reads.entries) {
    const step = reads.steps[entry.id];
    if (step === undefined) continue;
    const at = step?.occurred_at ?? entry.updated_at;
    if (!at) continue;
    all.push({
      kind: 'entry', id: entry.id, title: entry.title, href: `/items/${entry.id}`, at,
      actorId: step?.actor.id ?? undefined, actorKind: step?.actor.type, code: step?.code, collectionId: entry.collection_id,
    });
  }
  // A row without its time says nothing of when; it is left out rather than sorted as now.
  for (const page of reads.pages) {
    if (!page.updated_at) continue;
    all.push({ kind: 'page', id: page.id, title: page.title, href: `/pages/${page.id}`, at: page.updated_at, actorId: page.updated_by, actorKind: 'USER' });
  }
  for (const board of reads.boards) {
    if (!board.updated_at) continue;
    all.push({ kind: 'board', id: board.id, title: board.title, href: `/whiteboards/${board.id}`, at: board.updated_at, actorId: board.updated_by, actorKind: 'USER' });
  }
  return all.sort((a, b) => b.at.localeCompare(a.at)).slice(0, shown);
}
