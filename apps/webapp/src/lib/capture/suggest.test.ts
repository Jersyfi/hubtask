// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { offersFor, pick, tokenAt } from './suggest.ts';

test('the token is the sigil and what is typed after it, up to the caret', () => {
  assert.deepEqual(tokenAt('Texte schreiben @Web', 20), { sigil: '@', query: 'Web', start: 16, end: 20 });
  assert.deepEqual(tokenAt('#', 1), { sigil: '#', query: '', start: 0, end: 1 });
  assert.deepEqual(tokenAt('call +Ma', 8), { sigil: '+', query: 'Ma', start: 5, end: 8 });
  // A sigil inside a word is not a token: an address, a sum.
  assert.equal(tokenAt('mail j@example.org', 18), undefined);
  assert.equal(tokenAt('2+2', 3), undefined);
  // A space ends the token: the caret is past it.
  assert.equal(tokenAt('@Web Relaunch', 13), undefined);
});

test('the names that start with it come first, then a word that does, then any that hold it', () => {
  const candidates = [
    { id: '1', name: 'Shopping' },
    { id: '2', name: 'Website-Relaunch', hint: 'Agentur' },
    { id: '3', name: 'Our web shop' },
    { id: '4', name: 'Cobweb' },
  ];
  assert.deepEqual(offersFor('web', candidates).map((each) => each.id), ['2', '3', '4']);
  assert.deepEqual(offersFor('', candidates, 2).map((each) => each.id), ['1', '2']);
  // Accents and case do not decide.
  assert.deepEqual(offersFor('ein', [{ id: 'e', name: 'Einkäufe' }, { id: 'x', name: 'Rein' }]).map((each) => each.id), ['e', 'x']);
  assert.deepEqual(offersFor('kaufe', [{ id: 'e', name: 'Einkäufe' }]).map((each) => each.id), ['e']);
});

test('picking writes the whole name and a space, and puts the caret after it', () => {
  const text = 'Texte schreiben @Web morgen';
  const token = tokenAt(text, 20);
  assert.ok(token);
  assert.deepEqual(pick(text, token, 'Website Relaunch'), { text: 'Texte schreiben @Website Relaunch morgen', caret: 34 });
});
