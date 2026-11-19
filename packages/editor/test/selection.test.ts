// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { readSelectionFirst } from '../src/selection.ts';

// A key that collapses a selection (the line's end) and Enter right after it: the browser tells the
// editor of the new selection a moment later, and Enter acted on the old one - the selected words
// were replaced by the new line. A key reads the browser's selection before it acts.
const viewThat = () => {
  const view = { flushed: 0, composing: false, domObserver: { flush: () => { view.flushed += 1; } } };
  return view;
};
const press = (view: ReturnType<typeof viewThat>, key: string) =>
  readSelectionFirst.props.handleKeyDown?.call(readSelectionFirst, view as never, { key, isComposing: false } as KeyboardEvent);

test('a key that writes reads the selection the browser holds before it acts', () => {
  const view = viewThat();
  assert.equal(press(view, 'Enter'), false, 'the key is still the keymap\'s');
  assert.equal(view.flushed, 1);
  press(view, 'Backspace');
  press(view, 'a');
  assert.equal(view.flushed, 3);
});

test('a modifier alone reads nothing', () => {
  const view = viewThat();
  for (const key of ['Shift', 'Meta', 'Control', 'Alt']) press(view, key);
  assert.equal(view.flushed, 0);
});
