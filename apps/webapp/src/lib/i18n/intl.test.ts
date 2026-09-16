// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The four concerns F5-09 moved out of components and under `Intl`: the reader's order, the
// reader's characters, the reader's decimal mark, the reader's week. Each is a question with an
// answer a test can assert without a browser.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { byName } from './collation.ts';
import { decimalSeparatorOf, formatDecimal, parseDecimal } from './number.ts';
import { codePoints, graphemes, truncateGraphemes } from './text.ts';
import { firstWeekdayOf, weekStartKeyOf } from './week.ts';

test('names are ordered the way the reader reads them, not by code unit', () => {
  const names = ['Zoe', 'Ärzte', 'anna', 'Émile', 'Bob'];
  assert.deepEqual([...names].sort(), ['Bob', 'Zoe', 'anna', 'Ärzte', 'Émile'], 'code-unit order, for contrast');
  assert.deepEqual([...names].sort(byName('de')), ['anna', 'Ärzte', 'Bob', 'Émile', 'Zoe']);
  // Numeric: "entry 10" after "entry 9", which a code-unit order gets wrong.
  assert.deepEqual(['entry 10', 'entry 9'].sort(byName('en')), ['entry 9', 'entry 10']);
});

test('a cut falls between graphemes and never inside one', () => {
  const flagAndFamily = '🇩🇪👨‍👩‍👧x';
  assert.deepEqual(graphemes(flagAndFamily), ['🇩🇪', '👨‍👩‍👧', 'x']);
  assert.equal(truncateGraphemes(flagAndFamily, 1), '🇩🇪');
  assert.equal(truncateGraphemes(flagAndFamily, 2), '🇩🇪👨‍👩‍👧');
  assert.equal(truncateGraphemes('short', 80), 'short');
  // `slice` would have cut the flag in half; the family is 7 code points and 11 units.
  assert.equal(codePoints('👨‍👩‍👧'), 5);
  assert.equal('👨‍👩‍👧'.length, 8);
});

test('the decimal mark is the manifest’s, else Intl’s, else a dot', () => {
  assert.equal(decimalSeparatorOf('de', [{ locale: 'de', direction: 'ltr', decimal_separator: ',' }]), ',');
  assert.equal(decimalSeparatorOf('de'), ',');
  assert.equal(decimalSeparatorOf('en'), '.');
  assert.equal(decimalSeparatorOf('zz-not-a-locale'), '.');
});

test('1,5 under de is 1.5 to the contract, and a grouping separator is not guessed at', () => {
  assert.equal(parseDecimal('1,5', ','), 1.5);
  assert.equal(parseDecimal('1.5', ','), 1.5, 'a keypad dot still reads as the decimal mark');
  assert.equal(parseDecimal('-0,25', ','), -0.25);
  assert.equal(parseDecimal(' 42 ', ','), 42);
  assert.equal(parseDecimal('', ','), null, 'empty is no value');
  assert.equal(parseDecimal('1.500,5', ','), undefined, 'two marks is not a number');
  assert.equal(parseDecimal('abc', ','), undefined);
  // Under a dot the comma is a grouping mark, and a grouping mark is not guessed at either.
  assert.equal(parseDecimal('1,5', '.'), undefined);
  assert.equal(formatDecimal(1.5, 'de'), '1,5');
  assert.equal(formatDecimal(1234.5, 'de'), '1234,5', 'no grouping in a field being edited');
  assert.equal(formatDecimal(1.5, 'en'), '1.5');
});

test('the week starts where the account says, then the manifest, then the locale, then Monday', () => {
  const supported = [{ locale: 'en-US', direction: 'ltr' as const, week_start: 'SUNDAY' as const }];
  assert.equal(firstWeekdayOf('SATURDAY', 'en-US', supported), 6, 'the account wins');
  assert.equal(firstWeekdayOf(null, 'en-US', supported), 0, 'the manifest next');
  assert.equal(firstWeekdayOf(null, 'de', []), 1, 'the locale: Germany starts on Monday');
  // en-US by the locale alone, where the engine knows its week info.
  const byLocale = firstWeekdayOf(null, 'en-US', []);
  assert.ok(byLocale === 0 || byLocale === 1, 'Sunday where Intl reports week info, Monday where it does not');
  assert.equal(weekStartKeyOf('SUNDAY', 'de'), 'SU');
  assert.equal(weekStartKeyOf(null, 'de'), 'MO');
});
