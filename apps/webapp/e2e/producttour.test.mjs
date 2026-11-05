// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A product's tour (user-experience.md §3.4): the first visit of a product's home points once at
// where to create there - the first of three steps -, ending it writes the product to the account's
// products_seen, an account that has seen it meets no mark, and the empty home offers the tour again.
//
// Chromium only; what the server keeps is its own tests'.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, MANIFEST, PAGE, forgetUnstubbed, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, account) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  forgetUnstubbed();
  const written = [];
  let me = account;
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, modules_on: ['WORKFLOWS', 'KNOWLEDGE'] } });
    if (path === '/accounts/me' || path === `/accounts/${ACCOUNT.id}`) return route.fulfill({ json: me });
    if (path === `/accounts/${ACCOUNT.id}/preferences` && request.method() === 'PATCH') {
      const body = request.postDataJSON();
      written.push(body);
      me = { ...me, ...body };
      return route.fulfill({ json: me });
    }
    if (path === '/containers') return route.fulfill({ json: { ...PAGE, data: [] } });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  return { page, written, failures, close: () => context.close() };
}

test('chromium: a product home\'s first visit is marked once, and ending it writes the product seen', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { products_seen: _seen, ...newcomer } = ACCOUNT;
  const { page, written, failures, close } = await open(browser, newcomer);
  t.after(close);

  await page.goto(`${served.origin}/pages`);
  const mark = page.getByRole('dialog').filter({ hasText: /Step \d of 3/u });
  await mark.waitFor({ timeout: 15_000 });
  // The first step points at the empty home's create act.
  assert.match((await mark.textContent()) ?? '', /Step 1 of 3.*Start here/su);
  assert.equal(await page.locator('[data-tour="product-create"]').count(), 1);
  await mark.getByRole('button', { name: 'Got it' }).click();
  await mark.waitFor({ state: 'detached', timeout: 10_000 });
  for (let tries = 0; tries < 50 && written.length === 0; tries++) await page.waitForTimeout(100);
  assert.deepEqual(written, [{ products_seen: ['KNOWLEDGE'] }]);

  // The empty home offers the tour again, and it walks from the same first step.
  const again = page.locator('[data-product-tour="KNOWLEDGE"]');
  await again.waitFor({ timeout: 10_000 });
  await again.click();
  await mark.waitFor({ timeout: 10_000 });
  await mark.getByRole('button', { name: 'Got it' }).click();
  await mark.waitFor({ state: 'detached', timeout: 10_000 });
  assert.equal(written.length, 1, 'a product already seen was written again');
  assert.deepEqual(failures, []);
});

test('chromium: an account that has seen the product meets no mark on its home', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, written, failures, close } = await open(browser, ACCOUNT);
  t.after(close);
  await page.goto(`${served.origin}/pages`);
  await page.locator('[data-product-tour="KNOWLEDGE"]').waitFor({ timeout: 15_000 });
  await page.waitForTimeout(500);
  assert.equal(await page.getByRole('dialog').filter({ hasText: /Step \d of 3/u }).count(), 0, 'a seen product was marked again');
  assert.deepEqual(written, []);
  assert.deepEqual(failures, []);
});
