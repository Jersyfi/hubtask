// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { timeWays } from './ways.ts';

const word = (code: string) => code;
const noop = () => {};

// Scene emptytime: three ways to record, the billable one only where its template is offered.
test('time starts from the timer, the week and the billable template', () => {
  let opened = '';
  const templates = [
    { key: 'time_simple', label: 'Simple', icon: 'clock' as const, onclick: noop },
    { key: 'time_consulting', label: 'Consulting', icon: 'clock' as const, onclick: () => (opened = 'consulting') },
  ];
  const ways = timeWays(templates, word, { timer: noop, week: noop });
  assert.deepEqual(ways.map((each) => each.key), ['timer', 'week', 'time_consulting']);
  assert.equal(ways[2]?.label, 'app.time.way_billable');
  ways[2]?.onclick();
  assert.equal(opened, 'consulting', 'the billable card opens its template');
  assert.deepEqual(timeWays([], word, { timer: noop, week: noop }).map((each) => each.key), ['timer', 'week']);
});
