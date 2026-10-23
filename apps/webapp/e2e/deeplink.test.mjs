// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// An address opened directly keeps its screen for an account that has not taken the tour yet -
// the owner of a fresh installation, whose `onboarding_completed_at` is null. The tour's first
// step is on the overview and navigates there, so it starts on the overview only and never
// replaces an address somebody opened by link, bookmark or mail (design-system.md §8).
// Chromium: a set of addresses, with no engine-specific part.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, signedIn } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const UNTOURED = { ...ACCOUNT, id: '01a0e2f7-0000-7000-8000-000000000001', onboarding_completed_at: null };

test('chromium: a profile screen opened by its address stays, and the tour waits for the overview', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const tab = await signedIn(browser, 1280, 900);
  t.after(() => tab.context.close());
  // Registered after the fixture's, so it answers first.
  await tab.context.route('**/api/v1/accounts/me', (route) => route.fulfill({ json: UNTOURED }));
  const { page } = tab;

  for (const path of ['/profile/security', '/profile/sessions']) {
    const read = page.waitForResponse((response) => response.url().endsWith('/api/v1/accounts/me'));
    await page.goto(`${served.origin}${path}`);
    await read;
    await page.locator('main h1').first().waitFor({ timeout: 15_000 });
    // The tour would have navigated by now: its start follows the account's arrival.
    await page.waitForTimeout(1_000);
    assert.equal(await page.evaluate(() => location.pathname), path, `${path} was replaced`);
    assert.equal(await page.locator('.spotlight').count(), 0, `the tour started over ${path}`);
  }

  // On the overview it starts as before.
  await page.goto(`${served.origin}/`);
  await page.locator('.spotlight').waitFor({ state: 'attached', timeout: 15_000 })
    .catch(() => assert.fail('the tour did not start on the overview'));
  assert.deepEqual(tab.failures, [], 'the bundle threw');
});
