// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Workflows as the client reads them (UC-FLW-02 … 05): which one runs where, a status's name and
 * category, a board's columns, and the designer's draft edits. Pure functions, tested beside.
 */

import type { Workflow, WorkflowList, WorkflowShape, WorkflowStatus, WorkflowStep } from '@hubtask/sync-engine';

/** The four categories, in the order a process runs through them. */
export const CATEGORIES = ['BACKLOG', 'TODO', 'IN_PROGRESS', 'DONE'] as const;
export type Category = (typeof CATEGORIES)[number];

/** The shipped workflow's key. */
export const SYSTEM_WORKFLOW = 'default';

/**
 * The workflow in force for an entry type in a collection under a hub: the collection's row for
 * the type, then for every type, then the hub's - as the server resolves it (work.ResolveWorkflow).
 */
export function workflowFor(
  list: WorkflowList | undefined,
  collectionId: string,
  hubId: string | undefined,
  entryType: string,
): Workflow | undefined {
  if (!list) return undefined;
  for (const container of [collectionId, hubId]) {
    if (!container) continue;
    for (const wanted of [entryType, null]) {
      const row = list.scheme.find((scheme) => scheme.container_id === container && (scheme.entry_type ?? null) === wanted);
      if (row) return list.data.find((workflow) => workflow.key === row.workflow);
    }
  }
  return undefined;
}

/** The scheme row a container holds for a type (`null` for every type), if any. */
export function schemeOf(list: WorkflowList | undefined, containerId: string, entryType: string | null): string | undefined {
  return list?.scheme.find((row) => row.container_id === containerId && (row.entry_type ?? null) === entryType)?.workflow;
}

/**
 * The message code of a status the workspace did not name: the shipped workflow's statuses are
 * named by the client (rule 8), any other by its category.
 */
export function statusCode(status: Pick<WorkflowStatus, 'key' | 'category'>): string {
  if (status.key === 'todo' || status.key === 'in_progress' || status.key === 'done') return `workflows.status.${status.key}`;
  return `workflows.category.${status.category.toLowerCase()}`;
}

/** A status's label: its name, else the client's own word. */
export function statusLabel(status: Pick<WorkflowStatus, 'key' | 'name' | 'category'>, t: (code: string) => string): string {
  return status.name ?? t(statusCode(status));
}

/** A step's label: its name, else the status it leads to. */
export function stepLabel(step: Pick<WorkflowStep, 'name' | 'to'>, shape: WorkflowShape, t: (code: string) => string): string {
  if (step.name) return step.name;
  const target = shape.statuses.find((status) => status.key === step.to);
  return target ? statusLabel(target, t) : step.to;
}

/** Whether entering a status completes the entry (UC-FLW-03 check 4). */
export function completes(shape: WorkflowShape, key: string | null | undefined): boolean {
  return shape.statuses.find((status) => status.key === key)?.category === 'DONE';
}

/**
 * Where an entry stands: its status where the shape knows it on the same side of done, else where
 * entering the shape puts it - as the server answers (work.WorkflowShape.Entering).
 */
export function standing(shape: WorkflowShape, status: string | null | undefined, completed: boolean): string {
  const known = shape.statuses.find((candidate) => candidate.key === status);
  if (known && (known.category === 'DONE') === completed) return known.key;
  if (completed) return shape.statuses.find((candidate) => candidate.category === 'DONE')?.key ?? '';
  return shape.initial ?? shape.statuses.find((candidate) => candidate.category !== 'DONE')?.key ?? '';
}

/** The steps that leave a status, in the workflow's order. */
export function stepsFrom(shape: WorkflowShape, status: string): readonly WorkflowStep[] {
  return shape.steps.filter((step) => step.to !== status && (!step.from?.length || step.from.includes(status)));
}

/** The draft a designer starts from: the workflow's draft, else a copy of what is in force. */
export function startingDraft(workflow: Workflow): WorkflowShape {
  const source = (workflow.draft as WorkflowShape | null | undefined) ?? workflow.published;
  return structuredClone(source);
}

/** A key from a name: small letters, digits and underscores, starting with a letter. */
export function keyOf(name: string, taken: readonly string[]): string {
  const base =
    name
      .normalize('NFKD')
      .replace(/[̀-ͯ]/g, '')
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '_')
      .replace(/^[^a-z]+|_+$/g, '')
      .slice(0, 40) || 'status';
  let key = base;
  for (let n = 2; taken.includes(key); n++) key = `${base}_${n}`;
  return key;
}

