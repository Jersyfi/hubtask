// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { keepThenReplace, type Listed } from './restore.ts';

function steps(listed: Listed[]) {
  const done: string[] = [];
  return {
    done,
    save: async () => void done.push('save'),
    versions: async () => listed,
    name: async (seq: number, name: string) => void done.push(`name ${seq} ${name}`),
    replace: () => (done.push('replace'), true),
  };
}

test('what stands is saved and named before a version replaces it, so the history keeps it (UC-CNV-06 check 3)', async () => {
  const s = steps([{ seq: 7, name: null }, { seq: 3, name: null }]);
  assert.equal(await keepThenReplace(s, 3, 'Before bringing back 3'), true);
  assert.deepEqual(s.done, ['save', 'name 7 Before bringing back 3', 'replace', 'save']);
});

test('a newest version the person already named keeps its name', async () => {
  const s = steps([{ seq: 7, name: 'After the workshop' }]);
  await keepThenReplace(s, 3, 'Before bringing back 3');
  assert.deepEqual(s.done, ['save', 'replace', 'save']);
});
