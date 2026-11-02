// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { compareBodies } from '../src/compare.ts';
import { fromMarkdown, parseAddress, toMarkdown } from '../src/markdown.ts';

const ENTRY = '00000000-0000-0000-0000-0000000000e1';
const VIEW = '00000000-0000-0000-0000-0000000000e2';

test('a body survives the editor: what is read back is written the same (UC-DOC-02 check 4)', () => {
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

test('a mention, a card and a live view are references in the notes grammar (UC-DOC-04 check 1)', () => {
  const doc = fromMarkdown(`See [the task](hubtask:entry/${ENTRY})\n\n![](hubtask:entry/${ENTRY})\n\n![board](hubtask:view/${VIEW}#top)`);
  const [paragraph, card, view] = doc.content ?? [];
  assert.equal(paragraph?.content?.[1]?.type, 'reference');
  assert.equal(card?.type, 'referenceBlock');
  assert.deepEqual(view?.attrs, { kind: 'view', id: VIEW, anchor: 'top' });
  // The label is never written: a title would show what a reader may not see.
  assert.match(toMarkdown(doc), new RegExp(`See \\[\\]\\(hubtask:entry/${ENTRY}\\)`));
});

test('a picture or a file the page holds is a block of its own, written as the import writes it (UC-DOC-14 check 1)', () => {
  const PICTURE = '00000000-0000-0000-0000-0000000000e3';
  const body = `The flow:\n\n![](hubtask:media/${PICTURE})\n`;
  const doc = fromMarkdown(body);
  assert.equal(doc.content?.[1]?.type, 'referenceBlock');
  assert.deepEqual(doc.content?.[1]?.attrs, { kind: 'media', id: PICTURE, anchor: null });
  assert.equal(toMarkdown(doc), body);
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

test('two versions compare line by line (UC-DOC-06 check 3)', () => {
  const lines = compareBodies('a\nb\nc\nd', 'a\nB\nc\nd\ne');
  assert.deepEqual(lines.map((each) => `${each.kind[0]}${each.text}`), ['sa', 'rb', 'aB', 'sc', 'sd', 'ae']);
});

test('the wiki blocks survive the editor: callouts, toggles, contents, child pages, embeds, dates and statuses (UC-DOC-01 check 10)', () => {
  const body = [
    '::toc',
    '',
    ':::warning',
    'Read this **first**.',
    '',
    '- one',
    ':::',
    '',
    '::::details How do I start?',
    'Open the hub.',
    '',
    ':::success',
    'Done on :date[2026-10-31], now :status[In progress]{info}.',
    ':::',
    '::::',
    '',
    '::children',
    '',
    '::embed https://videos.example.org/watch?v=42',
    '',
  ].join('\n');
  const doc = fromMarkdown(body);
  assert.deepEqual(doc.content?.map((node) => node.type), ['tableOfContents', 'callout', 'details', 'childPages', 'embed']);
  assert.equal(doc.content?.[1]?.attrs?.['tone'], 'warning');
  assert.equal(doc.content?.[2]?.attrs?.['summary'], 'How do I start?');
  assert.equal(doc.content?.[2]?.content?.[1]?.type, 'callout');
  const line = doc.content?.[2]?.content?.[1]?.content?.[0]?.content ?? [];
  assert.deepEqual(line.filter((node) => node.type !== 'text'), [
    { type: 'date', attrs: { date: '2026-10-31' } },
    { type: 'status', attrs: { label: 'In progress', tone: 'info' } },
  ]);
  assert.equal(toMarkdown(doc), body);
});

test('an embed takes an https address alone, and a directive typed as words stays words', () => {
  for (const address of ['http://videos.example.org/1', 'javascript:alert(1)', 'https://user:pw@videos.example.org/1']) {
    const doc = fromMarkdown(`::embed ${address}\n`);
    assert.notEqual(doc.content?.[0]?.type, 'embed', address);
  }
  const words = { type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: '::toc and :date[soon]' }] }] };
  const written = toMarkdown(words);
  assert.deepEqual(fromMarkdown(written).content?.[0], words.content[0]);
  const unknownTone = toMarkdown({ type: 'doc', content: [{ type: 'callout', attrs: { tone: 'purple' }, content: [{ type: 'paragraph', content: [{ type: 'text', text: 'x' }] }] }] });
  assert.equal(unknownTone, ':::info\nx\n:::\n');
});

test('a Markdown table is read as a table and written back the same (fourth answers, 2)', () => {
  const body = [
    '## Bausteine',
    '',
    '| Baustein | Verantwortung | Team |',
    '| --- | --- | --- |',
    '| App (iOS/Android) | Oberfläche und **Offline** | Mobile |',
    '| Pipe \\| im Text |  | Plattform |',
    '',
    'Danach ein Absatz.',
    '',
  ].join('\n');
  const doc = fromMarkdown(body);
  const table = doc.content?.[1];
  assert.equal(table?.type, 'table', 'the table is not a table');
  assert.equal(table?.content?.length, 3);
  assert.deepEqual(table?.content?.[0]?.content?.map((cell) => cell.type), ['tableHeader', 'tableHeader', 'tableHeader']);
  assert.equal(table?.content?.[2]?.content?.[0]?.content?.[0]?.content?.[0]?.text, 'Pipe | im Text');
  assert.equal(toMarkdown(doc), body);
  assert.equal(doc.content?.[2]?.type, 'paragraph');
});

test('a table as a seed writes it - without spaces, alignment marks or a ragged row - is read too', () => {
  const doc = fromMarkdown('|a|b|\n|:---|---:|\n|1|\n');
  const table = doc.content?.[0];
  assert.equal(table?.type, 'table');
  assert.equal(table?.content?.[1]?.content?.length, 2, 'a short row is filled to the width');
  assert.equal(toMarkdown(doc), '| a | b |\n| --- | --- |\n| 1 |  |\n');
});
