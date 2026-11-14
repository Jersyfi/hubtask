// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The work surface's coherence across products (work-surface concept §4, §7): one control, one
// look and one behaviour wherever it stands. Read from the source, so every screen is held whether
// a walk opens it or not.

import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

const srcDir = fileURLToPath(new URL('../', import.meta.url));

function svelteFiles(dir = srcDir): string[] {
  const found: string[] = [];
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) found.push(...svelteFiles(path));
    else if (name.endsWith('.svelte')) found.push(path);
  }
  return found;
}

const sources = svelteFiles().map((path) => ({ path: path.slice(srcDir.length), text: readFileSync(path, 'utf8') }));

/** Every component tag of this name in the source, from `<Name` to its closing `>`, braces counted. */
function tagsOf(text: string, name: string): string[] {
  const tags: string[] = [];
  const pattern = new RegExp(`<${name}\\b`, 'gu');
  for (const match of text.matchAll(pattern)) {
    let depth = 0;
    for (let i = match.index; i < text.length; i += 1) {
      const ch = text[i];
      if (ch === '{') depth += 1;
      else if (ch === '}') depth -= 1;
      else if (ch === '>' && depth === 0) { tags.push(text.slice(match.index, i + 1)); break; }
    }
  }
  return tags;
}

test('a pressed toggle never turns primary: the pressed look is the design system’s (WB-2)', () => {
  const offenders: string[] = [];
  for (const { path, text } of sources) {
    for (const tag of [...tagsOf(text, 'Button'), ...tagsOf(text, 'IconButton')]) {
      if (/aria-pressed=/u.test(tag) && /tone=\{[^}]*'primary'/u.test(tag)) offenders.push(`${path}: ${tag.slice(0, 80)}`);
    }
  }
  assert.deepEqual(offenders, []);
});

test('no screen paints a design-system button with a tone of its own', () => {
  const offenders = sources.filter(({ text }) => /:global\(\[data-bar='task'\]\[data-tone=/u.test(text)).map(({ path }) => path);
  assert.deepEqual(offenders, []);
});

test('the application names the sheet a menu becomes on a phone, and only the navigation keeps its menus anchored', () => {
  const app = sources.find(({ path }) => path === 'App.svelte');
  assert.match(app?.text ?? '', /setSheetCloseLabel\(t\('app\.sheet_close'\)\)/u);
  const anchored = sources.filter(({ text }) => /<Menu keepsAnchor/u.test(text)).map(({ path }) => path);
  for (const path of anchored) assert.match(path, /^lib\/frame\//u, `${path} keeps a menu at its trigger outside the navigation`);
});
