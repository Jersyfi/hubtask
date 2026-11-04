// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { eventGroups, eventKey, scopeChoiceOf, scopeOfChoice, subscriptionBody, webhookFigure } from './webhooks.ts';

const HUB = '01936f2a-7c1e-7000-8000-000000000f31';

test('a hub or a collection travels with its level, the workspace as nothing', () => {
  assert.deepEqual(scopeOfChoice(`HUB:${HUB}`), { type: 'HUB', id: HUB });
  assert.deepEqual(scopeOfChoice(`COLLECTION:${HUB}`), { type: 'COLLECTION', id: HUB });
  assert.equal(scopeOfChoice('TENANT'), undefined);
  // A level without its container is not a scope this form can send.
  assert.equal(scopeOfChoice('HUB:'), undefined);
});

test('a stored scope reads back as the choice that made it', () => {
  assert.equal(scopeChoiceOf({ type: 'HUB', id: HUB }), `HUB:${HUB}`);
  assert.equal(scopeChoiceOf({ type: 'TENANT' }), 'TENANT');
  assert.equal(scopeChoiceOf(undefined), 'TENANT');
});

test('a blank condition and the whole workspace are left out of the body', () => {
  const plain = subscriptionBody({
    target_url: ' https://example.org/hooks ',
    event_types: ['de.hubtask.work.item.created.v1'],
    condition: '   ',
    scope: 'TENANT',
  });
  assert.deepEqual(plain, {
    target_url: 'https://example.org/hooks',
    event_types: ['de.hubtask.work.item.created.v1'],
  });

  const narrowed = subscriptionBody({
    target_url: 'https://example.org/hooks',
    event_types: ['de.hubtask.work.item.created.v1'],
    condition: ' data.title.startsWith("Ship") ',
    scope: `HUB:${HUB}`,
  });
  assert.equal(narrowed.filter, 'data.title.startsWith("Ship")');
  assert.deepEqual(narrowed.scope, { type: 'HUB', id: HUB });
});

test('a chat room sends its format and language, and only a Matrix room its room and token', () => {
  const matrix = subscriptionBody({
    target_url: 'https://matrix.example.org',
    event_types: ['de.hubtask.work.item.created.v1'],
    condition: '',
    scope: 'TENANT',
    format: 'MATRIX',
    room: ' !team:example.org ',
    credential: ' syt_token ',
    locale: ' de ',
  });
  assert.deepEqual(matrix, {
    target_url: 'https://matrix.example.org',
    event_types: ['de.hubtask.work.item.created.v1'],
    format: 'MATRIX',
    room: '!team:example.org',
    credential: 'syt_token',
    locale: 'de',
  });

  const slack = subscriptionBody({
    target_url: 'https://chat.example.org/hooks/x',
    event_types: ['de.hubtask.work.item.created.v1'],
    condition: '',
    scope: 'TENANT',
    format: 'SLACK',
    room: '!left:over',
    credential: 'left over',
    locale: '',
  });
  assert.deepEqual(slack, {
    target_url: 'https://chat.example.org/hooks/x',
    event_types: ['de.hubtask.work.item.created.v1'],
    format: 'SLACK',
  });

  const cloudEvent = subscriptionBody({
    target_url: 'https://example.org/hooks',
    event_types: ['de.hubtask.work.item.created.v1'],
    condition: '',
    scope: 'TENANT',
    format: 'CLOUDEVENTS',
    locale: 'de',
  });
  assert.deepEqual(cloudEvent, {
    target_url: 'https://example.org/hooks',
    event_types: ['de.hubtask.work.item.created.v1'],
  });
});

// user-experience.md §5.4, §7.10: an event type is offered by its words, never by its key; the
// types the manifest declares are grouped by what they are about, in the manifest's order.
test('an event type reads by its words, grouped by what it is about', () => {
  assert.equal(eventKey('de.hubtask.work.item.created.v1'), 'work.item.created');
  assert.equal(eventKey('de.hubtask.automation.rule_run.failed.v1'), 'automation.rule_run.failed');
  assert.equal(eventKey('somebody.else.v2'), 'somebody.else.v2', 'a type of another shape is kept whole');
  const groups = eventGroups([
    'de.hubtask.work.container.created.v1', 'de.hubtask.work.item.created.v1', 'de.hubtask.work.comment.created.v1',
    'de.hubtask.work.bucket.created.v1', 'de.hubtask.work.label.created.v1', 'de.hubtask.work.attachment.added.v1',
    'de.hubtask.automation.rule_run.failed.v1', 'de.hubtask.jumble.entry.received.v1', 'de.hubtask.work.recurrence.occurrence_created.v1', 'other.v1',
  ]);
  assert.deepEqual(groups.map((group) => [group.id, group.types.length]), [
    ['entries', 3], ['comments', 1], ['places', 1], ['board', 2], ['automation', 1], ['inbox', 1], ['other', 1],
  ]);
});

test('the head says how many deliver and how many were switched off after failures', () => {
  assert.deepEqual(webhookFigure([{ state: 'ACTIVE' }, { state: 'ACTIVE' }, { state: 'PAUSED' }, { state: 'DISABLED' }]), { active: 2, paused: 1, disabled: 1 });
  assert.deepEqual(webhookFigure([]), { active: 0, paused: 0, disabled: 0 });
});
