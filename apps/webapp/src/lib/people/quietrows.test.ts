// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { quietRowsInView } from './quietrows.ts';

const at = (minute: number) => `2026-11-13T10:${String(minute).padStart(2, '0')}:00Z`;

test('All keeps only what happened since the oldest post shown', () => {
  const events = [{ at: at(1) }, { at: at(5) }, { at: at(9) }];
  assert.deepEqual(quietRowsInView(events, [{ at: at(5) }, { at: at(8) }]), [{ at: at(5) }, { at: at(9) }]);
});

test('where nothing was said yet, All draws the newest three events', () => {
  const events = [1, 2, 3, 4, 5].map((minute) => ({ at: at(minute) }));
  assert.deepEqual(quietRowsInView(events, []).map((event) => event.at), [at(5), at(4), at(3)]);
});
