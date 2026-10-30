// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { WorkflowList, WorkflowShape } from '@hubtask/sync-engine';

import {
  addApprovalLevel, addStatus, addStep, changeApproval, changeApprovalLevel, checkMessage, completes, layout, moveStatus, removeApprovalLevel,
  removeStatus, standing, stepsFrom, toggleApproval, workflowChoices, workflowFor,
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

test('offers each workflow with its steps, a draft’s new ones too, for a rule on a step (UC-FLW-04 check 4)', () => {
  const workflow = {
    key: 'review', name: 'Review', system: false, published: shape, published_at: null, version: 2,
    draft: { ...shape, steps: [...shape.steps, { key: 'reopen', name: 'Reopen', from: ['done'], to: 'todo' }] },
  } as unknown as Parameters<typeof workflowChoices>[0][number];
  const [choice] = workflowChoices([workflow], (code) => code);
  assert.equal(choice?.value, 'review');
  assert.deepEqual(choice?.steps.map((step) => step.value), ['start', 'finish', 'reopen']);
  assert.equal(choice?.steps[2]?.label, 'Reopen');
});

// UC-FLW-04 check 2: a check speaks the reader's language where the team wrote it, else its one
// text, else the picked shipped message, else its key.
test('a check message is read in the reader\'s language', () => {
  const t = (code: string) => `<${code}>`;
  const check = { key: 'amount', message: 'Above zero', messages: { de: 'Über null', pt: 'Acima de zero' }, message_code: 'amount_positive' };
  assert.equal(checkMessage(check, 'de', t), 'Über null');
  assert.equal(checkMessage(check, 'pt-BR', t), 'Acima de zero');
  assert.equal(checkMessage(check, 'fr', t), 'Above zero');
  assert.equal(checkMessage({ key: 'amount', message: null, message_code: 'amount_positive' }, 'fr', t), '<workflows.check_message.amount_positive>');
  assert.equal(checkMessage({ key: 'amount' }, 'fr', t), 'amount');
});

test('an approval step keeps what the server accepts of a level (UC-SVC-08 check 1)', () => {
  let draft = toggleApproval(shape, 'finish', true);
  assert.deepEqual(draft.steps[1]?.approval, { levels: [{ rule: 'ANY' }], decline_to: null, due_hours: 0 });
  draft = changeApprovalLevel(draft, 'finish', 0, { rule: 'N_OF', roles: ['ADMIN'] });
  assert.deepEqual(draft.steps[1]?.approval?.levels[0], { rule: 'N_OF', roles: ['ADMIN'], count: 1 });
  draft = changeApprovalLevel(draft, 'finish', 0, { rule: 'ALL' });
  assert.deepEqual(draft.steps[1]?.approval?.levels[0], { rule: 'ALL' });
  draft = addApprovalLevel(draft, 'finish');
  assert.equal(draft.steps[1]?.approval?.levels.length, 2);
  draft = removeApprovalLevel(draft, 'finish', 0);
  draft = removeApprovalLevel(draft, 'finish', 0);
  assert.equal(draft.steps[1]?.approval?.levels.length, 1, 'the last level stays');
  for (let n = 0; n < 6; n++) draft = addApprovalLevel(draft, 'finish');
  assert.equal(draft.steps[1]?.approval?.levels.length, 5);
  assert.equal(changeApproval(shape, 'finish', { due_hours: 4 }), shape, 'a step without an approval is left alone');
  assert.equal(toggleApproval(draft, 'finish', false).steps[1]?.approval, null);
});
