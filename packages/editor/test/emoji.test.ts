// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { EMOJI_SHORTCODES, emojiOf, shortcodeAt } from '../src/emoji.ts';
import { fromMarkdown, toMarkdown } from '../src/markdown.ts';

// A `:code:` typed after a space or at a line's start is replaced by its emoji; one the table does
// not hold, one inside a word or a time like 10:30:, stays what was typed.
test('a known shortcode at the end of the typed text is its emoji', () => {
  assert.deepEqual(shortcodeAt('Shipped :tada:'), { index: 8, text: ':tada:', emoji: '🎉' });
  assert.deepEqual(shortcodeAt(':+1:'), { index: 0, text: ':+1:', emoji: '👍' });
  assert.equal(shortcodeAt('Shipped :no_such_code:'), null);
  assert.equal(shortcodeAt('at 10:30:'), null);
  assert.equal(shortcodeAt('word:tada:'), null);
  assert.equal(shortcodeAt(':tada: done'), null);
  assert.equal(emojiOf('constructor'), undefined);
});

// The page keeps the Unicode emoji, which its Markdown carries as it is.
test('every emoji of the table survives the Markdown a page saves', () => {
  for (const [code, emoji] of Object.entries(EMOJI_SHORTCODES)) {
    const body = `Done ${emoji}\n`;
    assert.equal(toMarkdown(fromMarkdown(body)), body, code);
  }
});
