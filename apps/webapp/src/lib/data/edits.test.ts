// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { entryEditOf, type EntryDraft } from './edits.ts';

const opened: EntryDraft = { title: 'Order lunch', notes: 'for twelve', language: 'en' };

test('a form nobody touched writes nothing', () => {
  assert.deepEqual(entryEditOf(opened, opened), {});
  // A space typed and deleted again is not a change.
  assert.deepEqual(entryEditOf(opened, { ...opened, title: ' Order lunch ' }), {});
});

test('only the field that moved is in the write (issue 779)', () => {
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
