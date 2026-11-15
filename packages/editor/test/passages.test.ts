// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { Schema } from '@tiptap/pm/model';

import { blockWords, findPassage, selectedWords } from '../src/passages.ts';

const schema = new Schema({
  nodes: {
    doc: { content: 'block+' },
    paragraph: { group: 'block', content: 'inline*', attrs: { blockId: { default: null } } },
    text: { group: 'inline' },
  },
  marks: { strong: {} },
});

const page = schema.node('doc', null, [
  schema.node('paragraph', { blockId: 'intro' }, [schema.text('The budget is high.')]),
  schema.node('paragraph', { blockId: 'plan' }, [
    schema.text('We keep '), schema.text('the budget', [schema.mark('strong')]), schema.text(' for travel.'),
  ]),
]);

// UC-DOC-11 check 4: a comment on exactly the words selected finds them again in its block - across
// a change of formatting - and before the same words elsewhere on the page.
test('a passage is found in the block it names before anywhere else', () => {
  const range = findPassage(page, { id: 'c1', anchor: 'plan', quote: 'the budget on' });
  assert.equal(range, undefined, 'words that are not in the page are not found');
  const found = findPassage(page, { id: 'c1', anchor: 'plan', quote: 'the budget' });
  assert.ok(found);
  assert.equal(page.textBetween(found.from, found.to), 'the budget');
  assert.ok(found.from > page.child(0).nodeSize, 'the passage is in the named block, not the first');
});

test('a passage whose block is unknown is looked for in the whole page', () => {
  const found = findPassage(page, { id: 'c2', anchor: null, quote: 'budget is high' });
  assert.ok(found);
  assert.equal(page.textBetween(found.from, found.to), 'budget is high');
  assert.equal(findPassage(page, { id: 'c3', anchor: 'plan', quote: '   ' }), undefined);
});

test('the words selected inside one block are what a comment quotes; across blocks nothing', () => {
  assert.equal(selectedWords(page, 5, 11), 'budget');
  assert.equal(selectedWords(page, 5, 5), undefined);
  assert.equal(selectedWords(page, 5, page.child(0).nodeSize + 4), undefined);
});

const tables = new Schema({
  nodes: {
    doc: { content: 'block+' },
    paragraph: { group: 'block', content: 'inline*', attrs: { blockId: { default: null } } },
    table: { group: 'block', content: 'row+', attrs: { blockId: { default: null } } },
    row: { content: 'cell+' },
    cell: { content: 'paragraph+' },
    text: { group: 'inline' },
  },
});
const cell = (words: string) => tables.node('cell', null, [tables.node('paragraph', null, words ? [tables.text(words)] : [])]);
const grid = tables.node('doc', null, [
  tables.node('table', { blockId: 'roles' }, [
    tables.node('row', null, [cell('Baustein'), cell('Verantwortung')]),
    tables.node('row', null, [cell('Team'), cell('App')]),
  ]),
]);

// A comment on a table quotes its cells apart, and the passage is found again by those words; a
// quote written before, with the cells glued, is still found.
test('a table block is quoted cell by cell and found again', () => {
  const table = grid.child(0);
  assert.equal(blockWords(table), 'Baustein Verantwortung Team App');
  const found = findPassage(grid, { id: 'c4', anchor: 'roles', quote: 'Verantwortung Team' });
  assert.ok(found, 'the words across two cells are found');
  assert.equal(grid.textBetween(found.from, found.to, ' '), 'Verantwortung Team');
  assert.equal(selectedWords(grid, found.from, found.to), 'Verantwortung Team');
  assert.ok(findPassage(grid, { id: 'c5', anchor: 'roles', quote: 'BausteinVerantwortung' }), 'a glued quote of before');
});
