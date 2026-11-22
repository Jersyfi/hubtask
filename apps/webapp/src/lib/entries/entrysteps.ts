// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Where an entry stands and where it can go next, as `StatusButton` draws it (work-surface concept
// §6.9, E-23), on each of the three levels (UC-FLW-06 check 5): in a list (L0) open and done; on a
// board (L1, `state_mode` COLUMNS) the board's columns, the done one done; under a workflow (L2)
// the server's answer to `GET /items/{id}/steps`. L0 and L1 need nothing of the Workflows module and
// are built from the base reads where it is off; a workflow in force with the module off is paused.
// The server decides what is allowed and why the rest is not (`WorkflowGate`, rule 2); this only
// names and orders it, and says what one press does (`primaryOf`).

import type { StatusCategory, StatusRef, WorkflowStep } from '@hubtask/design-system/components';
import type { Bucket, EntrySteps, StepVerdict, WorkflowStatus } from '@hubtask/sync-engine';

type BlockedStep = NonNullable<EntrySteps['blocked']>[number];

/** L0's two words, resolved by the caller: *Open*, *Done* (E7). */
export interface ListWords {
  readonly open: string;
  readonly done: string;
}

/** The words the model needs, resolved by the caller (no sentence here). */
export interface StepWords {
  status(status: Pick<WorkflowStatus, 'key' | 'name' | 'category'>): string;
  /** What taking an allowed step asks or needs, one line; undefined when nothing. */
  asks(step: StepVerdict): string | undefined;
  /** Why a blocked step may not be taken by this reader (P-05: information, not a control). */
  reason(step: BlockedStep): string;
  /** *Done is reached by: Request review → Approve*. */
  doneRoute(names: readonly string[]): string;
  readonly list: ListWords;
}

export interface EntryStatusModel {
  readonly current: StatusRef;
  readonly steps: readonly WorkflowStep[];
  /** Without a workflow: the states to choose among (open and done, or the columns). */
  readonly statuses?: readonly StatusRef[];
  readonly doneHint?: string;
  /** No workflow decides: completing and reopening are the base product's own write. */
  readonly byDefault: boolean;
  /** At L1: choosing a state moves the entry into this column (the server completes or reopens). */
  readonly columns?: boolean;
  /** A workflow in force while the Workflows module is off: its status is kept, nothing moves it. */
  readonly paused?: boolean;
}

/** What one press of the state control does, the same question on every level. */
export type Primary = { readonly kind: 'complete' | 'reopen' } | { readonly kind: 'step'; readonly step: WorkflowStep };

export function categoryOf(category: WorkflowStatus['category']): StatusCategory {
  if (category === 'DONE') return 'done';
  if (category === 'IN_PROGRESS') return 'progress';
  return 'todo';
}

function choicesOf(states: readonly StatusRef[], current: StatusRef): WorkflowStep[] {
  return states.filter((each) => each.id !== current.id).map((to) => ({ id: to.id, label: to.label, to, isNext: to.category === 'done' }));
}

/**
 * At L1 the board's columns as states: the first open, the done one done, the rest in progress. An
 * entry in no column reads as the first when open, as the done one when completed (Board).
 */
export function columnSteps(
  columns: readonly Pick<Bucket, 'id' | 'name' | 'is_done_bucket'>[],
  columnId: string | null | undefined,
  isCompleted: boolean,
): EntryStatusModel | undefined {
  const states: StatusRef[] = columns.map((column, index) => ({
    id: column.id,
    label: column.name,
    category: column.is_done_bucket ? 'done' : index === 0 ? 'todo' : 'progress',
  }));
  const current = states.find((each) => each.id === columnId)
    ?? (isCompleted ? states.find((each) => each.category === 'done') : undefined)
    ?? states[0];
  if (!current) return undefined;
  return { current, byDefault: true, columns: true, statuses: states, steps: choicesOf(states, current) };
}

/** In a list (L0): open and done; the entry holds no "in progress" of its own. */
export function listSteps(isCompleted: boolean, words: ListWords): EntryStatusModel {
  const states: StatusRef[] = [
    { id: 'todo', label: words.open, category: 'todo' },
    { id: 'done', label: words.done, category: 'done' },
  ];
  const current = states[isCompleted ? 1 : 0] as StatusRef;
  return { current, byDefault: true, statuses: states, steps: choicesOf(states, current) };
}

/** A workflow in force with its module off: the state says so, and no choice is offered. */
export function pausedSteps(isCompleted: boolean, label: string): EntryStatusModel {
  return { current: { id: 'paused', label, category: isCompleted ? 'done' : 'todo' }, byDefault: true, paused: true, steps: [] };
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
    const columns = answer.state_mode === 'COLUMNS'
      ? (answer.columns ?? []).map((column) => ({ id: column.id, name: column.name, is_done_bucket: column.completes }))
      : [];
    return columnSteps(columns, answer.column_id, isCompleted) ?? listSteps(isCompleted, words.list);
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

/**
 * What one press does: without a workflow it completes or reopens (L0, L1 - the server moves the
 * entry into the done column and out of it); a paused workflow completes where the server lets it
 * and offers nothing once done; under a workflow the next step towards done, and out of a done
 * status the first step out. Undefined: no step leads on.
 */
export function primaryOf(model: EntryStatusModel): Primary | undefined {
  const isDone = model.current.category === 'done';
  if (model.paused) return isDone ? undefined : { kind: 'complete' };
  if (model.byDefault) return { kind: isDone ? 'reopen' : 'complete' };
  const allowed = model.steps.filter((each) => each.blockedReason === undefined);
  const step = isDone
    ? allowed[0]
    : allowed.find((each) => each.isNext) ?? allowed.find((each) => each.to.category !== 'todo' && each.to.category !== model.current.category);
  return step ? { kind: 'step', step } : undefined;
}
