// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Where an entry can go next, as `StatusButton` draws it (work-surface concept §6.9, E-23): the
// server's answer to `GET /items/{id}/steps` turned into the button's model. The server decides
// what is allowed and why the rest is not (`WorkflowGate`, rule 2); this only names and orders it.
// Without a workflow the default holds (`by_default`): to do and done, and completing reaches done.

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
}

export function categoryOf(category: WorkflowStatus['category']): StatusCategory {
  if (category === 'DONE') return 'done';
  if (category === 'IN_PROGRESS') return 'progress';
  return 'todo';
}

// Without a workflow an entry holds no "in progress" of its own until board columns carry it
// (build-plan.md §5, L1); the default offers what the server keeps: to do and done.
const DEFAULT_KEYS = ['todo', 'done'] as const;

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
    const current = ref(isCompleted ? 'done' : 'todo');
    const choices = DEFAULT_KEYS.map(ref);
    return {
      current,
      byDefault: true,
      statuses: choices,
      steps: choices.filter((each) => each.id !== current.id).map((to) => ({ id: to.id, label: to.label, to, isNext: to.category === 'done' })),
    };
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
