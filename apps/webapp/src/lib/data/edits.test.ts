// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { entryEditOf, inlineWriteOf, type EntryDraft } from './edits.ts';

const opened: EntryDraft = { title: 'Order lunch', notes: 'for twelve', language: 'en' };

test('a form nobody touched writes nothing', () => {
  assert.deepEqual(entryEditOf(opened, opened), {});
  // A space typed and deleted again is not a change.
  assert.deepEqual(entryEditOf(opened, { ...opened, title: ' Order lunch ' }), {});
});

test('only the field that moved is in the write', () => {
  assert.deepEqual(entryEditOf(opened, { ...opened, notes: 'for thirteen' }), { notes: 'for thirteen' });
  assert.deepEqual(entryEditOf(opened, { ...opened, title: 'Order dinner' }), { title: 'Order dinner' });
  assert.deepEqual(entryEditOf(opened, { ...opened, language: 'de' }), { content_language: 'de' });
  assert.deepEqual(
    entryEditOf(opened, { title: 'Order dinner', notes: 'for thirteen', language: 'de' }),
    { title: 'Order dinner', notes: 'for thirteen', content_language: 'de' },
  );
});

test('emptied notes and an emptied language clear rather than write the empty string', () => {
  assert.deepEqual(entryEditOf(opened, { ...opened, notes: '   ' }), { notes: null });
  assert.deepEqual(entryEditOf(opened, { ...opened, language: '' }), { content_language: null });
  // Notes that were empty and stay empty - whatever the whitespace - are untouched.
  assert.deepEqual(entryEditOf({ ...opened, notes: '' }, { ...opened, notes: '  ' }), {});
  // Notes written where there were none.
  assert.deepEqual(entryEditOf({ ...opened, notes: '' }, { ...opened, notes: 'new' }), { notes: 'new' });
});

// UC-SYN-02 check 10: each field decided against what it was when the person began.
test('an edit in place writes only what the person changed, and stops where both changed it', () => {
  const base: EntryDraft = { title: 'Order lunch', notes: 'for twelve', language: 'en' };
  const cases: { name: string; mine: Partial<EntryDraft>; theirs: Partial<EntryDraft>; edit: object; conflicts: string[] }[] = [
    { name: 'own change of the notes only', mine: { notes: 'for thirteen' }, theirs: {}, edit: { notes: 'for thirteen' }, conflicts: [] },
    { name: 'own change of the title only', mine: { title: 'Order dinner' }, theirs: {}, edit: { title: 'Order dinner' }, conflicts: [] },
    { name: 'a foreign title only: not sent back', mine: {}, theirs: { title: 'Order brunch' }, edit: {}, conflicts: [] },
    { name: 'a foreign title while the notes were typed', mine: { notes: 'for thirteen' }, theirs: { title: 'Order brunch' }, edit: { notes: 'for thirteen' }, conflicts: [] },
    { name: 'a foreign notes change while the title was typed', mine: { title: 'Order dinner' }, theirs: { notes: 'for six' }, edit: { title: 'Order dinner' }, conflicts: [] },
    { name: 'both, the same notes', mine: { notes: 'for six' }, theirs: { notes: 'for six' }, edit: {}, conflicts: [] },
    { name: 'both, the same title', mine: { title: 'Order brunch' }, theirs: { title: ' Order brunch' }, edit: {}, conflicts: [] },
    { name: 'both, different notes', mine: { notes: 'for thirteen' }, theirs: { notes: 'for six' }, edit: {}, conflicts: ['notes'] },
    { name: 'both, different titles', mine: { title: 'Order dinner' }, theirs: { title: 'Order brunch' }, edit: {}, conflicts: ['title'] },
    { name: 'notes emptied against notes rewritten', mine: { notes: '' }, theirs: { notes: 'for six' }, edit: {}, conflicts: ['notes'] },
    { name: 'a conflict on one field holds the other back', mine: { title: 'Order dinner', notes: 'for thirteen' }, theirs: { notes: 'for six' }, edit: { title: 'Order dinner' }, conflicts: ['notes'] },
  ];
  for (const each of cases) {
    const written = inlineWriteOf(base, { ...base, ...each.mine }, { ...base, ...each.theirs });
    assert.deepEqual(written.edit, each.edit, each.name);
    assert.deepEqual(written.conflicts, each.conflicts, each.name);
  }
});
