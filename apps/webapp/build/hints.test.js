// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A visible help text is two lines at most (design-system.md §4, "Help text"): a hint, a row's
// help, a section's or a page's introduction over about 120 characters in English keeps what is
// needed now and moves its why behind an ⓘ, as a `…_more` text. The texts left longer stand in
// `hints.json` with a one-word reason - a description is never moved (the owner's condition 1) -
// and that list only shrinks: a text that became short enough leaves it, and none joins it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const catalogue = JSON.parse(readFileSync(join(here, '..', '..', '..', 'locales', 'en.json'), 'utf8'));
const exceptions = JSON.parse(readFileSync(join(here, 'hints.json'), 'utf8'));

/** Two lines of a hint, in English characters. */
const LIMIT = 120;
/** How many texts the list held when it was written; it may only go down. */
const LISTED = 166;
/** What a text left long may be. */
const REASONS = new Set(['description', 'needed', 'short', 'option', 'status', 'dialog', 'empty', 'unused', 'spoken', 'unseen']);

/** A code whose last part names visible help: `…_hint`, `…_help`, `…_intro`, `…_description` and their variants. */
const isHelp = (code) =>
  !code.startsWith('email.') && !code.endsWith('_more') && /(^|_)(hint|help|intro|description)(_|$)/.test(code.split('.').pop() ?? '');

test('every visible help text over two lines is on the exception list, with its reason', () => {
  const long = Object.keys(catalogue).filter((code) => isHelp(code) && catalogue[code].length > LIMIT);
  const unlisted = long.filter((code) => !(code in exceptions));
  assert.deepEqual(unlisted, [], `longer than ${LIMIT} characters: keep what is needed now and move the why to a …_more text behind an ⓘ`);
});

test('the exception list only shrinks', () => {
  const stale = Object.keys(exceptions).filter((code) => !(code in catalogue) || catalogue[code].length <= LIMIT || !isHelp(code));
  assert.deepEqual(stale, [], 'no longer long, or gone: remove it from hints.json');
  assert.ok(Object.keys(exceptions).length <= LISTED, `hints.json holds ${Object.keys(exceptions).length}; it held ${LISTED} and may only shrink`);
  const unreasoned = Object.entries(exceptions).filter(([, reason]) => !REASONS.has(reason));
  assert.deepEqual(unreasoned, [], `a reason is one of ${[...REASONS].join(', ')}`);
});

test('every text behind an ⓘ belongs to a visible one and is three sentences at most', () => {
  const more = Object.keys(catalogue).filter((code) => code.endsWith('_more') && isHelp(code.slice(0, -'_more'.length)));
  assert.ok(more.length > 0);
  for (const code of more) {
    assert.ok(code.slice(0, -'_more'.length) in catalogue, `${code} has no visible text it belongs to`);
    const sentences = catalogue[code].split(/[.!?]["”]?(?:\s+|$)/).filter((part) => part.trim() !== '');
    assert.ok(sentences.length <= 3, `${code} is ${sentences.length} sentences: three at most`);
  }
});
