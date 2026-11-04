// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A message whose text holds a parameter is never drawn without one: `t('app.trash.empty_body')`
// with no count put a literal "{count}" on the empty trash. Read from the source, because a code
// reused for a second place drops its parameters there.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const MESSAGES = JSON.parse(fs.readFileSync(path.resolve(ROOT, '..', '..', 'locales', 'en.json'), 'utf8'));
// `t('code')` and `t('code', {})` alike: an empty object carries no parameter either.
const BARE = /\bt\('([a-z0-9_.]+)'(?:,\s*\{\s*\})?\)/g;

function* sources(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) yield* sources(full);
    else if (/\.(svelte|ts)$/.test(entry.name) && !entry.name.endsWith('.test.ts')) yield full;
  }
}

test('no message with a parameter is drawn without its parameters', () => {
  const found = [];
  for (const file of sources(path.join(ROOT, 'src'))) {
    for (const match of fs.readFileSync(file, 'utf8').matchAll(BARE)) {
      const text = MESSAGES[match[1]];
      if (typeof text === 'string' && /\{[a-z_]+[},]/i.test(text)) found.push(`${path.relative(ROOT, file)} ${match[1]}`);
    }
  }
  assert.deepEqual(found, [], 'pass the parameters, or give the place its own message');
});
