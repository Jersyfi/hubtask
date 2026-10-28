// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-FLW-01 checks 3, 5 and 8 on the client: a new entry is offered the types of its level that are
// not archived, the level's own first; a row marks a type of the workspace's own and never the
// level's; and without the workflows module nothing is shown.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Capabilities, EntryType } from '@hubtask/sync-engine';

import { iconOf, offeredTypes, rowMarkOf, typesShown } from './entrytypes.ts';

const base = {
  completable: true, workflow: false, record: false, schedulable: true, estimable: false,
  allowed_children: [] as string[], archived: false, version: 1,
};

const types: EntryType[] = [
  { ...base, key: 'feature', level: 'TASK', system: false, name: 'Feature', icon: 'star', color_token: 'blue', position: 2 },
  { ...base, key: 'task', level: 'TASK', system: true, name: null, icon: 'square-check', color_token: 'slate', position: 0, version: 0 },
  { ...base, key: 'bug', level: 'TASK', system: false, name: 'Bug', icon: 'not-an-icon', color_token: 'red', position: 1 },
  { ...base, key: 'old', level: 'TASK', system: false, name: 'Old', icon: 'tag', color_token: 'red', position: 0, archived: true },
  { ...base, key: 'subtask', level: 'WORK_PACKAGE', system: false, name: 'Sub-task', icon: 'check', color_token: 'teal', position: 0 },
];

test('a new task is offered the task types in use, the level own first', () => {
  assert.deepEqual(offeredTypes(types, 'TASK').map((type) => type.key), ['task', 'bug', 'feature']);
  assert.deepEqual(offeredTypes(types, 'WORK_PACKAGE').map((type) => type.key), ['subtask']);
});

test('a row marks a type of the workspace own, an archived one too, and never the level own', () => {
  assert.equal(rowMarkOf(types, 'TASK', undefined), undefined);
  assert.equal(rowMarkOf(types, 'TASK', 'task'), undefined);
  assert.deepEqual(rowMarkOf(types, 'TASK', 'feature'), { icon: 'star', colorToken: 'blue', name: 'Feature' });
  assert.equal(rowMarkOf(types, 'TASK', 'old')?.name, 'Old');
});

test('an icon the set does not have is drawn as a tag', () => {
  assert.equal(iconOf({ icon: 'not-an-icon' }), 'tag');
});

test('without the workflows module no type is shown', () => {
  assert.equal(typesShown(undefined), false);
  assert.equal(typesShown({ modules_on: [] } as unknown as Capabilities), false);
  assert.equal(typesShown({ modules_on: ['WORKFLOWS'] } as unknown as Capabilities), true);
});
