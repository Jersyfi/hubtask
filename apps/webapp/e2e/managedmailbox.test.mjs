// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What needs a mailbox, for somebody without one (UC-ID-20 check 6): the managed person's own
// notification preferences say "not possible without a mailbox" instead of a table of switches for
// mail that has nowhere to go - and a person with an address still gets the table.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

/** Lena, as `GET /accounts/me` answers a managed account: a sign-in name and no address. */
const { email: _none, ...withoutAddress } = ACCOUNT;
const LENA = { ...withoutAddress, display_name: 'Lena', sign_in_name: 'lena' };

async function open(browser, me) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith('/api/v1/accounts/me') || path.endsWith(`/api/v1/accounts/${ACCOUNT.id}`)) {
      return route.fulfill({ json: me });
    }
    if (path.endsWith('/notification-preferences')) return route.fulfill({ json: [] });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  return { page, close: () => context.close() };
}

test('a managed person reads that mail is not possible without a mailbox', async () => {
  const browser = await chromium.launch();
  const { page, close } = await open(browser, LENA);
  try {
    await page.goto(`${served.origin}/profile/notifications`);
    await page.getByText('Not possible without a mailbox', { exact: false }).waitFor();
    assert.equal(await page.getByRole('table').count(), 0, 'a table of switches for mail that goes nowhere');
  } finally {
    await close();
    await browser.close();
  }
});

test('a person with an address still gets the table', async () => {
  const browser = await chromium.launch();
  const { page, close } = await open(browser, ACCOUNT);
  try {
    await page.goto(`${served.origin}/profile/notifications`);
    await page.getByText('One row for each thing this workspace tells people about.').waitFor();
    assert.equal(await page.getByText('Not possible without a mailbox', { exact: false }).count(), 0);
  } finally {
    await close();
    await browser.close();
  }
});
