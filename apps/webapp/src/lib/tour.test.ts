// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { stepsFor } from './tour.ts';

test('a workspace with a collection and an entry is walked through all six steps, and led to add an entry', () => {
  const steps = stepsFor({ collectionId: 'col-1', itemId: 'it-1' });
  assert.deepEqual(steps.map((s) => s.id), ['hubs', 'layouts', 'entry', 'jumble', 'search', 'profile', 'make']);
  assert.equal(steps[1]?.route, '/collections/col-1');
  assert.equal(steps[2]?.route, '/items/it-1');
  assert.equal(steps.at(-1)?.selector, '[data-opener="add-entry"]');
  // Every step names a route and an element; nothing is pointed at by guess.
  for (const step of steps) assert.ok(step.route.startsWith('/') && step.selector.startsWith('['), step.id);
});

test('a fresh workspace leaves out what it cannot point at, and the seventh leads to the first hub', () => {
  const steps = stepsFor({});
  assert.deepEqual(steps.map((s) => s.id), ['hubs', 'jumble', 'search', 'profile', 'make']);
  assert.equal(steps.at(-1)?.route, '/');
  assert.equal(steps.at(-1)?.selector, '[data-tour="hubs"]');
});

test('a collection without an entry keeps the list step and drops the entry step', () => {
  assert.deepEqual(stepsFor({ collectionId: 'c' }).map((s) => s.id), ['hubs', 'layouts', 'jumble', 'search', 'profile', 'make']);
});
