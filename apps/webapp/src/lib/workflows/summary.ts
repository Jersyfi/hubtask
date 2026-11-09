// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A workflow's statuses and steps as one line each (page-system PS-12, the mockup's *Workflows*):
 * what a status is - *In progress · new entries start here* - and what a step does - *In progress
 * → Done · Owner, Member · needs approval*.
 */

import type { WorkflowShape, WorkflowStatus, WorkflowStep } from '@hubtask/sync-engine';

import { statusLabel } from '../data/workflows.ts';

type Words = (code: string, params?: Record<string, string | number>) => string;

/** The tone a status's name is drawn in, by its category. */
export function statusTone(category: string): 'neutral' | 'info' | 'warning' | 'success' {
  if (category === 'DONE') return 'success';
  if (category === 'IN_PROGRESS') return 'warning';
  if (category === 'TODO') return 'info';
  return 'neutral';
}

/** The status where new entries start: the one named, else the first not done. */
export const initialOf = (shape: WorkflowShape): string | undefined =>
  shape.initial ?? shape.statuses.find((status) => status.category !== 'DONE')?.key;

export function statusSummary(shape: WorkflowShape, status: WorkflowStatus, t: Words): string {
  return [
    t(`workflows.category.${status.category.toLowerCase()}`),
    ...(initialOf(shape) === status.key ? [t('app.workflow_designer.starts_here')] : []),
  ].join(' · ');
}

export function stepSummary(shape: WorkflowShape, step: WorkflowStep, t: Words, groupName: (id: string) => string | undefined): string {
  const name = (key: string) => {
    const status = shape.statuses.find((each) => each.key === key);
    return status ? statusLabel(status, t) : key;
  };
  const from = step.from && step.from.length > 0 ? step.from.map(name).join(', ') : t('app.workflow_designer.any_status');
  const who = [
    ...(step.who?.roles ?? []).map((role) => t(`app.people.role.${role.toLowerCase()}`)),
    ...(step.who?.groups ?? []).map((id) => groupName(id) ?? t('app.workflow_designer.a_group')),
  ];
  return [
    `${from} → ${name(step.to)}`,
    who.length > 0 ? who.join(', ') : (step.who?.person_fields?.length ?? 0) > 0 || step.who?.condition ? t('app.workflow_designer.who_narrowed') : t('app.workflow_designer.anyone'),
    ...(step.approval ? [t('app.workflow_designer.needs_approval')] : []),
  ].join(' · ');
}
