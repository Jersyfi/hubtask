// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The operator's hour at the installation holds (UC-INS-04 check 3): a token refresh and a reload
// keep it, and neither asks for a proof again or starts a second hour. A tab that lost its copy of
// the hour adopts the running end the server names instead of starting another (check 4).
//
// Against a stubbed API. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const census = {
  workspaces_active: 3, workspaces_suspended: 0, workspaces_pending_deletion: 0,
  accounts_active: 7, accounts_total: 9, workspaces_service_only: 0,
};

async function signedIn(browser, answer) {
  const sent = { raises: 0, refreshes: 0, expired: 0 };
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sessions:elevate') {
      sent.raises += 1;
      return answer(route);
    }
    if (path === '/auth/sessions:refresh') {
      sent.refreshes += 1;
      return route.fulfill({
        json: {
          access_token: `e2e-bearer-${sent.refreshes + 1}`, refresh_token: `e2e-refresh-${sent.refreshes + 1}`,
          token_type: 'Bearer', expires_in: 900,
        },
      });
    }
    if (path === '/admin/overview') return route.fulfill({ json: census });
    if (path === '/admin/journal' && request.headers().authorization === 'Bearer e2e-bearer') {
      // The access token runs out under the reader: one refusal, which the engine answers with a
      // refresh and one retry.
      sent.expired += 1;
      return route.fulfill({
        status: 401,
        json: { code: 'unauthenticated', detail_code: 'access.credential_expired', status: 401 },
      });
    }
    if (path === '/admin/journal') {
      return route.fulfill({ json: { data: [], page: { next_cursor: null, has_more: false } } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    if (sessionStorage.getItem('hubtask.bearer') === null) {
      sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
      sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    }
  });
  return { context, sent };
}

test('chromium: a refresh and a reload keep the hour, and ask for nothing again', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const until = new Date(Date.now() + 40 * 60_000).toISOString();
  const { context, sent } = await signedIn(browser, (route) => route.fulfill({ json: { elevated_until: until } }));
  const page = await context.newPage();

  await page.goto(`${served.origin}/instance`);
  await page.getByText('Accounts in all').waitFor();
  assert.equal(sent.raises, 1);

  // The journal is read with the access token that has run out: refused once, refreshed, retried.
  await page.goto(`${served.origin}/instance/journal`);
  await page.waitForFunction(() => sessionStorage.getItem('hubtask.bearer') !== 'e2e-bearer');
  assert.equal(sent.refreshes, 1, `the expired access token was not refreshed: ${JSON.stringify(sent)}`);

  await page.goto(`${served.origin}/instance`);
  await page.getByText('Accounts in all').waitFor();
  assert.match(await page.locator('[aria-live="polite"]').first().textContent() ?? '', /40 minutes left|39 minutes left/);
  assert.equal(sent.raises, 1, 'the reload after a refresh raised the session a second time');
  assert.equal(await page.getByRole('dialog').count(), 0, 'a proof was asked for again');
  assert.equal(
    await page.evaluate(() => sessionStorage.getItem('hubtask.elevated_until')),
    until,
    'the refresh dropped the hour this tab kept',
  );
  await context.close();
});

test('chromium: a tab that lost its copy adopts the running hour instead of starting another', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const until = new Date(Date.now() + 25 * 60_000).toISOString();
  const { context, sent } = await signedIn(browser, (route) => route.fulfill({
    status: 409,
    json: {
      code: 'conflict', detail_code: 'auth.session_already_elevated', status: 409,
      params: { until },
    },
  }));
  const page = await context.newPage();

  await page.goto(`${served.origin}/instance`);
  await page.getByText('Accounts in all').waitFor();
  assert.match(await page.locator('[aria-live="polite"]').first().textContent() ?? '', /25 minutes left|24 minutes left/);
  assert.equal(sent.raises, 1);
  assert.equal(await page.getByRole('dialog').count(), 0, 'a proof was asked for a running hour');
  await context.close();
});
