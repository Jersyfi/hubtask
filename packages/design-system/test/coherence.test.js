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

test('a menu opens as a bottom sheet on a phone’s width, and its rows reach the target', () => {
  const menu = source('Menu.svelte');
  assert.match(menu, /sheetName = isAnchored \? undefined : \(closeLabel \?\? closeName\(\)\)/);
  assert.match(menu, /isSheet = sheetName !== undefined && isCompactWidth\(\)/);
  assert.match(menu, /<Sheet[\s\S]*?\{items\}/);
  assert.match(menu, /\.item \{[^}]*min-block-size: var\(--density-control-sm-min\)/);
});

test('a pressed toggle has one look in every tone: the subtle tint with the accent edge, never the primary fill', () => {
  for (const [file, cls] of [['Button.svelte', 'button'], ['IconButton.svelte', 'icon-button']]) {
    const text = source(file);
    assert.match(text, new RegExp(`\\.${cls}\\[aria-pressed='true'\\] \\{[^}]*background: var\\(--accent-primary-subtle\\)`), file);
    assert.match(text, new RegExp(`\\.${cls}\\[aria-pressed='true'\\] \\{[^}]*border-color: var\\(--accent-primary\\)`), file);
  }
});

test('a popover becomes a bottom sheet on a phone, and can hang from a control another door opened it from', () => {
  const popover = source('Popover.svelte');
  assert.match(popover, /const name = isAnchored \? undefined : closeName\(\);/);
  assert.match(popover, /sheetName = name !== undefined && isCompactWidth\(\) \? name : undefined/);
  assert.match(popover, /<Sheet bind:isOpen title=\{label\} closeLabel=\{sheetName\}/);
  assert.match(popover, /const from = at \?\? anchor;/);
});

test('every overlay’s ✕ has the one name the application gives, and a dialog has one title size on every rung', () => {
  for (const file of ['Dialog.svelte', 'Sheet.svelte']) assert.match(source(file), /<IconButton icon="x" label=\{closeName\(\) \?\? /, file);
  const dialog = source('Dialog.svelte');
  assert.doesNotMatch(dialog, /\.dialog\[data-size\] \.title \{[^}]*font-size/);
});

test('a dialog’s foot ends on its one primary, and its ways out are secondary', () => {
  const dialog = source('Dialog.svelte');
  assert.match(dialog, /\.actions :global\(\.button\[data-tone='primary'\]\),[^{]*\{ order: 1; \}/);
  assert.match(dialog, /\.actions :global\(\.button\[data-tone='subtle'\]\) \{[^}]*background: var\(--bg-surface\)/);
});

test('a head\'s read-only state stands as tall as its status button beside it (H-2)', () => {
  const lozenge = rulesFor(source('StateLozenge.svelte'), 'lozenge').find((rule) => rule.includes('density-control-sm-min')) ?? '';
  assert.match(lozenge, /box-sizing: border-box;/u);
  const button = source('StatusButton.svelte');
  assert.match(button, /min-block-size: var\(--density-control-sm-min\);\n\s+padding: var\(--sp-050\) var\(--sp-150\);/u);
});

test('an empty section is one line, and a work object\'s parts take the entry\'s section head (H-6, H-7, WB-3)', () => {
  const fold = source('FoldingSection.svelte');
  assert.match(fold, /\{#if emptyLabel !== undefined\}<span class="none">\{emptyLabel\}<\/span>\{\/if\}/u);
  assert.match(fold, /\{#if emptyLabel === undefined\}\n\s+<!-- A link/u, 'an empty section draws no body');
  const record = source('RecordPage.svelte');
  assert.match(record, /\{#if objectHead\}\n\s+<!--[^>]*-->\n\s+<FoldingSection id=\{`part-\$\{item\.id\}`\} title=\{item\.title\} count=\{item\.count\} emptyLabel=\{item\.emptyLabel\}>/u);
});

test('a head\'s next step is the Button of its size, no local type of its own (B-4)', () => {
  assert.doesNotMatch(source('ObjectHead.svelte'), /\.step :global\(\.button\) \{[^}]*font-(size|weight)/u);
});

// D-1: a menu on a phone is a sheet; its actions stay menu items, moved by the arrows.
test('a sheet\'s list of actions is a menu of menu items, the arrows moving among them', () => {
  const sheet = source('Sheet.svelte');
  assert.match(sheet, /<ul class="items" role="menu" aria-labelledby=\{titleId\} onkeydown=\{moveAmongItems\}>/);
  assert.match(sheet, /<li role="none">\s*<button\s*type="button"\s*role="menuitem"/);
  for (const key of ['ArrowDown', 'ArrowUp', 'Home', 'End']) assert.match(sheet, new RegExp(`event\\.key === '${key}'`));
});

