// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import test from 'node:test';
import assert from 'node:assert/strict';

import type { AppUpdate, AppUpdates } from '../platform/index.ts';
import { LATER_KEY, UpdateOffer } from './appupdate.ts';

function updates(found: AppUpdate | undefined, over: Partial<AppUpdates> = {}) {
  const calls = { check: 0, install: 0 };
  const platform: AppUpdates = {
    current: '1.2.0',
    offersOn: 'every-page',
    laterLasts: 'start',
    check: async () => {
      calls.check += 1;
      return found;
    },
    install: async () => {
      calls.install += 1;
    },
    open: async () => {},
    ...over,
  };
  return { platform, calls };
}

function storage() {
  const held = new Map<string, string>();
  return { getItem: (key: string) => held.get(key) ?? null, setItem: (key: string, value: string) => void held.set(key, value), held };
}

const ready: AppUpdate = { state: 'ready', version: '1.3.0' };

test('the installed app looks once a start, and the browser never looks', async () => {
  const desktop = updates(ready);
  const offer = new UpdateOffer(desktop.platform, storage());
  await offer.start();
  await offer.start();
  assert.equal(desktop.calls.check, 1);
  assert.deepEqual(offer.banner, ready);

  const browser = new UpdateOffer(undefined, storage());
  await browser.start();
  assert.equal(browser.banner, undefined);
  assert.equal(browser.current, undefined);
});

test('an update is installed only after the person agrees, and only one the shell checked (UC-SYN-07 check 5)', async () => {
  const desktop = updates(ready);
  const offer = new UpdateOffer(desktop.platform, storage());
  await offer.start();
  assert.equal(desktop.calls.install, 0, 'found and checked, not installed');
  await offer.install();
  assert.equal(desktop.calls.install, 1);
  assert.equal(offer.isInstalling, true);

  const refused = updates({ state: 'refused', version: '1.3.0', refused: 'package' });
  const nothing = new UpdateOffer(refused.platform, storage());
  await nothing.start();
  await nothing.install();
  assert.equal(refused.calls.install, 0, 'a refused update is never installed');
});

test('on the desktop, Later hides the banner until the next start, and About keeps the offer', async () => {
  const kept = storage();
  const offer = new UpdateOffer(updates(ready).platform, kept);
  await offer.start();
  offer.later();
  assert.equal(offer.banner, undefined);
  assert.deepEqual(offer.found, ready);
  const next = new UpdateOffer(updates(ready).platform, kept);
  await next.start();
  assert.deepEqual(next.banner, ready);
});

test('in the Android app, Later hides the notice until a newer version (UC-SYN-07 check 6)', async () => {
  const kept = storage();
  const android = (version: string) => updates({ state: 'available', version }, { offersOn: 'today', laterLasts: 'version' }).platform;
  const offer = new UpdateOffer(android('1.3.0'), kept);
  await offer.start();
  assert.equal(offer.offersOn, 'today');
  offer.later();
  assert.equal(kept.held.get(LATER_KEY), '1.3.0');

  const again = new UpdateOffer(android('1.3.0'), kept);
  await again.start();
  assert.equal(again.banner, undefined);
  assert.deepEqual(again.found, { state: 'available', version: '1.3.0' });

  const newer = new UpdateOffer(android('1.4.0'), kept);
  await newer.start();
  assert.deepEqual(newer.banner, { state: 'available', version: '1.4.0' });
});

test('a failed install leaves the offer standing', async () => {
  const failing = updates(ready, { install: async () => Promise.reject(new Error('update.install_failed')) });
  const offer = new UpdateOffer(failing.platform, storage());
  await offer.start();
  await offer.install();
  assert.equal(offer.isInstalling, false);
  assert.deepEqual(offer.banner, ready);
});
