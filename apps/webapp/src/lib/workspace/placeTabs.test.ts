// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { placeTabOf, placeTabs, tabNodeId, tabOfNode } from './placeTabs.ts';
import { placeMenu } from './placeMenu.ts';

const ALL = ['WORKFLOWS', 'PLANNING', 'KNOWLEDGE', 'WHITEBOARD', 'SERVICE_DESK', 'ASSETS', 'TIME', 'GOALS', 'AGENTS'];
const ids = (tabs: { id: string }[]) => tabs.map((tab) => tab.id);

test('a hub with every module on draws its tabs in the one fixed order (user-experience.md §2.4)', () => {
  assert.deepEqual(ids(placeTabs({ type: 'HUB', id: 'h', modulesOn: ALL })), ['overview', 'plan', 'pages', 'whiteboards', 'desk', 'assets', 'time', 'goals']);
});

test('a hub with no module on keeps only its overview (UC-INS-21 check 3)', () => {
  assert.deepEqual(ids(placeTabs({ type: 'HUB', id: 'h', modulesOn: [] })), ['overview']);
});

test('each module switched off takes exactly its own tab away and reorders nothing', () => {
  const full = ids(placeTabs({ type: 'HUB', id: 'h', modulesOn: ALL }));
  for (const [module, tab] of [['PLANNING', 'plan'], ['KNOWLEDGE', 'pages'], ['WHITEBOARD', 'whiteboards'], ['SERVICE_DESK', 'desk'], ['ASSETS', 'assets'], ['TIME', 'time'], ['GOALS', 'goals']] as const) {
    const without = ids(placeTabs({ type: 'HUB', id: 'h', modulesOn: ALL.filter((each) => each !== module) }));
    assert.deepEqual(without, full.filter((each) => each !== tab), module);
  }
});

test('a collection offers its portal only where it acts as a desk', () => {
  assert.deepEqual(ids(placeTabs({ type: 'COLLECTION', id: 'c', modulesOn: ALL, runsSprints: true })), ['entries', 'plan', 'sprints', 'time']);
  assert.deepEqual(ids(placeTabs({ type: 'COLLECTION', id: 'c', modulesOn: ALL, isDesk: true, runsSprints: true })), ['entries', 'plan', 'sprints', 'portal', 'time']);
});

test('a collection offers its sprints only where it runs sprints, and only with the plan on', () => {
  assert.deepEqual(ids(placeTabs({ type: 'COLLECTION', id: 'c', modulesOn: ALL })), ['entries', 'plan', 'time']);
  assert.deepEqual(ids(placeTabs({ type: 'COLLECTION', id: 'c', modulesOn: ALL.filter((each) => each !== 'PLANNING'), runsSprints: true })), ['entries', 'time']);
});

test('every view a tab holds is reached by a route that names that tab current', () => {
  const routeOf: Record<string, string> = {
    '/hubs/h': 'hub', '/hubs/h/plan': 'hub-plan', '/hubs/h/releases': 'hub-releases', '/hubs/h/pages': 'hub-pages',
    '/hubs/h/whiteboards': 'hub-whiteboards', '/hubs/h/desk': 'hub-desk', '/hubs/h/assets': 'hub-assets', '/hubs/h/actuals': 'hub-actuals', '/hubs/h/workload': 'hub-workload',
  };
  for (const tab of placeTabs({ type: 'HUB', id: 'h', modulesOn: ALL })) {
    for (const href of [tab.href, ...(tab.views ?? []).map((view) => view.href)]) {
      if (href.startsWith('/goals')) continue;
      assert.equal(placeTabOf(routeOf[href] ?? '')?.tab, tab.id, href);
    }
  }
  assert.deepEqual(placeTabOf('goals', { hub: 'h', tab: 'dashboards' }), { tab: 'goals' });
  assert.equal(placeTabOf('goals', {}), undefined, 'the workspace goals belong to no place');
});

test('no product appears in the place menu: it holds only verbs on the place', () => {
  const products = new Set(placeTabs({ type: 'HUB', id: 'h', modulesOn: ALL }).map((tab) => tab.code));
  const menu = placeMenu({ type: 'HUB', mayShape: true, mayDelete: true, archival: 'active', privacy: 'make_private' }, (code) => code);
  assert.deepEqual(menu.map((item) => item.id), ['settings', 'import', 'position', 'archive', 'privacy', 'trash']);
  assert.ok(menu.every((item) => !products.has(item.label)));
});

test('a hub view\'s tree id names its hub and its tab, and no container id reads as one', () => {
  const hub = '01a0e2e0-0000-7000-8000-000000000002';
  assert.deepEqual(tabOfNode(tabNodeId(hub, 'time')), { placeId: hub, tab: 'time' });
  assert.equal(tabOfNode(hub), undefined);
  assert.equal(tabOfNode('today'), undefined);
});
