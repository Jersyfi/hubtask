// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { atLocal, dayOf, timeOf } from './moments.ts';

test('a day and a time typed come back as the same day and time', () => {
  const at = atLocal('2026-11-16', '08:00');
  assert.ok(at);
  assert.equal(dayOf(at), '2026-11-16');
  assert.equal(timeOf(at), '08:00');
});

test('an empty or unreadable field is no moment, never now', () => {
  assert.equal(atLocal('', '08:00'), undefined);
  assert.equal(atLocal('2026-11-16', ''), undefined);
  assert.equal(atLocal('16.11.2026', '08:00'), undefined);
  assert.equal(dayOf('soon'), '');
  assert.equal(timeOf('soon'), '');
});
