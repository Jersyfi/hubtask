// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A screen that throws while drawing - data it does not expect after a migration, an enum value a
// newer server sent - becomes a sentence with a reference, Reload and Copy details inside the
// frame, and the frame's navigation still leads elsewhere. Without the boundary the whole tree
// unmounted and the page went blank.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { signedIn } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

test('chromium: 1280 px — a screen that throws while drawing leaves the frame usable', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await signedIn(browser, 1280, 1000);
  t.after(close);
  // The archive screen builds its search link while it is set up; that one construction throws, as
  // a screen meeting a shape it never expected would. Nothing else is touched.
  await page.addInitScript(() => {
    const Original = window.URLSearchParams;
    window.URLSearchParams = class extends Original {
      constructor(init) {
        if (init && typeof init === 'object' && init.f === 'is:archived') throw new TypeError('forced for the test');
        super(init);
      }
    };
  });

  await page.goto(`${served.origin}/archive`);
  const alert = page.locator('main').getByRole('alert');
  await alert.waitFor({ timeout: 15_000 });
  assert.match(await alert.innerText(), /This screen stopped working/);
  assert.match(await alert.innerText(), /Reference C-[0-9A-Z]{8}/);
  assert.equal(await page.getByRole('button', { name: 'Copy details' }).count(), 1);
  assert.equal(await page.getByRole('button', { name: 'Reload' }).count(), 1);

  // The frame is still there, and it still leads somewhere: the next screen draws in its place.
  const keeping = page.getByRole('list', { name: 'What is kept' });
  await keeping.getByRole('link', { name: 'Trash' }).click();
  await page.waitForFunction(() => location.pathname === '/trash', undefined, { timeout: 5_000 });
  await page.getByRole('heading', { level: 1, name: 'Trash' }).waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('main').getByText('This screen stopped working').count(), 0);
});

// A screen the frame draws directly, without a chunk of its own, is held by the frame's boundary.
test('chromium: 1280 px — the start screen throwing while drawing leaves the frame usable', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await signedIn(browser, 1280, 1000);
  t.after(close);
  await page.addInitScript(() => {
    const Original = window.URLSearchParams;
    window.URLSearchParams = class extends Original {
      constructor(init) {
        if (init && typeof init === 'object' && init.f === 'who:me is:open due:today') throw new TypeError('forced for the test');
        super(init);
      }
    };
  });

  await page.goto(`${served.origin}/`);
  const alert = page.locator('main').getByRole('alert');
  await alert.waitFor({ timeout: 15_000 });
  assert.match(await alert.innerText(), /This screen stopped working/);

  await page.getByRole('list', { name: 'What is kept' }).getByRole('link', { name: 'Archive' }).click();
  await page.getByRole('heading', { level: 1, name: 'Archive' }).waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('main').getByText('This screen stopped working').count(), 0);
});

test('chromium: 1280 px — a promise nobody awaited is told with a reference', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await signedIn(browser, 1280, 1000);
  t.after(close);
  await page.goto(`${served.origin}/archive`);
  await page.getByRole('heading', { level: 1, name: 'Archive' }).waitFor({ timeout: 15_000 });
  await page.evaluate(() => {
    void Promise.reject(new TypeError('forced for the test'));
  });
  await page.getByRole('paragraph').filter({ hasText: /Something did not work\. Reference: C-[0-9A-Z]{8}/ }).waitFor({ timeout: 5_000 });
});
