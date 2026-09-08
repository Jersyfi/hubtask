// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { BulkResult } from '@hubtask/sync-engine';

import {
  BULK_OPERATIONS,
  allApplied,
  byItem,
  createOperations,
  isDestructive,
  needsItem,
  operationsFor,
  outcomeOf,
  tallyOf,
} from './bulk.ts';

const result = (over: Partial<BulkResult>): BulkResult => ({ index: 0, status: 200, ...over }) as BulkResult;

test('the nine are the contract’s nine', () => {
  assert.equal(BULK_OPERATIONS.length, 9);
  assert.deepEqual(
    [...BULK_OPERATIONS].sort(),
    ['ADD_LABEL', 'ASSIGN', 'COMPLETE_ITEM', 'CREATE_ITEM', 'MOVE_ITEM', 'REMOVE_LABEL', 'REOPEN_ITEM', 'TRASH_ITEM', 'UPDATE_ITEM'],
  );
  // The one with no entry yet, and the one that asks first.
  assert.equal(needsItem('CREATE_ITEM'), false);
  assert.equal(needsItem('TRASH_ITEM'), true);
  assert.equal(isDestructive('TRASH_ITEM'), true);
  assert.equal(isDestructive('COMPLETE_ITEM'), false);
});

test('one operation per selected entry, in the order they are drawn', () => {
  const operations = operationsFor('ADD_LABEL', ['a', 'b'], { label_id: 'l-1' });
  assert.deepEqual(operations, [
    { op: 'ADD_LABEL', item_id: 'a', payload: { label_id: 'l-1' } },
    { op: 'ADD_LABEL', item_id: 'b', payload: { label_id: 'l-1' } },
  ]);
});

test('CREATE_ITEM is one per line, and carries no entry', () => {
  const operations = createOperations(' First \n\n Second \n', { collection_id: 'c-1', type: 'TASK' });
  assert.equal(operations.length, 2);
  assert.deepEqual(operations[0], {
    op: 'CREATE_ITEM',
    payload: { collection_id: 'c-1', type: 'TASK', title: 'First' },
  });
  assert.equal(operations[0]?.item_id, undefined);
});

test('a rolled-back operation is told from a refusal by what it does not carry', () => {
  // The contract's own distinction: an operation of an atomic bulk that never ran carries neither
  // an item nor a problem. Reading the 409 alone would call a version conflict a rollback.
  assert.equal(outcomeOf(result({ status: 200, item: {} as never })), 'applied');
  assert.equal(outcomeOf(result({ status: 201, item: {} as never })), 'applied');
  assert.equal(outcomeOf(result({ status: 409 })), 'not_applied');
  // …and the shape this server actually sends: a 409 that does carry a problem, whose detail code
  // names the rollback. Found by walking a real atomic bulk; the schema describes only the first.
  assert.equal(
    outcomeOf(result({ status: 409, problem: { detail_code: 'bulk.rolled_back' } as never })),
    'not_applied',
  );
  assert.equal(outcomeOf(result({ status: 409, problem: { code: 'version_conflict' } as never })), 'refused');
  assert.equal(outcomeOf(result({ status: 403, problem: {} as never })), 'refused');
});

test('a half-applied bulk is not a failed one', () => {
  const results = [result({ index: 0 }), result({ index: 1, status: 403, problem: {} as never })];
  assert.equal(allApplied(results), false);
  assert.equal(allApplied([result({ index: 0 })]), true);
  assert.deepEqual(tallyOf(results), { applied: 1, refused: 1, not_applied: 0 });
});

test('a result is matched to its entry by index, never by the item it answered', () => {
  // A refused operation carries no item — and that is precisely the one whose entry the reader
  // most needs named.
  const operations = operationsFor('COMPLETE_ITEM', ['a', 'b']);
  const results = [result({ index: 0 }), result({ index: 1, status: 403, problem: {} as never })];

  const found = byItem(operations, results);
  assert.equal(found.get('a')?.status, 200);
  assert.equal(found.get('b')?.status, 403);
});
