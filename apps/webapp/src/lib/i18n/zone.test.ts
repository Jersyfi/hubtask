// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { dateIn, instantOf, timeIn, todayIn, zoneName } from './zone.ts';

test('a wall clock in a zone becomes the instant behind it', () => {
  // Berlin is UTC+1 in winter and UTC+2 in summer, and the same wall clock is therefore two
  // different instants. A client that subtracted one fixed offset would be an hour out for half
  // the year.
  assert.equal(instantOf('2026-01-15', '09:00', 'Europe/Berlin'), '2026-01-15T08:00:00.000Z');
  assert.equal(instantOf('2026-07-15', '09:00', 'Europe/Berlin'), '2026-07-15T07:00:00.000Z');
  assert.equal(instantOf('2026-07-15', '09:00', 'UTC'), '2026-07-15T09:00:00.000Z');
});

test('the instant is solved for, not guessed at, across a daylight-saving boundary', () => {
  // 29 March 2026 is the morning Berlin springs forward. The hour before the jump and the hour
  // after it have different offsets, which is exactly what one subtraction gets wrong.
  assert.equal(instantOf('2026-03-29', '01:30', 'Europe/Berlin'), '2026-03-29T00:30:00.000Z');
  assert.equal(instantOf('2026-03-29', '03:30', 'Europe/Berlin'), '2026-03-29T01:30:00.000Z');
});

test('an all-day date is midnight in its own zone', () => {
  // Not midnight UTC: an all-day date set in São Paulo and stored as midnight in Greenwich would
  // be the previous evening there, which is the defect the zone exists to prevent.
  assert.equal(instantOf('2026-07-15', undefined, 'America/Sao_Paulo'), '2026-07-15T03:00:00.000Z');
});

test('an instant is read back as the date and time its own zone shows', () => {
  const instant = '2026-07-15T07:00:00.000Z';
  assert.equal(dateIn(instant, 'Europe/Berlin'), '2026-07-15');
  assert.equal(timeIn(instant, 'Europe/Berlin'), '09:00');
  // The same moment, four hours earlier on a São Paulo clock — and still the same day, which is
  // the case a client that only stored the instant would get right by luck.
  assert.equal(timeIn(instant, 'America/Sao_Paulo'), '04:00');
});

test('a date can be the day before somewhere else, and both readings are right', () => {
  // The whole reason the zone travels with the date.
  const instant = '2026-07-15T02:00:00.000Z';
  assert.equal(dateIn(instant, 'Europe/Berlin'), '2026-07-15');
  assert.equal(dateIn(instant, 'America/Sao_Paulo'), '2026-07-14');
});

test('midnight reads as 00:00 rather than 24:00', () => {
  // `hour12: false` answers 24 for midnight in some engines. The same moment, written in a way
  // that would sort wrongly and read oddly.
  assert.equal(timeIn('2026-07-15T00:00:00.000Z', 'UTC'), '00:00');
});

test('today is today where the reader is', () => {
  // Just after midnight in Berlin is still the previous day in São Paulo, and "today" has to
  // answer differently in the two.
  const justAfterMidnightInBerlin = Date.parse('2026-07-15T22:30:00.000Z');
  assert.equal(todayIn('Europe/Berlin', justAfterMidnightInBerlin), '2026-07-16');
  assert.equal(todayIn('America/Sao_Paulo', justAfterMidnightInBerlin), '2026-07-15');
});

test('a value that is not a date is not turned into one', () => {
  assert.equal(instantOf('not a date', '09:00', 'UTC'), undefined);
  assert.equal(dateIn('not an instant', 'UTC'), undefined);
  assert.equal(timeIn('not an instant', 'UTC'), undefined);
  // A zone name is the honest fallback when there is no instant to name it at.
  assert.equal(zoneName('not an instant', 'Europe/Berlin', 'en'), 'Europe/Berlin');
});
