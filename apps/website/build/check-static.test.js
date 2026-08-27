// SPDX-License-Identifier: BUSL-1.1
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

test('a comment does not hide a finding and does not create one', () => {
  assert.equal(violations('<!-- <script>ok in a comment</script> -->').length, 0);
  assert.equal(violations('<!-- x --><script>real</script>').length, 1);
});
