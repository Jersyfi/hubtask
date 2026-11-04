// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { isBelowUse, limitStanding, useOf, countByLimit, narrowedByLimit } from './quotause.ts';

const entry = (quotas: { quota: 'items' | 'accounts'; used?: number; limit: number }[]) => ({
  tenant_id: '01a0e2e0-0000-7000-8000-0000000005a1', slug: 'house', quotas,
});

test('a workspace is at a limit where one quota is used up, near one at 90% or more', () => {
  assert.equal(limitStanding(undefined), undefined);
  assert.equal(limitStanding(entry([])), undefined);
  assert.equal(limitStanding(entry([{ quota: 'items', used: 95, limit: 100 }])), 'near');
  assert.equal(limitStanding(entry([{ quota: 'items', used: 95, limit: 100 }, { quota: 'accounts', used: 10, limit: 10 }])), 'at');
});

test('the use of a quota is read where answered, and a ceiling below it is noticed', () => {
  const house = entry([{ quota: 'items', used: 95, limit: 100 }]);
  assert.equal(useOf(house, 'items'), 95);
  assert.equal(useOf(house, 'accounts'), undefined);
  assert.equal(isBelowUse(90, 95), true);
  assert.equal(isBelowUse(95, 95), false);
  assert.equal(isBelowUse(undefined, 95), false);
  assert.equal(isBelowUse(10, undefined), false);
});

// The workspaces' *Limits* filter (user-experience.md §8.4): near takes in those at a limit too,
// at only those with a quota used up; how many there are in all is the installation's answer.
test('the limits filter keeps the workspaces it names and counts them all', () => {
  const near = { ...entry([{ quota: 'items', limit: 100, used: 92 }]), tenant_id: 'a' };
  const at = { ...entry([{ quota: 'items', limit: 100, used: 100 }]), tenant_id: 'b' };
  const byId = new Map([['a', near], ['b', at]]);
  const rows = [{ id: 'a' }, { id: 'b' }, { id: 'c' }];
  assert.deepEqual(narrowedByLimit(rows, byId, '').map((row) => row.id), ['a', 'b', 'c']);
  assert.deepEqual(narrowedByLimit(rows, byId, 'near').map((row) => row.id), ['a', 'b']);
  assert.deepEqual(narrowedByLimit(rows, byId, 'at').map((row) => row.id), ['b']);
  assert.equal(countByLimit([near, at], 'near'), 2);
  assert.equal(countByLimit([near, at], 'at'), 1);
});
