// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import test from 'node:test';

import { blocks, firstSentence, inlines } from '../src/lib/api/markdown.ts';

test('the inline dialect: code, strong, em and links; anything else is text', () => {
  assert.deepEqual(inlines('A `code` and **bold** and *soft* and [a link](https://x.y/z).'), [
    { kind: 'text', text: 'A ' },
    { kind: 'code', text: 'code' },
    { kind: 'text', text: ' and ' },
    { kind: 'strong', text: 'bold' },
    { kind: 'text', text: ' and ' },
    { kind: 'em', text: 'soft' },
    { kind: 'text', text: ' and ' },
    { kind: 'link', text: 'a link', href: 'https://x.y/z' },
    { kind: 'text', text: '.' },
  ]);
  assert.deepEqual(inlines('<b>not markup</b>'), [{ kind: 'text', text: '<b>not markup</b>' }]);
});

test('the block dialect: paragraphs, lists with continuation lines, fenced code', () => {
  const source = 'First line\nsecond line.\n\n* one\n* two\n  continued\n\n```json\n{"a": 1}\n```\nLast.';
  const out = blocks(source);
  assert.equal(out.length, 4);
  assert.equal(out[0].kind, 'paragraph');
  assert.equal(out[0].inlines[0].text, 'First line second line.');
  assert.equal(out[1].kind, 'list');
  assert.deepEqual(out[1].items.map((item) => item[0].text), ['one', 'two continued']);
  assert.deepEqual(out[2], { kind: 'code', text: '{"a": 1}', language: 'json' });
  assert.equal(out[3].inlines[0].text, 'Last.');
});

test('the first sentence is one line of plain text', () => {
  assert.equal(firstSentence('Finds `entries` by words. More later.\n\nAnother paragraph.'), 'Finds entries by words.');
  assert.equal(firstSentence(''), '');
  assert.equal(firstSentence('No full stop'), 'No full stop');
});
