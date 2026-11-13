// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { Schema } from '@tiptap/pm/model';
import { EditorState } from '@tiptap/pm/state';

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
