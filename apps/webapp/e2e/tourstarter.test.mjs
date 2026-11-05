// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// One onboarding order (user-experience.md §3.1): at a fresh workspace's first sign-in the starter
// is asked and the base tour waits - also where the account arrives before the workspace says it
// offers the starter -, and an address opened while the starter is unanswered stays where it is
// rather than being walked back to Today by a tour. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, WORKSPACE, forgetUnstubbed, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const FRESH = { ...ACCOUNT, id: '01a0e2e0-0000-7000-8000-0000000032b1', onboarding_completed_at: null };

for (const [width, height] of [[1440, 900], [390, 844]]) {
  test(`chromium: the tour waits for the starter at ${width}x${height}`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const context = await browser.newContext({ viewport: { width, height } });
    t.after(() => context.close());
    forgetUnstubbed();
    await context.route('**/api/v1/**', async (route) => {
      const url = new URL(route.request().url());
      if (url.pathname.endsWith('/api/v1/accounts/me')) return route.fulfill({ json: FRESH });
      if (url.pathname.endsWith('/api/v1/tenant') && route.request().method() === 'GET') {
        // The workspace answers after the account: the order a slow read gives.
        await new Promise((resolve) => setTimeout(resolve, 1200));
        return route.fulfill({ json: { ...WORKSPACE, starter_offered: true } });
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
    await page.locator('[data-starter]').waitFor({ timeout: 15_000 });
    await page.waitForTimeout(1000);
    assert.equal(await page.locator('.spotlight').count(), 0, 'the tour opened together with the starter');

    for (const path of ['/search', '/administration/people', '/trash']) {
      await page.goto(`${served.origin}${path}`);
      await page.waitForTimeout(2500);
      assert.equal(new URL(page.url()).pathname, path, `${path} was sent elsewhere while the starter is unanswered`);
    }
    assert.deepEqual(failures, []);
  });
}
