// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { scopeChoiceOf, scopeOfChoice, subscriptionBody } from './webhooks.ts';

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