/** Adds a status to a draft in a category. */
export function addStatus(shape: WorkflowShape, name: string, category: Category): WorkflowShape {
  const key = keyOf(name, shape.statuses.map((status) => status.key));
  return { ...shape, statuses: [...shape.statuses, { key, name, category, color_token: category === 'DONE' ? 'green' : 'slate' }] };
}

/** Removes a status and every step into it; a step from it loses that origin. */
export function removeStatus(shape: WorkflowShape, key: string): WorkflowShape {
  const steps = shape.steps.flatMap((step) => {
    if (step.to === key) return [];
    if (!step.from?.includes(key)) return [step];
    const from = step.from.filter((origin) => origin !== key);
    // A step that left only from the removed status goes with it; "from any" stays.
    return from.length > 0 ? [{ ...step, from }] : [];
  });
  const statuses = shape.statuses.filter((status) => status.key !== key);
  const initial = shape.initial === key ? statuses.find((status) => status.category !== 'DONE')?.key : shape.initial;
  return { ...shape, statuses, steps, ...(initial ? { initial } : {}) };
}

/** Moves a status one place up or down. */
export function moveStatus(shape: WorkflowShape, key: string, by: -1 | 1): WorkflowShape {
  const at = shape.statuses.findIndex((status) => status.key === key);
  const to = at + by;
  if (at < 0 || to < 0 || to >= shape.statuses.length) return shape;
  const statuses = [...shape.statuses];
  [statuses[at], statuses[to]] = [statuses[to] as WorkflowStatus, statuses[at] as WorkflowStatus];
  return { ...shape, statuses };
}

/** Adds a step from some statuses - none for any - to one. */
export function addStep(shape: WorkflowShape, from: readonly string[], to: string, name = ''): WorkflowShape {
  const key = keyOf(name || `to_${to}`, shape.steps.map((step) => step.key));
  return { ...shape, steps: [...shape.steps, { key, name: name || null, from: [...from], to }] };
}

/** Replaces one step. */
export function changeStep(shape: WorkflowShape, key: string, change: Partial<WorkflowStep>): WorkflowShape {
  return { ...shape, steps: shape.steps.map((step) => (step.key === key ? { ...step, ...change, key: step.key } : step)) };
}

/** Removes one step. */
export function removeStep(shape: WorkflowShape, key: string): WorkflowShape {
  return { ...shape, steps: shape.steps.filter((step) => step.key !== key) };
}

/** Replaces one status. */
export function changeStatus(shape: WorkflowShape, key: string, change: Partial<WorkflowStatus>): WorkflowShape {
  return { ...shape, statuses: shape.statuses.map((status) => (status.key === key ? { ...status, ...change, key: status.key } : status)) };
}

/**
 * Where the diagram draws each status: one column per category, statuses stacked in their order.
 * Positions in grid cells, so the component scales them by its tokens.
 */
export function layout(shape: WorkflowShape): ReadonlyMap<string, { readonly column: number; readonly row: number }> {
  const out = new Map<string, { column: number; row: number }>();
  const rows = new Map<string, number>();
  for (const status of shape.statuses) {
    const column = CATEGORIES.indexOf(status.category as Category);
    const row = rows.get(status.category) ?? 0;
    rows.set(status.category, row + 1);
    out.set(status.key, { column: Math.max(column, 0), row });
  }
  return out;
}

/** The statuses a draft removes that entries are in, from a publish preview. */
export function removedOf(preview: { readonly removed: readonly { readonly status: string; readonly count: number }[] } | undefined) {
  return preview?.removed ?? [];
}

/**
 * The workflows a rule on a step may name, each with its steps (UC-FLW-04 check 4): what is in
 * force and what the draft adds, so a rule can be written for a step before it is published.
 */
export function workflowChoices(
  list: readonly Workflow[],
  t: (code: string) => string,
): { value: string; label: string; steps: { value: string; label: string }[] }[] {
  return list.map((workflow) => {
    const steps = new Map<string, string>();
    for (const shape of [workflow.published, workflow.draft ?? undefined]) {
      if (!shape) continue;
      for (const step of shape.steps) if (!steps.has(step.key)) steps.set(step.key, stepLabel(step, shape, t));
    }
    return {
      value: workflow.key,
      label: workflow.name ?? t('workflows.default_name'),
      steps: [...steps].map(([value, label]) => ({ value, label })),
    };
  });
}
