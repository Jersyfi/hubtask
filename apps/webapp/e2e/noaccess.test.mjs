// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A person nobody has given anything yet - such as somebody a sign-in provider brought in with
// nothing - is told that an administrator still has to give access, rather than landing on an empty
// workspace without explanation (UC-ID-08 check 2). *Create hub* is absent exactly there: the server
// refuses every hub, a private one of one's own included, to an account holding no role anywhere.
// Wherever the account holds access - a person whose only role is their own private hub, a guest on
// one entry - *Create hub* stays as before and no notice is drawn (UC-ID-16 check 1).
//
// Against a stubbed API with an empty workspace. Chromium only.

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

const NOTICE = 'An administrator of this workspace still has to give you access.';

async function open(browser, hasAccess) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const account = { ...ACCOUNT, has_access: hasAccess };
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/accounts/me') return route.fulfill({ json: account });
    if (path === '/sync:snapshot') {
      return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: '{"cursor":"c-e2e"}\n' });
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
  return { page, failures, close: () => context.close() };
}

test('chromium: a person holding no access is told so, and offered no hub the server would refuse', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, false);
  t.after(close);

  await page.goto(`${served.origin}/`);
  const main = page.getByRole('main');
  await main.getByText(NOTICE).waitFor();
  // In the overview and in the navigation: one sentence each, and no control the server refuses.
  assert.equal(await page.getByText(NOTICE).count(), 2);
  assert.equal(await page.getByRole('button', { name: 'Create hub' }).count(), 0, 'Create hub offered without access');
  assert.equal(await page.getByText('No hubs yet', { exact: false }).count(), 0, 'the empty-workspace sentence stands beside the notice');
  assert.deepEqual(failures, []);
});

test('chromium: a person holding access sees no notice, and Create hub as before', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, true);
  t.after(close);

  await page.goto(`${served.origin}/`);
  await page.getByRole('button', { name: 'Create hub' }).first().waitFor();
  assert.equal(await page.getByText(NOTICE).count(), 0, 'the notice was drawn for a person with access');
  assert.deepEqual(failures, []);
});
