// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Which edge of a scroll box shows a shade: the arithmetic `scrollEdges` answers with. Whether the
// shade is drawn there, over the rows and inside the frame's corners, is the browser walks' part
// (workbench-menu, menuroom).

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { edgesOf } from '../src/scrollEdges.ts';

test('a list that does not scroll has no shade', () => {
  assert.deepEqual(edgesOf({ scrollTop: 0, scrollHeight: 200, clientHeight: 200 }), { start: false, end: false });
  // A fraction of a pixel more content is a zoomed page's rounding, not something to scroll to.
  assert.deepEqual(edgesOf({ scrollTop: 0, scrollHeight: 200.5, clientHeight: 200 }), { start: false, end: false });
});

test('at the top only the end has more, at the bottom only the start, between them both', () => {
  assert.deepEqual(edgesOf({ scrollTop: 0, scrollHeight: 600, clientHeight: 200 }), { start: false, end: true });
  assert.deepEqual(edgesOf({ scrollTop: 200, scrollHeight: 600, clientHeight: 200 }), { start: true, end: true });
  assert.deepEqual(edgesOf({ scrollTop: 400, scrollHeight: 600, clientHeight: 200 }), { start: true, end: false });
});

test('a scroll that stops within a pixel of an edge counts as at that edge', () => {
  assert.deepEqual(edgesOf({ scrollTop: 0.5, scrollHeight: 600, clientHeight: 200 }), { start: false, end: true });
  assert.deepEqual(edgesOf({ scrollTop: 399.5, scrollHeight: 600, clientHeight: 200 }), { start: true, end: false });
});
