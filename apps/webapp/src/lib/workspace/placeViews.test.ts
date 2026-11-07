// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { collectionViews, hubPlaceOf, hubPlaces, toolViews } from './placeViews.ts';
import { placeMenu } from './placeMenu.ts';
import { HUB_TOOLS } from '../navigation.ts';

const ALL = ['WORKFLOWS', 'PLANNING', 'KNOWLEDGE', 'WHITEBOARD', 'SERVICE_DESK', 'ASSETS', 'TIME', 'GOALS', 'AGENTS'];
const ids = (views: readonly { id: string }[]) => views.map((view) => view.id);

test('a collection offers its portal only where it acts as a desk, and time as two views of one bar', () => {
  assert.deepEqual(ids(collectionViews({ id: 'c', modulesOn: ALL, runsSprints: true })), ['entries', 'plan', 'sprints', 'actuals', 'workload']);
  assert.deepEqual(ids(collectionViews({ id: 'c', modulesOn: ALL, isDesk: true, runsSprints: true })), ['entries', 'plan', 'sprints', 'portal', 'actuals', 'workload']);
});

test('a collection offers its sprints only where it runs sprints, and only with the plan on', () => {
  assert.deepEqual(ids(collectionViews({ id: 'c', modulesOn: ALL })), ['entries', 'plan', 'actuals', 'workload']);
  assert.deepEqual(ids(collectionViews({ id: 'c', modulesOn: ALL.filter((each) => each !== 'PLANNING'), runsSprints: true })), ['entries', 'actuals', 'workload']);
  assert.deepEqual(ids(collectionViews({ id: 'c', modulesOn: [] })), ['entries']);
});

test('a hub tool with tabs of its own holds them in one bar; a hub itself has none', () => {
  assert.deepEqual(ids(toolViews('h', 'plan')), ['plan', 'releases']);
  assert.deepEqual(ids(toolViews('h', 'time')), ['actuals', 'workload']);
  assert.deepEqual(ids(toolViews('h', 'overview')), []);
});

test('the palette reaches a hub\'s tools and contents only where their module is on there', () => {
  assert.deepEqual(ids(hubPlaces('h', ALL)), ['plan', 'inventory', 'time', 'goals', 'pages', 'whiteboards']);
  assert.deepEqual(ids(hubPlaces('h', [])), []);
  assert.equal(hubPlaceOf('h', 'PLANNING')?.href, '/hubs/h/plan');
  assert.equal(hubPlaceOf('h', 'KNOWLEDGE')?.href, '/hubs/h/pages');
  assert.equal(hubPlaceOf('h', 'AGENTS'), undefined, 'agents are the workspace\'s only');
});

test('no product appears in the place menu: it holds only verbs on the place', () => {
  const products = new Set(HUB_TOOLS.map((tool) => tool.code));
  const menu = placeMenu({ type: 'HUB', mayShape: true, mayDelete: true, archival: 'active', privacy: 'make_private' }, (code) => code);
  assert.deepEqual(menu.map((item) => item.id), ['settings', 'import', 'position', 'archive', 'privacy', 'trash']);
  assert.ok(menu.every((item) => !products.has(item.label)));
});
