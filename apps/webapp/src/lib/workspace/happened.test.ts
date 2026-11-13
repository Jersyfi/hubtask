// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { happenedVerb, latestHappenings, statusNameOf } from './happened.ts';

const entry = (id: string, updated: string) => ({ id, title: `Entry ${id}`, collection_id: 'c1', updated_at: updated }) as never;
const step = (code: string, at: string, change_set: object = {}) => ({ id: 's', item_id: 'x', code, actor: { type: 'USER', id: 'a1' }, occurred_at: at, change_set }) as never;

test('the happenings are entries, pages and boards, newest first, at most as many as shown', () => {
  const said = latestHappenings({
    entries: [entry('e1', '2026-11-13T10:00:00Z'), entry('e2', '2026-11-13T08:00:00Z')],
    steps: { e1: step('activity.item_created', '2026-11-13T10:00:00Z'), e2: step('activity.item_step_taken', '2026-11-13T08:00:00Z', { step: { to: 'start' }, status: { from: 'todo', to: 'in_progress' } }) },
    pages: [{ id: 'p1', title: 'Handbook', updated_at: '2026-11-13T09:00:00Z', updated_by: 'a2' }],
    boards: [{ id: 'b1', title: 'Retro', updated_at: '2026-11-12T09:00:00Z', updated_by: 'a3' }],
  }, 3);
  assert.deepEqual(said.map((each) => [each.kind, each.id, happenedVerb(each)]), [
    ['entry', 'e1', 'created'], ['page', 'p1', 'page_changed'], ['entry', 'e2', 'set_status'],
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

test('a status a step set is named as its workflow names it, else by the client, else by its key', () => {
  const list = {
    data: [{ key: 'review', published: { statuses: [{ key: 'checking', name: 'Im Review', category: 'IN_PROGRESS' }] } }],
    scheme: [{ container_id: 'c1', entry_type: null, workflow: 'review' }],
  } as never;
  const t = (code: string) => ({ 'workflows.status.in_progress': 'In Arbeit' })[code] ?? code;
  const said = { collectionId: 'c1', entryType: 'task' };
  assert.equal(statusNameOf({ ...said, statusKey: 'checking' }, 'h1', list, t), 'Im Review');
  assert.equal(statusNameOf({ ...said, statusKey: 'in_progress' }, 'h1', undefined, t), 'In Arbeit');
  assert.equal(statusNameOf({ ...said, statusKey: 'parked' }, 'h1', list, t), 'parked');
});
