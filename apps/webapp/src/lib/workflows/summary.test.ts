// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { WorkflowShape } from '@hubtask/sync-engine';

import { statusSummary, statusTone, stepSummary } from './summary.ts';

const words: Record<string, string> = {
  'workflows.category.todo': 'To do',
  'workflows.category.in_progress': 'In progress',
  'workflows.category.done': 'Done',
  'app.workflow_designer.starts_here': 'new entries start here',
  'app.workflow_designer.any_status': 'Any status',
  'app.workflow_designer.anyone': 'anyone who may write',
  'app.workflow_designer.who_narrowed': 'some people',
  'app.workflow_designer.needs_approval': 'needs approval',
  'app.people.role.owner': 'Owner',
  'app.people.role.member': 'Member',
};
const t = (code: string) => words[code] ?? code;

const shape: WorkflowShape = {
  statuses: [
    { key: 'todo', name: 'To do', category: 'TODO', color_token: 'slate' },
    { key: 'doing', name: 'In progress', category: 'IN_PROGRESS', color_token: 'blue' },
    { key: 'done', name: 'Done', category: 'DONE', color_token: 'green' },
  ],
  initial: 'todo',
  steps: [
    { key: 'start', from: ['todo'], to: 'doing' },
    { key: 'finish', from: ['doing'], to: 'done', who: { roles: ['OWNER', 'MEMBER'] }, approval: { levels: [{ rule: 'ANY' }] } },
    { key: 'close', from: [], to: 'done', who: { condition: 'entry.priority > 2' } },
  ],
} as WorkflowShape;

test('a status reads as its category, and where new entries start says so', () => {
  assert.equal(statusSummary(shape, shape.statuses[0]!, t), 'To do · new entries start here');
  assert.equal(statusSummary(shape, shape.statuses[1]!, t), 'In progress');
  assert.equal(statusTone('DONE'), 'success');
});

test('a step reads as where it leads, who may take it and whether it waits for a yes', () => {
  assert.equal(stepSummary(shape, shape.steps[0]!, t, () => undefined), 'To do → In progress · anyone who may write');
  assert.equal(stepSummary(shape, shape.steps[1]!, t, () => undefined), 'In progress → Done · Owner, Member · needs approval');
  assert.equal(stepSummary(shape, shape.steps[2]!, t, () => undefined), 'Any status → Done · some people');
});
