// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A collection's list when it shows nothing (user-experience §4, WB-6): empty because of what is
// seen says so and leads back to everything; truly empty invites the first entry, a template the
// side way. Read from the source, as the ladder test does.

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';

const source = readFileSync(new URL('./EntryList.svelte', import.meta.url), 'utf8');

test('a list empty by its filter says so and offers everything again, not the empty collection\'s invitation', () => {
  assert.match(source, /<EmptyState kind="filtered" title=\{t\('app\.entries\.none_matching'\)\}/u);
  assert.match(source, /onclick=\{onshowall\}>\{t\('app\.entries\.show_all'\)\}/u);
  assert.ok(source.indexOf('kind="filtered"') < source.indexOf('kind="unused" title={t(root'), 'the filtered case is not decided first');
});

test('an empty collection offers its first entry, and a template as the side way (WB-6)', () => {
  assert.match(source, /<Button tone="secondary" icon="copy" onclick=\{ontemplate\}>\{t\('app\.templates\.from'\)\}<\/Button>/u);
});
