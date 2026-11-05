// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import * as Y from 'yjs';

// @ts-expect-error a plain script beside the sources, without declarations
import { converged, summarise } from '../scripts/liveload-report.mjs';

// S5's bar (test/load/PLAN.md): every writer's document equal at the end. Two documents that
// exchanged every update are one; one that missed an update is not, even with the same length.
test('the writers are equal only once each holds every update', () => {
  const a = new Y.Doc();
  const b = new Y.Doc();
  a.getText('load').insert(0, 'w0 e0 ');
  b.getText('load').insert(0, 'w1 e0 ');
  assert.equal(converged([a, b]), false);

  Y.applyUpdate(b, Y.encodeStateAsUpdate(a));
  assert.equal(converged([a, b]), false, 'a still lacks b');
  Y.applyUpdate(a, Y.encodeStateAsUpdate(b));
  assert.equal(converged([a, b]), true);

  const late = new Y.Doc();
  const report = summarise({
    writers: [{ edits: 1, cursors: 10, unsent: 0 }, { edits: 1, cursors: 9, unsent: 0 }],
    docs: [a, b],
    fresh: late,
    windowMs: 1000,
    settled: true,
    awarenessSeen: 19,
    closes: {},
  });
  assert.equal(report.equal, true);
  assert.equal(report.fresh_equal, false, 'a fresh opener that never synced is not equal');
  assert.equal(report.edits, 2);
  assert.equal(report.cursors, 19);
  assert.equal(report.length, 12);
});
