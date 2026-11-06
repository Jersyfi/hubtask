// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The look the shell's loopback page is handed: this page's tokens as resolved, and nothing that
// could end the style element.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { loopbackStyle } from './loopbackPage.ts';

test('the card reads the resolved tokens, and a value that could leave the stylesheet is dropped', () => {
  const resolved: Record<string, string> = {
    '--bg-surface': ' white ',
    '--text-primary': 'black</style><script>',
    '--r-lg': 'large}body{',
  };
  const style = loopbackStyle((property) => resolved[property] ?? '');
  assert.match(style, /^:root\{--bg-surface:white\}/);
  assert.doesNotMatch(style, /<|script/);
  assert.doesNotMatch(style, /--r-lg:/);
  assert.match(style, /main\{/);
});
