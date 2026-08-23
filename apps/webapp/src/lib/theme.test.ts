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
