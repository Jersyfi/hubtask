// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { holdersByRole } from './roleholders.ts';

test('each role counts its people and its groups once', () => {
  const held = holdersByRole([
    { role: 'OWNER', account_id: 'a' },
    { role: 'MEMBER', account_id: 'b' },
    { role: 'MEMBER', account_id: 'c' },
    { role: 'MEMBER', account_id: 'c' },
    { role: 'MEMBER', group_id: 'g' },
  ], true);
  assert.deepEqual(held?.get('OWNER'), { people: 1, groups: 0 });
  assert.deepEqual(held?.get('MEMBER'), { people: 2, groups: 1 });
  assert.equal(held?.get('VIEWER'), undefined, 'a role nobody holds has no entry');
});

test('a first page is no count', () => {
  assert.equal(holdersByRole([{ role: 'OWNER', account_id: 'a' }], false), undefined);
});
