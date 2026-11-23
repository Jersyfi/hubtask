// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A filled CreatePage on a phone, a Dialog that is a step of it and a Disclosure that is a row of
// its form (design-system.md §4), checked
// without a browser as a reading of their source - the bargain `conventions.test.js` makes. The
// walk that draws them is the webapp's `createhub` suite.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const sourceDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src');
const read = (file) => fs.readFileSync(path.join(sourceDir, file), 'utf8');
const parts = (source) => ({
  markup: source.slice(source.lastIndexOf('</script>'), source.indexOf('<style>')),
  style: source.slice(source.indexOf('<style>')),
});

test('a filled create page draws its primary once more in a bar at the foot, only below medium', () => {
  const { markup, style } = parts(read('CreatePage.svelte'));
  assert.match(markup, /\{#if isFilled && head\.primary\?\.onclick && !refused\}/, 'the foot is a filled page’s, and not over a refusal');
  assert.match(markup, /<div class="thumb"[^\n]*\n\s*<Button tone="primary" isFull onclick=\{head\.primary\.onclick\} disabledReason=\{head\.primary\.disabledReason\}/,
    'the foot holds the head’s primary across the width, with its reason');
  assert.match(style, /\.thumb \{ display: none; \}/, 'the foot is not drawn from medium up');
  const phone = style.slice(style.indexOf('@media (width < 600px)'));
  assert.match(phone, /\[data-page-kind\] > header \.actions\) \{ display: none; \}/, 'the head’s actions give way to ✕ and the foot');
  assert.match(phone, /\.thumb \{[^}]*position: sticky;[^}]*inset-block-end: 0;/, 'the foot stays at the bottom of the screen');
  assert.match(phone, /env\(safe-area-inset-bottom, 0\)/, 'the foot stands above the safe area');
  assert.match(phone, /env\(safe-area-inset-top, 0\)/, 'the bar stands under the safe area');
});

test('the foot of a filled create page rides above the on-screen keyboard', () => {
  const source = read('CreatePage.svelte');
  assert.match(source, /window\.innerHeight - seen\.height - seen\.offsetTop/, 'the keyboard is measured from the visual viewport');
  assert.match(source, /style:inset-block-end=\{keyboardInset > 0 \? `\$\{keyboardInset\}px` : undefined\}/, 'the foot stands on it');
});
