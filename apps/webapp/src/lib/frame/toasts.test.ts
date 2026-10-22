// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { ToastModel, isOffering, millisecondsOf, undoRefusal } from './toasts.ts';

// The dwells are tokens in the product; any numbers do here, because the rules are relative.
const PLAIN = 100;
const OFFERING = 300;
const model = () => new ToastModel((offering) => (offering ? OFFERING : PLAIN));
const texts = (stack: ToastModel) => stack.entries.map((entry) => entry.text);

test('a plain confirmation replaces the plain one before it', () => {
  const stack = model();
  stack.show({ text: 'Saved' }, 0);
  stack.show({ text: 'Saved again' }, 10);
  assert.deepEqual(texts(stack), ['Saved again']);
});

test('a message that offers Undo stays when a later one arrives', () => {
  // "Label deleted · Undo", then "Saved": a second message must not take the first one's way back.
  const stack = model();
  stack.show({ text: 'Label deleted', undo: () => {} }, 0);
  stack.show({ text: 'Saved' }, 10);
  stack.show({ text: 'Archived', action: { label: 'Open', run: () => {} } }, 20);
  stack.show({ text: 'Saved again' }, 30);
  assert.deepEqual(texts(stack), ['Label deleted', 'Archived', 'Saved again']);
});

test('a message runs out after its dwell, and not before', () => {
  const stack = model();
  stack.show({ text: 'Saved' }, 0);
  assert.equal(stack.expire(PLAIN - 1), false);
  assert.equal(stack.expire(PLAIN), true);
  assert.deepEqual(texts(stack), []);
});

test('a message that offers something stays the longer dwell', () => {
  // `--dur-dwell-offering` is at least eight seconds: time to read, decide and reach Undo.
  const stack = model();
  stack.show({ text: 'Label deleted', undo: () => {} }, 0);
  assert.equal(stack.nextExpiry(), OFFERING);
  assert.equal(stack.expire(PLAIN), false, 'an Undo left at the plain dwell');
  assert.equal(stack.expire(OFFERING), true);
});

test('a held message does not run out, and starts its time again when released', () => {
  // WCAG 2.2.1: nobody is timed out of a message they are reading or about to press.
  const stack = model();
  const id = stack.show({ text: 'Moved to the trash', undo: () => {} }, 0);
  stack.hold(id);
  assert.equal(stack.expire(10 * OFFERING), false);
  assert.equal(stack.nextExpiry(), undefined);
  stack.release(id, 1000);
  assert.equal(stack.nextExpiry(), 1000 + OFFERING);
  assert.equal(stack.expire(1000 + OFFERING - 1), false);
  assert.equal(stack.expire(1000 + OFFERING), true);
});

test('without a readable dwell a message stays until it is dismissed', () => {
  // A time made up here would be a duration outside tokens.json (rule 15).
  const stack = new ToastModel(() => undefined);
  const id = stack.show({ text: 'Saved' }, 0);
  assert.equal(stack.expire(Number.MAX_SAFE_INTEGER), false);
  stack.dismiss(id);
  assert.deepEqual(texts(stack), []);
});

test('the next expiry is the earliest of those not held', () => {
  const stack = model();
  const first = stack.show({ text: 'A', undo: () => {} }, 0);
  stack.show({ text: 'B', undo: () => {} }, 50);
  assert.equal(stack.nextExpiry(), OFFERING);
  stack.hold(first);
  assert.equal(stack.nextExpiry(), 50 + OFFERING);
});

test('clearing at sign-out leaves nothing, Undo included', () => {
  const stack = model();
  stack.show({ text: 'Moved to the trash', undo: () => {} }, 0);
  stack.clear();
  assert.deepEqual(texts(stack), []);
});

test('offline, Undo is shown with its reason rather than offered or hidden', () => {
  assert.equal(undoRefusal(true), 'app.offline.undo_waits');
  assert.equal(undoRefusal(false), undefined);
});

test('what offers something is what carries Undo or an action, not a link', () => {
  assert.equal(isOffering({ text: 'x' }), false);
  assert.equal(isOffering({ text: 'x', link: { label: 'Open', href: '/items/1' } }), false);
  assert.equal(isOffering({ text: 'x', undo: () => {} }), true);
});

test('a duration property is read in either unit', () => {
  // The value a computed style answers, spelled as the browser spells it: built here rather than
  // written, because it is a parsing fixture and no duration of the product's.
  const unit = (amount: number, suffix: string) => `${amount}${suffix}`;
  assert.equal(millisecondsOf(unit(250, 'ms')), 250);
  assert.equal(millisecondsOf(` ${unit(2, 's')} `), 2000);
  assert.equal(millisecondsOf(''), undefined);
});
