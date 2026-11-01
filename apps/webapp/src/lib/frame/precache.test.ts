// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Every screen's chunk is fetched once the signed-in session is idle, so a screen never opened
// before going offline opens from the copy (offline-sync.md §13).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { precacheWhenIdle } from './precache.ts';

const idleNow = (run: () => void) => {
  run();
  return () => {};
};

test('every loader is fetched once, one after the other, a failing one passed over', async () => {
  const loaded: string[] = [];
  let running = 0;
  const loader = (name: string, fails = false) => async () => {
    running++;
    assert.equal(running, 1, 'one fetch at a time');
    await Promise.resolve();
    running--;
    loaded.push(name);
    if (fails) throw new Error('gone');
    return {};
  };
  await precacheWhenIdle([loader('pages'), loader('gone', true), loader('whiteboards')], { idle: idleNow, online: () => true });
  assert.deepEqual(loaded, ['pages', 'gone', 'whiteboards']);
});

test('nothing is fetched offline or on a connection asking to save data', async () => {
  let fetched = 0;
  const loaders = [async () => fetched++];
  await precacheWhenIdle(loaders, { idle: idleNow, online: () => false });
  await precacheWhenIdle(loaders, { idle: idleNow, online: () => true, saveData: () => true });
  assert.equal(fetched, 0);
});

test('the app precaches its table of lazy screens', () => {
  const app = readFileSync(new URL('../../App.svelte', import.meta.url), 'utf8');
  assert.match(app, /precacheWhenIdle\(Object\.values\(views\)/);
});
