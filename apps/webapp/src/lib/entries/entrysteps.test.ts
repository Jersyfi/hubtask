// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The status button's model from the server's answer (UC-FLW-06, UC-WRK-44): allowed steps with
// target and asks, blocked ones with their reason, how done is reached, and the default without a
// workflow.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { EntrySteps } from '@hubtask/sync-engine';
import { columnSteps, listSteps, pausedSteps, primaryOf, readSteps, stepsAskable, type StepWords } from './entrysteps.ts';

const words: StepWords = {
  status: (status) => status.name ?? `status:${status.key}`,
  asks: (step) => (step.asks.length > 0 ? `asks:${step.asks.join(',')}` : undefined),
  reason: (step) => `${step.reason}:${step.roles.join(',')}`,
  doneRoute: (names) => `route:${names.join('>')}`,
  list: { open: 'Open', done: 'Done' },
};

const picture = {
  initial: 'open',
  statuses: [
    { key: 'open', name: 'Open', category: 'TODO', color_token: null },
    { key: 'doing', name: 'In progress', category: 'IN_PROGRESS', color_token: null },
    { key: 'review', name: 'In review', category: 'IN_PROGRESS', color_token: null },
    { key: 'closed', name: 'Closed', category: 'DONE', color_token: null },
  ],
  steps: [],
} as unknown as EntrySteps['picture'];

const verdict = (key: string, to: string, asks: string[] = []) =>
  ({ key, name: null, to, requires: [], asks, allowed: true, missing: [], checks: [], holds: true }) as unknown as EntrySteps['steps'][number];

test('a workflow: allowed steps with target and asks, blocked steps with a reason, the route to done', () => {
  const answer = {
    workflow: 'story', status: 'doing', picture, by_default: false, may_design: false,
    steps: [verdict('back', 'open'), verdict('ask', 'review', ['custom_fields.reviewer'])],
    blocked: [{ key: 'close', name: 'Close', to: 'closed', reason: 'workflows.step_who_unmet', roles: ['owner'], people: [], groups: [] }],
    done_route: [{ key: 'ask', name: null, to: 'review' }, { key: 'close', name: 'Close', to: 'closed' }],
  } as unknown as EntrySteps;
  const model = readSteps(answer, false, words);
  assert.equal(model.byDefault, false);
  assert.deepEqual(model.current, { id: 'doing', label: 'In progress', category: 'progress' });
  assert.deepEqual(model.steps.map((step) => [step.id, step.label, step.to.id, step.asks, step.blockedReason, step.isNext]), [
    ['back', 'Open', 'open', undefined, undefined, false],
    ['ask', 'In review', 'review', 'asks:custom_fields.reviewer', undefined, true],
    ['close', 'Close', 'closed', undefined, 'workflows.step_who_unmet:owner', undefined],
  ]);
  assert.equal(model.doneHint, 'route:In review>Close');
  assert.equal(model.statuses, undefined);
});

test('without a workflow: to do and done, done is the next step, and a completed entry stands in done', () => {
  const answer = { workflow: null, status: null, steps: [], blocked: [], done_route: [], by_default: true, may_design: false, picture } as unknown as EntrySteps;
  const open = readSteps(answer, false, words);
  assert.equal(open.byDefault, true);
  assert.equal(open.current.category, 'todo');
  assert.deepEqual(open.statuses?.map((status) => status.category), ['todo', 'done']);
  assert.deepEqual(open.steps.map((step) => [step.to.category, step.isNext]), [['done', true]]);
  const done = readSteps(answer, true, words);
  assert.equal(done.current.category, 'done');
  assert.deepEqual(done.steps.map((step) => [step.to.category, step.isNext]), [['todo', false]]);
});

test('on a board without a workflow (L1): the columns are the states, the done column completes', () => {
  const columns = [
    { id: 'c1', name: 'Ideas', completes: false },
    { id: 'c2', name: 'Doing', completes: false },
    { id: 'c3', name: 'Shipped', completes: true },
  ];
  const answer = { state_mode: 'COLUMNS', columns, column_id: 'c2', workflow: null, status: null, steps: [], by_default: true } as unknown as EntrySteps;
  const model = readSteps(answer, false, words);
  assert.equal(model.columns, true);
  assert.deepEqual(model.current, { id: 'c2', label: 'Doing', category: 'progress' });
  assert.deepEqual(model.statuses?.map((state) => [state.label, state.category]), [['Ideas', 'todo'], ['Doing', 'progress'], ['Shipped', 'done']]);
  assert.deepEqual(model.steps.map((step) => [step.id, step.isNext]), [['c1', false], ['c3', true]]);
  const unplaced = readSteps({ ...answer, column_id: null } as EntrySteps, true, words);
  assert.equal(unplaced.current.id, 'c3');
});

