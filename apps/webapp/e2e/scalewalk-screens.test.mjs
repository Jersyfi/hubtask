// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { gzipSync } from 'node:zlib';
import { screenOf, tally } from './scalewalk-screens.mjs';

// S7 (test/load/PLAN.md): per screen the script and style bytes, raw and as gzip, and nothing
// else the screen fetched.
test('a screen counts its scripts and styles, raw and compressed', () => {
  const script = Buffer.from('export const a = 1;'.repeat(50));
  const style = Buffer.from('.a{color:var(--x)}'.repeat(20));
  const got = tally([
    { kind: 'script', body: script },
    { kind: 'stylesheet', body: style },
    { kind: 'image', body: Buffer.from('not counted') },
  ]);
  assert.deepEqual(got, {
    script_bytes: script.length,
    script_gzip_bytes: gzipSync(script).length,
    style_bytes: style.length,
    style_gzip_bytes: gzipSync(style).length,
    files: 2,
  });
  assert.ok(got.script_gzip_bytes < got.script_bytes);
  assert.deepEqual(tally([]), { script_bytes: 0, script_gzip_bytes: 0, style_bytes: 0, style_gzip_bytes: 0, files: 0 });
});

test('the screens of different workspaces line up by their masked path', () => {
  assert.equal(screenOf('/hubs/01936f2a-7c1e-7000-8000-000000000001'), '/hubs/?');
  assert.equal(screenOf('/plans'), '/plans');
});
