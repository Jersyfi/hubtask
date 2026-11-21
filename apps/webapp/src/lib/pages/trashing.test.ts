// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { trashAndLeave } from './trashing.ts';

test('a trashed page leaves for its hub overview, though the delete took the page away', async () => {
  // The screen's page is derived from the read: once the delete invalidates it, it is gone.
  let isDeleted = false;
  const page = { get hub_id(): string { return isDeleted ? (undefined as never) : 'hub-1'; } };
  const happened: string[] = [];
  const answer = await trashAndLeave(page, {
    drop: () => happened.push(isDeleted ? 'drop after delete' : 'drop'),
    trash: async () => { happened.push('delete'); isDeleted = true; return { trashed: 1 }; },
    leave: (path) => happened.push(`leave ${path}`),
  });
  assert.deepEqual(answer, { trashed: 1 });
  assert.deepEqual(happened, ['drop', 'delete', 'leave /hubs/hub-1/pages']);
});

test('a refused delete stays on the page', async () => {
  const left: string[] = [];
  const answer = await trashAndLeave({ hub_id: 'hub-1' }, { drop: () => {}, trash: async () => undefined, leave: (path) => left.push(path) });
  assert.equal(answer, undefined);
  assert.deepEqual(left, []);
});
