// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A dialog is never overloaded (page-system §5.1, its hard limits): every `Dialog` names its rung
// (`size` s, m or l), and none holds a table, tabs, a second dialog - *Discard changes?* is the one
// allowed over another - or more than one `Disclosure`. What a dialog holds includes what the
// components drawn inside it draw. Read from the source, as the page-kind gate is.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

import { SRC, elements, read, reachable, relative, svelteFiles, uses, within } from './markup.js';

/** The confirmation the ladder allows over another dialog. */
const DISCARD = 'DiscardDialog';

/**
 * A failure's *Details for support* (PS-15) folds the failure, not the dialog's content: it is
 * drawn only after a refusal and is no second fold of what the dialog asks.
 */
const FAILURE = reachable(path.join(SRC, 'lib/frame/FailureNote.svelte'));

/**
 * A subject drawn `isEmbedded` inside a dialog - *Capture*'s template and import steps - is that
 * dialog's next step in place: `PanelOrDialog` then draws its content as a section, never a
 * dialog of its own. What the content holds is still held to the limits.
 */
const IN_PLACE = path.join(SRC, 'lib/workspace/PanelOrDialog.svelte');

/** A component's opening tag from its `<`, braces and the arrows inside them skipped. */
function tagAt(markup, at) {
  let depth = 0;
  for (let index = at; index < markup.length; index += 1) {
    const char = markup[index];
    if (char === '{') depth += 1;
    else if (char === '}') depth -= 1;
    else if (char === '>' && depth === 0) return markup.slice(at, index + 1);
  }
  return markup.slice(at);
}

/** Everything the components drawn inside a range draw, by file. */
function drawnInside(file, ranges) {
  const { markup } = read(file);
  const files = new Set();
  for (const use of uses(file)) {
    if (use.name === DISCARD || !within(ranges, use.at) || !fs.existsSync(use.file)) continue;
    const isInPlace = /\sisEmbedded(?=[\s/>])/.test(tagAt(markup, use.at));
    for (const inner of reachable(use.file)) if (!(isInPlace && inner === IN_PLACE)) files.add(inner);
  }
  return files;
}

const count = (markup, pattern) => [...markup.matchAll(pattern)].length;

/** What is wrong with each dialog of a file. */
function faults(file) {
  const { markup } = read(file);
  const found = [];
  for (const dialog of elements(markup, 'Dialog')) {
    const where = `${relative(file)}:${markup.slice(0, dialog.start).split('\n').length}`;
    if (!/\bsize=|\{size\}/.test(dialog.tag)) found.push(`${where}: no size`);
    const body = markup.slice(dialog.start + dialog.tag.length, dialog.end);
    const inner = [...drawnInside(file, [dialog])].map((each) => read(each).markup);
    const all = [body, ...inner];
    if (all.some((text) => /<(Table|DataTable)(?=[\s/>])|<table(?=[\s>])/.test(text))) found.push(`${where}: a table`);
    if (all.some((text) => /<(Tabs|PlaceTabs)(?=[\s/>])/.test(text))) found.push(`${where}: tabs`);
    if (all.some((text) => /<Dialog(?=[\s/>])/.test(text))) found.push(`${where}: a second dialog`);
    const folds = [body, ...[...drawnInside(file, [dialog])].filter((each) => !FAILURE.has(each)).map((each) => read(each).markup)];
    if (folds.reduce((sum, text) => sum + count(text, /<Disclosure(?=[\s/>])/g), 0) > 1) found.push(`${where}: more than one Disclosure`);
  }
  return found;
}

const files = () => svelteFiles().filter((file) => /<Dialog(?=[\s/>])/.test(read(file).markup));

test('every dialog names its size and holds no table, tabs, second dialog or second Disclosure', () => {
  const wrong = files().flatMap(faults);
  assert.deepEqual(wrong, []);
});
