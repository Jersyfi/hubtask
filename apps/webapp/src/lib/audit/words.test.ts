// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { actionWords, type Catalogue } from './words.ts';

const ENTRIES: Record<string, string> = {
  'app.audit.what': '{thing} {verb}',
  'app.audit.what_other': 'Something done to: {thing}',
  'app.audit.thing_other': 'Something',
  'app.audit.verb.read': 'read',
  'app.audit.verb.created': 'created',
  'app.audit.verb.signed_in': 'signed in',
  'app.audit.thing.approval': 'Approval',
  'app.audit.thing.access_token': 'Access token',
  'app.audit.thing.auth': 'Person',
};
const words: Catalogue = {
  t: (code, params) => Object.entries(params ?? {}).reduce((text, [key, value]) => text.replace(`{${key}}`, value), ENTRIES[code] ?? code),
  has: (code) => code in ENTRIES,
};

test('an action reads as its thing and its verb, never as its code', () => {
  assert.equal(actionWords('approval.read', 'approval', words), 'Approval read');
  assert.equal(actionWords('access.token_created', 'access_token', words), 'Access token created');
});

test('a two-word verb is read whole, and a missing target falls back to the code\'s first part', () => {
  assert.equal(actionWords('auth.passkey_signed_in', null, words), 'Person signed in');
});

test('an unknown verb or thing still reads as words', () => {
  assert.equal(actionWords('approval.frobnicated', 'approval', words), 'Something done to: Approval');
  assert.equal(actionWords('mystery.read', 'mystery', words), 'Something read');
});
