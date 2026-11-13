// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { happenedVerb, latestHappenings } from './happened.ts';

const entry = (id: string, updated: string) => ({ id, title: `Entry ${id}`, collection_id: 'c1', updated_at: updated }) as never;
const step = (code: string, at: string) => ({ id: 's', item_id: 'x', code, actor: { type: 'USER', id: 'a1' }, occurred_at: at, change_set: {} }) as never;

test('the happenings are entries, pages and boards, newest first, at most as many as shown', () => {
  const said = latestHappenings({
    entries: [entry('e1', '2026-11-13T10:00:00Z'), entry('e2', '2026-11-13T08:00:00Z')],
    steps: { e1: step('activity.item_created', '2026-11-13T10:00:00Z'), e2: step('activity.item_step_taken', '2026-11-13T08:00:00Z') },
    pages: [{ id: 'p1', title: 'Handbook', updated_at: '2026-11-13T09:00:00Z', updated_by: 'a2' }],
    boards: [{ id: 'b1', title: 'Retro', updated_at: '2026-11-12T09:00:00Z', updated_by: 'a3' }],
  }, 3);
  assert.deepEqual(said.map((each) => [each.kind, each.id, happenedVerb(each)]), [
    ['entry', 'e1', 'created'], ['page', 'p1', 'page_changed'], ['entry', 'e2', 'moved_on'],
  ]);
  assert.equal(said[0]?.actorId, 'a1');
  assert.equal(said[1]?.href, '/pages/p1');
});

test('an entry waits for its step, one without a history is said as changed, and an undated row is left out', () => {
  const said = latestHappenings({
    entries: [entry('e1', '2026-11-13T10:00:00Z'), entry('e2', '2026-11-13T08:00:00Z')],
    steps: { e2: null },
    pages: [{ id: 'p1', title: 'Undated' } as never],
    boards: [],
  }, 5);
  assert.deepEqual(said.map((each) => [each.id, happenedVerb(each), each.actorId]), [['e2', 'changed', undefined]]);
});
