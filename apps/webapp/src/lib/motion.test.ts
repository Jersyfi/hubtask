// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { applyMotion, MOTION_KEY, readMotionChoice, storeMotionChoice } from './motion.ts';

function fakeRoot() {
  const attributes: Record<string, string> = {};
  return {
    attributes,
    setAttribute(name: string, value: string) {
      attributes[name] = value;
    },
    removeAttribute(name: string) {
      delete attributes[name];
    },
  };
}

function fakeStore(initial: Record<string, string> = {}) {
  const kept = { ...initial };
  return {
    kept,
    getItem: (key: string) => kept[key] ?? null,
    setItem: (key: string, value: string) => {
      kept[key] = value;
    },
    removeItem: (key: string) => {
      delete kept[key];
    },
  };
}

test('reduced sets data-motion, and system removes it so the media query decides', () => {
  const root = fakeRoot();
  applyMotion(root, 'reduced');
  assert.equal(root.attributes['data-motion'], 'reduced');
  applyMotion(root, 'system');
  assert.equal('data-motion' in root.attributes, false);
});

test('the choice is kept on the device, and only reduced is a choice', () => {
  const store = fakeStore();
  assert.equal(readMotionChoice(store), 'system');
  storeMotionChoice(store, 'reduced');
  assert.equal(store.kept[MOTION_KEY], 'reduced');
  assert.equal(readMotionChoice(store), 'reduced');
  storeMotionChoice(store, 'system');
  assert.equal(MOTION_KEY in store.kept, false);
  // There is no "full": a device that asked for less keeps less whatever was written here.
  assert.equal(readMotionChoice(fakeStore({ [MOTION_KEY]: 'full' })), 'system');
  assert.equal(readMotionChoice(undefined), 'system');
});
