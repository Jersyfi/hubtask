// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// After an upgrade a tab opened before it asks for a screen whose chunk the server no longer has.
// The page reloads once onto the new release by itself; if the chunk is still missing it says
// Hubtask was updated, with Reload as the primary act - news, not an alarm (user-experience.md
// §5.8). The release is simulated: the screen's chunk is refused and the served document names
// another entry script.

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

test('chromium: 1280 px — a screen from an older release reloads once, then says Hubtask was updated', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await signedIn(browser, 1280, 1000);
  t.after(close);

  await page.route(/\/assets\/ArchiveView-[^/]+\.js$/, (route) => route.fulfill({ status: 404, body: '' }));
  await page.route(`${served.origin}/`, async (route) => {
    const answer = await route.fetch();
    const html = (await answer.text()).replace(/(<script[^>]*type="module"[^>]*src=")[^"]+/, '$1/assets/index-next.js');
    await route.fulfill({ response: answer, body: html });
  });
  let loads = 0;
  page.on('load', () => { loads += 1; });

  await page.goto(`${served.origin}/archive`);
  const notice = page.locator('main [data-kind="updated"]');
  await notice.waitFor({ timeout: 15_000 });
  assert.equal(loads, 2, 'reloaded once by itself, and not again');
  assert.equal(await notice.getAttribute('role'), 'status', 'news, politely announced, not an alert');
  assert.match(await notice.innerText(), /Hubtask was updated/);
  assert.equal(await page.locator('main').getByRole('alert').count(), 0);
  assert.equal(await notice.getByRole('button', { name: 'Reload' }).count(), 1);
  assert.equal(await page.locator('main').getByRole('link', { name: 'Back to the start' }).count(), 1);
});
