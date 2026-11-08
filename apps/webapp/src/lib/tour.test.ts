// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import german from '../../../../locales/de.json' with { type: 'json' };

import { SOURCE } from './i18n/source.ts';
import { hubsInTourOrder, startsAt, stepsFor } from './tour.ts';

test('the layouts step names every layout a collection may offer, the timeline too', () => {
  const de = german as Record<string, string>;
  for (const layout of ['list', 'board', 'timeline']) assert.match(SOURCE['app.tour.layouts.body'] ?? '', new RegExp(layout));
  for (const layout of ['Liste', 'Board', 'Zeitleiste']) assert.match(de['app.tour.layouts.body'] ?? '', new RegExp(layout));
});

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

test('where a product is on, the second step points at the Products section\'s first row', () => {
  const steps = stepsFor({ collectionId: 'c', hasJumble: false, firstProduct: 'KNOWLEDGE' });
  assert.deepEqual(steps.slice(0, 3).map((s) => s.id), ['hubs', 'products', 'capture']);
  assert.equal(steps[1]?.selector, '[data-tour="hubs"] [role="treeitem"][data-id="KNOWLEDGE"]');
  assert.ok(!stepsFor({ hasJumble: false }).some((s) => s.id === 'products'), 'a workspace with nothing on is shown no band');
});

test('a width that draws no column or no palette field counts no step for them, and the tree is Browse\'s on a phone', () => {
  const phone = stepsFor({ collectionId: 'c', hasJumble: false, firstProduct: 'KNOWLEDGE', frame: 'phone' });
  assert.deepEqual(phone.map((s) => s.id), ['hubs', 'capture', 'layouts', 'levels', 'search', 'profile', 'make']);
  assert.equal(phone[0]?.words, 'hubs_browse', 'the first step on a phone says the hubs are behind Browse');
  assert.equal(phone[0]?.selector, 'nav a[href="/browse"]');
  const drawer = stepsFor({ collectionId: 'c', hasJumble: false, firstProduct: 'KNOWLEDGE', frame: 'drawer' });
  assert.deepEqual(drawer.slice(0, 3).map((s) => s.id), ['hubs', 'capture', 'palette'], 'the drawer draws the palette\'s way in, but no column for the products');
  assert.equal(stepsFor({ collectionId: 'c', hasJumble: false }).at(0)?.words, undefined);
});

test('the tour looks for its collection in the hub the starter made first, then in the hubs as listed (user-experience.md §3.1)', () => {
  assert.deepEqual(hubsInTourOrder(['general', 'team'], 'team'), ['team', 'general']);
  assert.deepEqual(hubsInTourOrder(['general', 'team'], undefined), ['general', 'team']);
  assert.deepEqual(hubsInTourOrder(['general'], 'gone'), ['general'], 'a hub no longer listed is not walked');
});
