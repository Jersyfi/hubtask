// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { PortfolioRoadmap } from '@hubtask/sync-engine';

import { roadmapRows } from './roadmap.ts';
import { outsiderRows, roadmapLinkAddress, roadmapTokenOf, sharedRoadmapPath } from './roadmaplink.ts';

// UC-GOL-03 check 15: the link carries its token in the fragment and reads it back; anything that
// is no roadmap token is no token.
test('a roadmap link carries its token in the fragment', () => {
  const address = roadmapLinkAddress('https://hub.example.org/', 'hbt_road_abc_DEF-1');
  assert.equal(address, 'https://hub.example.org/shared-roadmap#hbt_road_abc_DEF-1');
  assert.equal(roadmapTokenOf(new URL(address).hash), 'hbt_road_abc_DEF-1');
  assert.equal(roadmapTokenOf('#hbt_page_abc'), '');
  assert.equal(roadmapTokenOf('#hbt_road_a<b'), '');
  assert.equal(sharedRoadmapPath('hbt_road_x', 'Europe/Berlin'), '/public/roadmaps/hbt_road_x?zone=Europe%2FBerlin');
});

// A visitor's rows lead nowhere into the workspace: no lane names a hub to open.
test('an outsider sees the roadmap without a press into the workspace', () => {
  const roadmap = {
    portfolio_id: 'p', name: 'Programme', status: 'ON_TRACK', start: null, finish: null,
    lanes: [{ hub_id: 'h1', name: 'Product', projects: [{
      container_id: 'c1', kind: 'COLLECTION', name: 'Launch', start_on: null, end_on: null, status: 'NONE', finish: null,
      bars: [{ item_id: 'i1', title: 'Ship', start: '2026-11-02', finish: '2026-11-20', milestone: false, done: false, critical: false }],
    }] }],
  } as unknown as PortfolioRoadmap;
  const inside = roadmapRows(roadmap, 'A hub');
  assert.ok(inside.every((row) => row.href !== undefined));
  const rows = outsiderRows(inside);
  assert.deepEqual(rows.map((row) => row.title), ['Product', 'Launch', 'Ship']);
  assert.ok(rows.every((row) => row.href === undefined));
});
