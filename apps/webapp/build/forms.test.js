// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A file control is the design system's UploadField (user-experience.md §5.10): a native file
// input drawn by a screen shows the browser's own control, whose focus ring and words are not
// ours (WCAG 2.4.7). Read from the source, because a new screen copies its neighbour's shape.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const SRC = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src');

function sources(dir) {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return sources(full);
    return entry.name.endsWith('.svelte') ? [full] : [];
  });
}

test('no screen draws a native file input of its own', () => {
  const found = sources(SRC)
    .filter((file) => /type="file"/.test(fs.readFileSync(file, 'utf8')))
    .map((file) => path.relative(SRC, file));
  assert.deepEqual(found, [], 'use UploadField (variant "button" in a toolbar)');
});
