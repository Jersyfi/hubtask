// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The keyboard rules of design-system.md §4 (Wave 1), as arithmetic.
//
// "A menu is fully operable from the keyboard" and "focus returns to the trigger when a dialog
// closes" are the two criteria that would otherwise need a driven browser. What can be checked
// without one is the logic they rest on, which is where the mistakes actually live: an off-by-one
// at the wrap, an arrow that means "next" in English and "previous" in Arabic, and a restore that
// focuses an element which is no longer in the document.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { clearanceUnder, escapeHandler, focusReturn, keptFocus, rovingIndex, typeAheadIndex } from '../src/focus.ts';
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

// --- the save bar's clearance --------------------------------------------------------------------

test('a field the save bar covers is moved up by what it covers; nothing that is clear or too tall to clear', () => {
  // The bar's top at 850 in a page whose content starts at 49 under the app bar.
  assert.equal(clearanceUnder({ top: 800, bottom: 880 }, 850, 49), 30);
  assert.equal(clearanceUnder({ top: 700, bottom: 760 }, 850, 49), 0);
  // A region taking focus because it was pressed - `main`, taller than the room above the bar -
  // cannot be cleared, and moving it would move what the pointer is pressing.
  assert.equal(clearanceUnder({ top: 49, bottom: 930 }, 850, 49), 0);
});

// Dialog keeps its focus through every change of its content: an alert that grows to the full
// screen hides its foot, a run replaces its answers, a stop draws them again.
test('a dialog\'s focus lost to the body goes to the safe answer, else the heading, and back to the answer', () => {
  const control = (name, isDrawn = true, isDisabled = false) => ({ name, offsetParent: isDrawn ? {} : null, getAttribute: (key) => (key === 'aria-disabled' && isDisabled ? 'true' : null) });
  const heading = control('heading');
  const keep = control('keep');
  const off = control('off');
  const close = control('close');
  const alert = { isAlert: true, heading, isOnFallback: false, isInside: true };
  assert.equal(keptFocus({ ...alert, active: null, foot: [keep, off], dismiss: close }).target, keep, 'lost: the safe answer, never the destructive one');
  assert.equal(keptFocus({ ...alert, active: keep, foot: [keep, off], dismiss: close }), undefined, 'kept where it is');
  // At a phone's width the foot is hidden: the focus on Keep it on is lost, and the close control is the same answer.
  const hidden = control('keep', false);
  assert.equal(keptFocus({ ...alert, active: hidden, foot: [hidden, control('off', false)], dismiss: close }).target, close);
  // The safe answer unavailable: never the destructive primary beside it, but the close control, else the heading.
  const danger = { ...control('off'), getAttribute: (key) => (key === 'data-tone' ? 'danger' : null) };
  const waiting = control('keep', true, true);
  assert.equal(keptFocus({ ...alert, active: null, foot: [waiting, danger], dismiss: close }).target, close);
  assert.equal(keptFocus({ ...alert, active: null, foot: [waiting, danger], dismiss: undefined }).target, heading);
  // A run with no answer and no close: the heading, marked as a fallback.
  const run = keptFocus({ ...alert, active: null, foot: [], dismiss: undefined });
  assert.deepEqual([run.target, run.isFallback], [heading, true]);
  // Stopped, the answers come back and the focus leaves the heading for the safe one.
  assert.equal(keptFocus({ ...alert, active: heading, isOnFallback: true, foot: [keep, off], dismiss: close }).target, keep);
  assert.equal(keptFocus({ ...alert, active: heading, isOnFallback: false, foot: [keep, off], dismiss: close }), undefined, 'a heading the person reached is left alone');
  // Another layer over it - the confirmation a dismissal opens - holds its own focus.
  assert.equal(keptFocus({ ...alert, active: control('other'), isInside: false, foot: [keep], dismiss: close }), undefined);
  // Not an alert: the close control, else the heading - never a button of the foot.
  assert.equal(keptFocus({ ...alert, isAlert: false, active: null, foot: [off], dismiss: close }).target, close);
  assert.equal(keptFocus({ ...alert, isAlert: false, active: null, foot: [off], dismiss: undefined }).target, heading);
});
