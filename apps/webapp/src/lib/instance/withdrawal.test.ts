// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  NOTICE_DAYS,
  defaultWithdrawalDate,
  earliestWithdrawalDate,
  withdrawalMoment,
  withdrawalPhase,
} from './withdrawal.ts';

const noon = Date.parse('2026-10-02T12:00:00Z');

test('the default date is two weeks ahead, on the operator’s own calendar', () => {
  assert.equal(NOTICE_DAYS, 14);
  assert.equal(defaultWithdrawalDate('UTC', noon), '2026-10-16');
  // Late in the evening in Sydney it is already the next day there: the fortnight counts from it.
  assert.equal(defaultWithdrawalDate('Australia/Sydney', Date.parse('2026-10-02T14:00:00Z')), '2026-10-17');
});

test('an announcement names a day at least a day ahead: anything sooner is Withdraw now', () => {
  assert.equal(earliestWithdrawalDate('UTC', noon), '2026-10-04');
  // Late in the evening, tomorrow's midnight is a few hours away; the earliest day is still a day out.
  assert.equal(earliestWithdrawalDate('UTC', Date.parse('2026-10-02T23:00:00Z')), '2026-10-04');
});

test('a chosen date ends the offer when that day begins where the operator is', () => {
  assert.equal(withdrawalMoment('2026-10-16', 'UTC'), '2026-10-16T00:00:00.000Z');
  assert.equal(withdrawalMoment('2026-10-16', 'Europe/Berlin'), '2026-10-15T22:00:00.000Z');
  assert.equal(withdrawalMoment('not a date', 'UTC'), undefined);
});

test('a row is offered, being withdrawn, or withdrawn', () => {
  assert.equal(withdrawalPhase({ enabled: true, withdraw_at: null }, noon), 'offered');
  assert.equal(withdrawalPhase({ enabled: true }, noon), 'offered');
  assert.equal(withdrawalPhase({ enabled: true, withdraw_at: '2026-10-16T00:00:00Z' }, noon), 'withdrawing');
  assert.equal(withdrawalPhase({ enabled: true, withdraw_at: '2026-10-02T12:00:00Z' }, noon), 'withdrawn');
  // An offer an older binary ended through the form is withdrawn too, and cancelling restores it.
  assert.equal(withdrawalPhase({ enabled: false, withdraw_at: null }, noon), 'withdrawn');
});
