// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { compareBodies } from '../src/compare.ts';
import { fromMarkdown, parseAddress, toMarkdown } from '../src/markdown.ts';

const ENTRY = '00000000-0000-0000-0000-0000000000e1';
const VIEW = '00000000-0000-0000-0000-0000000000e2';

test('a body survives the editor: what is read back is written the same (UC-DOC-02 check 2)', () => {
  const body = [
    '# Onboarding',
    '',
    'Read **the handbook** and *ask* `early`, see [the wiki](https://example.org) and [](hubtask:entry/' + ENTRY + ').',
    '',
    '- one',
    '- two',
    '  - nested',
    '',
    '1. first',
    '2. second',
    '',
    '- [ ] open',
    '- [x] done',
    '',
    '> quoted',
    '',
    '```go',
    'func main() {}',
    '```',
    '',
    '---',
    '',
    '![](hubtask:view/' + VIEW + ')',
    '',
  ].join('\n');
  assert.equal(toMarkdown(fromMarkdown(body)), body);
});

test('a mention, a card and a live view are references in the notes grammar (UC-DOC-02 check 3)', () => {
  const doc = fromMarkdown(`See [the task](hubtask:entry/${ENTRY})\n\n![](hubtask:entry/${ENTRY})\n\n![board](hubtask:view/${VIEW}#top)`);
  const [paragraph, card, view] = doc.content ?? [];
  assert.equal(paragraph?.content?.[1]?.type, 'reference');
  assert.equal(card?.type, 'referenceBlock');
  assert.deepEqual(view?.attrs, { kind: 'view', id: VIEW, anchor: 'top' });
  // The label is never written: a title would show what a reader may not see.
  assert.match(toMarkdown(doc), new RegExp(`See \\[\\]\\(hubtask:entry/${ENTRY}\\)`));
});

test('text that looks like Markdown is escaped, not turned into it', () => {
  const doc = { type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: '# not a heading *nor bold*' }] }] };
  const written = toMarkdown(doc);
  assert.equal(written, '\\# not a heading \\*nor bold\\*\n');
  assert.deepEqual(fromMarkdown(written).content?.[0]?.content, [{ type: 'text', text: '# not a heading *nor bold*' }]);
});

test('an address is read only in its exact grammar', () => {
  assert.deepEqual(parseAddress(`hubtask:page/${ENTRY.toUpperCase()}`), { kind: 'page', id: ENTRY, anchor: null });
  assert.equal(parseAddress('hubtask:page/not-an-id'), undefined);
  assert.equal(parseAddress('https://example.org'), undefined);
});

test('an empty body is one empty paragraph', () => {
  assert.deepEqual(fromMarkdown(''), { type: 'doc', content: [{ type: 'paragraph' }] });
});

test('two versions compare line by line (UC-DOC-04 check 2)', () => {
  const lines = compareBodies('a\nb\nc\nd', 'a\nB\nc\nd\ne');
  assert.deepEqual(lines.map((each) => `${each.kind[0]}${each.text}`), ['sa', 'rb', 'aB', 'sc', 'sd', 'ae']);
});
