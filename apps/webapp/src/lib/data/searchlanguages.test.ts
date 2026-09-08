// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  baseTagOf,
  canOfferRest,
  readerLanguages,
  remainingLanguages,
  shouldWiden,
} from './searchlanguages.ts';

/** What the walked installation indexes: thirty tags, alphabetically, meaning nothing. */
const INSTALLATION = [
  'ar', 'ca', 'da', 'de', 'el', 'en', 'es', 'eu', 'fi', 'fr', 'ga', 'hi', 'hu', 'hy', 'id',
  'it', 'lt', 'nb', 'ne', 'nl', 'nn', 'no', 'pt', 'ro', 'ru', 'sr', 'sv', 'ta', 'tr', 'yi',
];

test('a language is compared by its base tag, because an index is', () => {
  assert.equal(baseTagOf('de-DE'), 'de');
  assert.equal(baseTagOf('EN'), 'en');
  assert.equal(baseTagOf(''), undefined);
  assert.equal(baseTagOf(undefined), undefined);
});

test('the automatic widening is the reader’s own languages, and only the indexed ones', () => {
  // A hit in a language somebody reads is a hit they can act on. One their browser lists and this
  // installation cannot index is not offered, because it cannot be searched at all.
  assert.deepEqual(readerLanguages('de', INSTALLATION, ['de-DE', 'en-GB', 'ja']), ['en']);
  // The reader's own language is what the first search already asked.
  assert.deepEqual(readerLanguages('de', INSTALLATION, ['de', 'de-AT']), []);
  assert.deepEqual(readerLanguages('de', INSTALLATION, []), []);
});

test('R-08’s reader has only German, so nothing is asked automatically', () => {
  // The browser in the evidence was `['de', 'de-DE']`. That is the case the offered search exists
  // for: there is nothing to widen to on their behalf, and thirty languages is not something to
  // spend without being asked.
  assert.deepEqual(readerLanguages('de', INSTALLATION, ['de', 'de-DE']), []);
  const rest = remainingLanguages('de', INSTALLATION, []);
  assert.equal(rest.length, 29);
  assert.ok(rest.includes('en'));
});

test('the rest is everything else, once each, and never what was already asked', () => {
  const automatic = readerLanguages('de', INSTALLATION, ['en-GB']);
  const rest = remainingLanguages('de', INSTALLATION, automatic);
  assert.equal(rest.includes('en'), false, 'already asked');
  assert.equal(rest.includes('de'), false, 'the reader’s own');
  assert.equal(rest.length, 28);
});

test('a subset of an alphabetical list would have been a coin flip', () => {
  // The measurement that decided the design: English sits at position 5 of 29 on the installation
  // walked — inside a cap of five by luck, and outside it the moment an installation indexes one
  // more language beginning with a, b, c or d.
  const rest = remainingLanguages('de', INSTALLATION, []);
  assert.equal(rest.indexOf('en'), 4);
});

test('widening happens only after a silence, and only when nobody chose', () => {
  const wider = ['en'];
  assert.equal(shouldWiden({ found: 0, chosenLanguage: undefined, wider }), true);
  // Something was found under the reader's own language: that is the answer they wanted, and
  // widening would put worse matches under better ones.
  assert.equal(shouldWiden({ found: 3, chosenLanguage: undefined, wider }), false);
  // Somebody who chose a language asked a precise question. Answering a wider one ignores it.
  assert.equal(shouldWiden({ found: 0, chosenLanguage: 'fr', wider }), false);
  assert.equal(shouldWiden({ found: 0, chosenLanguage: undefined, wider: [] }), false);
});

test('the rest is offered under the same three conditions', () => {
  const remaining = ['en', 'fr'];
  assert.equal(canOfferRest({ found: 0, chosenLanguage: undefined, remaining }), true);
  assert.equal(canOfferRest({ found: 2, chosenLanguage: undefined, remaining }), false);
  assert.equal(canOfferRest({ found: 0, chosenLanguage: 'en', remaining }), false);
  assert.equal(canOfferRest({ found: 0, chosenLanguage: undefined, remaining: [] }), false);
});
