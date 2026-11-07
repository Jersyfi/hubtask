// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { Schema } from '@tiptap/pm/model';
import { EditorState } from '@tiptap/pm/state';

import { MIRROR_META, linkLine, mirrorTicks, taskLineAt, taskLines, tickedEntries } from '../src/tasks.ts';

const ENTRY = '00000000-0000-0000-0000-0000000000e1';

const schema = new Schema({
  nodes: {
    doc: { content: 'block+' },
    paragraph: { group: 'block', content: 'inline*' },
    taskList: { group: 'block', content: 'taskItem+' },
    taskItem: { content: 'paragraph block*', attrs: { checked: { default: false } } },
    reference: { group: 'inline', inline: true, atom: true, attrs: { kind: { default: 'entry' }, id: { default: '' }, anchor: { default: null } } },
    text: { group: 'inline' },
  },
});

const page = () => EditorState.create({
  schema,
  doc: schema.node('doc', null, [
    schema.node('paragraph', null, [schema.text('Before the trip')]),
    schema.node('taskList', null, [
      schema.node('taskItem', { checked: false }, [schema.node('paragraph', null, [schema.text('Book the train')])]),
      schema.node('taskItem', { checked: true }, [schema.node('paragraph', null, [schema.text('Pack')])]),
    ]),
  ]),
});

// UC-DOC-03 check 1: the line the caret stands in becomes a mention of the entry it was turned
// into, so it reads as the entry's title from then on and the page names the entry.
test('a checklist line is linked to the entry it became', () => {
  const state = page();
  const inLine = state.doc.child(0).nodeSize + 4;
  const line = taskLineAt(state.doc, inLine);
  assert.ok(line);
  assert.equal(line.text, 'Book the train');
  assert.equal(line.entryId, undefined);
  assert.equal(taskLineAt(state.doc, 3), undefined, 'a paragraph outside a checklist is no line');

  const tr = linkLine(state, line, ENTRY);
  assert.ok(tr);
  const linked = state.apply(tr);
  const lines = taskLines(linked.doc);
  assert.equal(lines[0]?.entryId, ENTRY);
  assert.equal(lines[1]?.entryId, undefined);
});

// UC-DOC-03 check 2: the tick mirrors the entry both ways - the page draws the entry's completion,
// and ticking the line asks for the entry to be completed; the mirror itself asks for nothing.
test('a linked line\'s tick mirrors the entry\'s completion', () => {
  const state = page();
  const line = taskLineAt(state.doc, state.doc.child(0).nodeSize + 4)!;
  const linked = state.apply(linkLine(state, line, ENTRY)!);

  const mirror = mirrorTicks(linked, new Map([[ENTRY, true]]));
  assert.ok(mirror);
  assert.equal(mirror.getMeta(MIRROR_META), true);
  const mirrored = linked.apply(mirror);
  assert.equal(taskLines(mirrored.doc)[0]?.checked, true);
  assert.equal(mirrorTicks(mirrored, new Map([[ENTRY, true]])), undefined, 'a tick that agrees is left alone');

  const unticked = mirrored.apply(mirrored.tr.setNodeMarkup(taskLines(mirrored.doc)[0]!.pos, undefined, { checked: false }));
  assert.deepEqual(tickedEntries(mirrored.doc, unticked.doc), [{ entryId: ENTRY, checked: false }]);
  assert.deepEqual(tickedEntries(linked.doc, linked.doc), []);
});
