// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { matchesPath } from '@hubtask/sync-engine';

import { ANY_ENTRY_DOCUMENT, ENTRY_LISTS, TOUCHES_ANY_ENTRY, entryDocument, touchesOf } from './touches.ts';

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
  other: `/items/${OTHER}?expand=labels`,
};

const covered = (names: readonly string[]) =>
  Object.entries(OPEN).filter(([, path]) => names.some((name) => matchesPath(name, path))).map(([what]) => what);

test('a write to one entry re-reads its document, its history and the lists - and nothing that hangs under it', () => {
  // Issue 877: `/items` re-read the thread, the reminders, the series and the attachments for a title.
  assert.deepEqual(covered(touchesOf(ID)), ['document', 'history', 'list']);
});

test('a write that may reach other entries re-reads every open document, and still no thread', () => {
  assert.deepEqual(covered(TOUCHES_ANY_ENTRY), ['document', 'history', 'list', 'other']);
});

test('the names are the engine\'s marks, spelled once', () => {
  assert.equal(entryDocument(ID), `/items/${ID}$`);
  assert.equal(ENTRY_LISTS, '/items:query');
  assert.equal(matchesPath(ANY_ENTRY_DOCUMENT, '/items:query'), false, 'the lists are not a document');
});
