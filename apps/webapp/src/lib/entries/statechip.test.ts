// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// build-plan §5: the state control by level - none at L0 (the checkbox completes), the columns at L1,
// the steps out of the status at L2.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { advanceOf, chipOf } from './statechip.ts';

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

test('a swipe advances to the next column at L1, and nowhere past the last (§7, UC-WRK-45 check 7)', () => {
  const chip = chipOf('COLUMNS', { bucket_id: 'a' }, columns, undefined, names)!;
  assert.equal(advanceOf(chip)?.id, 'column:b');
  assert.equal(advanceOf(chipOf('COLUMNS', { bucket_id: 'c' }, columns, undefined, names)!), undefined);
  assert.equal(advanceOf(chipOf('COLUMNS', {}, columns, undefined, names)!)?.id, 'column:a', 'without a column, into the first');
});

test('a swipe advances along the first step out of the status at L2 (UC-WRK-45 check 7)', () => {
  const chip = { label: 'Neu', category: 'todo' as const, choices: [{ id: 'step:start', label: 'Start', isCurrent: false }, { id: 'step:drop', label: 'Drop', isCurrent: false }] };
  assert.equal(advanceOf(chip)?.id, 'step:start');
  assert.equal(advanceOf({ ...chip, choices: [] }), undefined);
});
