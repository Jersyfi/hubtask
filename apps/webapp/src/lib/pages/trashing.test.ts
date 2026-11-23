// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { oneTrashAtATime, trashAndLeave } from './trashing.ts';

test("a trashed page leaves for its hub's pages among All contents, though the delete took the page away", async () => {
  // The screen's page is derived from the read: once the delete invalidates it, it is gone.
  let isDeleted = false;
  const page = { get hub_id(): string { return isDeleted ? (undefined as never) : 'hub-1'; } };
  const happened: string[] = [];
  const answer = await trashAndLeave(page, {
    drop: () => happened.push(isDeleted ? 'drop after delete' : 'drop'),
    trash: async () => { happened.push('delete'); isDeleted = true; return { trashed: 1 }; },
    leave: (path) => happened.push(`leave ${path}`),
  }, 'pages');
  assert.deepEqual(answer, { trashed: 1 });
  assert.deepEqual(happened, ['drop', 'delete', 'leave /hubs/hub-1/contents?kind=pages']);
});

test("a trashed post leaves for its hub's All contents, unnarrowed: a post is not among them", async () => {
  const left: string[] = [];
  await trashAndLeave({ hub_id: 'hub-1' }, { drop: () => {}, trash: async () => ({ trashed: 1 }), leave: (path) => left.push(path) });
  assert.deepEqual(left, ['/hubs/hub-1/contents']);
});

test('a refused delete stays on the page', async () => {
  const left: string[] = [];
  const answer = await trashAndLeave({ hub_id: 'hub-1' }, { drop: () => {}, trash: async () => undefined, leave: (path) => left.push(path) });
  assert.equal(answer, undefined);
  assert.deepEqual(left, []);
});

test('a refused delete gives the held saves back, an accepted one does not', async () => {
  for (const [answer, restored] of [[undefined, true], [{ trashed: 1 }, false]] as const) {
    let back = false;
    await trashAndLeave({ hub_id: 'hub-1' }, {
      drop: () => () => { back = true; },
      trash: async () => answer,
      leave: () => {},
    });
    assert.equal(back, restored);
  }
});

test('a second trash while one is pending is not started, so a refused first cannot free the saves of an accepted second', async () => {
  const happened: string[] = [];
  let refuse: (value: undefined) => void = () => {};
  const trashOnce = oneTrashAtATime(() =>
    trashAndLeave({ hub_id: 'hub-1' }, {
      drop: () => { happened.push('hold'); return () => happened.push('give back'); },
      trash: () => new Promise<undefined>((resolve) => { refuse = resolve; }),
      leave: () => happened.push('leave'),
    }));
  const first = trashOnce();
  const second = trashOnce();
  refuse(undefined);
  await Promise.all([first, second]);
  assert.deepEqual(happened, ['hold', 'give back'], 'one delete held the saves and gave them back');
  // Settled, the next press trashes again.
  void trashOnce();
  assert.equal(happened.at(-1), 'hold');
});
