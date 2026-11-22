// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-WRK-05 check 13: the settings name the columns of a collection that offers no board - only to
// a reader who may shape it, and never where a workflow runs for the tasks.

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { unusedColumns, type ColumnsAtPlace } from './unusedColumns.ts';

const place: ColumnsAtPlace = {
  isCollection: true,
  mayShape: true,
  level: 'COLUMNS',
  columns: 3,
  offered: ['LIST_COLLAPSED', 'LIST_EXPANDED', 'TIMELINE'],
};

test('a collection with columns and no board offered names how many', () => {
  assert.equal(unusedColumns(place), 3);
});

for (const [why, change] of [
  ['a board is offered', { offered: ['LIST_COLLAPSED', 'KANBAN'] }],
  ['a workflow runs for the tasks', { level: 'WORKFLOW' }],
  ['the collection has no columns', { level: 'NONE', columns: 0 }],
  ['the level is not read yet', { level: undefined }],
  ['the columns are not read yet', { columns: 0 }],
  ['the reader may not shape it', { mayShape: false }],
  ['it is a hub', { isCollection: false }],
] as const) {
  test(`no hint where ${why}`, () => {
    assert.equal(unusedColumns({ ...place, ...change }), undefined);
  });
}
