// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-INS-21 check 14: what switching Workflows off strands, summed from the scheme and the previews
// of removing each row - the places, the open entries in a status, those that could then not even
// be completed, and the column each status keeps.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { WorkflowList, WorkflowPreview } from '@hubtask/sync-engine';
import { needsChoice, rowsInReach, strandedBy } from './workflowsoff.ts';

const offers = {
  key: 'offer', published: {
    initial: 'draft',
    statuses: [
      { key: 'draft', name: 'Draft', category: 'TODO' },
      { key: 'check', name: 'In review', category: 'IN_PROGRESS' },
      { key: 'released', name: 'Released', category: 'IN_PROGRESS' },
      { key: 'sent', name: 'Sent', category: 'DONE' },
    ],
    steps: [
      { key: 'submit', from: ['draft'], to: 'check' },
      { key: 'release', from: ['check'], to: 'released' },
      { key: 'send', from: ['released'], to: 'sent' },
    ],
  },
} as unknown as WorkflowList['data'][number];

const move = (collection_id: string, from: string, count: number, completed = false) =>
  ({ collection_id, entry_type: 'task', from, to: null, completed, count });

test('at the workspace every row is in reach; at a hub its own row and its collections\' rows', () => {
  const scheme = [
    { container_id: 'hub-a', entry_type: null, workflow: 'offer' },
    { container_id: 'col-a1', entry_type: 'task', workflow: 'offer' },
    { container_id: 'col-b1', entry_type: null, workflow: 'offer' },
  ];
  assert.equal(rowsInReach(scheme, undefined, []).length, 3);
  assert.deepEqual(rowsInReach(scheme, 'hub-a', ['col-a1']).map((row) => row.container_id), ['hub-a', 'col-a1']);
});

test('the open entries by status, the stuck ones, and the column each status keeps', () => {
  const preview = {
    moves: [move('col', 'draft', 6), move('col', 'check', 7), move('col', 'released', 4), move('col', 'sent', 23, true)],
    entries: 40, removed: [], columns_before: [], columns_after: [],
    boards: [{ collection_id: 'col' }],
  } as unknown as WorkflowPreview;
  const columns = [
    { id: 'open', statuses: ['draft'], is_done_bucket: false },
    { id: 'doing', statuses: ['check', 'released'], is_done_bucket: false },
    { id: 'done', statuses: ['sent'], is_done_bucket: true },
  ];
  const stranded = strandedBy([{ row: { container_id: 'col', entry_type: null, workflow: 'offer' }, preview }], [offers], () => columns);
  assert.deepEqual(stranded.places, ['col']);
  assert.deepEqual(stranded.collections, ['col']);
  assert.equal(stranded.open, 17, 'the done ones are not counted: they stay done');
  // Only Released has a step straight into done: Draft and In review would be stuck while paused.
  assert.equal(stranded.stuck, 13);
  assert.deepEqual(stranded.lines.map((line) => [line.status, line.count, line.columnId, line.isStuck]), [
    ['draft', 6, 'open', true], ['check', 7, 'doing', true], ['released', 4, 'doing', false],
  ]);
  assert.equal(needsChoice(stranded), true);
});

test('a status no column shows, or a list without columns, has no column of its own; nothing open needs no choice', () => {
  const preview = { moves: [move('list', 'draft', 2), move('list', 'sent', 1, true)], entries: 3, removed: [], columns_before: [], columns_after: [] } as unknown as WorkflowPreview;
  const stranded = strandedBy([{ row: { container_id: 'list', entry_type: null, workflow: 'offer' }, preview }], [offers], () => []);
  assert.equal(stranded.lines[0]?.columnId, undefined);
  const quiet = strandedBy([{ row: { container_id: 'list', entry_type: null, workflow: 'offer' }, preview: { ...preview, moves: [move('list', 'sent', 1, true)] } }], [offers], () => []);
  assert.equal(needsChoice(quiet), false);
  assert.equal(needsChoice(undefined), false);
});
