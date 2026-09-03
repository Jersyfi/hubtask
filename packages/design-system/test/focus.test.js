// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// F1-06's keyboard acceptance, as arithmetic.
//
// "A menu is fully operable from the keyboard" and "focus returns to the trigger when a dialog
// closes" are the two criteria that would otherwise need a driven browser. What can be checked
// without one is the logic they rest on, which is where the mistakes actually live: an off-by-one
// at the wrap, an arrow that means "next" in English and "previous" in Arabic, and a restore that
// focuses an element which is no longer in the document.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { escapeHandler, focusReturn, rovingIndex, typeAheadIndex } from '../src/focus.ts';
import { LayerRegister } from '../src/layers.ts';

test('the arrows walk the list and wrap at both ends', () => {
  assert.equal(rovingIndex('ArrowDown', 0, 3), 1);
  assert.equal(rovingIndex('ArrowDown', 2, 3), 0);
  assert.equal(rovingIndex('ArrowUp', 0, 3), 2);
  assert.equal(rovingIndex('Home', 2, 3), 0);
  assert.equal(rovingIndex('End', 0, 3), 2);
  assert.equal(rovingIndex('Tab', 0, 3), null, 'Tab belongs to the browser, not to the menu');
});

test('opening a menu and pressing down lands on the first item', () => {
  // -1 is "nothing focused yet". Treating it as 0 would skip the first item, which is the bug this
  // case exists for.
  assert.equal(rovingIndex('ArrowDown', -1, 3), 0);
  assert.equal(rovingIndex('ArrowUp', -1, 3), 2);
});

test('a horizontal list follows the writing direction', () => {
  assert.equal(rovingIndex('ArrowRight', 0, 3, { orientation: 'horizontal' }), 1);
  assert.equal(rovingIndex('ArrowRight', 0, 3, { orientation: 'horizontal', dir: 'rtl' }), 2);
  assert.equal(rovingIndex('ArrowLeft', 0, 3, { orientation: 'horizontal', dir: 'rtl' }), 1);
  assert.equal(rovingIndex('ArrowRight', 0, 3, { orientation: 'vertical' }), null);
});

test('type-ahead starts after the current item, so a repeated letter walks its group', () => {
  const labels = ['Archive', 'Assign', 'Copy', 'Delete'];
  assert.equal(typeAheadIndex('a', -1, labels), 0);
  assert.equal(typeAheadIndex('a', 0, labels), 1);
  assert.equal(typeAheadIndex('a', 1, labels), 0, 'and wraps within the group');
  assert.equal(typeAheadIndex('c', 0, labels), 2);
  assert.equal(typeAheadIndex('z', 0, labels), null);
  assert.equal(typeAheadIndex(' ', 0, labels), null, 'space activates an item, it does not search');
});

/** A keyboard event with only the four things the handler reads. */
function keydown(key) {
  return {
    key,
    defaultPrevented: false,
    preventDefault() {
      this.defaultPrevented = true;
    },
  };
}

test('two open layers see the same key and only one of them closes', () => {
  // The rule layers.ts exists to keep, at the point where the components could break it: a dialog
  // and a popover each install a handler, and both are called for one press.
  const register = new LayerRegister();
  const closed = [];
  register.open('dialog', () => closed.push('dialog'));
  register.open('popover', () => closed.push('popover'));

  const fromDialog = escapeHandler(register);
  const fromPopover = escapeHandler(register);

  const event = keydown('Escape');
  fromDialog(event);
  fromPopover(event);
  assert.deepEqual(closed, ['popover']);
  assert.equal(register.size, 1);

  const second = keydown('Escape');
  fromDialog(second);
  fromPopover(second);
  assert.deepEqual(closed, ['popover', 'dialog']);
});

test('a key that closes nothing is left for the page', () => {
  const register = new LayerRegister();
  const event = keydown('Escape');
  escapeHandler(register)(event);
  assert.equal(event.defaultPrevented, false, 'nothing was open, so nothing was consumed');

  register.open('dialog', () => {});
  const other = keydown('a');
  escapeHandler(register)(other);
  assert.equal(other.defaultPrevented, false);
});

test('focus goes back to the trigger, unless the trigger is gone', () => {
  let focused = 0;
  const trigger = { isConnected: true, focus: () => (focused += 1) };
  assert.equal(focusReturn(trigger), true);
  assert.equal(focused, 1);

  // The menu item that deleted the row it sat in. Focusing a detached element moves focus to the
  // body without saying so, which is why the caller is told instead.
  assert.equal(focusReturn({ isConnected: false, focus: () => (focused += 1) }), false);
  assert.equal(focusReturn(null), false);
  assert.equal(focused, 1);
});
