// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { CodeLink } from '@hubtask/sync-engine';

import { codeGroups, codeHref, codeIntakeAddress, codeRef, codeTone } from './code.ts';

const link = (over: Partial<CodeLink>): CodeLink => ({
  id: 'l', item_id: 'i', hub_id: 'h', collection_id: 'c', hook_id: 'k', kind: 'COMMIT', repository: 'team/web',
  ref: '0123456789abcdef', title: 't', url: '', author: '', state: 'PUSHED', happened_at: '2026-10-30T09:00:00Z',
  updated_at: '2026-10-30T09:00:00Z', ...over,
});

// UC-INT-13 check 1: the address a forge is given names the workspace and the hook.
test('a hook\'s address names the workspace and the hook', () => {
  assert.equal(codeIntakeAddress('https://app.example/', 't-1', 'h-1'), 'https://app.example/api/v1/code-intake/t-1/h-1');
});

// UC-INT-13 check 3: a link reads by its kind, its state as a tone, and opens only a web address.
test('a code link reads by its kind and opens only a web address', () => {
  assert.equal(codeRef(link({})), '01234567');
  assert.equal(codeRef(link({ kind: 'PULL_REQUEST', ref: '7' })), '#7');
  assert.equal(codeRef(link({ kind: 'BRANCH', ref: 'feature/WEB-42' })), 'feature/WEB-42');
  assert.equal(codeTone('MERGED'), 'success');
  assert.equal(codeTone('OPEN'), 'info');
  assert.equal(codeHref('javascript:alert(1)'), undefined);
  assert.equal(codeHref('https://forge.example/pr/7'), 'https://forge.example/pr/7');
  const groups = codeGroups([link({ id: 'a' }), link({ id: 'b', kind: 'PULL_REQUEST' }), link({ id: 'c', kind: 'BRANCH' })]);
  assert.deepEqual(groups.map((g) => g.kind), ['PULL_REQUEST', 'BRANCH', 'COMMIT']);
});
