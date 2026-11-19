// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { UNMAPPED, columnRefusals, mappedColumns, statusPlaces, unmappedStatuses } from './boardlevel.ts';

const step = (to: string, allowed = true) => ({ key: `to_${to}`, name: null, to, requires: [], asks: [], allowed, missing: [], checks: [] }) as never;

test('L1: only "no column" refuses, with the server code', () => {
  const refused = columnRefusals('COLUMNS', [{ id: 'a' }, { id: 'b' }], [], undefined, 'a');
  assert.deepEqual([...refused], [['none', 'board.column_required']]);
});

test('L0 and an unknown level dim nothing', () => {
  assert.equal(columnRefusals('NONE', [{ id: 'a' }], [], undefined, 'a').size, 0);
  assert.equal(columnRefusals(undefined, [{ id: 'a' }], [], undefined, 'a').size, 0);
});

test('L2: a column without status, a blocked step and no step each say their reason', () => {
  const columns = [
    { id: 'todo', statuses: ['open'] },
    { id: 'doing', statuses: ['doing', 'review'] },
    { id: 'empty', statuses: [] },
    { id: 'done', statuses: ['done'] },
    { id: 'approved', statuses: ['approved'] },
  ];
  const steps = { status: 'open', steps: [step('doing'), step('approved', false)], blocked: [] };
  const refused = columnRefusals('WORKFLOW', columns, ['parked'], steps, 'todo');
  assert.equal(refused.get('doing'), undefined, 'a step reaches one of its statuses');
  assert.equal(refused.get('empty'), 'board.column_without_status');
  assert.equal(refused.get('done'), 'board.no_step');
  assert.equal(refused.get('approved'), 'board.step_blocked');
  assert.equal(refused.get('none'), 'board.no_step', 'Ohne Spalte is judged by its unmapped statuses');
});

test('L2 before the steps arrive: only the structural refusals', () => {
  const refused = columnRefusals('WORKFLOW', [{ id: 'a', statuses: ['x'] }, { id: 'b', statuses: [] }], [], undefined, 'a');
  assert.deepEqual([...refused], [['b', 'board.column_without_status'], ['none', 'board.no_step']]);
});

test('unmapped statuses are the workflow statuses no column shows', () => {
  assert.deepEqual(unmappedStatuses([{ id: 'a', statuses: ['x', 'y'] }, { id: 'b' }], ['x', 'y', 'z']), ['z']);
});

test('a scheme preview\'s mapping: the proposal read, a status moved, sent per column', () => {
  const board = {
    columns: [
      { bucket_id: 'a', name: 'Offen', is_done_bucket: false, statuses: ['open'], completes: false },
      { bucket_id: 'b', name: 'Arbeit', is_done_bucket: false, statuses: ['doing'], completes: false },
    ],
    unmapped: [{ status: 'review', suggested_bucket_id: 'b' }],
  };
  const places = statusPlaces(board);
  assert.deepEqual(places, { open: 'a', doing: 'b', review: UNMAPPED });
  assert.deepEqual(mappedColumns(board, { ...places, review: 'b' }), [
    { bucket_id: 'a', statuses: ['open'] },
    { bucket_id: 'b', statuses: ['doing', 'review'] },
  ]);
});
