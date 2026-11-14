// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { healthMenu } from './tone.ts';

// A goal's and a portfolio's status was a lozenge without ⌄ beside an entry's status button. The
// button lists the statuses a check-in gives, the current one ticked, and every other one opens
// the check-in with it - the status set nothing on its own.
test('a goal status button lists the check-in statuses and steps to every other', () => {
  const menu = healthMenu('AT_RISK', ['ON_TRACK', 'AT_RISK', 'OFF_TRACK', 'DONE', 'DROPPED'], (status) => `«${status}»`);
  assert.deepEqual(menu.current, { id: 'AT_RISK', label: '«AT_RISK»', category: 'progress' });
  assert.deepEqual(menu.statuses.map((each) => each.category), ['progress', 'progress', 'progress', 'done', 'done']);
  assert.deepEqual(menu.steps.map((each) => each.to.id), ['ON_TRACK', 'OFF_TRACK', 'DONE', 'DROPPED']);
  // A portfolio without a report yet: no status is ticked, and each of the three may be reported.
  const none = healthMenu('NONE', ['ON_TRACK', 'AT_RISK', 'OFF_TRACK'], (status) => status);
  assert.equal(none.current.category, 'todo');
  assert.equal(none.steps.length, 3);
});
