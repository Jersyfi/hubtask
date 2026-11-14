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
  assert.match(app?.text ?? '', /setCloseLabel\(t\('app\.sheet_close'\)\)/u);
  const anchored = sources.filter(({ text }) => /<Menu keepsAnchor/u.test(text)).map(({ path }) => path);
  for (const path of anchored) assert.match(path, /^lib\/frame\//u, `${path} keeps a menu at its trigger outside the navigation`);
});

/** The dialogs in a file: their opening tag and their body up to the closing tag. */
function dialogsOf(text: string): { tag: string; body: string }[] {
  return tagsOf(text, 'Dialog').map((tag) => {
    const start = text.indexOf(tag) + tag.length;
    return { tag, body: text.slice(start, text.indexOf('</Dialog>', start)) };
  });
}

test('the products’ dialogs hand their one primary to the dialog and keep no submit in their body (D-2)', () => {
  const files = [
    'views/GoalsView.svelte', 'views/TimeView.svelte', 'views/DashboardView.svelte', 'views/PagesView.svelte',
    'lib/entries/DuplicateDialog.svelte',
  ];
  for (const file of files) {
    const text = sources.find(({ path }) => path === file)?.text ?? '';
    for (const { tag, body } of dialogsOf(text)) {
      const inBody = body.split('{#snippet actions()}')[0] ?? '';
      const submits = tagsOf(inBody, 'Button').filter((button) => !/size="sm"/u.test(button) && (!/tone=/u.test(button) || /tone="(primary|danger)"/u.test(button)));
      if (submits.length > 0) assert.match(tag, /primary=\{/u, `${file}: ${tag.slice(0, 60)} keeps its submit in the body`);
      assert.doesNotMatch(body, /<Button tone="subtle"[^>]*>\{t\('app\.(pages|links|workspace)\.cancel'\)\}/u, `${file}: a cancel is subtle`);
    }
  }
});

test('a read-only dialog has no primary: the workflow’s picture closes by its ✕', () => {
  const text = sources.find(({ path }) => path === 'lib/entries/EntryStatus.svelte')?.text ?? '';
  const picture = dialogsOf(text).find(({ tag }) => tag.includes('app.workflows.picture'));
  assert.ok(picture);
  assert.doesNotMatch(picture.body, /tone="primary"/u);
});

test('the date, person and labels pickers open from the ⋯ as the rail opens them: a popover, a sheet on a phone (D-3)', () => {
  const text = sources.find(({ path }) => path === 'lib/entries/EntryActions.svelte')?.text ?? '';
  assert.match(text, /<Popover label=\{title\} bind:isOpen=\{isPickerOpen\} at=\{door\}/u);
  assert.doesNotMatch(text, /<Dialog size="m" isOpen=\{true\} title=\{t\('app\.(due\.title|people\.assignee|entry_menu\.labels)'\)/u);
  assert.match(text, /title=\{t\('app\.entry_menu\.move_title'/u);
});

test('the board’s start is the ladder’s dialog, not a panel of its own (D-4)', () => {
  const text = sources.find(({ path }) => path === 'lib/whiteboards/BoardCanvas.svelte')?.text ?? '';
  assert.doesNotMatch(text, /role="dialog"/u);
  assert.match(text, /<Dialog size="l" isOpen=\{true\} title=\{t\('app\.whiteboards\.begin_title'\)\}/u);
});

test('a row’s and a card’s state chip follow its entry type’s own level, as the server answers it per type (L-5)', () => {
  const view = sources.find(({ path }) => path === 'views/ContainerView.svelte')?.text ?? '';
  assert.match(view, /const mode = container\?\.state_modes\?\.\[entryType\];/u);
  assert.equal((view.match(/\{levelOfType\}/gu) ?? []).length, 2, 'the list and the board both receive it');
  const list = sources.find(({ path }) => path === 'lib/entries/EntryList.svelte')?.text ?? '';
  assert.match(list, /const mode = own\?\.mode \?\? stateMode;/u);
  const board = sources.find(({ path }) => path === 'lib/entries/Board.svelte')?.text ?? '';
  assert.match(board, /chipOf\(own\?\.mode \?\? level, card/u);
});
