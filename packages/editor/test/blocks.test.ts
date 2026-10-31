// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { getSchema } from '@tiptap/core';
import { TaskItem, TaskList } from '@tiptap/extension-list';
import { Node as PMNode } from '@tiptap/pm/model';
import StarterKit from '@tiptap/starter-kit';

import { Callout, ChildPages, DateChip, Details, Embed, Reference, embedFrame, ReferenceBlock, StatusChip, TableOfContents } from '../src/editor.ts';
import { fromMarkdown, toMarkdown, type DocNode } from '../src/markdown.ts';

const schema = getSchema([
  StarterKit, TaskList, TaskItem, Reference, ReferenceBlock, Callout, Details, TableOfContents, ChildPages, Embed, DateChip, StatusChip,
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
