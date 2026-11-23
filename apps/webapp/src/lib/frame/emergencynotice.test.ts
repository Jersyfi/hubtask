// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-ID-16 check 11: when the notice interrupts, what the bar's mark shows, what a hub's row says.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { EmergencyNotice } from '@hubtask/sync-engine';

import { confirmEach, hubMarkOf, mayInterrupt, noticeStateOf, waitingNotices, type Moment } from './emergencynotice.ts';

const NOW = Date.parse('2026-11-22T09:30:00Z');
const HOUR = 3_600_000;
const DAY = 24 * HOUR;

const notice = (id: string, openedAgo: number, acknowledged: string | null = null, hub = 'hub-a'): EmergencyNotice => ({
  id, hub_id: hub, opened_by: 'owner', reason: 'r',
  opened_at: new Date(NOW - openedAgo).toISOString(),
  ends_at: new Date(NOW - openedAgo + HOUR).toISOString(),
  acknowledged_at: acknowledged,
});

const calm: Moment = { waiting: 1, isSettled: true, isLayerOpen: false, isTyping: false, isTourOpen: false, isStarterOpen: false };

test('the notice interrupts once the frame has settled and nothing else holds the person', () => {
  assert.equal(mayInterrupt(calm), true);
});

test('the notice waits while nothing waits, the frame settles, a layer is open, the person types, a tour or the starter is on screen', () => {
  assert.equal(mayInterrupt({ ...calm, waiting: 0 }), false, 'nothing unconfirmed');
  assert.equal(mayInterrupt({ ...calm, isSettled: false }), false, 'the account or the workspace still arriving');
  assert.equal(mayInterrupt({ ...calm, isLayerOpen: true }), false, 'over a dialog the person opened');
  assert.equal(mayInterrupt({ ...calm, isTyping: true }), false, 'in the middle of typing');
  assert.equal(mayInterrupt({ ...calm, isTourOpen: true }), false, 'over the tour');
  assert.equal(mayInterrupt({ ...calm, isStarterOpen: true }), false, 'over the starter');
});

test('only unconfirmed notices of the last thirty days wait, and one shown since the app opened no longer does', () => {
  const notices = [notice('new', HOUR), notice('done', 2 * HOUR, '2026-11-22T09:00:00Z'), notice('old', 31 * DAY)];
  assert.deepEqual(waitingNotices(notices, new Set(), NOW).map((each) => each.id), ['new']);
  assert.deepEqual(waitingNotices(notices, new Set(['new']), NOW).map((each) => each.id), []);
});

test('two notices waiting are both shown, in one go', () => {
  const notices = [notice('a', HOUR, null, 'hub-a'), notice('b', 2 * DAY, null, 'hub-b')];
  assert.deepEqual(waitingNotices(notices, new Set(), NOW).map((each) => each.id), ['a', 'b']);
});

test('the mark is raised while a notice is unconfirmed, quiet once all are, absent without one', () => {
  assert.equal(noticeStateOf([notice('a', HOUR)], NOW), 'unconfirmed');
  assert.equal(noticeStateOf([notice('a', HOUR, '2026-11-22T09:10:00Z')], NOW), 'quiet', 'after a confirmation');
  assert.equal(noticeStateOf([notice('a', HOUR, '2026-11-22T09:10:00Z'), notice('b', DAY)], NOW), 'unconfirmed');
  assert.equal(noticeStateOf([], NOW), 'none');
  assert.equal(noticeStateOf([notice('a', 31 * DAY)], NOW), 'none', 'after thirty days');
});

test('a refused confirmation keeps the dot: the notices are unchanged and the refusal is answered', async () => {
  const notices = [notice('a', HOUR), notice('b', DAY)];
  const sent: string[] = [];
  const refusal = new Error('offline');
  const refused = await confirmEach(['a', 'b'], async (id) => {
    sent.push(id);
    if (id === 'a') throw refusal;
  });
  assert.equal(refused, refusal);
  assert.deepEqual(sent, ['a', 'b'], 'the other is still sent');
  assert.equal(noticeStateOf(notices, NOW), 'unconfirmed');
  assert.equal(await confirmEach(['a'], async () => undefined), undefined, 'a confirmation that lands answers nothing');
});

test('a hub says it was opened while the hour runs, then on which day, and nothing after thirty days', () => {
  const running = notice('a', 10 * 60_000);
  assert.deepEqual(hubMarkOf([running], 'hub-a', NOW), { notice: running, isRunning: true });
  const ended = notice('b', 3 * HOUR);
  assert.deepEqual(hubMarkOf([ended], 'hub-a', NOW), { notice: ended, isRunning: false });
  assert.equal(hubMarkOf([notice('c', 31 * DAY)], 'hub-a', NOW), undefined);
  assert.equal(hubMarkOf([running], 'hub-b', NOW), undefined, 'another hub');
  assert.equal(hubMarkOf([ended, running], 'hub-a', NOW)?.notice, running, 'the newest access speaks');
});
