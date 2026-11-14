// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SOURCE } from '../i18n/source.ts';
import { DEFAULT_KINDS, isTimeUnused, owesDefaultKinds, writeDefaultKinds } from './kinds.ts';

// user-experience.md §4: the workspace's kinds of work are there on first use, so the timer starts.
test('a workspace without a kind of work is given the usual ones by whoever may shape it', () => {
  assert.equal(owesDefaultKinds([], true), true);
  assert.equal(owesDefaultKinds(undefined, true), false, 'nothing is written before the kinds are read');
  assert.equal(owesDefaultKinds([], false), false, 'a reader who may not shape the workspace writes nothing');
  assert.equal(owesDefaultKinds([{ key: 'x', archived: true }], true), false, 'a workspace that had kinds keeps its own choice');
});

test('every usual kind has a key the server accepts and words in the catalogue', () => {
  assert.ok(DEFAULT_KINDS.length >= 3);
  for (const kind of DEFAULT_KINDS) {
    assert.match(kind.key, /^[a-z][a-z0-9_]{0,39}$/u);
    assert.ok(kind.code in SOURCE, `${kind.code} has no words`);
  }
});

// Scene emptytime: Time switched on and never used shows its start page, and writes nothing yet.
test('time is unused while it has no kind of work at all and the week holds nothing', () => {
  assert.equal(isTimeUnused([], 0), true);
  assert.equal(isTimeUnused(undefined, 0), false, 'not before the kinds are read');
  assert.equal(isTimeUnused([], undefined), false, 'not before the week is read');
  assert.equal(isTimeUnused([], 2), false, 'a week with time is in use');
  assert.equal(isTimeUnused([{ key: 'x', archived: true }], 0), false, 'a workspace that had kinds is in use');
});

test('the usual kinds are written in their order, in the reader\'s words', async () => {
  const written: { key: string; name: string; position: number }[] = [];
  await writeDefaultKinds(async (key, fields) => { written.push({ key, name: fields.name, position: fields.position }); }, (code) => `«${code}»`);
  assert.deepEqual(written.map((each) => each.key), DEFAULT_KINDS.map((each) => each.key));
  assert.deepEqual(written.map((each) => each.position), DEFAULT_KINDS.map((_, at) => at + 1));
  assert.equal(written[0]?.name, `«${DEFAULT_KINDS[0]?.code}»`);
});
