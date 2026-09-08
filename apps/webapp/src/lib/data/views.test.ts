// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  exportFileName,
  feedStateOf,
  isTruncated,
  needsStructure,
  queryDocumentOf,
  queryOf,
  sharingRefusal,
} from './views.ts';

test('the two refusals are the server’s own codes', () => {
  // One fact, one sentence, whether this client saw it coming or the server sent it.
  assert.equal(sharingRefusal('PRIVATE', 'COLLECTION'), undefined);
  assert.equal(sharingRefusal('SCOPE', 'COLLECTION'), undefined);
  assert.equal(sharingRefusal('PUBLIC_LINK', 'COLLECTION'), 'views.public_link_not_available');
  // A personal view names no audience, so there is nobody to share it with.
  assert.equal(sharingRefusal('SCOPE', 'ACCOUNT'), 'views.account_scope_not_shareable');
  // …and `PUBLIC_LINK` is refused wherever it is asked, which is why that test comes first.
  assert.equal(sharingRefusal('PUBLIC_LINK', 'ACCOUNT'), 'views.public_link_not_available');
});

test('saving privately asks no permission, and saving anywhere else does', () => {
  assert.equal(needsStructure('ACCOUNT'), false);
  assert.equal(needsStructure('COLLECTION'), true);
  assert.equal(needsStructure('HUB'), true);
  assert.equal(needsStructure('TENANT'), true);
});

test('a view saves the question and not the place it was asked', () => {
  // A view saved on one collection and opened on another asks the same thing of different entries,
  // which is what a saved question is for.
  const query = { filter: { op: 'EQ', field: 'is_completed', value: false } as never, sort: [{ field: 'due_at', dir: 'ASC' as const }] };
  const stored = queryDocumentOf(query);
  assert.deepEqual(Object.keys(stored).sort(), ['filter', 'sort']);
  assert.equal('scope' in stored, false);
  assert.deepEqual(queryDocumentOf(undefined), {});
});

test('opening a view asks what it saved, grouping included', () => {
  const stored = { filter: { op: 'EQ', field: 'is_completed', value: false } };
  const back = queryOf(stored, { field: 'bucket_id', limit_per_group: 50 });
  assert.deepEqual(back.filter, stored.filter);
  assert.deepEqual(back.group, { field: 'bucket_id', limit_per_group: 50 });
  // A grouping with no field is no grouping, rather than one the board would try to draw.
  assert.equal(queryOf(stored, {}).group, undefined);
  assert.deepEqual(queryOf(undefined, undefined), {});
});

test('the file is named after the view, and is safe to write down', () => {
  assert.equal(exportFileName('Overdue', 'CSV'), 'Overdue.csv');
  assert.equal(exportFileName('Q3 / planning', 'JSON'), 'Q3-planning.json');
  assert.equal(exportFileName('  ', 'ICS'), 'view.ics');
  assert.equal(exportFileName('a'.repeat(200), 'CSV').length, 84);
});

test('a truncated export says so, and only a plain false is not truncated', () => {
  // A result that reached the cap is answered whole up to it; handing the file over without
  // saying so would hand over something that looks complete.
  assert.equal(isTruncated(new Map([['export-truncated', 'true']])), true);
  assert.equal(isTruncated(new Map([['export-truncated', 'TRUE']])), true);
  assert.equal(isTruncated(new Map([['export-truncated', 'false']])), false);
  assert.equal(isTruncated(new Map()), false);
});

test('a feed that was revoked is revoked, whatever became of its view', () => {
  // Saying "it serves nothing" about a token that already stopped working would answer a question
  // nobody asked.
  assert.equal(feedStateOf({ view_id: 'v-1' }), 'active');
  assert.equal(feedStateOf({ view_id: null }), 'orphaned');
  assert.equal(feedStateOf({ view_id: null, revoked_at: '2026-09-01T00:00:00Z' }), 'revoked');
  assert.equal(feedStateOf({ view_id: 'v-1', revoked_at: '2026-09-01T00:00:00Z' }), 'revoked');
});
