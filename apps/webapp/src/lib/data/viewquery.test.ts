// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { asWords, changedParts, groupingOf, lanesOf, lineOf, monthGrid, personNamed, readQuery, spokenPiece, type Names, type ViewSetup } from './viewquery.ts';

const AMELIA = '0192f000-0000-7000-8000-0000000000a1';
const LABEL = '0192f000-0000-7000-8000-0000000000b1';
const ATLAS = '0192f000-0000-7000-8000-0000000000c1';
const names: Names = {
  person: (name) => (name.toLowerCase() === 'amelia hughes' ? AMELIA : undefined),
  label: (name) => (name === 'kunden-app' ? LABEL : undefined),
  collection: (name) => (name === 'Team Atlas' ? ATLAS : undefined),
  nameOf: (_field, id) => ({ [AMELIA]: 'Amelia Hughes', [LABEL]: 'kunden-app', [ATLAS]: 'Team Atlas' })[id],
};

// UC-WRK-17: names become identifiers before anything is sent (ADR-0102); me and none stay words the server reads.
test('a line sends a tree whose names are identifiers', () => {
  const read = readQuery('who: me, "Amelia Hughes" label: kunden-app in: "Team Atlas" is: open', names);
  assert.ok(read.ok);
  assert.deepEqual(read.query.filter, {
    op: 'AND',
    nodes: [
      { field: 'assignee_id', op: 'IN', value: ['@me', AMELIA] },
      { field: 'labels', op: 'CONTAINS', value: LABEL },
      { field: 'collection_id', op: 'EQ', value: ATLAS },
      { field: 'is_completed', op: 'EQ', value: false },
    ],
  });
  assert.deepEqual(read.unknown, []);
});

test('a name nobody has stays as typed and is named', () => {
  const read = readQuery('who: Nobody', names);
  assert.ok(read.ok);
  assert.deepEqual(read.unknown, ['Nobody']);
});

test('free words are one text match beside the tree, the panel order before the line', () => {
  const read = readQuery('passkey is: open sort: -due', names, { field: 'title', dir: 'ASC' });
  assert.ok(read.ok);
  assert.deepEqual(read.query.filter, { op: 'AND', nodes: [{ field: 'is_completed', op: 'EQ', value: false }, { field: 'text', op: 'MATCHES', value: 'passkey' }] });
  assert.deepEqual(read.query.sort, [{ field: 'title', dir: 'ASC' }]);
  const own = readQuery('sort: -due', names);
  assert.ok(own.ok);
  assert.deepEqual(own.query.sort, [{ field: 'due_at', dir: 'DESC' }]);
});

// The orchestrator's decision: a word: that names no field is searched as words, never a dead end.
test('a word: that names no field is searched as words', () => {
  const words = asWords('Re: Besprechung is: open');
  assert.deepEqual(words.pieces, ['Re: Besprechung']);
  const read = readQuery('Re: Besprechung is: open', names);
  assert.ok(read.ok);
  assert.deepEqual(read.words, ['Re: Besprechung']);
  assert.match(JSON.stringify(read.query.filter), /MATCHES/u);
  assert.equal(asWords('is: open').pieces.length, 0);
});

test('a line that cannot be read answers its code', () => {
  const read = readQuery('(is: open', names);
  assert.equal(read.ok, false);
});

test('a stored tree is spelled back with names', () => {
  assert.equal(lineOf({ filter: { field: 'assignee_id', op: 'EQ', value: AMELIA } }, names), 'who: "Amelia Hughes"');
  assert.equal(lineOf({}, names), '');
});

const base: ViewSetup = { layout: 'LIST_COLLAPSED', line: '', order: undefined, grouping: 'none', done: 'inline', fields: ['due'], columnsBy: 'bucket_id', lanesBy: '', dateField: 'due_at', limits: true };

// UC-WRK-17: "Ansicht · n geändert" counts the parts that differ from the tab's view.
test('the changed parts are counted, the two list layouts one', () => {
  assert.equal(changedParts(base, base), 0);
  assert.equal(changedParts({ ...base, layout: 'LIST_EXPANDED' }, base), 0);
  assert.equal(changedParts({ ...base, line: 'is: open', grouping: 'assignee', lanesBy: 'assignee_id' }, base), 3);
});

