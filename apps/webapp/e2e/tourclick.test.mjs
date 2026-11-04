// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The first sign-in's tour is finished by clicking (design-system.md §8): at a desktop and a
// phone size every step's mark stands inside the viewport and its Next takes a pointer - none is
// clipped at an edge or covered by its own text. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, MANIFEST, forgetUnstubbed, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const FRESH = { ...ACCOUNT, id: '01a0e2e0-0000-7000-8000-0000000032a1', onboarding_completed_at: null };

for (const [width, height] of [[1440, 900], [1280, 860], [390, 844]]) {
  test(`chromium: the tour is clicked through at ${width}x${height}`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const context = await browser.newContext({ viewport: { width, height } });
    t.after(() => context.close());
    forgetUnstubbed();
    const written = [];
    await context.route('**/api/v1/**', async (route) => {
      const url = new URL(route.request().url());
      if (url.pathname.endsWith('/api/v1/accounts/me')) return route.fulfill({ json: FRESH });
      if (url.pathname.endsWith('/preferences') && route.request().method() === 'PATCH') {
        written.push(route.request().postDataJSON());
        return route.fulfill({ json: { ...FRESH, ...route.request().postDataJSON() } });
      }
      return stub(route);
    });
    await context.addInitScript(() => {
      sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
      sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    });
    const page = await context.newPage();
    const failures = [];
    page.on('pageerror', (error) => failures.push(String(error)));
    await page.goto(`${served.origin}/`);
    const spotlight = page.locator('.spotlight');
    await spotlight.waitFor({ state: 'attached', timeout: 15_000 });

    const seen = [];
    let firstWords = '';
    // A step may move to another screen first: the next mark is waited for, the end is its absence.
    const another = () => spotlight.waitFor({ state: 'attached', timeout: 8_000 }).then(() => true, () => false);
    for (let step = 0; step < 20 && (await another()); step += 1) {
      const mark = page.locator('[role="dialog"][aria-labelledby^="coach-"]');
      await mark.waitFor({ state: 'visible', timeout: 5_000 });
      await page.waitForTimeout(150);
      const count = (await mark.locator('.count').textContent())?.trim() ?? '';
      if (step === 0) firstWords = (await mark.innerText()) ?? '';
      const box = await mark.boundingBox();
      assert.ok(box && box.y >= 0 && box.x >= 0 && box.y + box.height <= height + 1 && box.x + box.width <= width + 1,
        `${count}: the mark ${JSON.stringify(box)} leaves the ${width}x${height} viewport`);
      const next = mark.getByRole('button').first();
      // A plain click: Playwright refuses one whose point another element covers.
      await next.click({ timeout: 3_000 }).catch((error) => assert.fail(`${count}: Next cannot be clicked - ${String(error).split('\n')[0]}`));
      seen.push(count);
    }
    assert.equal(await spotlight.count(), 0, `the tour did not end after ${JSON.stringify(seen)}`);
    // Step 8 points at the whole of Your settings, taller than the screen: the one that was
    // clipped. The fixture draws no entry opener, so the tour passes over its last step.
    // A phone draws neither the products' column nor the palette in its bar: those are no steps
    // there, so the count runs without a gap (the fixture's workspace has no product on).
    const isPhone = width < 600;
    const settings = isPhone ? 'Step 7 of 8' : 'Step 8 of 9';
    assert.ok(seen.includes(settings), `${settings} was not shown: ${JSON.stringify(seen)}`);
    seen.forEach((count, index) => assert.match(count, new RegExp(`^Step ${index + 1} of `), `the count skips at ${JSON.stringify(seen)}`));
    // Below `expanded` the first step points at the bar's menu button, and says so.
    if (width < 1280) assert.match(firstWords, /This button opens the tree/);
    else assert.doesNotMatch(firstWords, /This button/);
    // A tour walked to its end is the account's no more: the next sign-in does not start it again.
    for (let wait = 0; wait < 20 && !written.some((body) => typeof body?.onboarding_completed_at === 'string'); wait += 1) await page.waitForTimeout(100);
    assert.ok(written.some((body) => typeof body?.onboarding_completed_at === 'string'), `the walked tour was not written: ${JSON.stringify(written)}`);
    assert.deepEqual(failures, []);
  });
}

// user-experience.md §3.4: where a product is on, the second step points at the products band.
test('chromium: with a product on, the tour shows where the products live', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 860 } });
  t.after(() => context.close());
  forgetUnstubbed();
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith('/api/v1/accounts/me')) return route.fulfill({ json: FRESH });
    if (url.pathname.endsWith('/api/v1/meta/capabilities')) return route.fulfill({ json: { ...MANIFEST, modules_on: ['KNOWLEDGE'] } });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/`);
  const mark = page.locator('[role="dialog"][aria-labelledby^="coach-"]');
  await mark.waitFor({ state: 'visible', timeout: 15_000 });
  await mark.getByRole('button').first().click();
  await page.waitForTimeout(300);
  await mark.waitFor({ state: 'visible', timeout: 5_000 });
  assert.match((await mark.textContent()) ?? '', /Your products live here/u);
});
