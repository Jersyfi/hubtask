// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { getSchema } from '@tiptap/core';
import { Schema } from '@tiptap/pm/model';
import { EditorState, TextSelection } from '@tiptap/pm/state';
import StarterKit from '@tiptap/starter-kit';
import { updateYFragment, yXmlFragmentToProseMirrorRootNode } from '@tiptap/y-tiptap';
import * as Y from 'yjs';

import { lineBelow, lineStart, moveLine, removeLine } from '../src/lines.ts';

const schema = new Schema({
  nodes: {
    doc: { content: 'block+' },
    paragraph: { group: 'block', content: 'inline*' },
    heading: { group: 'block', content: 'inline*' },
    text: { group: 'inline' },
  },
});

const page = () => EditorState.create({
  schema,
  doc: schema.node('doc', null, [
    schema.node('heading', null, [schema.text('Open points')]),
    schema.node('paragraph', null, [schema.text('First')]),
    schema.node('paragraph', null, [schema.text('Second')]),
  ]),
});
const lines = (state: EditorState) => state.doc.children.map((each) => each.textContent);

test('a position inside a line finds the line', () => {
  const state = page();
  const second = state.doc.child(0).nodeSize;
  assert.equal(lineStart(state, second + 3), second);
  assert.equal(lineStart(state, 2), 0);
});

test('a line moves one place down and up, and not past the page', () => {
  const state = page();
  const second = state.doc.child(0).nodeSize;
  const down = moveLine(state, second, 1);
  assert.ok(down);
  assert.deepEqual(lines(state.apply(down)), ['Open points', 'Second', 'First']);
  const up = moveLine(state, second, -1);
  assert.ok(up);
  assert.deepEqual(lines(state.apply(up)), ['First', 'Open points', 'Second']);
  assert.equal(moveLine(state, 0, -1), null, 'the first line goes no higher');
});

test('a removed line is gone; the last one leaves an empty line', () => {
  const state = page();
  const removed = removeLine(state, 0);
  assert.ok(removed);
  assert.deepEqual(lines(state.apply(removed)), ['First', 'Second']);
  const one = EditorState.create({ schema, doc: schema.node('doc', null, [schema.node('paragraph', null, [schema.text('Only')])]) });
  const emptied = removeLine(one, 0);
  assert.ok(emptied);
  assert.deepEqual(lines(one.apply(emptied)), ['']);
});

test('"+" puts an empty line below and the caret in it', () => {
  const state = page();
  const tr = lineBelow(state, 0);
  assert.ok(tr);
  const next = state.apply(tr);
  assert.deepEqual(lines(next), ['Open points', '', 'First', 'Second']);
  assert.equal(next.selection.$from.parent.textContent, '');
  assert.equal(next.selection.$from.index(0), 1);
});

// The live document carries a moved line whole, and a line written below it after (the binding
// writes each state into the document as `updateYFragment` does, keeping its mapping between).
test('a moved line keeps its words in the live document while writing goes on below it', () => {
  const full = getSchema([StarterKit]);
  const ydoc = new Y.Doc();
  const fragment = ydoc.getXmlFragment('default');
  const meta = { mapping: new Map(), isOMark: new Map() };
  let state = EditorState.create({
    schema: full,
    doc: full.node('doc', null, [
      full.node('paragraph', null, [full.text('Welcome', [full.marks['bold']!.create()])]),
      full.node('paragraph', null, [full.text('Next line')]),
    ]),
  });
  const write = () => ydoc.transact(() => updateYFragment(ydoc, fragment, state.doc, meta as never));
  write();
  state = state.apply(moveLine(state, 0, 1)!);
  write();
  state = state.apply(state.tr.setSelection(TextSelection.create(state.doc, state.selection.$from.end())));
  state = state.apply(state.tr.split(state.selection.from));
  write();
  for (const letter of 'Below') {
    state = state.apply(state.tr.insertText(letter));
    write();
  }
  const read = yXmlFragmentToProseMirrorRootNode(fragment, full);
  assert.deepEqual(read.children.map((each) => each.textContent), ['Next line', 'Welcome', 'Below']);
});
