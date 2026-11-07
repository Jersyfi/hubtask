// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the first screen downloads, held to a budget. The doors (sign-in, setup, an invitation, a
// reset) draw from the entry and the chunks index.html preloads; the frame, the work's screens and
// the editor (yjs, the block editor) load after sign-in. The catalogues are chunks loaded on demand
// (`chunks.test.js`) and are not counted: they grow with the product, the code around it must not.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import zlib from 'node:zlib';
import { fileURLToPath } from 'node:url';

const DIST = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const built = fs.existsSync(path.join(DIST, 'index.html'));

/** The gzip bytes the first screen may cost. */
const INITIAL_BUDGET = 200_000;

function initialFiles() {
  const html = fs.readFileSync(path.join(DIST, 'index.html'), 'utf8');
  const scripts = [...html.matchAll(/<script[^>]*\ssrc="\/?([^"]+)"/g)].map((m) => m[1]);
  const preloads = [...html.matchAll(/<link[^>]*rel="modulepreload"[^>]*href="\/?([^"]+)"/g)].map((m) => m[1]);
  return [...new Set([...scripts, ...preloads])];
}

test('the first screen stays within its budget', { skip: !built && 'no build in dist/' }, () => {
  const files = initialFiles();
  const bytes = files.reduce((sum, file) => sum + zlib.gzipSync(fs.readFileSync(path.join(DIST, file))).length, 0);
  assert.ok(bytes <= INITIAL_BUDGET, `the first screen costs ${bytes} gzip bytes, over the budget of ${INITIAL_BUDGET}`);
});

test('the editor is not in the first screen', { skip: !built && 'no build in dist/' }, () => {
  for (const file of initialFiles()) {
    const map = path.join(DIST, `${file}.map`);
    if (!fs.existsSync(map)) continue;
    const sources = JSON.parse(fs.readFileSync(map, 'utf8')).sources;
    const editor = sources.filter((source) => /node_modules\/(yjs|prosemirror-|@tiptap\/)/.test(source));
    assert.deepEqual(editor, [], `${file} carries the editor`);
  }
});
