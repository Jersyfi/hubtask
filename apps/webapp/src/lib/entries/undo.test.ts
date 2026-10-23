// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { ancestorsFirst, sentenceOf, takenBackCode, type Act } from './undo.ts';

const ACTS: readonly Act[] = [
  { kind: 'trash', itemId: 'i1', title: 'Buy milk' },
  { kind: 'archive', itemId: 'i1', title: 'Buy milk' },
  { kind: 'label-removed', itemId: 'i1', labelId: 'l1', title: 'Buy milk' },
  { kind: 'label-deleted', collectionId: 'c1', labelId: 'l1', name: 'Urgent' },
  { kind: 'bulk-trash', itemIds: ['i1', 'i2'] },
  { kind: 'bulk-label-removed', itemIds: ['i1', 'i2'], labelId: 'l1' },
];

test('every act has its sentence, with a count where it is several', () => {
  assert.deepEqual(ACTS.map((act) => sentenceOf(act).code), [
    'app.undo.trashed', 'app.undo.archived', 'app.undo.label_removed',
    'app.undo.label_deleted', 'app.undo.bulk_trashed', 'app.undo.bulk_label_removed',
  ]);
  assert.equal(sentenceOf(ACTS[4] as Act).params.count, 2);
  assert.equal(sentenceOf(ACTS[3] as Act).params.name, 'Urgent');
});

test('a capture says where it went, and a jumble capture taken back is dismissed, not gone', () => {
  const captured: Act = { kind: 'captured', itemIds: ['i1', 'i2'], place: 'Groceries' };
  const jumbled: Act = { kind: 'jumbled', entryId: 'e1' };
  assert.deepEqual(sentenceOf(captured), { code: 'app.capture.added', params: { count: '2', place: 'Groceries' } });
  assert.equal(sentenceOf(jumbled).code, 'app.capture.jumbled');
  assert.equal(takenBackCode(jumbled), 'app.capture.dismissed');
  assert.equal(takenBackCode(captured), 'app.undo.done');
  assert.equal(takenBackCode(ACTS[0] as Act), 'app.undo.done');
});

test('a selection comes back ancestors first, otherwise in the order it was picked', () => {
  const picked = [
    { id: 'child', depth: 2 },
    { id: 'other', depth: 1 },
    { id: 'parent', depth: 1 },
    { id: 'grandchild', depth: 3 },
  ];
  assert.deepEqual(ancestorsFirst(picked), ['other', 'parent', 'child', 'grandchild']);
  assert.deepEqual(ancestorsFirst([]), []);
});
