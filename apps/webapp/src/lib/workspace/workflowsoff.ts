// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What switching the Workflows module off strands (UC-INS-21 check 14): a workflow
 * stays in force on the server with its module off (`WorkflowGate`), so the entries standing in its
 * statuses can no longer move on, and some not even be completed. Composed from reads the module
 * still answers while it is on - the scheme (`GET /workflows`) and, per scheme row, the preview of
 * removing it (`POST /workflow-scheme:preview`), which changes nothing - and from each board's
 * columns. No new server read: the numbers are the web app's sum of those answers.
 */

import type { Bucket, WorkflowList, WorkflowPreview, WorkflowShape } from '@hubtask/sync-engine';

export type SchemeRow = WorkflowList['scheme'][number];

/** One scheme row with the preview of taking it away. */
export interface RowReach {
  readonly row: SchemeRow;
  readonly preview: WorkflowPreview;
}

/** Where the open entries of one status of one collection go when the workflow goes. */
export interface StatusLine {
  readonly collectionId: string;
  readonly status: string;
  readonly count: number;
  /** The column that showed the status and keeps the entries; undefined: the first column or, without a board, open. */
  readonly columnId?: string;
  /** No step leads from the status straight into a done one: paused, they cannot be completed. */
  readonly isStuck: boolean;
}

export interface Stranded {
  /** The places a workflow runs in, as the scheme names them: a hub or a collection. */
  readonly places: readonly string[];
  /** The collections reached that hold an entry or a board. */
  readonly collections: readonly string[];
  readonly open: number;
  readonly stuck: number;
  readonly lines: readonly StatusLine[];
}

/**
 * The scheme rows switching off reaches: every row at the workspace; at a hub the hub's own row and
 * those of its collections - a row elsewhere keeps running there.
 */
export function rowsInReach(scheme: readonly SchemeRow[], hubId: string | undefined, collectionsOfHub: readonly string[]): SchemeRow[] {
  if (!hubId) return [...scheme];
  return scheme.filter((row) => row.container_id === hubId || collectionsOfHub.includes(row.container_id));
}

/** Whether a status has a step straight into a done status. */
function reachesDone(shape: WorkflowShape | undefined, status: string): boolean {
  if (!shape) return true;
  const isDone = (key: string) => shape.statuses.find((each) => each.key === key)?.category === 'DONE';
  return shape.steps.some((step) => {
    const sources = step.from ?? [];
    return isDone(step.to) && (sources.length === 0 || sources.includes(status));
  });
}

/**
 * Sums the previews: the open entries standing in a status, by collection and status, with the
 * column that showed it and whether they could still be completed while paused.
 */
export function strandedBy(
  reaches: readonly RowReach[],
  workflows: WorkflowList['data'],
  columnsOf: (collectionId: string) => readonly Pick<Bucket, 'id' | 'statuses' | 'is_done_bucket'>[],
): Stranded {
  const lines = new Map<string, StatusLine>();
  // Counted per move: two rows of one collection may run different workflows through one status.
  let stuck = 0;
  const collections = new Set<string>();
  for (const { row, preview } of reaches) {
    const shape = workflows.find((each) => each.key === row.workflow)?.published ?? undefined;
    for (const board of preview.boards ?? []) collections.add(board.collection_id);
    for (const move of preview.moves) {
      collections.add(move.collection_id);
      if (move.completed || !move.from) continue;
      const key = `${move.collection_id} ${move.from}`;
      const before = lines.get(key);
      const isStuck = !reachesDone(shape, move.from);
      if (isStuck) stuck += move.count;
      const column = columnsOf(move.collection_id).find((each) => !each.is_done_bucket && (each.statuses ?? []).includes(move.from ?? ''));
      lines.set(key, {
        collectionId: move.collection_id,
        status: move.from,
        count: (before?.count ?? 0) + move.count,
        columnId: column?.id,
        isStuck: (before?.isStuck ?? false) || isStuck,
      });
    }
  }
  const all = [...lines.values()];
  return {
    places: [...new Set(reaches.map((each) => each.row.container_id))],
    collections: [...collections],
    open: all.reduce((sum, line) => sum + line.count, 0),
    stuck,
    lines: all,
  };
}

/** What the person chose for the stranded entries; nothing is chosen at first (E2). */
export type OffChoice = 'convert' | 'pause' | undefined;

/** Whether the switch-off waits for a choice: only where entries stand in a status. */
export function needsChoice(stranded: Stranded | undefined): boolean {
  return (stranded?.open ?? 0) > 0;
}
