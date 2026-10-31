// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { getSchema } from '@tiptap/core';
import { TaskItem, TaskList } from '@tiptap/extension-list';
import { Node as PMNode } from '@tiptap/pm/model';
import StarterKit from '@tiptap/starter-kit';

import { Callout, ChildPages, DateChip, Details, Embed, Reference, ReferenceBlock, StatusChip, TableOfContents } from '../src/editor.ts';
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
