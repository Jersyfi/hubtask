// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { getSchema } from '@tiptap/core';
import { TaskItem, TaskList } from '@tiptap/extension-list';
import { Node as PMNode } from '@tiptap/pm/model';
import StarterKit from '@tiptap/starter-kit';

import { createLowlight, common } from 'lowlight';

import { diagramSource, highlight } from '../src/code.ts';
import { Callout, ChildPages, DateChip, Details, Embed, PageTable, Reference, embedFrame, ReferenceBlock, StatusChip, TableOfContents } from '../src/editor.ts';
import { fromMarkdown, toMarkdown, type DocNode } from '../src/markdown.ts';
import { MathBlock, loadKatex, mathMarkup } from '../src/math.ts';

const schema = getSchema([
  StarterKit, TaskList, TaskItem, Reference, ReferenceBlock, Callout, Details, PageTable, TableOfContents, ChildPages, Embed, DateChip, StatusChip, MathBlock,
]);

// A page imported from a wiki keeps its panels, expands and contents in the editor itself: the
// document the Markdown reads is one the editor's schema holds, and what the editor answers is
// written back the same (UC-DOC-01 check 10).
test("the wiki blocks are the editor schema's own, and come back as written", () => {
  const body = [
    '::toc',
    '',
    ':::error',
    'Do not restart the **primary**.',
    ':::',
    '',
    '::::details Rollback',
    ':::info',
    'Since :date[2026-10-01]: :status[Approved]{success}',
    ':::',
    '::::',
    '',
    '::children',
    '',
    '::embed https://videos.example.org/v/7',
    '',
  ].join('\n');
  const doc = PMNode.fromJSON(schema, fromMarkdown(body));
  doc.check();
  assert.equal(toMarkdown(doc.toJSON() as DocNode), body);
});

// An embed is framed only where the installation frames its origin (HUBTASK_UI_FRAME_SOURCES),
// and then sandboxed so the framed page never holds its own origin's rights beside its scripts;
// every other embed - and every address that is not https - stays a link card.
test('an embed is framed, sandboxed, only for an origin the installation frames', () => {
  const frames = ['https://dashboards.example.org'];
  const framed = embedFrame('https://dashboards.example.org/d/7?kiosk', frames);
  assert.deepEqual(framed, {
    src: 'https://dashboards.example.org/d/7?kiosk',
    sandbox: 'allow-scripts allow-popups allow-presentation',
    referrerpolicy: 'no-referrer',
  });
  assert.ok(!framed.sandbox.split(' ').includes('allow-same-origin'));
  assert.equal(embedFrame('https://videos.example.org/v/7', frames), undefined);
  assert.equal(embedFrame('https://dashboards.example.org.evil.example/d', frames), undefined);
  assert.equal(embedFrame('http://dashboards.example.org/d/7', ['http://dashboards.example.org']), undefined);
  assert.equal(embedFrame('https://dashboards.example.org/d/7', undefined), undefined);
  assert.equal(embedFrame('https://dashboards.example.org/d/7', []), undefined);
});

// A table seeded through the API (the handbook's architecture page) is a table the editor's
// schema holds, and comes back as written (fourth answers, 2).
test("a Markdown table is the editor schema's own table, and comes back as written", () => {
  const body = [
    '| Baustein | Verantwortung | Team |',
    '| --- | --- | --- |',
    '| App (iOS/Android) | Oberfläche und Offline | Mobile |',
    '',
  ].join('\n');
  const doc = PMNode.fromJSON(schema, fromMarkdown(body));
  doc.check();
  assert.equal(doc.firstChild?.type.name, 'table');
  assert.equal(toMarkdown(doc.toJSON() as DocNode), body);
});

// A formula, a diagram and a highlighted block are the editor schema's own and come back as
// written (fourth answers, 2): the formula as `$$` lines, the diagram and the code as their fence.
test('a formula, a diagram and highlighted code come back as written', () => {
  const body = [
    '$$',
    '\\sum_{i=1}^{n} i = \\frac{n(n+1)}{2}',
    '$$',
    '',
    '```mermaid',
    'graph TD',
    '  A[Request] --> B{Approved?}',
    '```',
    '',
    '```go',
    'func main() { fmt.Println("hi") }',
    '```',
    '',
  ].join('\n');
  const doc = PMNode.fromJSON(schema, fromMarkdown(body));
  doc.check();
  assert.deepEqual([...Array(doc.childCount).keys()].map((at) => doc.child(at).type.name), ['mathBlock', 'codeBlock', 'codeBlock']);
  assert.equal(doc.child(0).attrs['latex'], '\\sum_{i=1}^{n} i = \\frac{n(n+1)}{2}');
  assert.equal(toMarkdown(doc.toJSON() as DocNode), body);
});

// Dollars in a sentence stay a sentence: only a line of `$$` opens a formula, one that never closes
// is text, and a paragraph that starts with `$$` is escaped so it is not read back as one.
test('dollars outside a formula are never read as one', () => {
  const prices = fromMarkdown('It costs $5 and $6.\n');
  assert.equal(prices.content?.[0]?.type, 'paragraph');
  const open = fromMarkdown('$$\nx + y\n');
  assert.deepEqual(open.content?.map((node) => node.type), ['paragraph']);
  const written = toMarkdown({ type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: '$$ is a lot' }] }] });
  assert.equal(written, '\\$$ is a lot\n');
  assert.equal(fromMarkdown(written).content?.[0]?.type, 'paragraph');
  assert.equal(toMarkdown(fromMarkdown('$$a^2$$\n')), '$$\na^2\n$$\n');
  // A `$$` line inside the TeX would close the block early; it is written as `$ $`.
  assert.equal(toMarkdown({ type: 'doc', content: [{ type: 'mathBlock', attrs: { latex: 'a\n$$\nb' } }] }), '$$\na\n$ $\nb\n$$\n');
});

// KaTeX draws MathML only, and refuses what `trust` would have to allow.
test('a formula is drawn as MathML, and TeX that asks for trust is refused', async () => {
  const katex = await loadKatex();
  const markup = mathMarkup(katex, 'x^2');
  assert.match(markup ?? '', /^<span class="katex"><math /u);
  assert.doesNotMatch(markup ?? '', /style=/u);
  assert.equal(mathMarkup(katex, '\\frac{'), undefined);
  assert.doesNotMatch(mathMarkup(katex, '\\href{javascript:alert(1)}{x}') ?? '', /href=/u);
});

test('a known language is coloured, an unknown one or a huge block is drawn plain', () => {
  const lowlight = createLowlight(common);
  const runs = highlight(lowlight, 'go', 'func main() {}');
  assert.deepEqual(runs[0], { from: 0, to: 4, classes: 'hljs-function hljs-keyword' });
  assert.deepEqual(highlight(lowlight, 'klingon', 'qapla'), []);
  assert.deepEqual(highlight(lowlight, '', 'x'), []);
  assert.deepEqual(highlight(lowlight, 'go', 'x'.repeat(100_001)), []);
});

test("a diagram's picture is an image address on its paper, and the colour cannot break out", () => {
  const src = diagramSource('<svg xmlns="http://www.w3.org/2000/svg"><g/></svg>', 'paper"/><script>');
  assert.ok(src.startsWith('data:image/svg+xml;charset=utf-8,'));
  const svg = decodeURIComponent(src.slice(src.indexOf(',') + 1));
  assert.equal(svg, '<svg xmlns="http://www.w3.org/2000/svg"><rect width="100%" height="100%" fill="paperscript"/><g/></svg>');
});