test('a view stores its columns, lanes and calendar date in its grouping', () => {
  assert.deepEqual(groupingOf({ ...base, lanesBy: 'assignee_id' }), { field: 'bucket_id', limit_per_group: 50, lanes_by: 'assignee_id', date_field: 'due_at', list: 'none', done: 'inline' });
});

// UC-WRK-05: lanes by a field, the entries without a value last.
test('lanes are the values in order, the empty lane last', () => {
  assert.deepEqual(lanesOf([{ assignee_id: null }, { assignee_id: 'b' }, { assignee_id: 'a' }, { assignee_id: 'b' }], 'assignee_id'), ['b', 'a', '']);
  assert.deepEqual(lanesOf([{ custom_fields: { prio: 'P1' } }, {}], 'custom_fields.prio'), ['P1', '']);
});

test('a month is whole weeks from the account week start', () => {
  const weeks = monthGrid(2026, 10);
  assert.equal(weeks[0]?.[0], '2026-10-26');
  assert.equal(weeks.at(-1)?.at(-1), '2026-12-06');
  assert.equal(monthGrid(2026, 10, true)[0]?.[0], '2026-11-01');
});

// §6.11 Q-2, ADR-0102: a release and a sprint are named in the line and the chip; the tree sends their ids.
const SPRING = '0192f000-0000-7000-8000-0000000000d1';
const FOURTH = '0192f000-0000-7000-8000-0000000000e1';
const planned: Names = {
  ...names,
  release: (name) => (name === 'Spring' ? SPRING : undefined),
  sprint: (name) => (name === 'Sprint 4' ? FOURTH : undefined),
  nameOf: (field, id) => ({ [SPRING]: 'Spring', [FOURTH]: 'Sprint 4' })[id] ?? names.nameOf?.(field, id),
};

test('a release and a sprint typed by name are sent by id; none stays none', () => {
  const read = readQuery('plan.release: Spring plan.sprint: "Sprint 4" -plan.release: none', planned);
  assert.ok(read.ok);
  assert.match(JSON.stringify(read.query.filter), new RegExp(`"release_id","op":"EQ","value":"${SPRING}"`, 'u'));
  assert.match(JSON.stringify(read.query.filter), new RegExp(`"sprint_id","op":"EQ","value":"${FOURTH}"`, 'u'));
  assert.match(JSON.stringify(read.query.filter), /IS_NULL/u);
  assert.deepEqual(read.unknown, []);
  assert.equal(lineOf({ filter: { field: 'release_id', op: 'EQ', value: SPRING } }, planned), 'plan.release: Spring');
});

test('a chip shows the name where its piece holds an id, and the piece itself otherwise', () => {
  assert.equal(spokenPiece(`plan.release: ${SPRING}`, planned), 'plan.release: Spring');
  assert.equal(spokenPiece(`plan.sprint: ${FOURTH}`, planned), 'plan.sprint: "Sprint 4"');
  assert.equal(spokenPiece('plan.release: Spring', planned), 'plan.release: Spring');
  assert.equal(spokenPiece('is: open', planned), 'is: open');
  assert.equal(spokenPiece('passkey', planned), 'passkey');
});

// UC-GOL-04 check 18: a dashboard's filter names people as a collection's line does - "who: Amelia"
// is the person of that name among those who may be named there, whatever the case typed.
test('a person is named in a dashboard filter as in a collection', () => {
  const nameOf = (id: string) => ({ [AMELIA]: 'Amelia', [LABEL]: 'Ben' })[id];
  const dashboard: Names = { person: (name) => personNamed(name, [LABEL, AMELIA], nameOf), label: () => undefined, collection: () => undefined };
  const read = readQuery('who: amelia', dashboard);
  assert.ok(read.ok);
  assert.deepEqual(read.query.filter, { field: 'assignee_id', op: 'EQ', value: AMELIA });
  assert.deepEqual(read.unknown, []);
  const carla = readQuery('who: Carla', dashboard);
  assert.ok(carla.ok);
  assert.deepEqual(carla.unknown, ['Carla']);
});
