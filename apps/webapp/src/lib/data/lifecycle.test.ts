// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { WorkItem } from '@hubtask/sync-engine';

import { archivalOfItem, isContainer, remainingDays } from './lifecycle.ts';

const entry = (extra: Partial<WorkItem> = {}): WorkItem =>
  ({ id: 'i', type: 'TASK', title: 'Buy milk', version: 1, ...extra }) as WorkItem;

test('an archived entry and an entry under an archived thing are different answers', () => {
  // Different offers, which is why they are different answers: the first has an unarchive control
  // here, the second has one on the thing above it and none here.
  assert.equal(archivalOfItem(entry(), false), 'active');
  assert.equal(archivalOfItem(entry({ archived_at: '2026-09-01T00:00:00Z' }), false), 'archived');
  assert.equal(archivalOfItem(entry(), true), 'inherited');
});

test('an entry archived in its own right stays archived inside an archived collection', () => {
  // Both are true, and the one that decides the control is its own: unarchiving the collection
  // leaves this entry archived, so the screen must not offer the collection's control for it.
  assert.equal(
    archivalOfItem(entry({ archived_at: '2026-09-01T00:00:00Z' }), true),
    'archived',
  );
});

const now = Date.parse('2026-09-10T12:00:00Z');

test('the remaining window is whole days, rounded up', () => {
  // Eleven hours left is a day left as far as a decision goes. Rounding down would tell somebody
  // they had none while restore still worked.
  assert.equal(remainingDays('2026-09-01T00:00:00Z', 30, now), 21);
  assert.equal(remainingDays('2026-09-10T01:00:00Z', 1, now), 1);
});

test('a row past its period has none left rather than a negative number', () => {
  // The retention job has not reached it yet. "−2 days" is not a sentence.
  assert.equal(remainingDays('2026-07-01T00:00:00Z', 30, now), 0);
});

test('an unknown period is unknown rather than thirty', () => {
  // The period is the workspace's configuration and reading it needs `retention:read`, which an
  // ordinary member does not have. A number invented here would be a promise this client cannot
  // keep — so the screen says when it was deleted and nothing it cannot stand behind.
  assert.equal(remainingDays('2026-09-01T00:00:00Z', undefined, now), undefined);
  assert.equal(remainingDays('not a date', 30, now), undefined);
});

test('which endpoint restores a row is the row’s own answer', () => {
  // The trash mixes containers and entries by design, so every row carries the kind. Guessing from
  // the subtype would be reading HUB and TASK out of a field the contract documents as free text.
  assert.equal(isContainer('CONTAINER'), true);
  assert.equal(isContainer('ITEM'), false);
});
