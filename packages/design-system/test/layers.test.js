// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The ordering rule of design-system.md §9, tested before the five components that need it exist.
//
// That is the point of writing the register first. "`Escape` closes one layer at a time with two
// open" is F1-06's acceptance criterion, and by the time `Dialog` and `Popover` are both written
// it is a criterion that can only be checked by opening two of them and pressing a key. Here it is
// four assertions.
//
// The module is plain TypeScript with erasable syntax only, which Node runs directly.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { LayerRegister, DISMISSIBLE_LAYERS, handleEscape } from '../src/layers.ts';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const tokens = JSON.parse(fs.readFileSync(path.join(packageRoot, 'tokens', 'tokens.json'), 'utf8'));

test('the layer scale is strictly ascending, and the register knows the same order', () => {
  const scale = Object.entries(tokens.primitive.layer)
    .filter(([key]) => !key.startsWith('$'))
    .map(([key, token]) => [key, token.$value]);

  const values = scale.map(([, value]) => value);
  assert.deepEqual(
    values,
    [...values].sort((a, b) => a - b),
    'the layer steps are not in ascending order - the paint order is the order they are declared in',
  );
  assert.equal(new Set(values).size, values.length, 'two layers share a z-index');

  // Every layer Escape can reach is a layer that exists in the scale. The reverse does not hold:
  // `tooltip` and `toast` paint but are not dismissed by a key.
  const names = scale.map(([key]) => key);
  for (const layer of DISMISSIBLE_LAYERS) {
    assert.ok(names.includes(layer), `the register knows a layer '${layer}' that tokens.json does not`);
  }
});

test('nothing open dismisses nothing', () => {
  const register = new LayerRegister();
  assert.equal(register.top(), null);
  assert.equal(register.dismissTop(), false);
  assert.equal(handleEscape('Escape', register), false);
});

test('Escape closes one layer at a time with two open', () => {
  const register = new LayerRegister();
  const closed = [];
  register.open('dialog', () => closed.push('dialog'));
  register.open('popover', () => closed.push('popover'));

  assert.equal(handleEscape('Escape', register), true);
  assert.deepEqual(closed, ['popover'], 'the popover inside the dialog goes first');
  assert.equal(register.size, 1, 'the dialog is still open');

  assert.equal(handleEscape('Escape', register), true);
  assert.deepEqual(closed, ['popover', 'dialog']);
  assert.equal(register.size, 0);
});

test('rank beats the order of opening', () => {
  // The failure this guards: a drawer is open, a dialog opens over it, Escape closes the drawer
  // because it registered second. Rank decides first, so the dialog goes.
  const register = new LayerRegister();
  const closed = [];
  register.open('dialog', () => closed.push('dialog'));
  register.open('overlay', () => closed.push('overlay'));

  register.dismissTop();
  assert.deepEqual(closed, ['dialog']);
});

test('among equals the last opened goes first', () => {
  const register = new LayerRegister();
  const closed = [];
  register.open('popover', () => closed.push('first'));
  register.open('popover', () => closed.push('second'));

  register.dismissTop();
  assert.deepEqual(closed, ['second']);
});

test('a released layer is no longer reachable, and releasing twice is not an error', () => {
  const register = new LayerRegister();
  const closed = [];
  register.open('dialog', () => closed.push('dialog'));
  const popover = register.open('popover', () => closed.push('popover'));

  popover.release();
  popover.release();

  assert.equal(register.size, 1);
  register.dismissTop();
  assert.deepEqual(closed, ['dialog'], 'a layer closed by its own trigger must not be dismissed again');
});

test('a key that is not Escape reaches nothing', () => {
  const register = new LayerRegister();
  const closed = [];
  register.open('dialog', () => closed.push('dialog'));

  assert.equal(handleEscape('Enter', register), false);
  assert.equal(handleEscape('Esc', register), false, 'the IE spelling is not the key name');
  assert.deepEqual(closed, []);
});

test('a dismiss that opens something else survives its own release', () => {
  // The race the entry is removed *before* the callback for: a confirm dialog that closes and
  // immediately opens a second one must not have the second one dropped.
  const register = new LayerRegister();
  register.open('dialog', () => register.open('dialog', () => {}));

  register.dismissTop();
  assert.equal(register.size, 1);
});
