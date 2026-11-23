// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A row's badge in the navigation (design-system.md §4, NavItem) with a tone, checked without a
// browser as a reading of its source - the bargain `conventions.test.js` makes. The walk that
// draws it is the webapp's `privatehubs` suite.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const source = fs.readFileSync(path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src', 'NavItem.svelte'), 'utf8');

test('a badge may carry the warning tone, on its surface, and keeps its name', () => {
  assert.match(source, /readonly tone\?: 'warning';/);
  assert.match(source, /<span class="badge" data-tone=\{badge\.tone\} title=\{badge\.label\}><Icon name=\{badge\.icon\} label=\{badge\.label\} size="sm" \/><\/span>/,
    'the tone is an attribute beside the name, never instead of it');
  assert.match(source, /\.badge\[data-tone='warning'\] \{ border-radius: var\(--r-sm\); background: var\(--status-warning-surface\); color: var\(--status-warning-accent\); \}/,
    'on its tone’s surface, never colour alone');
});
