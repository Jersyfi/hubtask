// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Where an operator finds the installation (UC-INS-04 check 1 and its story): on a single-workspace
// installation as the last group of Administration, and not in the account menu; on a multi-workspace
// installation in the account menu. At desktop width and at 375 px.
//
// Against a stubbed API. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, MANIFEST, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

async function administration(browser, tenancy, viewport) {
  const context = await browser.newContext({ viewport });
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, tenancy_mode: tenancy, instance: { reachable: true } } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/administration/workspace`);
  return { context, page };
}

/** The account menu's rows, opened from the bar. */
async function menuRows(page) {
  await page.getByRole('button', { name: new RegExp(ACCOUNT.display_name) }).first().click();
  await page.getByText('Take the tour again').waitFor();
  const text = await page.getByText('Take the tour again').locator('xpath=ancestor::*[.//text()[contains(., "Sign out")]][1]').innerText();
  return text;
}

test('chromium: on a private installation the installation is the administration\'s last group', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  const desktop = { width: 1280, height: 900 };
  const { context, page } = await administration(browser, 'single', desktop);
  const row = page.getByRole('navigation').getByText('Applies to every workspace you create');
  await row.waitFor();
  const column = await page.getByRole('navigation').filter({ hasText: 'Applies to every workspace you create' }).innerText();
  assert.ok(column.trimEnd().endsWith('Applies to every workspace you create'), 'the installation is not the last group');
  assert.doesNotMatch(await menuRows(page), /\bInstallation\b/, 'the menu offers it a second time');
  await page.keyboard.press('Escape');
  await row.click();
  await page.waitForURL(/\/instance$/);
  await context.close();

  const multi = await administration(browser, 'multi', desktop);
  await multi.page.getByRole('navigation').getByText('Sign-in provider').waitFor();
  assert.equal(await multi.page.getByText('Applies to every workspace you create').count(), 0);
  assert.match(await menuRows(multi.page), /\bInstallation\b/);
  await multi.context.close();
});

test('chromium: at 375 px the administration carries the installation too', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  const { context, page } = await administration(browser, 'single', { width: 375, height: 812 });
  await page.getByText('Applies to every workspace you create').first().waitFor({ state: 'attached' });
  await context.close();
});
