// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What happened in a hub (work-surface §6.1, scene hub): the entries changed last with their last
// step, and the pages and boards changed last, merged newest first into one list of sentences.

import type { ActivityEntry, Page, Whiteboard, WorkflowList, WorkItem } from '@hubtask/sync-engine';

import { statusLabel, workflowFor } from '../data/workflows.ts';

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
  readonly entryType?: string;
  /** The status a step set, from its change set; the sentence names it. */
  readonly statusKey?: string;
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
  'activity.item_commented': 'commented',
};

/** The message suffix of a happening's sentence (`app.hub_overview.did_<verb>`). */
export function happenedVerb(each: Happening): string {
  if (each.kind === 'page') return 'page_changed';
  if (each.kind === 'board') return 'board_changed';
  const verb = VERBS[each.code ?? ''];
  if (verb) return verb;
  if (each.statusKey) return 'set_status';
  return 'changed';
}

/** The status a step set, where its change set kept one. */
function statusSet(step: ActivityEntry | null | undefined): string | undefined {
  const status = (step?.change_set as Record<string, { to?: unknown } | undefined> | undefined)?.status;
  return typeof status?.to === 'string' ? status.to : undefined;
}

/**
 * A status's name as the entry's workflow names it, else as any workflow does, else the client's
 * word for a shipped status; the key itself only where nothing names it.
 */
export function statusNameOf(
  each: Pick<Happening, 'statusKey' | 'collectionId' | 'entryType'>, hubId: string, list: WorkflowList | undefined,
  t: (code: string) => string,
): string {
  const key = each.statusKey ?? '';
  const own = each.collectionId ? workflowFor(list, each.collectionId, hubId, each.entryType ?? 'task') : undefined;
  const shapes = [own, ...(list?.data ?? [])].flatMap((workflow) => (workflow?.published ? [workflow.published] : []));
  for (const shape of shapes) {
    const status = shape.statuses.find((candidate) => candidate.key === key);
    if (status) return statusLabel(status, t);
  }
  if (key === 'todo' || key === 'in_progress' || key === 'done') return t(`workflows.status.${key}`);
  return key;
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
      entryType: entry.type, statusKey: statusSet(step),
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
