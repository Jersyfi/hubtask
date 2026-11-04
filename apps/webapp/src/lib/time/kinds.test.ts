// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SOURCE } from '../i18n/catalogue.ts';
import { DEFAULT_KINDS, owesDefaultKinds } from './kinds.ts';

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
