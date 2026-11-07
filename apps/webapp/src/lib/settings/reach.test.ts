// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The preview save bar (configuration/ui.md A3): before a save it says what the change reaches.

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { StructurePreview } from '@hubtask/sync-engine';

import { reachOf } from './reach.ts';

const t = (code: string, params?: Record<string, string>) => (params ? `${code}:${params.count}` : code);

const NOTHING = {
  narrows: false,
  levels: [],
  fields: [],
  views: [],
  automation: [],
  keeping: [],
  flatten: { entries: 0 },
} as unknown as StructurePreview;

test('a change that hides nothing says so', () => {
  assert.equal(reachOf(NOTHING, t), 'app.settings.reach_nothing');
});

test('a narrowing change counts the entries, the values and the views it stops showing', () => {
  const preview = {
    ...NOTHING,
    narrows: true,
    levels: [
      { type: 'TASK', entries: 30 },
      { type: 'SUBTASK', entries: 7 },
    ],
    fields: [{ type: 'TASK', capability: 'due', entries: 4 }],
    views: ['KANBAN', 'TIMELINE'],
  } as unknown as StructurePreview;
  assert.equal(reachOf(preview, t), 'app.settings.reach_entries:37 · app.settings.reach_values:4 · app.settings.reach_views:2');
});
