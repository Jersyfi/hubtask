// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The state control by level (build-plan §5), as data: one press - the mark - and where the entry
 * stands - the chip - on every level, the same on the list row, the board card and the phone sheet
 * (UC-WRK-05 check 11). L0 has the mark alone, a chip would only repeat it; L1 a column chip whose
 * menu lists the columns; L2 a status chip whose menu lists the steps out of the entry's status.
 * A workflow in force while the Workflows module is off is paused: its status is kept on the server,
 * the chip says so and offers nothing, and the mark completes where the server lets it.
 */

import type { Bucket, StateMode, WorkflowShape } from '@hubtask/sync-engine';

export type Category = 'todo' | 'progress' | 'done';

export interface ChipChoice {
  /** `column:<id>` or `step:<key>`. */
  readonly id: string;
  readonly label: string;
  readonly isCurrent: boolean;
}

export interface Chip {
  readonly label: string;
  readonly category: Category;
  readonly choices: readonly ChipChoice[];
  /** A workflow in force with its module off: the chip only reads, and says why. */
  readonly pausedReason?: string;
}

/**
 * What one press on the mark does: completes or reopens the entry by the base product's own write
 * (L0, L1, a paused workflow), or takes a workflow's step (L2) - `completes` where it lands in a
 * done status, so the mark is a tick there and an arrow otherwise (E5).
 */
export type Mark =
  | { readonly kind: 'complete' | 'reopen' }
  | { readonly kind: 'step'; readonly choice: ChipChoice; readonly completes: boolean; readonly reopens: boolean };

const CATEGORY: Record<string, Category> = { TODO: 'todo', IN_PROGRESS: 'progress', DONE: 'done' };

type Entry = { bucket_id?: string | null; status?: string | null; completion?: { is_completed?: boolean } | null };

type Names = {
  noColumn: string;
  statusName: (key: string) => string;
  stepName: (key: string, name: string | null | undefined, to: string) => string;
  /** The paused chip's word and its reason, where a workflow is in force with its module off. */
  paused?: { label: string; reason: string };
};

/** The steps of a shape out of a status: those that name it, and those open from anywhere. */
function stepsOut(workflow: WorkflowShape, from: string | null | undefined) {
  return workflow.steps.filter((step) => {
    const sources = step.from ?? [];
    return step.to !== from && (sources.length === 0 || sources.includes(from ?? ''));
  });
}

/**
 * The chip of one entry, or undefined at L0 (the mark alone) and wherever the level is unknown.
 * At L2 the workflow is paused where `names.paused` says so, whatever shape is still at hand.
 */
export function chipOf(
  level: StateMode | undefined,
  entry: Entry,
  columns: readonly Pick<Bucket, 'id' | 'name' | 'is_done_bucket'>[],
  workflow: WorkflowShape | undefined,
  names: Names,
): Chip | undefined {
  if (level === 'COLUMNS') {
    const here = columns.find((column) => column.id === entry.bucket_id);
    return {
      label: here?.name ?? names.noColumn,
      category: here?.is_done_bucket || entry.completion?.is_completed ? 'done' : columns[0]?.id === here?.id ? 'todo' : 'progress',
      choices: columns.map((column) => ({ id: `column:${column.id}`, label: column.name, isCurrent: column.id === entry.bucket_id })),
    };
  }
  if (level === 'WORKFLOW' && names.paused) {
    return {
      label: names.paused.label,
      category: entry.completion?.is_completed ? 'done' : 'todo',
      choices: [],
      pausedReason: names.paused.reason,
    };
  }
  if (level === 'WORKFLOW' && workflow) {
    const status = workflow.statuses.find((each) => each.key === entry.status);
    const from = entry.status ?? workflow.initial;
    return {
      label: entry.status ? names.statusName(entry.status) : names.noColumn,
      category: CATEGORY[status?.category ?? 'TODO'] ?? 'todo',
      choices: stepsOut(workflow, from).map((step) => ({ id: `step:${step.key}`, label: names.stepName(step.key, step.name, step.to), isCurrent: false })),
    };
  }
  return undefined;
}

/**
 * The first step on the shortest way from a status to a done one, or undefined where none leads
 * there. The server still decides whether the reader may take it (rule 2).
 */
function towardsDone(workflow: WorkflowShape, from: string) {
  const isDone = (key: string) => workflow.statuses.find((status) => status.key === key)?.category === 'DONE';
  const firstOf = new Map<string, (typeof workflow.steps)[number]>();
  const queue = [from];
  const seen = new Set(queue);
  while (queue.length > 0) {
    const at = queue.shift() ?? '';
    for (const step of stepsOut(workflow, at)) {
      if (seen.has(step.to)) continue;
      seen.add(step.to);
      const first = firstOf.get(at) ?? step;
      if (isDone(step.to)) return first;
      firstOf.set(step.to, first);
      queue.push(step.to);
    }
  }
  return undefined;
}

/**
 * The mark of one entry: at L0, L1 and while a workflow is paused it completes or reopens; at L2 it
 * takes the next step towards done, and out of a done status the first step out. Undefined where no
 * step leads on - the mark is drawn, not pressable.
 */
export function markOf(
  level: StateMode | undefined,
  entry: Entry,
  workflow: WorkflowShape | undefined,
  stepName: Names['stepName'],
): Mark | undefined {
  const isCompleted = entry.completion?.is_completed ?? false;
  if (level !== 'WORKFLOW' || !workflow) return { kind: isCompleted ? 'reopen' : 'complete' };
  const from = entry.status ?? workflow.initial ?? '';
  const category = (key: string) => workflow.statuses.find((status) => status.key === key)?.category;
  const step = category(from) === 'DONE' ? stepsOut(workflow, from)[0] : towardsDone(workflow, from);
  if (!step) return undefined;
  return {
    kind: 'step',
    choice: { id: `step:${step.key}`, label: stepName(step.key, step.name, step.to), isCurrent: false },
    completes: category(step.to) === 'DONE',
    reopens: category(from) === 'DONE',
  };
}
