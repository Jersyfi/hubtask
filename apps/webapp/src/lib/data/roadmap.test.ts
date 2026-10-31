// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { PortfolioRoadmap } from '@hubtask/sync-engine';

import { roadmapRows } from './roadmap.ts';

const roadmap: PortfolioRoadmap = {
  portfolio_id: 'p', name: 'Programme', status: 'NONE', start: '2026-10-01', finish: '2026-12-31',
  lanes: [
    {
      hub_id: 'h1', name: 'Product', projects: [
        {
          container_id: 'c1', kind: 'COLLECTION', name: 'Launch', start_on: null, end_on: null, status: 'ON_TRACK', finish: '2026-11-30',
          bars: [
            { item_id: 'e1', title: 'Beta', collection_id: 'c1', start: '2026-11-02', finish: '2026-11-20', milestone: false, done: false, critical: true },
            { item_id: 'e2', title: 'Go live', collection_id: 'c1', start: '2026-11-30', finish: '2026-11-30', milestone: true, done: false, critical: false },
          ],
        },
      ],
    },
    { hub_id: 'h2', name: '', projects: [{ container_id: 'h2', kind: 'HUB', name: 'Board', start_on: '2026-10-01', end_on: '2026-12-31', status: 'NONE', finish: null, bars: [] }] },
  ],
};

// UC-GOL-03 checks 10, 12 and 14: the rows the timeline and the table share.
test('a lane, its projects and their bars, in that order and depth', () => {
  const rows = roadmapRows(roadmap, 'A hub you cannot see');
  assert.deepEqual(rows.map((row) => [row.level, row.depth]), [
    ['lane', 0], ['project', 1], ['entry', 2], ['entry', 2], ['lane', 0], ['project', 1],
  ]);
  assert.equal(rows[4]?.title, 'A hub you cannot see');
  assert.equal(rows[4]?.href, undefined);
});

test("a project without planned dates spans its bars; a lane spans its projects", () => {
  const rows = roadmapRows(roadmap, '');
  assert.deepEqual([rows[1]?.start, rows[1]?.due], ['2026-11-02', '2026-11-30']);
  assert.deepEqual([rows[0]?.start, rows[0]?.due], ['2026-11-02', '2026-11-30']);
  assert.equal(rows[3]?.kind, 'milestone');
  assert.equal(rows[3]?.start, undefined);
  assert.equal(rows[2]?.isCritical, true);
  assert.equal(rows[5]?.href, '/hubs/h2/plan');
});
