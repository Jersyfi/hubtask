// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { WorkflowList, WorkflowShape } from '@hubtask/sync-engine';

import {
  addStatus, addStep, completes, layout, moveStatus, removeStatus, standing, stepsFrom, workflowFor,
} from './workflows.ts';

const shape: WorkflowShape = {
  statuses: [
    { key: 'todo', name: null, category: 'TODO', color_token: 'slate' },
    { key: 'in_progress', name: null, category: 'IN_PROGRESS', color_token: 'blue' },
    { key: 'done', name: null, category: 'DONE', color_token: 'green' },
  ],
  initial: 'todo',
  steps: [
    { key: 'start', from: [], to: 'in_progress' },
    { key: 'finish', from: ['in_progress'], to: 'done' },
  ],
};

test('resolves the workflow a collection runs, its own row before its hub’s (UC-FLW-02 check 5)', () => {
  const list = {
    data: [{ key: 'a' }, { key: 'b' }],
    scheme: [
      { container_id: 'hub', entry_type: null, workflow: 'a' },
      { container_id: 'col', entry_type: 'bug', workflow: 'b' },
    ],
  } as unknown as WorkflowList;
  assert.equal(workflowFor(list, 'col', 'hub', 'bug')?.key, 'b');
  assert.equal(workflowFor(list, 'col', 'hub', 'task')?.key, 'a');
  assert.equal(workflowFor(list, 'other', undefined, 'task'), undefined);
});

test('places an entry and knows which status completes (D-02)', () => {
  assert.equal(standing(shape, null, false), 'todo');
  assert.equal(standing(shape, null, true), 'done');
  assert.equal(standing(shape, 'in_progress', false), 'in_progress');
  assert.equal(completes(shape, 'done'), true);
  assert.deepEqual(stepsFrom(shape, 'todo').map((step) => step.key), ['start']);
});

test('edits a draft by list as the diagram does (UC-FLW-02 check 2)', () => {
  let draft = addStatus(shape, 'Waiting for customer', 'IN_PROGRESS');
  assert.equal(draft.statuses.at(-1)?.key, 'waiting_for_customer');
  draft = addStep(draft, ['in_progress'], 'waiting_for_customer');
  draft = moveStatus(draft, 'waiting_for_customer', -1);
  assert.equal(draft.statuses[2]?.key, 'waiting_for_customer');
  draft = removeStatus(draft, 'in_progress');
  assert.deepEqual(draft.steps.map((step) => step.key), []);
  assert.deepEqual(layout(draft).get('done'), { column: 3, row: 0 });
});
