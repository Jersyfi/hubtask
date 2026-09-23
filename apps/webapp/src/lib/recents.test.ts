// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import test from 'node:test';
import assert from 'node:assert/strict';

import { MAX, isUnchanged, keyFor, parse, promote, type Recent } from './recents.ts';

const row = (id: string, title = id): Recent => ({ kind: 'item', id, title });

test('the newest is at the front, and the same entry opened twice is one row', () => {
  let rows = promote([], row('a'));
  rows = promote(rows, row('b'));
  assert.deepEqual(rows.map((each) => each.id), ['b', 'a']);

  rows = promote(rows, row('a'));
  assert.deepEqual(rows.map((each) => each.id), ['a', 'b'], 'opening it again moved it rather than adding it');
});

test('two kinds may share an id, because two levels do', () => {
  const rows = promote([{ kind: 'collection', id: 'x', title: 'Kitchen' }], { kind: 'item', id: 'x', title: 'Milk' });
  assert.equal(rows.length, 2, 'a collection and an entry with the same id are one row');
});

test('the list is a glance, not a history', () => {
  let rows: readonly Recent[] = [];
  for (let each = 0; each < MAX + 3; each += 1) rows = promote(rows, row(`e${each}`));
  assert.equal(rows.length, MAX);
  assert.equal(rows.at(-1)?.id, `e${3}`, 'the oldest fell off the end');
});

test('a re-opened front row writes nothing, and a renamed one does', () => {
  const rows = [row('a', 'Milk'), row('b')];
  assert.equal(isUnchanged(rows, row('a', 'Milk')), true);
  assert.equal(isUnchanged(rows, row('a', 'Oat milk')), false, 'a new title is a change');
  assert.equal(isUnchanged(rows, row('b')), false, 'a row that is not at the front is a change');
  assert.equal(isUnchanged([], row('a')), false);
});

test('what cannot be read back is discarded rather than repaired', () => {
  assert.deepEqual(parse(null), []);
  assert.deepEqual(parse('not json'), []);
  assert.deepEqual(parse('{"kind":"item"}'), [], 'an object that is not a list');
  assert.deepEqual(parse('[{"kind":"planet","id":"a","title":"a"}]'), [], 'a kind this client does not draw');
  assert.deepEqual(parse('[{"id":"a"}]'), [], 'a row with no title');
  assert.deepEqual(parse(JSON.stringify([row('a'), { nonsense: true }])), [row('a')], 'the good rows survive the bad ones');
});

test('one account, one key', () => {
  assert.equal(keyFor('01a0'), 'hubtask.recent.01a0');
  assert.notEqual(keyFor('01a0'), keyFor('01a1'));
});
