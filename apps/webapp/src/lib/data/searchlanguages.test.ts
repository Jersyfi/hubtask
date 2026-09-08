// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { baseTagOf, shouldWiden, widenTo } from './searchlanguages.ts';

test('a language is compared by its base tag, because an index is', () => {
  assert.equal(baseTagOf('de-DE'), 'de');
  assert.equal(baseTagOf('EN'), 'en');
  assert.equal(baseTagOf(''), undefined);
  assert.equal(baseTagOf(undefined), undefined);
});

test('the reader’s own language is left out — the first search already asked it', () => {
  assert.deepEqual(widenTo('de', ['en', 'de', 'fr']), ['en', 'fr']);
  // A tag that differs only by region is the same language to an index.
  assert.deepEqual(widenTo('de-DE', ['en', 'de', 'fr']), ['en', 'fr']);
  assert.deepEqual(widenTo('en-GB', ['en-US', 'de']), ['de']);
});

test('the order is the installation’s, and each language is asked once', () => {
  assert.deepEqual(widenTo(undefined, ['fr', 'en', 'fr-CA', 'de']), ['fr', 'en', 'de']);
});

test('an installation with one language has nothing to widen to', () => {
  assert.deepEqual(widenTo('de', ['de']), []);
  assert.deepEqual(widenTo('de', []), []);
});

test('widening happens only after a silence, and only when nobody chose', () => {
  const wider = ['en'];
  assert.equal(shouldWiden({ found: 0, chosenLanguage: undefined, wider }), true);
  // Something was found under the reader's own language: that is the answer they wanted, and
  // widening would put worse matches under better ones.
  assert.equal(shouldWiden({ found: 3, chosenLanguage: undefined, wider }), false);
  // Somebody who chose a language asked a precise question. Answering a wider one ignores it.
  assert.equal(shouldWiden({ found: 0, chosenLanguage: 'fr', wider }), false);
  // Nothing else to ask.
  assert.equal(shouldWiden({ found: 0, chosenLanguage: undefined, wider: [] }), false);
});
