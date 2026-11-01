// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A second press after a lost answer must be the same intent, or the desk holds two tickets.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { Intents } from './intent.ts';

function intents(now: () => number = () => 0): Intents {
  let n = 0;
  return new Intents(() => `key-${++n}`, now);
}

const DRAFT = { collection_id: 'c1', entry_type: 'INCIDENT', title: 'Printer down' };

test('a retry after a timeout carries the key of the first press', () => {
  const keys = intents();
  const first = keys.keyFor('POST', '/requests', DRAFT);
  keys.settle('POST', '/requests', DRAFT, new Error('timed out'));
  assert.equal(keys.keyFor('POST', '/requests', { title: 'Printer down', entry_type: 'INCIDENT', collection_id: 'c1' }), first);
});

test('a retry after a 5xx carries the same key', () => {
  const keys = intents();
  const first = keys.keyFor('POST', '/time-records', { minutes: 30 });
  keys.settle('POST', '/time-records', { minutes: 30 }, { status: 503 });
  assert.equal(keys.keyFor('POST', '/time-records', { minutes: 30 }), first);
});

test('after a success the same content is a new intent', () => {
  const keys = intents();
  const first = keys.keyFor('POST', '/time-records', { minutes: 30 });
  keys.settle('POST', '/time-records', { minutes: 30 });
  assert.notEqual(keys.keyFor('POST', '/time-records', { minutes: 30 }), first);
});

test('an answered refusal releases the key, so the stored refusal is not replayed', () => {
  const keys = intents();
  const first = keys.keyFor('POST', '/requests', DRAFT);
  keys.settle('POST', '/requests', DRAFT, { status: 422 });
  assert.notEqual(keys.keyFor('POST', '/requests', DRAFT), first);
});

test('changed content is a new intent', () => {
  const keys = intents();
  const first = keys.keyFor('POST', '/requests', DRAFT);
  assert.notEqual(keys.keyFor('POST', '/requests', { ...DRAFT, title: 'Printer on fire' }), first);
  assert.notEqual(keys.keyFor('POST', '/other', DRAFT), first);
});

test('an undecided key is not kept past the server\'s replay window', () => {
  let at = 0;
  const keys = intents(() => at);
  const first = keys.keyFor('POST', '/requests', DRAFT);
  at = 24 * 60 * 60 * 1000;
  assert.notEqual(keys.keyFor('POST', '/requests', DRAFT), first);
});
