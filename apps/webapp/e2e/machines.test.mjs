// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *Machines and apps* on the integrations anatomy (user-experience.md §7.10): the head counts the
// service accounts - one without a role among them, since its tokens are refused by everything -
// and the apps, and adds to the tab that is open through a dialog; with none, each tab says what
// it is for and offers the add. Against the stubbed API; Chromium only, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const MACHINE = { id: '01a0e2e0-0000-7000-8000-00000000c602', display_name: 'Nightly export', kind: 'SERVICE' };

test('chromium: the head counts a service account without a role, and adds one through its dialog', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  const created = [];
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/service-accounts' && request.method() === 'POST') {
      created.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: { id: '01a0e2e0-0000-7000-8000-00000000c603', display_name: 'Ticket bridge', kind: 'SERVICE' } });
    }
    if (path === '/auth/service-accounts') return route.fulfill({ json: [MACHINE] });
    return stub(route);
  });

  await page.goto(`${served.origin}/administration/machines`);
  await page.locator('[data-machines-figure]').getByText('1 service account · 1 without a role · 0 apps').waitFor({ timeout: 10_000 });
  await page.getByText('No role yet').waitFor();

  await page.getByRole('button', { name: 'Add a service account' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Name', { exact: false }).first().fill('Ticket bridge');
  await dialog.getByRole('button', { name: 'Create it' }).click();
  await dialog.waitFor({ state: 'detached', timeout: 5_000 });
  assert.equal(created.length, 1);
  assert.deepEqual(created[0], { display_name: 'Ticket bridge' });
  assert.deepEqual(failures, []);
});

test('chromium: with no app registered the tab says what one is for and offers the add', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 375, 812);
  t.after(() => context.close());

  await page.goto(`${served.origin}/administration/machines?tab=apps`);
  await page.getByText('No apps are registered.').waitFor({ timeout: 10_000 });
  await page.getByText('Each person decides for themselves.', { exact: false }).first().waitFor();
  await page.getByRole('main').getByRole('button', { name: 'Register an app' }).last().click();
  await page.getByRole('dialog').getByLabel('Redirect', { exact: false }).first().waitFor();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.equal(overflow, 0, 'the screen scrolls sideways at phone width');
  assert.deepEqual(failures, []);
});