test('in a list (L0, state_mode NONE): open and done, no columns', () => {
  const answer = { state_mode: 'NONE', columns: [], column_id: null, workflow: null, status: null, steps: [], by_default: true } as unknown as EntrySteps;
  const model = readSteps(answer, false, words);
  assert.equal(model.columns, undefined);
  assert.deepEqual(model.statuses?.map((state) => state.category), ['todo', 'done']);
});

test('workflows off at the workspace or the entry\'s hub: the steps are not asked; on, or the workspace not yet known, they are', () => {
  assert.equal(stepsAskable(['TASKS'], ['TASKS']), false);
  assert.equal(stepsAskable(['TASKS', 'WORKFLOWS'], ['TASKS']), false);
  assert.equal(stepsAskable(['TASKS'], null), false);
  assert.equal(stepsAskable(['TASKS', 'WORKFLOWS'], ['TASKS', 'WORKFLOWS']), true);
  assert.equal(stepsAskable(undefined, ['WORKFLOWS']), true);
  assert.equal(stepsAskable(['WORKFLOWS'], null), true);
  // The hub's set not yet read: wait rather than ask what may be refused.
  assert.equal(stepsAskable(['WORKFLOWS'], undefined), undefined);
});

// UC-FLW-06 check 5 with the Workflows module off: the steps are not asked, and the level is
// built from the base reads - the same model the server's answer gives with the module on.
test('a list (L0) without an answer: Open and Done (E7), done next for an open entry, open for a done one', () => {
  const open = listSteps(false, words.list);
  assert.equal(open.byDefault, true);
  assert.deepEqual(open.current, { id: 'todo', label: 'Open', category: 'todo' });
  assert.deepEqual(open.steps.map((step) => [step.id, step.isNext]), [['done', true]]);
  const done = listSteps(true, words.list);
  assert.equal(done.current.id, 'done');
  assert.deepEqual(done.steps.map((step) => step.id), ['todo']);
});

test('a board (L1) without an answer: the columns from the column list, the same model the steps read gives', () => {
  const columns = [
    { id: 'c1', name: 'Ideas', is_done_bucket: false },
    { id: 'c2', name: 'Doing', is_done_bucket: false },
    { id: 'c3', name: 'Shipped', is_done_bucket: true },
  ];
  const fromBase = columnSteps(columns, 'c2', false);
  const answer = { state_mode: 'COLUMNS', columns: columns.map((each) => ({ id: each.id, name: each.name, completes: each.is_done_bucket })), column_id: 'c2', workflow: null, status: null, steps: [], by_default: true } as unknown as EntrySteps;
  assert.deepEqual(fromBase, readSteps(answer, false, words));
  assert.equal(fromBase?.columns, true);
  assert.equal(columnSteps([], null, false), undefined, 'no column list yet: nothing is drawn rather than a wrong state');
});

test('a workflow paused with its module off: the state says so, nothing is offered', () => {
  const paused = pausedSteps(false, 'Workflow paused');
  assert.equal(paused.paused, true);
  assert.equal(paused.current.label, 'Workflow paused');
  assert.deepEqual(paused.steps, []);
  assert.equal(paused.statuses, undefined);
});

// UC-FLW-06 check 5 and UC-WRK-05 check 11: one press beside the state on every level.
test('the one press: Done and Reopen without a workflow, the next step under one, the step out of done', () => {
  assert.deepEqual(primaryOf(listSteps(false, words.list)), { kind: 'complete' });
  assert.deepEqual(primaryOf(listSteps(true, words.list)), { kind: 'reopen' });
  const columns = [{ id: 'a', name: 'To do', is_done_bucket: false }, { id: 'z', name: 'Done', is_done_bucket: true }];
  assert.deepEqual(primaryOf(columnSteps(columns, 'a', false)!), { kind: 'complete' });
  assert.deepEqual(primaryOf(columnSteps(columns, 'z', true)!), { kind: 'reopen' });
  assert.deepEqual(primaryOf(pausedSteps(false, 'p')), { kind: 'complete' });
  assert.equal(primaryOf(pausedSteps(true, 'p')), undefined, 'a paused workflow offers no way back');

  const flow = (status: string, steps: unknown[], doneRoute: unknown[] = []) => readSteps({
    workflow: 'story', status, picture, by_default: false, may_design: false, steps, blocked: [], done_route: doneRoute,
  } as unknown as EntrySteps, status === 'closed', words);
  const next = primaryOf(flow('doing', [verdict('back', 'open'), verdict('ask', 'review')], [{ key: 'ask', name: null, to: 'review' }]));
  assert.equal(next?.kind === 'step' && next.step.id, 'ask');
  const out = primaryOf(flow('closed', [verdict('again', 'open')]));
  assert.equal(out?.kind === 'step' && out.step.id, 'again', 'out of a done status: the step out, by name');
  assert.equal(primaryOf(flow('closed', [])), undefined, 'no step back: no button');
});
