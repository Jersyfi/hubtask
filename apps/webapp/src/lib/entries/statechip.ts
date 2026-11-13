// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The state control by level (build-plan §5), as data: L0 completes with a checkbox and has no chip;
 * L1 a column chip whose menu lists the columns; L2 a status chip whose menu lists the steps out of
 * the entry's status. The same on the list row, the board card and the phone sheet.
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
}

const CATEGORY: Record<string, Category> = { TODO: 'todo', IN_PROGRESS: 'progress', DONE: 'done' };

/**
 * The chip of one entry, or undefined at L0 (a checkbox) and wherever the level is unknown.
 * `noColumn` names the entry without column or status; `statusName` a workflow status.
 */
export function chipOf(
  level: StateMode | undefined,
  entry: { bucket_id?: string | null; status?: string | null; completion?: { is_completed?: boolean } | null },
  columns: readonly Pick<Bucket, 'id' | 'name' | 'is_done_bucket'>[],
  workflow: WorkflowShape | undefined,
  names: { noColumn: string; statusName: (key: string) => string; stepName: (key: string, name: string | null | undefined, to: string) => string },
): Chip | undefined {
  if (level === 'COLUMNS') {
    const here = columns.find((column) => column.id === entry.bucket_id);
    return {
      label: here?.name ?? names.noColumn,
      category: here?.is_done_bucket || entry.completion?.is_completed ? 'done' : columns[0]?.id === here?.id ? 'todo' : 'progress',
      choices: columns.map((column) => ({ id: `column:${column.id}`, label: column.name, isCurrent: column.id === entry.bucket_id })),
    };
  }
  if (level === 'WORKFLOW' && workflow) {
    const status = workflow.statuses.find((each) => each.key === entry.status);
    const from = entry.status ?? workflow.initial;
    const steps = workflow.steps.filter((step) => {
      const sources = step.from ?? [];
      return step.to !== from && (sources.length === 0 || sources.includes(from ?? ''));
    });
    return {
      label: entry.status ? names.statusName(entry.status) : names.noColumn,
      category: CATEGORY[status?.category ?? 'TODO'] ?? 'todo',
      choices: steps.map((step) => ({ id: `step:${step.key}`, label: names.stepName(step.key, step.name, step.to), isCurrent: false })),
    };
  }
  return undefined;
}
