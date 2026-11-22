// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Where an entry can go next, as `StatusButton` draws it (work-surface concept §6.9, E-23): the
// server's answer to `GET /items/{id}/steps` turned into the button's model. The server decides
// what is allowed and why the rest is not (`WorkflowGate`, rule 2); this only names and orders it.
// Without a workflow the entry's level holds (UC-FLW-06 check 5): in a list (L0) to do and done; on
// a board (L1, `state_mode` COLUMNS) the board's columns are the states, the `completes` one done.

import type { StatusCategory, StatusRef, WorkflowStep } from '@hubtask/design-system/components';
import type { EntrySteps, StepVerdict, WorkflowStatus } from '@hubtask/sync-engine';

type BlockedStep = NonNullable<EntrySteps['blocked']>[number];

/** The words the model needs, resolved by the caller (no sentence here). */
export interface StepWords {
  status(status: Pick<WorkflowStatus, 'key' | 'name' | 'category'>): string;
  /** What taking an allowed step asks or needs, one line; undefined when nothing. */
  asks(step: StepVerdict): string | undefined;
  /** Why a blocked step may not be taken by this reader (P-05: information, not a control). */
  reason(step: BlockedStep): string;
  /** *Done is reached by: Request review → Approve*. */
  doneRoute(names: readonly string[]): string;
}

export interface EntryStatusModel {
  readonly current: StatusRef;
  readonly steps: readonly WorkflowStep[];
  /** Without a workflow: the statuses to choose among (to do, done). */
  readonly statuses?: readonly StatusRef[];
  readonly doneHint?: string;
  readonly byDefault: boolean;
  /** At L1: choosing a state moves the entry into this column (the server completes or reopens). */
  readonly columns?: boolean;
}

export function categoryOf(category: WorkflowStatus['category']): StatusCategory {
  if (category === 'DONE') return 'done';
  if (category === 'IN_PROGRESS') return 'progress';
  return 'todo';
}

// In a list (L0) an entry holds no "in progress" of its own; the default offers what the server
// keeps: to do and done.
const DEFAULT_KEYS = ['todo', 'done'] as const;

/** At L1 the board's columns as states: the first open, the done one done, the rest in progress. */
function readColumns(answer: EntrySteps, isCompleted: boolean): EntryStatusModel | undefined {
  const columns = answer.state_mode === 'COLUMNS' ? (answer.columns ?? []) : [];
  const states: StatusRef[] = columns.map((column, index) => ({
    id: column.id,
    label: column.name,
    category: column.completes ? 'done' : index === 0 ? 'todo' : 'progress',
  }));
  // An entry in no column reads as the first when open, as the done one when completed (Board).
  const current = states.find((each) => each.id === answer.column_id)
    ?? (isCompleted ? states.find((each) => each.category === 'done') : undefined)
    ?? states[0];
  if (!current) return undefined;
  return {
    current,
    byDefault: true,
    columns: true,
    statuses: states,
    steps: states.filter((each) => each.id !== current.id).map((to) => ({ id: to.id, label: to.label, to, isNext: to.category === 'done' })),
  };
}

function defaultFlow(isCompleted: boolean, ref: (key: string) => StatusRef): EntryStatusModel {
  const current = ref(isCompleted ? 'done' : 'todo');
  const choices = DEFAULT_KEYS.map(ref);
  return {
    current,
    byDefault: true,
    statuses: choices,
    steps: choices.filter((each) => each.id !== current.id).map((to) => ({ id: to.id, label: to.label, to, isNext: to.category === 'done' })),
  };
}

/**
 * Whether an entry's steps may be asked: not where workflows are off at the workspace or at the
 * entry's hub, which the server refuses with `module.off`. `placeOn` undefined is not read yet
 * (wait), null is unreadable (the workspace decides); an unknown workspace is asked, as
 * `modulereads.svelte.ts` does.
 */
export function stepsAskable(workspaceOn: readonly string[] | undefined, placeOn: readonly string[] | null | undefined): boolean | undefined {
  if (workspaceOn !== undefined && !workspaceOn.includes('WORKFLOWS')) return false;
  if (placeOn === undefined) return undefined;
  return placeOn === null || placeOn.includes('WORKFLOWS');
}

/** The default flow without the server's answer: where workflows are off, none can run. */
export function defaultSteps(isCompleted: boolean, words: Pick<StepWords, 'status'>): EntryStatusModel {
  return defaultFlow(isCompleted, (key) => {
    const category = key === 'done' ? 'DONE' : 'TODO';
    return { id: key, label: words.status({ key, name: null, category }), category: categoryOf(category) };
  });
}

export function readSteps(answer: EntrySteps, isCompleted: boolean, words: StepWords): EntryStatusModel {
  const statuses = answer.picture?.statuses ?? [];
  const ref = (key: string): StatusRef => {
    const status = statuses.find((each) => each.key === key);
    return status
      ? { id: status.key, label: words.status(status), category: categoryOf(status.category) }
      : { id: key, label: words.status({ key, name: null, category: key === 'done' ? 'DONE' : 'TODO' }), category: key === 'done' ? 'done' : 'todo' };
  };
  const stepName = (name: string | null | undefined, to: string) => name ?? ref(to).label;

  if (answer.by_default || !answer.workflow || !answer.status) {
    return readColumns(answer, isCompleted) ?? defaultFlow(isCompleted, ref);
  }

  const nextKey = answer.done_route?.[0]?.key;
  const allowed: WorkflowStep[] = answer.steps
    .filter((step) => step.allowed !== false)
    .map((step) => ({ id: step.key, label: stepName(step.name, step.to), to: ref(step.to), asks: words.asks(step), isNext: step.key === nextKey }));
  const blocked: WorkflowStep[] = (answer.blocked ?? []).map((step) => ({
    id: step.key,
    label: stepName(step.name, step.to),
    to: ref(step.to),
    blockedReason: words.reason(step),
  }));
  const route = answer.done_route ?? [];
  return {
    current: ref(answer.status),
    byDefault: false,
    steps: [...allowed, ...blocked],
    doneHint: route.length > 0 ? words.doneRoute(route.map((step) => stepName(step.name, step.to))) : undefined,
  };
}
