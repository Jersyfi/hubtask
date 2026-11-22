// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// MoveCheck (design-system.md §4, wave 5): its framed look, read from its source.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const sourceDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src');
const source = fs.readFileSync(path.join(sourceDir, 'MoveCheck.svelte'), 'utf8');
const markup = source.slice(source.lastIndexOf('</script>'), source.indexOf('<style>'));
const style = source.slice(source.indexOf('<style>'));

test('framed, the lines are one bordered group with a hairline between them', () => {
  assert.match(markup, /data-framed=\{isFramed \? '' : undefined\}/);
  assert.match(style, /\[data-framed\] \{\s*border: var\(--bw-hairline\) solid var\(--border-subtle\);\s*border-radius: var\(--r-md\);/);
  assert.match(style, /\[data-framed\] \.check \+ \.check \{ border-block-start: var\(--bw-hairline\) solid var\(--border-subtle\); \}/);
});

test('framed, each text is a heading over its quieter detail', () => {
  assert.match(markup, /\{#if item\.tone === 'obstacle' \|\| isFramed\}<strong>\{item\.text\}<\/strong>/);
  assert.match(markup, /<small class="detail">\{item\.detail\}<\/small>/);
  assert.match(style, /\.detail \{ display: block; color: var\(--text-subtle\); font-size: var\(--fs-075\); \}/);
});

test('every mark is named by its tone, whatever glyph a line brings', () => {
  assert.match(markup, /<Icon name=\{item\.icon \?\? glyphs\[item\.tone\]\} size="sm" label=\{toneLabels\[item\.tone\]\} \/>/);
});

test('a way out that is a place to go is a link, not a button', () => {
  assert.match(markup, /\{#if item\.way && 'href' in item\.way\}\s*<a class="way" href=\{item\.way\.href\}>/);
  assert.match(markup, /<button type="button" class="way" onclick=\{item\.way\.onselect\}>/);
});
