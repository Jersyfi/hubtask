// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Where an object can go next (work-surface concept §6.9, E-23): the model behind `StatusButton`.
// The server decides which steps are allowed and why the others are not (`WorkflowGate`, rule 2);
// this module only orders and splits what it is handed, so the button, the phone's sheet and a
// test agree on what "the next step" is.

/** The three status categories a status falls into; the order is the direction of work. */
export type StatusCategory = 'todo' | 'progress' | 'done';

const CATEGORY_ORDER: Record<StatusCategory, number> = { todo: 0, progress: 1, done: 2 };

/** A status as the reader sees it: its resolved name and its category. */
export interface StatusRef {
  readonly id: string;
  readonly label: string;
  readonly category: StatusCategory;
}

/** One step out of the current status, as the server answered it. */
export interface WorkflowStep {
  readonly id: string;
  /** The verb: *Request review*. Resolved text. */
  readonly label: string;
  readonly to: StatusRef;
  /** What taking it asks or checks, in one line: *asks: who reviews? · needs story points*. */
  readonly asks?: string;
  /** Present: the reader may not take it, and this is why (P-05) - *only the product owner*. */
  readonly blockedReason?: string;
  /** The workflow's own next step, drawn as the primary button beside the status. */
  readonly isNext?: boolean;
}

/** The steps a reader may take, and the ones they may not, each in the order the server gave. */
export function splitSteps(steps: readonly WorkflowStep[]): { allowed: WorkflowStep[]; blocked: WorkflowStep[] } {
  const allowed: WorkflowStep[] = [];
  const blocked: WorkflowStep[] = [];
  for (const step of steps) (step.blockedReason === undefined ? allowed : blocked).push(step);
  return { allowed, blocked };
}

/**
 * The step the primary button takes: the one the workflow marks as next, if the reader may take
 * it; otherwise the first allowed step that moves the work forward. A step back is never the
 * primary - *Put back* beside a status reads as the thing to do. No such step, no primary.
 */
export function primaryStep(current: StatusRef, steps: readonly WorkflowStep[]): WorkflowStep | undefined {
  const { allowed } = splitSteps(steps);
  const marked = allowed.find((step) => step.isNext);
  if (marked) return marked;
  return allowed.find((step) => CATEGORY_ORDER[step.to.category] > CATEGORY_ORDER[current.category]);
}

/**
 * The steps without a workflow (§6.9): every other status is reachable, nothing is asked and
 * nothing refused. The done status is the primary, because *✓ Done* always completes the entry.
 */
export function defaultSteps(current: StatusRef, statuses: readonly StatusRef[]): WorkflowStep[] {
  return statuses
    .filter((status) => status.id !== current.id)
    .map((status) => ({ id: `to-${status.id}`, label: status.label, to: status, isNext: status.category === 'done' }));
}
