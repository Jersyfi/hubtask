// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// F2-15's acceptance, as arithmetic: what the change set carries, what it deliberately does not,
// and who a step is attributed to when the contract gives the client no way to find out a name.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { ActivityEntry } from '@hubtask/sync-engine';

import { actorCodes, changesOf } from './activity.ts';

const step = (extra: Partial<ActivityEntry> = {}): ActivityEntry =>
  ({
    id: 's',
    item_id: 'i',
    code: 'activity.item_updated',
    actor: { type: 'USER', id: 'me' },
    occurred_at: '2026-09-04T15:39:41Z',
    change_set: {},
    ...extra,
  }) as ActivityEntry;

// --- what the change set carries --------------------------------------------------------------

test('a rename shows both titles', () => {
  assert.deepEqual(changesOf({ title: { from: 'Milk', to: 'Oat milk' } }), [
    { field: 'title', from: 'Milk', to: 'Oat milk', isOpaque: false },
  ]);
});

test('a note shows that it changed and not what it says', () => {
  // ADR-0017: no user content goes anywhere it is not needed. The model refused to carry the text,
  // and a client that went looking for it would put content where the model deliberately did not.
  const changes = changesOf({ notes: { changed: true } });
  assert.deepEqual(changes, [{ field: 'notes', from: undefined, to: undefined, isOpaque: true }]);
  assert.ok(!JSON.stringify(changes).includes('text'), 'no note text may appear');
});

test('a side with no value is absent rather than empty', () => {
  // "Each present only where there was a value on that side" — so a due date being set carries a
  // `to` and no `from`, and the sentence beside it says that rather than "from nothing".
  assert.deepEqual(changesOf({ due_at: { to: '2026-09-30T00:00:00Z' } }), [
    { field: 'due_at', from: undefined, to: '2026-09-30T00:00:00Z', isOpaque: false },
  ]);
  assert.deepEqual(changesOf({ due_at: { from: '2026-09-30T00:00:00Z' } }), [
    { field: 'due_at', from: '2026-09-30T00:00:00Z', to: undefined, isOpaque: false },
  ]);
});

test('nothing but from, to and changed is read out of a field', () => {
  // The guard against a client written today leaking a field the server carries more about
  // tomorrow. Anything else in the object is not read, so it cannot reach a screen.
  const changes = changesOf({
    title: { from: 'a', to: 'b', note_text: 'the whole note', secret: 'x' },
  });
  assert.deepEqual(changes, [{ field: 'title', from: 'a', to: 'b', isOpaque: false }]);
  assert.ok(!JSON.stringify(changes).includes('the whole note'));
  assert.ok(!JSON.stringify(changes).includes('secret'));
});

test('a value this client cannot write is left out rather than printed', () => {
  // `[object Object]` in a history is worse than a field with no detail beside it.
  assert.deepEqual(changesOf({ cover: { to: { kind: 'COLOR' } } }), [
    { field: 'cover', from: undefined, to: undefined, isOpaque: false },
  ]);
});

test('an activity’s compact history has nothing to show', () => {
  // The verb, the actor and the time are the whole of the step. There is nothing here that invents
  // a detail for one.
  assert.deepEqual(changesOf({}), []);
  assert.deepEqual(changesOf(undefined), []);
});

test('the order is the change set’s own', () => {
  // Sorting would claim an order the record does not have. The record's is at least the server's.
  assert.deepEqual(
    changesOf({ title: { from: 'a', to: 'b' }, bucket_id: { to: 'x' } }).map((c) => c.field),
    ['title', 'bucket_id'],
  );
});

// --- who did it ---------------------------------------------------------------------------------

test('the reader is named, and nobody else is', () => {
  // "The label is not here: the account is one request away" — and for anyone but the signed-in
  // account that request does not exist. So the feed distinguishes the reader from everybody else
  // and invents no names.
  assert.deepEqual(actorCodes(step(), 'me'), ['app.activity.actor_you']);
  assert.deepEqual(actorCodes(step(), 'somebody-else'), [
    'app.activity.actor_USER',
    'app.activity.actor_someone',
  ]);
});

test('a signed-out reader is nobody in particular', () => {
  assert.deepEqual(actorCodes(step(), undefined), [
    'app.activity.actor_USER',
    'app.activity.actor_someone',
  ]);
});

test('every other kind of actor has its own sentence', () => {
  for (const kind of ['SYSTEM', 'AUTOMATION', 'AI_AGENT', 'SERVICE_ACCOUNT'] as const) {
    assert.deepEqual(actorCodes(step({ actor: { type: kind, id: null } }), 'me'), [
      `app.activity.actor_${kind}`,
      'app.activity.actor_someone',
    ]);
  }
});

test('a kind this client has never heard of falls back rather than rendering a key', () => {
  // The kinds are an enum in the contract and §2's rule is that such sets grow with the
  // installation. The fallback is the one sentence true of every actor.
  const invented = step({ actor: { type: 'ROBOT' as never, id: 'x' } });
  assert.deepEqual(actorCodes(invented, 'me'), [
    'app.activity.actor_ROBOT',
    'app.activity.actor_someone',
  ]);
});
