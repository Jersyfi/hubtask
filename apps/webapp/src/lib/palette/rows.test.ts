// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { CAPS, EMPTY_ACTIONS, groupOf, indexOfKey, keyOf, rank, rowId, rowsOf, step, type Candidate } from './rows.ts';
import { QUICK as ALL_QUICK, type Quick } from '../data/searchquery.ts';

const QUICK: readonly Quick[] = [
  { code: 'a', line: 'who:me is:open', field: 'assignee_id' },
  { code: 'b', line: 'is:open due:week', field: 'due_at' },
];

const kitchen: Candidate = { kind: 'container', id: 'c1', label: 'Kitchen' };
const garden: Candidate = { kind: 'container', id: 'c2', label: 'Kitchen garden' };
const oldKitchen: Candidate = { kind: 'container', id: 'c3', label: 'Old kitchen' };
const newTask: Candidate = { kind: 'action', id: 'new-in:c1', label: 'New task in Kitchen' };
const board: Candidate = { kind: 'action', id: 'open-as:c1:KANBAN', label: 'Kitchen as a board' };
const settings: Candidate = { kind: 'place', id: 'profile', label: 'Your settings' };
const milk: Candidate = { kind: 'hit', id: 'i1', label: 'Buy milk' };

test('the rows come in groups, each ranked by match, the group with the best match first', () => {
  const ranked = rank([oldKitchen, newTask, board, garden, kitchen, settings], 'kitch');
  assert.deepEqual(
    ranked.map((each) => each.id),
    // The places: the two starting with it, then the one with a word starting with it; then the
    // actions, likewise.
    ['c2', 'c1', 'c3', 'open-as:c1:KANBAN', 'new-in:c1'],
  );
  assert.deepEqual([...new Set(ranked.map((each) => groupOf(each.kind)))], ['places', 'actions']);
});

test('a group whose best row matches better comes before one with only weaker matches', () => {
  const language: Candidate = { kind: 'setting', id: 'language', label: 'Language' };
  const page: Candidate = { kind: 'place', id: 'languages', label: 'All languages' };
  assert.deepEqual(rank([page, language], 'lang').map((each) => each.id), ['language', 'languages']);
});

test('a keyword finds a row in the reader\'s words, but below one that says them', () => {
  const add: Candidate = { kind: 'action', id: 'add', label: 'Eintrag hinzufügen', keywords: 'anlegen neu hinzufügen erstellen' };
  const hub: Candidate = { kind: 'action', id: 'hub', label: 'Hub anlegen', keywords: 'anlegen neu hinzufügen erstellen' };
  const name: Candidate = { kind: 'setting', id: 'name', label: 'Dein Name' };
  assert.deepEqual(rank([add, hub, name], 'anlegen', 'de').map((each) => each.id), ['add', 'hub']);
  assert.deepEqual(rank([add, hub], 'anle', 'de').map((each) => each.id), ['add', 'hub']);
  // The label's start outranks a keyword.
  const create: Candidate = { kind: 'action', id: 'create', label: 'Create hub', keywords: 'create new add make' };
  const capture: Candidate = { kind: 'action', id: 'capture', label: 'Capture', keywords: 'create new add make' };
  assert.deepEqual(rank([capture, create], 'create').map((each) => each.id), ['create', 'capture']);
});

test('the places opened last are offered with an empty field only, after the actions', () => {
  const recent: Candidate = { kind: 'recent', id: 'collection:c1', label: 'Kitchen' };
  assert.deepEqual(rank([recent, board, kitchen], '').map((each) => each.id), ['open-as:c1:KANBAN', 'collection:c1']);
  assert.deepEqual(rank([recent, kitchen], 'kitch').map((each) => each.id), ['c1']);
});

test('an empty field lists more actions than a typed answer, so the screen\'s own are not cut', () => {
  const many: Candidate[] = Array.from({ length: 14 }, (_, index) => ({ kind: 'action', id: `a${index}`, label: `Act ${index}` }));
  assert.equal(rank(many, '').length, EMPTY_ACTIONS);
  assert.equal(rank(many, 'act').length, CAPS.action);
});

test('rows that match nothing are dropped, but an entry the server found is kept', () => {
  const ranked = rank([settings, milk], 'kitch');
  assert.deepEqual(ranked.map((each) => each.id), ['i1']);
});

test('each kind is capped', () => {
  const many: Candidate[] = Array.from({ length: 9 }, (_, index) => ({ kind: 'container', id: `c${index}`, label: `Kitchen ${index}` }));
  assert.equal(rank(many, 'kitchen').length, CAPS.container);
});

test('with no words the palette lists the actions here, in their own order, and nothing else', () => {
  const ranked = rank([kitchen, board, newTask, settings, milk], '  ');
  assert.deepEqual(ranked.map((each) => each.id), ['open-as:c1:KANBAN', 'new-in:c1']);
});

test('with words, the rows are the matches, the sourced rows, the narrowings, and both ways on', () => {
  const rows = rowsOf('milk', [milk, kitchen], QUICK, ['ask']);
  assert.deepEqual(
    rows.map((row) => row.kind),
    ['hit', 'container', 'source', 'quick', 'quick', 'all', 'refine'],
  );
});

test('with an empty field there is nothing to refine, so that row is not offered', () => {
  const rows = rowsOf('   ', [], QUICK);
  assert.deepEqual(rows.map((row) => row.kind), ['quick', 'quick', 'all']);
});

test('all six quick narrowings are rows when offered', () => {
  const rows = rowsOf('', [], ALL_QUICK);
  assert.equal(rows.filter((row) => row.kind === 'quick').length, 6);
});

test('a row carries what pressing it needs: the candidate, or the narrowing as a line', () => {
  const rows = rowsOf('milk', [milk], QUICK);
  assert.equal(rows[0]?.id, 'i1');
  assert.equal(rows[1]?.id, 'who:me is:open');
});

test('the highlight follows its row when the entries arrive above it', () => {
  const before = rowsOf('kitch', [kitchen], QUICK);
  const held = keyOf(before[0]!);
  const after = rowsOf('kitch', [milk, kitchen], QUICK);
  assert.equal(indexOfKey(after, held), 1);
  assert.equal(indexOfKey(rowsOf('x', [], QUICK), held), -1);
  assert.equal(indexOfKey(after, undefined), -1);
});

test('a row id is its identity, written as a valid id', () => {
  const id = rowId({ kind: 'quick', id: 'who:me is:open' });
  assert.match(id, /^palette-row-[A-Za-z0-9_-]+$/);
  assert.notEqual(id, rowId({ kind: 'quick', id: 'who:me is:done' }));
  assert.equal(rowId({ kind: 'hit', id: 'i1' }), rowId({ kind: 'hit', id: 'i1' }));
});

test('the arrows walk down and come back to the words', () => {
  // -1 is "nothing highlighted", which is the state a field being typed in is always in: Enter
  // then searches for what was typed rather than opening whatever happened to be first.
  assert.equal(step(-1, 3, 1), 0);
  assert.equal(step(0, 3, 1), 1);
  assert.equal(step(2, 3, 1), -1);
});

test('and up, from the words to the last row', () => {
  assert.equal(step(-1, 3, -1), 2);
  assert.equal(step(0, 3, -1), -1);
});

test('an empty list has nowhere to go', () => {
  assert.equal(step(-1, 0, 1), -1);
  assert.equal(step(-1, 0, -1), -1);
});
