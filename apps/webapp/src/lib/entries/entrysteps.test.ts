// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The status button's model from the server's answer (UC-FLW-06, UC-WRK-44): allowed steps with
// target and asks, blocked ones with their reason, how done is reached, and the default without a
// workflow.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { EntrySteps } from '@hubtask/sync-engine';
import { readSteps, type StepWords } from './entrysteps.ts';

const words: StepWords = {
  status: (status) => status.name ?? `status:${status.key}`,
  asks: (step) => (step.asks.length > 0 ? `asks:${step.asks.join(',')}` : undefined),
  reason: (step) => `${step.reason}:${step.roles.join(',')}`,
  doneRoute: (names) => `route:${names.join('>')}`,
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
