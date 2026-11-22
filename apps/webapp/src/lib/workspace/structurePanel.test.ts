// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The folded rows of *Types & forms* stand where the section's other rows stand. The section pulls
// every `.setting` under its fold out to the region's edges (SettingsSection), and a `SettingRow` is
// one; a `.setting` wrapped around rows is pulled out a second time and shifts them. Read from the
// source: a pixel offset needs a browser, the wrapper that causes it does not.

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';

const source = readFileSync(new URL('./StructurePanel.svelte', import.meta.url), 'utf8');

test('no element of class setting wraps the folded rows of the levels', () => {
  const start = source.indexOf('{#snippet typesAdvanced()}');
  const end = source.indexOf('\n{/snippet}', start);
  assert.ok(start >= 0 && end > start, 'the snippet of the folded rows is found');
  const snippet = source.slice(start, end);
  assert.match(snippet, /data-setting="fields"/u, 'the fields keep their anchor');
  assert.doesNotMatch(snippet, /class="[^"]*\bsetting\b[^"]*"/u);
});
