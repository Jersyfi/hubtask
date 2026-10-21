// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { fold, score } from './rank.ts';

test('the label starting with the words outranks a word starting with them, which outranks inside', () => {
  assert.equal(score('Kitchen', 'kitch'), 3);
  assert.equal(score('Old kitchen', 'kitch'), 2);
  assert.equal(score('Mykitchen', 'kitch'), 1);
  assert.equal(score('Knit chairs', 'kitch'), 0);
});

test('case and accents are folded, so "buro" finds "Büro"', () => {
  assert.equal(score('Büro', 'buro'), 3);
  assert.equal(score('BÜRO', 'Bü'), 3);
  assert.equal(fold('Ärger'), 'arger');
});

test('a word boundary is anything that is not a letter or a digit', () => {
  assert.equal(score('House / Kitchen', 'kit'), 2);
  assert.equal(score('Re-kitchen', 'kit'), 2);
  assert.equal(score('a1kitchen', 'kit'), 1);
});

test('a later occurrence at a word start still counts as one', () => {
  assert.equal(score('mykit kitchen', 'kit'), 2);
});

test('empty words match nothing, and runs of spaces are one', () => {
  assert.equal(score('Kitchen', ''), 0);
  assert.equal(score('Kitchen', '   '), 0);
  assert.equal(score('New  task', 'new task'), 3);
});

test('the reader\'s language decides the fold', () => {
  assert.equal(score('İstanbul', 'i', 'tr'), 3);
});
