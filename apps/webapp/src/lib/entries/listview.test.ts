// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import test from 'node:test';
import assert from 'node:assert/strict';

import {
  defaultArrangement,
  dueBandOf,
  groupRows,
  offeredFields,
  offeredGroupings,
  openingArrangement,
  rememberArrangement,
  toggleField,
  type ListOffer,
} from './listview.ts';

const project: ListOffer = { isSimple: false, isLevelled: true, offersAmount: false, hasColumns: true, carriesDue: true, carriesPeople: true, carriesLabels: true };
const shopping: ListOffer = { isSimple: true, isLevelled: false, offersAmount: true, hasColumns: false, carriesDue: false, carriesPeople: false, carriesLabels: true };

function memory() {
  const kept = new Map<string, string>();
  return { getItem: (key: string) => kept.get(key) ?? null, setItem: (key: string, value: string) => void kept.set(key, value) };
}

test('a template brings its arrangement: a list its amount by category, done at the end; a project its fields', () => {
  assert.deepEqual(defaultArrangement(shopping), { fields: ['amount'], grouping: 'category', done: 'bottom' });
  assert.deepEqual(defaultArrangement(project), { fields: ['due', 'people', 'labels', 'progress'], grouping: 'none', done: 'inline' });
});

test('only what the collection carries is offered, as a field and as a grouping', () => {
  assert.deepEqual(offeredFields(shopping), ['amount', 'labels']);
  assert.deepEqual(offeredGroupings(shopping), ['none', 'category']);
  assert.deepEqual(offeredFields(project), ['column', 'due', 'people', 'labels', 'progress']);
  assert.deepEqual(offeredGroupings(project), ['none', 'category', 'column', 'assignee', 'due']);
});

test('the device keeps an arrangement, and a kept value the collection no longer offers falls back', () => {
  const store = memory();
  rememberArrangement(store, 'anna', 'kitchen', { fields: ['labels', 'due', 'amount'], grouping: 'column', done: 'hidden' });
  const opened = openingArrangement({ store, accountId: 'anna', collectionId: 'kitchen', offer: project });
  assert.deepEqual(opened, { fields: ['due', 'labels'], grouping: 'column', done: 'hidden' }, 'amount is not offered; the order is the columns\'');
  const elsewhere = openingArrangement({ store, accountId: 'anna', collectionId: 'kitchen', offer: shopping });
  assert.equal(elsewhere.grouping, 'category', 'a grouping the list does not offer falls back to the template\'s');
  store.setItem('hubtask.list.anna.garden', '{not json');
  assert.deepEqual(openingArrangement({ store, accountId: 'anna', collectionId: 'garden', offer: shopping }), defaultArrangement(shopping));
});

test('a field toggles in and out and the columns keep their order', () => {
  assert.deepEqual(toggleField(['labels'], 'due'), ['due', 'labels']);
  assert.deepEqual(toggleField(['due', 'labels'], 'due'), ['labels']);
});

interface R { id: string; depth: number; key: string | null; done: boolean }
const r = (id: string, depth: number, key: string | null, done = false): R => ({ id, depth, key, done });
const ids = (rows: readonly R[]) => rows.map((row) => row.id);

test('groups take whole blocks - a row with what is open under it - in the given order, the rest last', () => {
  const rows = [r('a', 0, 'doing'), r('a1', 1, null), r('b', 0, null), r('c', 0, 'todo'), r('d', 0, 'doing')];
  const { groups, done } = groupRows(rows, { depthOf: (x) => x.depth, keyOf: (x) => x.key, isDone: (x) => x.done, order: ['todo', 'doing'], done: 'inline', grouping: 'column' });
  assert.deepEqual(groups.map((group) => [group.key, ids(group.rows)]), [['todo', ['c']], ['doing', ['a', 'a1', 'd']], [null, ['b']]]);
  assert.deepEqual(done, []);
});

test('what is done is gathered at the end, left out, or left where it stands', () => {
  const rows = [r('a', 0, null), r('b', 0, null, true), r('b1', 1, null), r('c', 0, null)];
  const options = { depthOf: (x: R) => x.depth, keyOf: (x: R) => x.key, isDone: (x: R) => x.done, order: [], grouping: 'none' as const };
  const bottom = groupRows(rows, { ...options, done: 'bottom' });
  assert.deepEqual(ids(bottom.groups[0]!.rows), ['a', 'c']);
  assert.deepEqual(ids(bottom.done), ['b', 'b1']);
  assert.deepEqual(groupRows(rows, { ...options, done: 'hidden' }).done, []);
  assert.deepEqual(ids(groupRows(rows, { ...options, done: 'inline' }).groups[0]!.rows), ['a', 'b', 'b1', 'c']);
});

test('a date falls in its band on the reader\'s day', () => {
  assert.equal(dueBandOf('2026-10-20', '2026-10-24', '2026-10-30'), 'overdue');
  assert.equal(dueBandOf('2026-10-24', '2026-10-24', '2026-10-30'), 'today');
  assert.equal(dueBandOf('2026-10-28', '2026-10-24', '2026-10-30'), 'week');
  assert.equal(dueBandOf('2026-11-12', '2026-10-24', '2026-10-30'), 'later');
  assert.equal(dueBandOf(undefined, '2026-10-24', '2026-10-30'), null);
});

test('the entry type is a column and a grouping only where the workspace shows types', () => {
  const typed: ListOffer = { ...project, showsTypes: true };
  assert.deepEqual(offeredFields(typed), ['type', 'column', 'due', 'people', 'labels', 'progress']);
  assert.deepEqual(offeredGroupings(typed), ['none', 'type', 'category', 'column', 'assignee', 'due']);
  assert.equal(offeredFields(project).includes('type'), false);
  assert.equal(offeredGroupings(project).includes('type'), false);
  assert.deepEqual(toggleField(['due'], 'type'), ['type', 'due']);
  const store = memory();
  rememberArrangement(store, 'anna', 'kitchen', { fields: ['type'], grouping: 'type', done: 'inline' });
  assert.deepEqual(openingArrangement({ store, accountId: 'anna', collectionId: 'kitchen', offer: typed }), { fields: ['type'], grouping: 'type', done: 'inline' });
  assert.deepEqual(openingArrangement({ store, accountId: 'anna', collectionId: 'kitchen', offer: project }), { fields: [], grouping: 'none', done: 'inline' }, 'without the workflows module the type falls back');
});
