// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { collectionViews, hubPlaceOf, hubPlaces, layoutOfView, toolViews, viewOfLayout } from './placeViews.ts';
import { placeMenu } from './placeMenu.ts';
import { HUB_TOOLS } from '../navigation.ts';

const ALL = ['WORKFLOWS', 'PLANNING', 'KNOWLEDGE', 'WHITEBOARD', 'SERVICE_DESK', 'ASSETS', 'TIME', 'GOALS', 'AGENTS'];
const ids = (views: readonly { id: string }[]) => views.map((view) => view.id);

test('a collection draws one view bar: list, board, timeline, time and its saved views (UC-WRK-36 check 9)', () => {
  const saved = [{ id: 'v1', name: 'My open ones' }];
  assert.deepEqual(ids(collectionViews({ id: 'c', modulesOn: ALL, saved })), ['list', 'board', 'timeline', 'time', 'view:v1']);
  // Its former *Plan* is the *Timeline* view where the plan is on, the timeline layout where it is off.
  assert.equal(collectionViews({ id: 'c', modulesOn: ALL }).find((view) => view.id === 'timeline')?.href, '/collections/c/timeline');
  assert.equal(collectionViews({ id: 'c', modulesOn: [] }).find((view) => view.id === 'timeline')?.href, '/collections/c?layout=timeline');
  assert.deepEqual(ids(collectionViews({ id: 'c', modulesOn: [], layouts: ['LIST_COLLAPSED'] })), ['list']);
  assert.equal(collectionViews({ id: 'c', modulesOn: ALL, saved }).at(-1)?.href, '/collections/c?view=v1');
});

test('a collection that runs sprints opens with the board and the sprint; a desk keeps to its lists', () => {
  assert.deepEqual(ids(collectionViews({ id: 'c', modulesOn: ALL, runsSprints: true })), ['board', 'sprint', 'backlog', 'list', 'timeline', 'time']);
  // The sprint view names the running sprint (the mockup's *Sprint 42*), the backlog a view of its own.
  const named = collectionViews({ id: 'c', modulesOn: ALL, runsSprints: true, sprintName: 'Sprint 42' });
  assert.equal(named.find((view) => view.id === 'sprint')?.name, 'Sprint 42');
  assert.equal(named.find((view) => view.id === 'sprint')?.code, undefined);
  assert.equal(named.find((view) => view.id === 'backlog')?.href, '/collections/c/backlog');
  assert.equal(collectionViews({ id: 'c', modulesOn: ALL, runsSprints: true }).find((view) => view.id === 'sprint')?.code, 'app.nav.view_sprint');
  assert.deepEqual(ids(collectionViews({ id: 'c', modulesOn: ALL.filter((each) => each !== 'PLANNING'), runsSprints: true })), ['list', 'board', 'timeline', 'time']);
  assert.deepEqual(ids(collectionViews({ id: 'c', modulesOn: ALL, isDesk: true, saved: [{ id: 'v', name: 'Mine' }] })), ['list', 'board', 'view:v']);
  assert.equal(viewOfLayout('LIST_EXPANDED'), 'list');
  assert.equal(viewOfLayout('KANBAN'), 'board');
  assert.equal(layoutOfView('list', 'LIST_EXPANDED'), 'LIST_EXPANDED');
  assert.equal(layoutOfView('sprint', 'LIST_COLLAPSED'), undefined);
});

test('a hub tool with tabs of its own holds them in one bar; a hub itself has none', () => {
  assert.deepEqual(ids(toolViews('h', 'plan')), ['schedule', 'table', 'dependencies', 'releases', 'sprints']);
  assert.deepEqual(ids(toolViews('h', 'time')), ['actuals', 'workload']);
  assert.deepEqual(ids(toolViews('h', 'time', { mayCost: true })), ['actuals', 'workload', 'budget']);
  assert.deepEqual(ids(toolViews('h', 'goals')), ['goals', 'dashboards']);
  assert.deepEqual(toolViews('h', 'plan').map((view) => view.href), ['/hubs/h/plan', '/hubs/h/plan/table', '/hubs/h/plan/dependencies', '/hubs/h/plan/releases', '/hubs/h/plan/sprints']);
  assert.deepEqual(ids(toolViews('h', 'overview')), []);
});

test('the palette reaches a hub\'s tools and contents only where their module is on there', () => {
  assert.deepEqual(ids(hubPlaces('h', ALL)), ['plan', 'inventory', 'time', 'goals', 'contents']);
  assert.deepEqual(ids(hubPlaces('h', [])), []);
  assert.equal(hubPlaceOf('h', 'PLANNING')?.href, '/hubs/h/plan');
  assert.equal(hubPlaceOf('h', 'KNOWLEDGE')?.href, '/hubs/h/contents');
  assert.equal(hubPlaceOf('h', 'AGENTS'), undefined, 'agents are the workspace\'s only');
});

test('no product appears in the place menu: it holds only verbs on the place', () => {
  const products = new Set(HUB_TOOLS.map((tool) => tool.code));
  const menu = placeMenu({ type: 'HUB', mayShape: true, mayDelete: true, archival: 'active', privacy: 'make_private' }, (code) => code);
  assert.deepEqual(menu.map((item) => item.id), ['settings', 'import', 'position', 'archive', 'privacy', 'trash']);
  assert.ok(menu.every((item) => !products.has(item.label)));
});
