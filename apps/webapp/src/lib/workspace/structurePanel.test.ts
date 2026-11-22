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

// UC-WRK-23 check 10: while the draft is behind the settings in force, no path saves it - the
// change a save sends is `toSave`'s, which is none then, and the save bar's action applies the
// reader's change to the current settings instead of saving. The walk sees the bar; this holds
// the wiring every way into `save()` goes through.
test('no Save while the draft is behind: the bar applies, and the saved change is the one toSave allows (UC-WRK-23 check 10)', () => {
  assert.match(source, /const change = \$derived\(draft && structure \? toSave\(draft, structure, chosen\) : undefined\);/u);
  assert.doesNotMatch(source, /\bchangeOf\(/u, 'a change computed past toSave');
  assert.match(source, /saveLabel=\{behind \? t\('app\.structure\.meanwhile\.apply_short'\) : t\('app\.workspace\.save'\)\}/u);
  assert.match(source, /onSave=\{\(\) => \(behind \? applyMine\(\) : void save\(\)\)\}/u);
  assert.match(source, /async function save\(\) \{\n\s+if \(isSaving \|\| !change\) return;/u);
});
