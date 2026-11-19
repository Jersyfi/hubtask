// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { compareBlocks } from '@hubtask/editor';

import { changeRuns } from './history.ts';

const compared = compareBlocks(
  ['## Overview', 'Public API v2', 'Teams', 'Old rule'].join('\n\n'),
  ['## Overview', 'Public API v3', 'Teams', 'Passkeys first', 'Buffered orders'].join('\n\n'),
);

test('two versions are marked block by block: the older one with what went out, the page with what came in', () => {
  assert.deepEqual(compared.before, ['same', 'removed', 'same', 'removed']);
  assert.deepEqual(compared.after, ['same', 'added', 'same', 'added', 'added']);
});

test('two versions count each place that changed once, blocks out and in together', () => {
  const runs = changeRuns(compared.blocks);
  assert.equal(runs.length, 2);
  assert.equal(compared.places, 2);
  assert.deepEqual(runs[1]!.map((block) => `${block.kind}:${block.text.trim()}`), ['removed:Old rule', 'added:Passkeys first', 'added:Buffered orders']);
  assert.deepEqual(changeRuns(compareBlocks('same', 'same').blocks), []);
});

// N35: a version's text holds tables wider than its column; the column scrolls inside, so the
// table is read whole rather than cut at the column's edge.
test('a history column scrolls a wide table inside itself (N35)', async () => {
  const { readFileSync } = await import('node:fs');
  const source = readFileSync(new URL('./PageHistory.svelte', import.meta.url), 'utf8');
  assert.match(source, /\.side, \.run \{ overflow-x: auto; \}/u);
});
