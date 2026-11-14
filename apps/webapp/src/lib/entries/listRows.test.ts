// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A collection's rows (N12, N14): the status chip stands in a column of its own under its head, so
// every title starts at one place; the meta line under a title is one line from a tablet's width,
// so a row is two lines at most - also with the view panel open beside the list.

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';

const source = readFileSync(new URL('./EntryList.svelte', import.meta.url), 'utf8');

test('the status chip stands in a column of its own, one width in every row, under its head (N12)', () => {
  assert.match(source, /\{#if rowChip\}<span class="lead-state"><StateChip /u);
  assert.match(source, /\{#if hasStateColumn\}<span class="col-head state-head">\{t\('app\.list\.col_status'\)\}<\/span>\{\/if\}/u);
  assert.match(source, /\.state-head,\s*\.lead-state \{ flex: none; display: inline-flex; min-inline-size: calc\(var\(--sp-1000\) \* 1\.25\); \}/u);
});

test('the meta line is one line from a tablet\'s width, so a row is two lines at most (N12, N14)', () => {
  assert.match(source, /@media \(width >= 600px\) \{\s*\.meta-line \{ flex-wrap: nowrap; overflow: hidden; \}/u);
});
