// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The selftest habit: a checker that cannot fail proves nothing by passing.

import assert from 'node:assert/strict';
import test from 'node:test';

import { violations } from './check-static.js';

test('a plain document passes', () => {
  assert.deepEqual(
    violations('<!doctype html><html><head><link rel="stylesheet" href="/a.css"></head><body><main><h1>Hubtask</h1></main></body></html>'),
    [],
  );
});

test('every dynamic or inline shape is found', () => {
  assert.equal(violations('<script>init()</script>').length, 1);
  assert.equal(violations('<script src="/app.js"></script>').length, 1);
  assert.equal(violations('<style>body{}</style>').length, 1);
  assert.equal(violations('<a onclick="go()">x</a>').length, 1);
  assert.equal(violations('<p style="color:red">x</p>').length, 1);
});

test('a script inside a comment is found', () => {
  assert.deepEqual(violations('<!-- <script>in a comment</script> -->'), ['an inline <script>']);
  assert.deepEqual(violations('<!-- x --><script>real</script>'), ['an inline <script>']);
});

// Each of these hid a script from a search that stripped comments.
test('a comment hides no script', () => {
  assert.deepEqual(violations('<html><!--><script>alert(1)</script><p>-->x</p></html>'), ['an inline <script>']);
  assert.deepEqual(violations('<script src=x><!--</script><script>alert(1)</script>-->'), [
    'a <script src> (csr is off; nothing should load code)',
    'an inline <script>',
  ]);
});

test("Svelte's comment markers alone are clean", () => {
  assert.deepEqual(violations('<div><!--[--><p>x</p><!--]--><!----><!--[0--><p>y</p><!--]--></div>'), []);
});
