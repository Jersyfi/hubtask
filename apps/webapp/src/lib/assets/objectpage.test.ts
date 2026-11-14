// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { inheritedFrom, keepRelationsView, lifeSteps, missingRecommended, openKeysByObject, readRelationsView, workGroups } from './objectpage.ts';

const entry = (key: string, extra: Record<string, unknown> = {}) => ({
  item_id: key, key, title: key, entry_type: 'TASK', collection_id: 'c', hub_id: 'h', status: null, completed: false,
  start_at: null, due_at: null, reason: 'NAMES' as const, via: null, ...extra,
});

test('the recommended and required attributes without a value are the ones missing', () => {
  const attributes = [
    { key: 'url', kind: 'URL' as const, need: 'RECOMMENDED' as const },
    { key: 'version', kind: 'TEXT' as const, need: 'RECOMMENDED' as const },
    { key: 'owner_note', kind: 'TEXT' as const, need: 'OPTIONAL' as const },
    { key: 'serial', kind: 'TEXT' as const, need: 'REQUIRED' as const },
    { key: 'tags', kind: 'TEXT' as const },
  ];
  assert.deepEqual(missingRecommended(attributes, { url: 'https://x', version: '  ', serial: null }).map((each) => each.key), ['version', 'serial']);
  assert.deepEqual(missingRecommended(attributes, { url: 'u', version: '1', serial: 'S' }), []);
});

test('the work on an object is open first and the completed ones apart (IN-3)', () => {
  const { open, done } = workGroups([entry('SA-12'), entry('CHA-1', { completed: true }), entry('SA-8')]);
  assert.deepEqual(open.map((each) => each.key), ['SA-12', 'SA-8']);
  assert.deepEqual(done.map((each) => each.key), ['CHA-1']);
});

test('a neighbour carries the open entry that reaches the object through it, the first named', () => {
  const via = (ciId: string) => ({ ci_id: ciId, name: ciId, class: 'server' });
  const keys = openKeysByObject([
    entry('SA-12', { reason: 'BENEATH', via: via('db') }),
    entry('SA-13', { reason: 'BENEATH', via: via('db') }),
    entry('SA-14', { reason: 'BENEATH', via: via('app'), completed: true }),
    entry('SA-8'),
  ]);
  assert.deepEqual([...keys], [['db', 'SA-12']]);
});

test('the life shows every step, each reached one with its latest day and detail', () => {
  const steps = lifeSteps({
    state: 'DEPLOYED',
    moves: [
      { state: 'ORDERED', at: '2026-01-02T08:00:00Z', by: 'a', assigned_to: null, stockroom_id: null },
      { state: 'IN_STOCK', at: '2026-01-10T08:00:00Z', by: 'a', assigned_to: null, stockroom_id: 'room' },
      { state: 'DEPLOYED', at: '2026-01-12T08:00:00Z', by: 'a', assigned_to: 'p1', stockroom_id: null },
      { state: 'IN_STOCK', at: '2026-03-01T08:00:00Z', by: 'a', assigned_to: null, stockroom_id: 'room2' },
      { state: 'DEPLOYED', at: '2026-03-05T08:00:00Z', by: 'a', assigned_to: 'p2', stockroom_id: null },
    ],
  });
  assert.deepEqual(steps.map((each) => [each.state, each.isReached, each.isCurrent]), [
    ['REQUESTED', true, false], ['ORDERED', true, false], ['IN_STOCK', true, false], ['DEPLOYED', true, true], ['RETIRED', false, false], ['DISPOSED', false, false],
  ]);
  assert.equal(steps[0]!.at, undefined, 'passed over without a move of its own');
  assert.equal(steps[2]!.stockroom, 'room2', 'the latest arrival in the stockroom counts');
  assert.equal(steps[3]!.holder, 'p2');
  assert.equal(steps[3]!.at, '2026-03-05T08:00:00Z');
});

test('a record without moves still says where it stands', () => {
  const steps = lifeSteps({ state: 'IN_STOCK', moves: [] });
  assert.deepEqual(steps.filter((each) => each.isReached).map((each) => each.state), ['REQUESTED', 'ORDERED', 'IN_STOCK']);
  assert.ok(steps.every((each) => each.at === undefined));
});

test('the relations view is kept per reader and survives a store that throws', () => {
  const kept = new Map<string, string>();
  const store = { getItem: (key: string) => kept.get(key) ?? null, setItem: (key: string, value: string) => void kept.set(key, value) };
  assert.equal(readRelationsView(store, 'me'), undefined);
  keepRelationsView(store, 'list', 'me');
  assert.equal(readRelationsView(store, 'me'), 'list');
  assert.equal(readRelationsView(store, 'other'), undefined, 'another reader keeps a choice of their own');
  const refusing = { getItem: () => { throw new Error('denied'); }, setItem: () => { throw new Error('denied'); } };
  assert.equal(readRelationsView(refusing, 'me'), undefined);
  assert.doesNotThrow(() => keepRelationsView(refusing, 'picture', 'me'));
});

test('a raised need names the nearest object that suffers with that need of its own', () => {
  const known = new Map([
    ['portal', { protection: { c: 'HIGH' as const } }],
    ['shop', { protection: { c: 'HIGH' as const, a: 'VERY_HIGH' as const } }],
  ]);
  const reached = [{ ci_id: 'shop', depth: 2 }, { ci_id: 'portal', depth: 1 }];
  assert.equal(inheritedFrom('c', 'HIGH', reached, known), 'portal');
  assert.equal(inheritedFrom('a', 'VERY_HIGH', reached, known), 'shop');
  assert.equal(inheritedFrom('i', 'HIGH', reached, known), undefined);
  assert.equal(inheritedFrom('c', null, reached, known), undefined);
});
