// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { matchesPath, type ChangeRecord } from '@hubtask/sync-engine';

import { pathsFor, revokedContainerOf } from './live.ts';

const record = (over: Partial<ChangeRecord>): ChangeRecord =>
  ({ op: 'UPSERT', entity: 'item', entity_id: 'i-1', ...over }) as ChangeRecord;

test('an entry makes its lists, its own document and its history stale - and nothing under it', () => {
  const paths = pathsFor(record({ entity_id: 'i-1', container_id: 'c-1' }));
  assert.deepEqual(paths, ['/items:query', '/items/i-1$', '/items/i-1/activity']);
  // Issue 877: `/items` covered the thread, the reminders and the series of a retitled entry.
  assert.equal(paths.some((name) => matchesPath(name, '/items/i-1/comments')), false);
  assert.equal(paths.some((name) => matchesPath(name, '/items/i-1?expand=labels')), true, 'the document, as the page reads it');
  // A deletion is a row of the trash as well.
  assert.ok(pathsFor(record({ op: 'DELETE', entity_id: 'i-1' })).includes('/trash'));
});

test('an entry is recognised under both names the server writes', () => {
  // The change log records one as `item` and the purge records one as `work_item`. Guessing would
  // go blind for whichever writer was guessed against.
  assert.deepEqual(pathsFor(record({ entity: 'work_item' })), pathsFor(record({ entity: 'item' })));
});

test('a record this client does not draw makes nothing stale', () => {
  // Empty is a real answer and the common one. Reloading everything "to be safe" would turn one
  // person's edit into a reload of every screen in the workspace.
  assert.deepEqual(pathsFor(record({ entity: 'something_newer' })), []);
  assert.deepEqual(pathsFor(record({ entity: 'outbox_event' })), []);
});

test('each entity names what it is actually drawn on', () => {
  assert.deepEqual(pathsFor(record({ entity: 'container', entity_id: 'c-1' })), ['/containers$', '/containers/c-1$']);
  // The record names the reminder, not its entry: every open entry's reminders, and only those.
  assert.deepEqual(pathsFor(record({ entity: 'reminder', entity_id: 'r-9' })), ['/items/*/reminders']);
  assert.deepEqual(pathsFor(record({ entity: 'comment', entity_id: 'k-1', container_id: 'c-1' })), ['/items/*/comments']);
  assert.deepEqual(pathsFor(record({ entity: 'recurrence_rule', entity_id: 'q-1' })), ['/items/*/recurrence', '/items/*$']);
  assert.deepEqual(pathsFor(record({ entity: 'saved_view' })), ['/views']);
  assert.deepEqual(pathsFor(record({ entity: 'template' })), ['/templates']);
  assert.deepEqual(pathsFor(record({ entity: 'account', entity_id: 'a-1' })), ['/accounts/a-1']);
  // A label belongs to a collection and is drawn, expanded, on the entries in it.
  assert.deepEqual(pathsFor(record({ entity: 'label', container_id: 'c-1' })), ['/containers/c-1/labels', '/items:query', '/items/*$']);
  assert.deepEqual(pathsFor(record({ entity: 'bucket', container_id: 'c-1' })), ['/containers/c-1/buckets', '/items:query']);
});

test('a revocation names the container that was lost', () => {
  // §6 and §9 rule 3 bind a client to delete what it holds for a container it lost. A cache is all
  // this client holds, so "delete" is "drop it" — but it is the same obligation.
  assert.equal(
    revokedContainerOf(record({ op: 'ACCESS_REVOKED', entity: 'container', entity_id: 'c-1' })),
    'c-1',
  );
  // A record about something *inside* a lost container names the container beside it.
  assert.equal(
    revokedContainerOf(record({ op: 'ACCESS_REVOKED', entity: 'item', entity_id: 'i-1', container_id: 'c-2' })),
    'c-2',
  );
  assert.equal(revokedContainerOf(record({ op: 'UPSERT' })), undefined);
  assert.equal(revokedContainerOf(record({ op: 'DELETE' })), undefined);
});
