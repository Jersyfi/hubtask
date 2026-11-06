// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Every screen's chunk is fetched once the signed-in session is idle, so a screen never opened
// before going offline opens from the copy (offline-sync.md §13).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

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

test('the app precaches its table of lazy screens and the parts the screens load', () => {
  const app = readFileSync(new URL('../../App.svelte', import.meta.url), 'utf8');
  assert.match(app, /precacheWhenIdle\(\[\.\.\.Object\.values\(views\), \.\.\.partLoaders\(\)\]/);
});

// A chunk loaded anywhere else would be missing offline until its screen had been opened once.
test('every lazy chunk is in the app\'s table of screens or in the parts table', () => {
  const root = fileURLToPath(new URL('../../', import.meta.url));
  const tables = new Set([join(root, 'App.svelte'), join(root, 'lib/frame/parts.ts'), join(root, 'lib/frame/screens.ts')]);
  const outside: string[] = [];
  const walk = (dir: string) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const path = join(dir, entry.name);
      if (entry.isDirectory()) walk(path);
      else if (/\.(svelte|ts)$/.test(entry.name) && !/\.test\.ts$/.test(entry.name) && !tables.has(path)) {
        if (/\(\)\s*=>\s*import\(/.test(readFileSync(path, 'utf8'))) outside.push(path.slice(root.length));
      }
    }
  };
  walk(root);
  assert.deepEqual(outside, []);
});

// A screen imports its parts; the parts table loading a screen too would close a cycle (screens.ts).
test('the parts table loads no screen and no frame', () => {
  const parts = readFileSync(new URL('./parts.ts', import.meta.url), 'utf8');
  assert.doesNotMatch(parts, /import\(['"][^'"]*(\/views\/|AppFrame)/);
});
