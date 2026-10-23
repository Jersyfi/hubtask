// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { startsAt, stepsFor } from './tour.ts';

test('a workspace with a collection and an entry is walked through every step, and led to add an entry', () => {
  const steps = stepsFor({ collectionId: 'col-1', itemId: 'it-1', hasJumble: true });
  assert.deepEqual(steps.map((s) => s.id), ['hubs', 'capture', 'palette', 'layouts', 'entry', 'levels', 'jumble', 'search', 'profile', 'make']);
  assert.equal(steps[3]?.route, '/collections/col-1');
  assert.equal(steps[4]?.route, '/items/it-1');
  assert.equal(steps[5]?.route, '/items/it-1');
  assert.equal(steps.at(-1)?.selector, '[data-opener="add-entry"]');
  // Every step names a route and an element; nothing is pointed at by guess.
  for (const step of steps) assert.ok(step.route.startsWith('/') && step.selector.startsWith('['), step.id);
});

test('a fresh workspace leaves out what it cannot point at, and the last step leads to the first hub', () => {
  const steps = stepsFor({ hasJumble: true });
  assert.deepEqual(steps.map((s) => s.id), ['hubs', 'capture', 'palette', 'jumble', 'search', 'profile', 'make']);
  assert.equal(steps.at(-1)?.route, '/');
  assert.equal(steps.at(-1)?.selector, '[data-tour="hubs"]');
});

test('a collection without an entry keeps the list step, drops the entry step and tells the levels at the list', () => {
  const steps = stepsFor({ collectionId: 'c', hasJumble: true });
  assert.deepEqual(steps.map((s) => s.id), ['hubs', 'capture', 'palette', 'layouts', 'levels', 'jumble', 'search', 'profile', 'make']);
  const levels = steps.find((s) => s.id === 'levels');
  assert.equal(levels?.route, '/collections/c');
  assert.equal(levels?.selector, '[data-tour="levels-inside"]');
});

test('somebody the jumble is not drawn for is not toured through it', () => {
  assert.deepEqual(stepsFor({ collectionId: 'c', hasJumble: false }).map((s) => s.id), ['hubs', 'capture', 'palette', 'layouts', 'levels', 'search', 'profile', 'make']);
});

test('the tour starts on the overview only, so an address opened by link keeps its screen', () => {
  assert.equal(startsAt('/'), true);
  for (const path of ['/profile/security', '/profile/sessions', '/collections/c', '/search', '/sign-in']) assert.equal(startsAt(path), false, path);
});
