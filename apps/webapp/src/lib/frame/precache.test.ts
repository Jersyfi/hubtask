// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Every chunk of a screen the reader can open is fetched once the signed-in session is idle, so a
// screen never opened before going offline opens from the copy (offline-sync.md §14).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

import { precacheByReach, precacheWhenIdle } from './precache.ts';

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

test('the app precaches its table of lazy screens and the parts the screens load, by reach', () => {
  const app = readFileSync(new URL('../../App.svelte', import.meta.url), 'utf8');
  assert.match(app, /precacheByReach\(views, partLoaders\(\)\)/);
  assert.match(app, /precache\(\{ signedIn: session\.isSignedIn, administration: reachesAdministrationArea\(\) \}\)/);
});

/** A table of screens whose loaders record what was fetched. */
function screens(fetched: string[]) {
  const loader = (name: string) => async () => {
    fetched.push(name);
    return {};
  };
  return { views: { TodayView: loader('today'), AdministrationArea: loader('administration') }, parts: [loader('part')] };
}

const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
const always = { idle: idleNow, online: () => true };

test('a member\'s precache has no administration chunk; an administrator\'s has', async () => {
  for (const [administration, want] of [
    [false, ['today', 'part']],
    [undefined, ['today', 'part']],
    [true, ['today', 'part', 'administration']],
  ] as const) {
    const fetched: string[] = [];
    const { views, parts } = screens(fetched);
    precacheByReach(views, parts, always)({ signedIn: true, administration });
    await settled();
    assert.deepEqual(fetched, want, `reach ${String(administration)}`);
  }
});

test('reach turning true after the first run precaches the administration chunk, once', async () => {
  const fetched: string[] = [];
  const { views, parts } = screens(fetched);
  const precache = precacheByReach(views, parts, always);

  precache({ signedIn: false, administration: undefined });
  await settled();
  assert.deepEqual(fetched, [], 'nothing before sign-in');

  // The manifest is not read yet at the first run, and then says the reader reaches the area.
  precache({ signedIn: true, administration: undefined });
  precache({ signedIn: true, administration: true });
  precache({ signedIn: true, administration: true });
  await settled();
  precache({ signedIn: true, administration: false });
  precache({ signedIn: true, administration: true });
  await settled();
  assert.deepEqual(fetched, ['today', 'part', 'administration']);
});

test('the reach precache is still skipped offline and on a connection asking to save data', async () => {
  for (const conditions of [
    { idle: idleNow, online: () => false },
    { idle: idleNow, online: () => true, saveData: () => true },
  ]) {
    const fetched: string[] = [];
    const { views, parts } = screens(fetched);
    precacheByReach(views, parts, conditions)({ signedIn: true, administration: true });
    await settled();
    assert.deepEqual(fetched, []);
  }
});

// A chunk loaded anywhere else would be missing offline until its screen had been opened once.
test('every lazy chunk is in the app\'s table of screens or in the parts table', () => {
  const root = fileURLToPath(new URL('../../', import.meta.url));
  const tables = new Set([join(root, 'App.svelte'), join(root, 'lib/frame/parts.ts')]);
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
