// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { foundWords, kindsOffered, nameMatches, shows } from './kinds.ts';

// user-experience.md §5.3: one search for every kind, each kind only where its module is on.
test('the kinds offered follow the modules switched on', () => {
  assert.deepEqual(kindsOffered([]), ['all', 'entries', 'places']);
  assert.deepEqual(kindsOffered(['KNOWLEDGE', 'WHITEBOARD']), ['all', 'entries', 'pages', 'boards', 'places']);
  assert.deepEqual(kindsOffered(undefined), ['all', 'entries', 'places']);
  // Goals and configuration items were not searched at all: "FortiGate" found nothing.
  assert.deepEqual(kindsOffered(['ASSETS']), ['all', 'entries', 'assets', 'places']);
  // Portfolios, dashboards, releases and sprints were not searched either.
  assert.deepEqual(kindsOffered(['GOALS', 'PLANNING']), ['all', 'entries', 'goals', 'portfolios', 'dashboards', 'releases', 'sprints', 'places']);
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

// What the search says it found counts what it found: entries are named as entries only where
// nothing else is among the results; a page or an asset is a result, not an entry.
test('the count names entries only where only entries were found', () => {
  assert.deepEqual(foundWords(0, 0), { code: 'app.search.none', params: {} });
  assert.deepEqual(foundWords(3, 0), { code: 'app.search.found', params: { count: 3 } });
  assert.deepEqual(foundWords(0, 1), { code: 'app.search.found_any', params: { count: 1 } });
  assert.deepEqual(foundWords(2, 2), { code: 'app.search.found_any', params: { count: 4 } });
});
