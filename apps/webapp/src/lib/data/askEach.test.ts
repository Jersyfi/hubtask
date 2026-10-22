// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Asking about a selection (UC-JUM-07 check 12): a few at a time, a shared refusal stops the rest,
// a refusal about one entry does not.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { IN_FLIGHT, askEach } from './askEach.ts';

const ids = Array.from({ length: 10 }, (_, i) => `entry-${i}`);

test('a selection is asked about a few at a time, every entry once', async () => {
  let inFlight = 0;
  let most = 0;
  const asked: string[] = [];
  const result = await askEach(ids, async (id) => {
    inFlight += 1;
    most = Math.max(most, inFlight);
    await new Promise((resolve) => setTimeout(resolve, 5));
    asked.push(id);
    inFlight -= 1;
  });
  assert.equal(result.asked, ids.length);
  assert.equal(result.stoppedBy, undefined);
  assert.deepEqual([...asked].sort(), [...ids].sort());
  assert.equal(most, IN_FLIGHT);
});

test('a refusal that holds for all stops the rest and is kept once', async () => {
  const asked: string[] = [];
  const refusal = Object.assign(new Error('refused'), { detailCode: 'ai.unavailable', params: { reason: 'budget_spent' } });
  const result = await askEach(ids, async (id) => {
    asked.push(id);
    if (id === 'entry-1') throw refusal;
  }, 1);
  assert.equal(result.stoppedBy, refusal);
  assert.deepEqual(asked, ['entry-0', 'entry-1']);
  assert.equal(result.asked, 1);
});

test('a refusal about one entry leaves the others to be asked', async () => {
  const settled = Object.assign(new Error('settled'), { detailCode: 'jumble.entry_already_settled' });
  const result = await askEach(ids, async (id) => {
    if (id === 'entry-3') throw settled;
  });
  assert.equal(result.stoppedBy, undefined);
  assert.equal(result.asked, ids.length - 1);
  assert.deepEqual(result.failed, [{ id: 'entry-3', error: settled }]);
});

test('nothing selected asks nothing', async () => {
  const result = await askEach([], async () => assert.fail('asked'));
  assert.deepEqual(result, { asked: 0, failed: [] });
});
