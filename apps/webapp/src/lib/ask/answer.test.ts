// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { answerOf, answerSince, chipsOf, conditionsOf, hasChips, offersAsk, viewQueryOf, without } from './answer.ts';

const HUB = '0192f000-0000-7000-8000-0000000000a1';
const LISA = '0192f000-0000-7000-8000-0000000000b1';
const URGENT = '0192f000-0000-7000-8000-0000000000c1';

test('an answer is read off its suggestion with its sources and its chips', () => {
  const answer = answerOf({
    kind: 'ANSWER',
    payload: {
      answer: 'Two offers are open.',
      source_ids: ['e1', 'e2'],
      filter: { place_id: HUB, place_type: 'HUB', is_completed: false, due: 'next_7_days', assignee_id: LISA, label_ids: [URGENT] },
    },
  });
  assert.equal(answer?.text, 'Two offers are open.');
  assert.deepEqual(answer?.sourceIds, ['e1', 'e2']);
  assert.deepEqual(answer?.chips, {
    place: { id: HUB, type: 'HUB' }, isCompleted: false, due: 'next_7_days', assigneeId: LISA, labelIds: [URGENT],
  });
});

test('a suggestion of another kind, or one without words, is no answer', () => {
  assert.equal(answerOf({ kind: 'RULE', payload: { answer: 'x' } }), undefined);
  assert.equal(answerOf({ kind: 'ANSWER', payload: { source_ids: ['e1'] } }), undefined);
});

test('a part of a filter the server would not keep draws no chip', () => {
  assert.deepEqual(chipsOf({ place_id: HUB, due: 'someday', is_completed: 'no', expr: '1 = 1' }), {});
  assert.equal(hasChips(chipsOf(undefined)), false);
});

test('the chips stand for the item query, a span resolved against the reader’s today', () => {
  assert.deepEqual(conditionsOf({ isCompleted: false, due: 'next_7_days', assigneeId: LISA, labelIds: [URGENT, 'x'] }), [
    { field: 'is_completed', op: 'EQ', value: 'false' },
    { field: 'due_at', op: 'LTE', value: '@today+P7D' },
    { field: 'assignee_id', op: 'EQ', value: LISA },
    { field: 'labels', op: 'CONTAINS_ANY', value: `${URGENT},x` },
  ]);
  assert.deepEqual(conditionsOf({ due: 'today' }), [
    { field: 'due_at', op: 'GTE', value: '@today' },
    { field: 'due_at', op: 'LT', value: '@today+P1D' },
  ]);
  assert.deepEqual(conditionsOf({ due: 'overdue' }), [{ field: 'due_at', op: 'LT', value: '@today' }]);
  assert.deepEqual(conditionsOf({ due: 'this_month' }), [{ field: 'due_at', op: 'LTE', value: '@end_of_month' }]);
});

test('a removed chip is gone from what is saved (UC-AI-08 check 4)', () => {
  const chips = without({ isCompleted: false, assigneeId: LISA }, 'assigneeId');
  assert.deepEqual(conditionsOf(chips), [{ field: 'is_completed', op: 'EQ', value: 'false' }]);
});

test('a view is anchored at the place chip, and without one there is nothing to save', () => {
  const filter = { field: 'is_completed', op: 'EQ', value: false };
  assert.deepEqual(viewQueryOf({ place: { id: HUB, type: 'HUB' } }, filter), {
    scope_container_id: HUB, include_descendants: true, filter,
  });
  assert.deepEqual(viewQueryOf({ place: { id: HUB, type: 'COLLECTION' } }, undefined), {
    scope_container_id: HUB, include_descendants: false,
  });
  assert.equal(viewQueryOf({ isCompleted: false }, filter), undefined);
});

test('the answer a question produced is the newest proposed one since it was asked', () => {
  const at = '2026-10-23T09:00:00Z';
  const found = answerSince(
    [
      { id: 'old', kind: 'ANSWER', status: 'PROPOSED', created_at: '2026-10-23T08:59:00Z' },
      { id: 'rule', kind: 'RULE', status: 'PROPOSED', created_at: '2026-10-23T09:01:00Z' },
      { id: 'gone', kind: 'ANSWER', status: 'DISMISSED', created_at: '2026-10-23T09:02:00Z' },
      { id: 'new', kind: 'ANSWER', status: 'PROPOSED', created_at: '2026-10-23T09:01:30Z' },
    ],
    at,
  );
  assert.equal(found?.id, 'new');
});

test('the palette offers Ask for typed words only with consent to AI (UC-AI-08 check 1)', () => {
  assert.equal(offersAsk({ ai_suggestions: true }, 'What is open?'), true);
  assert.equal(offersAsk({ ai_suggestions: true }, '   '), false);
  assert.equal(offersAsk({ ai_suggestions: false }, 'What is open?'), false);
  assert.equal(offersAsk(undefined, 'What is open?'), false);
});
