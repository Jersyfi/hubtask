// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { caretLabelPlace, isNearOwnCaret } from '../src/carets.ts';

test("a co-writer's label stands where the space is free: above, else below, else tucked away", () => {
  assert.equal(caretLabelPlace(14, 0, 12), 'above');
  assert.equal(caretLabelPlace(4, 14, 12), 'below', 'a line with words above flips the label below');
  assert.equal(caretLabelPlace(4, 4, 12), 'tucked', 'a line in the middle of a paragraph shows the caret alone');
});

test("a co-writer's label fades near the reader's own caret", () => {
  const caret = { top: 100, bottom: 120, left: 200, right: 202 };
  assert.equal(isNearOwnCaret(caret, 'above', 60, 20, { top: 80, bottom: 98, left: 230, right: 231 }), true, 'the reader writes in the line above, under the label');
  assert.equal(isNearOwnCaret(caret, 'above', 60, 20, { top: 140, bottom: 160, left: 230, right: 231 }), false, 'two lines away');
  assert.equal(isNearOwnCaret(caret, 'below', 60, 20, { top: 122, bottom: 140, left: 210, right: 211 }), true);
  assert.equal(isNearOwnCaret(caret, 'above', 60, 20, { top: 80, bottom: 98, left: 600, right: 601 }), false, 'far along the line');
  assert.equal(isNearOwnCaret(caret, 'tucked', 60, 20, { top: 100, bottom: 120, left: 200, right: 201 }), false, 'a tucked label cannot cover');
  assert.equal(isNearOwnCaret(caret, 'above', 60, 20, undefined), false, 'no caret of the reader');
});
