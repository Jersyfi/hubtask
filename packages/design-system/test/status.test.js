// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// No status in a component is told by colour alone (design-system.md §6 rule 3). The rule and its
// reasons are in build/lint-status.js; the web app runs the same check over its own tree.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { lint, selftest } from '../build/lint-status.js';

const sourceDir = path.join(path.dirname(fileURLToPath(import.meta.url)), '..', 'src');

test('the status gate catches every planted shape before it is trusted', () => {
  assert.equal(selftest(), true);
});

test('no component tells a status by colour alone', () => {
  const found = lint(sourceDir).map((v) => `${path.relative(sourceDir, v.file)}:${v.line} ${v.what}`);
  assert.deepEqual(found, []);
});
