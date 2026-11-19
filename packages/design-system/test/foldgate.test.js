// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// When an empty composer folds (design-system.md §4, Composer). It folds once the focus has left
// it, but never between the press and the release of the pointer that took the focus away: the
// fold moves what stands below it, and the release would land where the pressed control no longer
// is, so the press is lost.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { FoldGate } from '../src/foldgate.ts';

test('closed until the focus comes in, open while it stays', () => {
  const gate = new FoldGate();
  assert.equal(gate.isOpen, false);
  gate.focusIn();
  assert.equal(gate.isOpen, true);
});

test('the keyboard taking the focus away folds it at once', () => {
  const gate = new FoldGate();
  gate.focusIn();
  gate.focusOut();
  assert.equal(gate.isOpen, false);
});

test('a press elsewhere keeps it open until that press has clicked, then folds it', () => {
  const gate = new FoldGate();
  gate.focusIn();
  gate.press();
  gate.focusOut();
  assert.equal(gate.isOpen, true, 'no layout jump between the blur and the click it belongs to');
  gate.settle();
  assert.equal(gate.isOpen, false);
});

test('a press that brings the focus back keeps it open after the click', () => {
  const gate = new FoldGate();
  gate.focusIn();
  gate.press();
  gate.focusOut();
  gate.focusIn();
  gate.settle();
  assert.equal(gate.isOpen, true);
});

test('a second press while it still waits does not fold it under that press', () => {
  // A drag that ended without a click leaves it waiting; the next press must not move the page.
  const gate = new FoldGate();
  gate.focusIn();
  gate.press();
  gate.focusOut();
  gate.press();
  assert.equal(gate.isOpen, true);
  gate.settle();
  assert.equal(gate.isOpen, false);
});

test('a press inside that keeps the focus changes nothing', () => {
  const gate = new FoldGate();
  gate.focusIn();
  gate.press();
  gate.settle();
  assert.equal(gate.isOpen, true);
});
