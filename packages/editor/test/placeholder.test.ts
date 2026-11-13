// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { Schema } from '@tiptap/pm/model';
import { EditorState } from '@tiptap/pm/state';
import type { DecorationSet } from '@tiptap/pm/view';

import { isEmptyDocument, placeholder, placeholderKey } from '../src/placeholder.ts';

const schema = new Schema({
  nodes: {
    doc: { content: 'block+' },
    paragraph: { group: 'block', content: 'inline*' },
    heading: { group: 'block', content: 'inline*' },
    text: { group: 'inline' },
  },
});

const stateOf = (...blocks: ReturnType<typeof schema.node>[]) =>
  EditorState.create({ schema, doc: schema.node('doc', null, blocks), plugins: [placeholder('Type / for blocks')] });

const marked = (state: EditorState) => {
  const plugin = placeholderKey.get(state)!;
  const set = plugin.props.decorations!.call(plugin, state) as DecorationSet;
  return set.find().map((each) => (each as unknown as { type: { attrs: Record<string, string> } }).type.attrs['data-placeholder']);
};

test('an empty page explains itself in its first line, and stops once anything is written', () => {
  const empty = stateOf(schema.node('paragraph'));
  assert.equal(isEmptyDocument(empty.doc), true);
  assert.deepEqual(marked(empty), ['Type / for blocks']);
  assert.deepEqual(marked(stateOf(schema.node('paragraph', null, [schema.text('Hallo')]))), []);
  assert.deepEqual(marked(stateOf(schema.node('heading'))), [], 'an empty heading has its own look');
  assert.deepEqual(marked(stateOf(schema.node('paragraph'), schema.node('paragraph'))), [], 'two lines are written lines');
});
