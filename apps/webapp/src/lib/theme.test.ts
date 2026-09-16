// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { applyTheme } from './theme.ts';

function fakeRoot() {
  const attributes: Record<string, string> = {};
  return {
    attributes,
    setAttribute(name: string, value: string) {
      attributes[name] = value;
    },
  };
}

test('the system preference lands on the document as data-theme', () => {
  const root = fakeRoot();
  assert.equal(applyTheme(root, true), 'dark');
  assert.equal(root.attributes['data-theme'], 'dark');
  assert.equal(applyTheme(root, false), 'light');
  assert.equal(root.attributes['data-theme'], 'light');
});

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

test('a choice is kept on the device, and following the system is the absence of one', async () => {
  const { readThemeChoice, storeThemeChoice, THEME_KEY } = await import('./theme.ts');
  const store = fakeStore();
  assert.equal(readThemeChoice(store), 'system');
  storeThemeChoice(store, 'dark');
  assert.equal(store.kept[THEME_KEY], 'dark');
  assert.equal(readThemeChoice(store), 'dark');
  storeThemeChoice(store, 'system');
  assert.equal(THEME_KEY in store.kept, false);
  // A value nobody wrote - an older build, a hand edit - reads as no choice rather than as a mode.
  assert.equal(readThemeChoice(fakeStore({ [THEME_KEY]: 'sepia' })), 'system');
  // No store at all is no choice, not a throw.
  assert.equal(readThemeChoice(undefined), 'system');
});
