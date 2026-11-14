// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The work surface's coherence (concept §4, §7): one control looks and behaves the same in every
// product. Read from the source and the built tokens, as `worksurface.test.js` does.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const sourceDir = path.join(root, 'src');
const source = (name) => fs.readFileSync(path.join(sourceDir, name), 'utf8');
const tokens = JSON.parse(fs.readFileSync(path.join(root, 'tokens', 'tokens.json'), 'utf8'));

/** The style rules of a component whose selector names the class. */
function rulesFor(style, cls) {
  const found = [];
  const pattern = new RegExp(`([^{}]*\\.${cls}\\b[^{}]*)\\{([^}]*)\\}`, 'g');
  for (const match of style.matchAll(pattern)) found.push(match[2]);
  return found;
}

test('under a thumb a small control reaches the target, as the medium one does (§7)', () => {
  assert.equal(tokens.density.spacious.control.sm.min.$value, '{layout.target}');
});

test('every native button the design system draws with words takes the page font, never the browser’s', () => {
  // Elements without a word of their own (a twist, a grabber, a knob) cannot show a font.
  const wordless = new Set(['twist', 'grabber', 'knob']);
  const missing = [];
  for (const file of fs.readdirSync(sourceDir).filter((name) => name.endsWith('.svelte') && !name.startsWith('_'))) {
    const text = source(file);
    const style = text.slice(text.indexOf('<style'));
    for (const match of text.matchAll(/<button\b[^>]*?class="([^"{]+)"/g)) {
      const classes = match[1].split(/\s+/).filter(Boolean);
      if (classes.some((cls) => wordless.has(cls))) continue;
      const inherits = classes.some((cls) => rulesFor(style, cls).some((rule) => /font:|font-family/.test(rule)));
      if (!inherits) missing.push(`${file} .${classes.join('.')}`);
    }
  }
  // The navigation's rows are their own concept's and set their font in the frame.
  const outside = missing.filter((name) => !/^(Nav|SideNav)/.test(name));
  assert.deepEqual(outside, []);
});

test('a menu given a close name opens as a bottom sheet on a phone’s width, and its rows reach the target', () => {
  const menu = source('Menu.svelte');
  assert.match(menu, /isSheet = closeLabel !== undefined && isCompactWidth\(\)/);
  assert.match(menu, /<Sheet[\s\S]*?\{items\}/);
  assert.match(menu, /\.item \{[^}]*min-block-size: var\(--density-control-sm-min\)/);
});
