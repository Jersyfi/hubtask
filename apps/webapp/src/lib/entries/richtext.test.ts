// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { catchUpOnBlur } from './richtext.ts';

test('leaving a text unchanged takes the value that arrived while it had the focus', () => {
  assert.equal(catchUpOnBlur('theirs', 'mine before', 'mine before'), 'theirs');
});

test("leaving a text keeps the writer's own change and draws nothing over it", () => {
  assert.equal(catchUpOnBlur('theirs', 'typed', 'before'), undefined);
});

test('leaving a text that already shows the value draws nothing', () => {
  assert.equal(catchUpOnBlur('same', 'same', 'same'), undefined);
});
