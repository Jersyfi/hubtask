// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { ChangeRecord } from '@hubtask/sync-engine';

import { pathsFor, revokedContainerOf } from './live.ts';

const record = (over: Partial<ChangeRecord>): ChangeRecord =>
  ({ op: 'UPSERT', entity: 'item', entity_id: 'i-1', ...over }) as ChangeRecord;

test('an entry makes its lists, its own document and its history stale', () => {
  const paths = pathsFor(record({ entity_id: 'i-1', container_id: 'c-1' }));
  assert.ok(paths.includes('/items'), 'the prefix every list and the entry itself share');
  assert.ok(paths.includes('/items/i-1/activity'), 'a change anywhere is a step somebody is reading');
  assert.ok(paths.includes('/containers/c-1'));
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
  assert.deepEqual(pathsFor(record({ entity: 'container', entity_id: 'c-1' })), ['/containers', '/items']);
  assert.deepEqual(pathsFor(record({ entity: 'reminder', entity_id: 'i-9' })), ['/items/i-9/reminders', '/items']);
  assert.deepEqual(pathsFor(record({ entity: 'saved_view' })), ['/views']);
  assert.deepEqual(pathsFor(record({ entity: 'template' })), ['/templates']);
  assert.deepEqual(pathsFor(record({ entity: 'account', entity_id: 'a-1' })), ['/accounts/a-1']);
  // A label belongs to a collection and is drawn on the entries in it.
  assert.deepEqual(pathsFor(record({ entity: 'label', container_id: 'c-1' })), ['/containers/c-1', '/items']);
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
