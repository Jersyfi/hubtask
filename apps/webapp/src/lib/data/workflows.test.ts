// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { WorkflowList, WorkflowShape } from '@hubtask/sync-engine';

import {
  addApprovalLevel, addStatus, addStep, changeApproval, changeStatus, changesBetween, moveStatusTo, changeApprovalLevel, checkMessage, completes, layout, moveStatus, removeApprovalLevel,
  removeStatus, shippedCopy, standing, startingDraft, stepsFrom, toggleApproval, workflowChoices, workflowCopyKey, workflowFor, workflowsAtHub, boardShapeIn, workflowsIn,
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

test('the designer writes an approval\'s levels, its decline status and the hours each level has (UC-SVC-08 check 6)', () => {
  let draft = toggleApproval(shape, 'finish', true);
  draft = changeApprovalLevel(draft, 'finish', 0, { people: ['acc-1'], groups: ['grp-1'], person_fields: ['custom_fields.manager'] });
  draft = addApprovalLevel(draft, 'finish');
  draft = changeApprovalLevel(draft, 'finish', 1, { rule: 'N_OF', count: 2, roles: ['ADMIN'] });
  draft = changeApproval(draft, 'finish', { decline_to: 'todo', due_hours: 24 });
  assert.deepEqual(draft.steps[1]?.approval, {
    levels: [
      { rule: 'ANY', people: ['acc-1'], groups: ['grp-1'], person_fields: ['custom_fields.manager'] },
      { rule: 'N_OF', count: 2, roles: ['ADMIN'] },
    ],
    decline_to: 'todo', due_hours: 24,
  });
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

test('a hub’s settings show the shared workflows apart from its own, never another hub’s (UC-FLW-02 check 9)', () => {
  const data = [
    { key: 'default', system: true },
    { key: 'service', system: false },
    { key: 'support', system: false, hub_id: 'hub-1', copied_from: 'service' },
    { key: 'sales', system: false, hub_id: 'hub-2' },
  ] as unknown as WorkflowList['data'];
  const atHub = workflowsAtHub(data, 'hub-1');
  assert.deepEqual(atHub.shared.map((w) => w.key), ['default', 'service']);
  assert.deepEqual(atHub.own.map((w) => w.key), ['support']);
  assert.equal(workflowCopyKey(data, 'service'), 'service_copy');
  assert.equal(workflowCopyKey([...data, { key: 'service_copy' } as never], 'service'), 'service_copy_2');
});

test('the shipped workflow is copied with the client’s own words for its statuses (rule 8)', () => {
  const copy = shippedCopy(shape, (code) => `«${code}»`);
  assert.deepEqual(copy.statuses.map((status) => status.name), ['«workflows.status.todo»', '«workflows.status.in_progress»', '«workflows.status.done»']);
  assert.deepEqual(copy.steps, shape.steps);
  assert.equal(shape.statuses[0]?.name, null);
});

test('the designer starts from a workflow held in a reactive store, which cannot be structured-cloned', () => {
  // A store's state is a proxy; structuredClone refuses one with a DataCloneError.
  const held = { key: 'support', draft: null, published: new Proxy(shape, {}) } as never;
  const draft = startingDraft(held);
  assert.deepEqual(draft, shape);
  assert.notEqual(draft, shape, 'the draft is a copy, not the shape in force');
});

// The scrum template maps its stories and bugs to workflows and nothing to `task`: a board asking
// for the task's workflow found none and put every story in *No column* (UC-FLW-03 check 2).
test('a board draws the statuses of every workflow its collection runs, each once', () => {
  const status = (key: string, category: string) => ({ key, name: key, category });
  const list = {
    data: [
      { key: 'story', published: { initial: 'backlog', steps: [], statuses: [status('backlog', 'BACKLOG'), status('in_progress', 'IN_PROGRESS'), status('done', 'DONE')] } },
      { key: 'bug', published: { initial: 'backlog', steps: [], statuses: [status('backlog', 'BACKLOG'), status('triage', 'TODO'), status('done', 'DONE')] } },
      { key: 'elsewhere', published: { initial: 'x', steps: [], statuses: [status('x', 'TODO')] } },
    ],
    scheme: [
      { container_id: 'hub', entry_type: 'story', workflow: 'story' },
      { container_id: 'hub', entry_type: 'bug', workflow: 'bug' },
      { container_id: 'other', entry_type: null, workflow: 'elsewhere' },
    ],
  } as unknown as WorkflowList;
  assert.deepEqual(boardShapeIn(list, 'col', 'hub')?.statuses.map((each) => each.key), ['backlog', 'triage', 'in_progress', 'done']);
  assert.equal(boardShapeIn(list, 'col', undefined), undefined);
  // The collection's own row for a type wins over its hub's.
  const own = { ...list, scheme: [...list.scheme, { container_id: 'col', entry_type: 'bug', workflow: 'elsewhere' }] } as unknown as WorkflowList;
  assert.deepEqual(boardShapeIn(own, 'col', 'hub')?.statuses.map((each) => each.key), ['backlog', 'x', 'in_progress', 'done']);
});

// The collection head said *Workflow:* with the task's workflow only, and nothing in a template's
// collection, which maps its own types (story, bug) and not `task`.
test('a collection names every workflow its types run there, each once, the collection first', () => {
  const list = {
    data: [{ key: 'story' }, { key: 'bug' }, { key: 'own' }],
    scheme: [
      { container_id: 'hub', entry_type: 'story', workflow: 'story' },
      { container_id: 'hub', entry_type: 'bug', workflow: 'bug' },
      { container_id: 'hub', entry_type: 'epic', workflow: 'story' },
      { container_id: 'col', entry_type: 'bug', workflow: 'own' },
    ],
  } as unknown as WorkflowList;
  assert.deepEqual(workflowsIn(list, 'col', 'hub').map((each) => each.key), ['own', 'story']);
  assert.deepEqual(workflowsIn(list, 'other', undefined), []);
  assert.deepEqual(workflowsIn(undefined, 'col', 'hub'), []);
});

test('the draft bar counts each status and step added, removed or changed, a new order and a new start once', () => {
  assert.equal(changesBetween(shape, shape), 0);
  assert.equal(changesBetween(addStatus(shape, 'Review', 'IN_PROGRESS'), shape), 1);
  assert.equal(changesBetween(removeStatus(shape, 'in_progress'), shape), 1 + shape.steps.filter((step) => step.to === 'in_progress' || step.from?.includes('in_progress')).length);
  assert.equal(changesBetween(changeStatus(shape, 'done', { name: 'Finished' }), shape), 1);
  assert.equal(changesBetween(moveStatus(shape, 'done', -1), shape), 1);
  assert.equal(changesBetween({ ...shape, initial: 'in_progress' }, shape), 1);
  assert.equal(changesBetween(addStep(shape, [], 'done', 'Close'), shape), 1);
});

test('a status moves to the place a block is dropped at, and stays inside the list', () => {
  assert.deepEqual(moveStatusTo(shape, 'done', 0).statuses.map((status) => status.key), ['done', 'todo', 'in_progress']);
  assert.deepEqual(moveStatusTo(shape, 'todo', 9).statuses.map((status) => status.key), ['in_progress', 'done', 'todo']);
  assert.equal(moveStatusTo(shape, 'missing', 0), shape);
});
