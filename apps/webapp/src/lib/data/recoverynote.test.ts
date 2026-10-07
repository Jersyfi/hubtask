// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-ID-02 check 6: after a sign-in with a recovery code the first page carries a note with
// the number left - and it stays in the tab across a reload until it is closed or the authenticator
// is replaced, because the number arrived with this sign-in and with nothing after it.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { dropNote, holdNote, readNote } from './recoverynote.ts';

function memory(): Pick<Storage, 'getItem' | 'setItem' | 'removeItem'> {
  const held = new Map<string, string>();
  return {
    getItem: (key) => held.get(key) ?? null,
    setItem: (key, value) => void held.set(key, value),
    removeItem: (key) => void held.delete(key),
  };
}

test('the note holds the number until it is dropped', () => {
  const store = memory();
  assert.equal(readNote(store), undefined);
  holdNote(store, 7);
  assert.deepEqual(readNote(store), { left: 7 });
  assert.deepEqual(readNote(store), { left: 7 }, 'reading is not taking: a reload shows it again');
  dropNote(store);
  assert.equal(readNote(store), undefined);
});

test('zero is a number to show, and nonsense is no note', () => {
  const store = memory();
  holdNote(store, 0);
  assert.deepEqual(readNote(store), { left: 0 });
  store.setItem('hubtask.recovery_note', 'not json');
  assert.equal(readNote(store), undefined);
  store.setItem('hubtask.recovery_note', JSON.stringify({ left: -3 }));
  assert.equal(readNote(store), undefined);
});
