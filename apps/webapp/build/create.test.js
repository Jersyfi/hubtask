// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The create act of a screen is its PageHeader's primary (user-experience.md §5.2): a create among
// the secondary acts while the head has no primary is the screen hiding its main door. Read from
// the source, because the shape is what a new screen copies from its neighbour.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const VIEWS = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src', 'views');

/** Each `<PageHeader …>` tag's own source, attributes and all, up to its closing `/>`. */
function headsOf(source) {
  const heads = [];
  let from = 0;
  for (;;) {
    const start = source.indexOf('<PageHeader', from);
    if (start < 0) return heads;
    let depth = 0;
    let end = start;
    for (; end < source.length; end++) {
      const char = source[end];
      if (char === '{') depth++;
      else if (char === '}') depth--;
      else if (depth === 0 && source.startsWith('/>', end)) break;
    }
    heads.push(source.slice(start, end));
    from = end;
  }
}

test('no screen puts its create act among the secondary acts of a head without a primary', () => {
  const found = [];
  for (const name of fs.readdirSync(VIEWS).filter((file) => file.endsWith('.svelte'))) {
    const source = fs.readFileSync(path.join(VIEWS, name), 'utf8');
    for (const head of headsOf(source)) {
      const hasPrimary = /\bprimary=|\{primary\}/.test(head);
      const secondary = /\bsecondary=\{([\s\S]*)/.exec(head)?.[1] ?? '';
      if (!hasPrimary && /icon: 'plus'/.test(secondary)) found.push(name);
    }
  }
  assert.deepEqual(found, [], 'make the create the head\'s primary');
});

/**
 * The source ranges inside a `<Dialog>`, a `<form>` or an `<EmptyState>`, where an add is the
 * form's own or the empty screen's first act (§5.5).
 */
function insideForms(source) {
  const ranges = [];
  for (const [open, close] of [['<Dialog', '</Dialog>'], ['<form', '</form>'], ['<PanelOrDialog', '</PanelOrDialog>'], ['<EmptyState', '</EmptyState>']]) {
    let from = 0;
    for (;;) {
      const start = source.indexOf(open, from);
      if (start < 0) break;
      from = start + open.length;
      // A self-closing tag holds nothing.
      const tag = tagAt(source, start);
      if (!/^\w/.test(source.slice(from, from + 1)) && !tag.endsWith('/')) {
        const end = source.indexOf(close, start);
        ranges.push([start, end < 0 ? source.length : end]);
      }
    }
  }
  return ranges;
}

/** The opening tag at `start`, up to its `>` outside braces (without it). */
function tagAt(source, start) {
  let depth = 0;
  let end = start;
  for (; end < source.length; end++) {
    const char = source[end];
    if (char === '{') depth++;
    else if (char === '}') depth--;
    else if (depth === 0 && char === '>') break;
  }
  return source.slice(start, end);
}

/** Each `<Button …>` opening tag, up to its `>` outside braces, with where it starts. */
function buttonsOf(source) {
  const buttons = [];
  for (let from = 0; ;) {
    const start = source.indexOf('<Button', from);
    if (start < 0) return buttons;
    const tag = tagAt(source, start);
    if (/^<Button\b/.test(tag)) buttons.push({ at: start, tag });
    from = start + tag.length;
  }
}

// §5.2: a `Button` with the plus outside a dialog, a form or a list's own add, on a screen whose
// head has no primary, is a create act the head should carry. A list's own add says so with
// `data-list-add`, so the exception is a decision written where it is made; adding an entry is
// capture (design-system.md §11.8), never a screen's create.
test('no screen without a head primary keeps a create button of its own outside a dialog, a form or a list', () => {
  const found = [];
  for (const name of fs.readdirSync(VIEWS).filter((file) => file.endsWith('.svelte'))) {
    const source = fs.readFileSync(path.join(VIEWS, name), 'utf8');
    const heads = headsOf(source);
    if (heads.length === 0 || heads.some((head) => /\bprimary=|\{primary\}/.test(head))) continue;
    const forms = insideForms(source);
    for (const { at, tag } of buttonsOf(source)) {
      if (!/\bicon="plus"/.test(tag) || /\bdata-list-add\b|capture\.summon/.test(tag)) continue;
      if (forms.some(([start, end]) => at > start && at < end)) continue;
      found.push(`${name}:${source.slice(0, at).split('\n').length}`);
    }
  }
  assert.deepEqual(found, [], 'make the create the head\'s primary, or mark a list\'s own add with data-list-add');
});
