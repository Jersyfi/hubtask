// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { matchesPath } from '@hubtask/sync-engine';

import { ANY_ENTRY_DOCUMENT, ENTRY_LISTS, TOUCHES_ANY_ENTRY, TOUCHES_ANY_STATUS, entryDocument, touchesOf } from './touches.ts';

const ID = '0192f000-0000-7000-8000-000000000001';
const OTHER = '0192f000-0000-7000-8000-000000000002';

/** The paths an entry page holds open, as `ItemView` reads them. */
const OPEN = {
  document: `/items/${ID}?expand=labels`,
  history: `/items/${ID}/activity`,
  comments: `/items/${ID}/comments`,
  reminders: `/items/${ID}/reminders`,
  recurrence: `/items/${ID}/recurrence`,
  attachments: `/items/${ID}/attachments`,
  list: '/items:query',
  steps: `/items/${ID}/steps`,
  sla: `/items/${ID}/sla`,
  other: `/items/${OTHER}?expand=labels`,
};

const covered = (names: readonly string[]) =>
  Object.entries(OPEN).filter(([, path]) => names.some((name) => matchesPath(name, path))).map(([what]) => what);

test('a write to one entry re-reads its document, its history and the lists - and nothing that hangs under it', () => {
  // A title touches the entry, its history and the list - not its thread, reminders, series or attachments.
  assert.deepEqual(covered(touchesOf(ID)), ['document', 'history', 'list']);
});

test('a write that may reach other entries re-reads every open document, and still no thread', () => {
  assert.deepEqual(covered(TOUCHES_ANY_ENTRY), ['document', 'history', 'list', 'other']);
});

test('ticking an entry off re-reads its status, its steps and its clocks, which a workflow moves with it', () => {
  assert.deepEqual(covered(TOUCHES_ANY_STATUS), ['document', 'history', 'list', 'steps', 'sla', 'other']);
});

test('the names are the engine\'s marks, spelled once', () => {
  assert.equal(entryDocument(ID), `/items/${ID}$`);
  assert.equal(ENTRY_LISTS, '/items:query');
  assert.equal(matchesPath(ANY_ENTRY_DOCUMENT, '/items:query'), false, 'the lists are not a document');
});

// A move to another collection can bring workflows on there; the entry's steps read before belong
// to the old place, and a status screen that subscribes again would get them without a load.
test('a move invalidates the steps of the entries it reaches', () => {
  const source = readFileSync(new URL('./items.svelte.ts', import.meta.url), 'utf8');
  const start = source.indexOf('`/items/${id}:move`');
  assert.ok(start >= 0, 'the move is found');
  const move = source.slice(start, source.indexOf('\n  }\n', start));
  assert.match(move, /invalidates: TOUCHES_ANY_STATUS\b/u);
});
