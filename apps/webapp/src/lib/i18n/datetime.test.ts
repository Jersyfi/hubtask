// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { formatDateTime, formatDue, formatRelative } from './datetime.ts';

test('a moment is written the way the locale writes moments', () => {
  // Not the exact string — that is the platform's and it moves between ICU versions. What is
  // asserted is that the two locales disagree, which is the whole reason this goes through Intl.
  const at = '2026-09-04T15:30:00Z';
  const english = formatDateTime(at, 'en-GB');
  const german = formatDateTime(at, 'de-DE');
  assert.notEqual(english, german);
  assert.match(english, /2026/);
  assert.match(german, /2026/);
});

test('a value this cannot read is shown as it arrived', () => {
  // Never a blank and never `Invalid Date`: the reader sees something they can quote.
  assert.equal(formatDateTime('not a moment', 'en'), 'not a moment');
  assert.equal(formatDateTime('', 'en'), '');
});

test('an all-day due date shows no time, in any locale', () => {
  // The flag is what says the instant is a day rather than a moment. A time drawn for one would be
  // a midnight that shifts with the viewer, which is the defect the flag exists to prevent.
  for (const locale of ['en', 'de', 'ja']) {
    const drawn = formatDue('2026-07-14T22:00:00Z', locale, 'Europe/Berlin', { allDay: true });
    assert.ok(!/\d\d:\d\d/.test(drawn), `${locale} drew a time: ${drawn}`);
  }
});

test('a timed due date is drawn on its own zone’s clock, and names it when asked', () => {
  const instant = '2026-07-15T07:00:00Z';
  // German writes a 24-hour clock, which is what makes the hour readable in an assertion. English
  // writes the same moment as "9:00 AM", and both are the locale's business rather than this
  // client's — which is the reason the formatting goes through Intl at all.
  assert.match(formatDue(instant, 'de', 'Europe/Berlin'), /09:00/);
  assert.match(formatDue(instant, 'de', 'America/Sao_Paulo'), /04:00/);
  assert.match(formatDue(instant, 'en', 'Europe/Berlin'), /9:00\s?AM/);
  // The zone is said only where it is worth saying — a date in the reader's own would be noise.
  assert.ok(formatDue(instant, 'en', 'Europe/Berlin', { showZone: true }).length >
    formatDue(instant, 'en', 'Europe/Berlin').length);
});

test('how far away it is comes from Intl, not from a phrase assembled here', () => {
  const now = Date.parse('2026-07-15T12:00:00Z');
  assert.match(formatRelative('2026-07-18T12:00:00Z', 'en', now), /3 days/);
  assert.match(formatRelative('2026-07-15T10:00:00Z', 'en', now), /2 hours ago/);
  // A different language words it differently, which is the whole reason it goes through Intl.
  assert.notEqual(
    formatRelative('2026-07-18T12:00:00Z', 'de', now),
    formatRelative('2026-07-18T12:00:00Z', 'en', now),
  );
});

test('a value that is not an instant is shown as it arrived', () => {
  assert.equal(formatDue('not an instant', 'en', 'UTC'), 'not an instant');
  assert.equal(formatRelative('not an instant', 'en'), 'not an instant');
});
