// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// build-plan §5: the state control by level - none at L0 (the checkbox completes), the columns at L1,
// the steps out of the status at L2.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { chipOf, markOf } from './statechip.ts';

const names = { noColumn: 'Ohne Spalte', statusName: (key: string) => key.toUpperCase(), stepName: (_key: string, name: string | null | undefined, to: string) => name ?? to };
const columns = [
  { id: 'a', name: 'Offen', is_done_bucket: false },
  { id: 'b', name: 'Diese Woche', is_done_bucket: false },
  { id: 'c', name: 'Erledigt', is_done_bucket: true },
];

test('L0 has no chip: the row keeps its checkbox', () => {
  assert.equal(chipOf('NONE', {}, columns, undefined, names), undefined);
});

test('L1: the chip is the column, its menu the columns, the done column reads done', () => {
  const chip = chipOf('COLUMNS', { bucket_id: 'b' }, columns, undefined, names);
  assert.equal(chip?.label, 'Diese Woche');
  assert.equal(chip?.category, 'progress');
  assert.deepEqual(chip?.choices.map((each) => [each.id, each.isCurrent]), [['column:a', false], ['column:b', true], ['column:c', false]]);
  assert.equal(chipOf('COLUMNS', { bucket_id: 'c' }, columns, undefined, names)?.category, 'done');
});

test('L2: the chip is the status, its menu the steps out of it', () => {
  const workflow = {
    initial: 'open',
    statuses: [{ key: 'open', category: 'TODO' }, { key: 'doing', category: 'IN_PROGRESS' }, { key: 'done', category: 'DONE' }],
    steps: [
      { key: 'start', name: 'Start', from: ['open'], to: 'doing' },
      { key: 'finish', name: null, from: ['doing'], to: 'done' },
      { key: 'anywhere', name: 'Park', to: 'open' },
    ],
  } as never;
  const chip = chipOf('WORKFLOW', { status: 'doing' }, columns, workflow, names);
  assert.equal(chip?.label, 'DOING');
  assert.equal(chip?.category, 'progress');
  assert.deepEqual(chip?.choices.map((each) => each.label), ['done', 'Park']);
});

// E6: at L1 the mark - and so a swipe - completes like the entry's step, instead of advancing one
// column; the server moves the entry into the done column.
test('the mark completes or reopens at L0 and L1 (UC-WRK-05 check 11, E6)', () => {
  assert.deepEqual(markOf('NONE', {}, undefined, names.stepName), { kind: 'complete' });
  assert.deepEqual(markOf('COLUMNS', { bucket_id: 'a' }, undefined, names.stepName), { kind: 'complete' });
  assert.deepEqual(markOf('COLUMNS', { bucket_id: 'c', completion: { is_completed: true } }, undefined, names.stepName), { kind: 'reopen' });
});

// E5: at L2 one press takes the next step towards done - a tick where it lands there, an arrow
// otherwise; out of a done status the first step out.
test('the mark takes the next step towards done at L2 (UC-WRK-45 check 7, E5)', () => {
  const workflow = {
    initial: 'new',
    statuses: [{ key: 'new', category: 'TODO' }, { key: 'check', category: 'IN_PROGRESS' }, { key: 'sent', category: 'DONE' }, { key: 'lost', category: 'DONE' }],
    steps: [
      { key: 'back', name: 'Back', from: ['check'], to: 'new' },
      { key: 'submit', name: 'Submit', from: ['new'], to: 'check' },
      { key: 'send', name: 'Send', from: ['check'], to: 'sent' },
      { key: 'reopen', name: 'Again', from: ['sent'], to: 'new' },
    ],
  } as never;
  const fresh = markOf('WORKFLOW', { status: 'new' }, workflow, names.stepName);
  assert.deepEqual(fresh, { kind: 'step', choice: { id: 'step:submit', label: 'Submit', isCurrent: false }, completes: false, reopens: false });
  const checking = markOf('WORKFLOW', { status: 'check' }, workflow, names.stepName);
  assert.equal(checking?.kind === 'step' && checking.choice.id, 'step:send');
  assert.equal(checking?.kind === 'step' && checking.completes, true);
  const sent = markOf('WORKFLOW', { status: 'sent', completion: { is_completed: true } }, workflow, names.stepName);
  assert.equal(sent?.kind === 'step' && sent.reopens, true);
  assert.equal(markOf('WORKFLOW', { status: 'lost', completion: { is_completed: true } }, workflow, names.stepName), undefined, 'no step leads on');
});

test('a workflow paused with its module off: the chip says so and offers nothing; the mark completes', () => {
  const paused = { label: 'Workflow paused', reason: 'off here' };
  const chip = chipOf('WORKFLOW', { status: 'doing' }, columns, undefined, { ...names, paused });
  assert.equal(chip?.label, 'Workflow paused');
  assert.equal(chip?.pausedReason, 'off here');
  assert.deepEqual(chip?.choices, []);
  assert.deepEqual(markOf('WORKFLOW', { status: 'doing' }, undefined, names.stepName), { kind: 'complete' });
});
