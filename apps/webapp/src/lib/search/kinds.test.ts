// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { kindsOffered, nameMatches, shows } from './kinds.ts';

// user-experience.md §5.3: one search for every kind, each kind only where its module is on.
test('the kinds offered follow the modules switched on', () => {
  assert.deepEqual(kindsOffered([]), ['all', 'entries', 'places']);
  assert.deepEqual(kindsOffered(['KNOWLEDGE', 'WHITEBOARD']), ['all', 'entries', 'pages', 'boards', 'places']);
  assert.deepEqual(kindsOffered(undefined), ['all', 'entries', 'places']);
  // Goals and configuration items were not searched at all: "FortiGate" found nothing.
  assert.deepEqual(kindsOffered(['GOALS', 'ASSETS']), ['all', 'entries', 'goals', 'assets', 'places']);
});

test('all shows every kind, a kind only itself', () => {
  assert.equal(shows('all', 'pages'), true);
  assert.equal(shows('pages', 'pages'), true);
  assert.equal(shows('entries', 'pages'), false);
});

test('a name matches when it holds every word, whatever the case and the accents', () => {
  assert.equal(nameMatches('Rétro of the autumn release', 'retro AUTUMN'), true);
  assert.equal(nameMatches('Kick-off', 'kick off'), true);
  assert.equal(nameMatches('Kick-off', 'retro'), false);
  assert.equal(nameMatches('Kick-off', '   '), false);
});
