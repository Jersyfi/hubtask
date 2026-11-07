// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A connector's provider return on the installation's host, walked (identity.md §10.1): the page
// asks where the return belongs before it draws anything, and goes to the workspace's host with
// the path and query unchanged - or, where the answer is this host or none, stays and is the page.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const STATE = '01936f2a-7c1e-7000-8000-000000000001.aGFuZGxl';
const WORKSPACE = 'http://anna.hub.example';

async function walk(answer) {
  const { origin, close } = await serve(DIST);
  const browser = await chromium.launch();
  const context = await browser.newContext();
  const asked = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.pathname.endsWith('/api/v1/auth/connector:locate')) {
      asked.push(request.postDataJSON());
      return route.fulfill(answer(origin));
    }
    if (url.pathname.endsWith('/api/v1/meta/capabilities')) {
      return route.fulfill({ json: { product_version: 'e2e', api_version: 'v1', tenancy_mode: 'multi', item_types: [], view_layouts: [], supported_locales: [{ locale: 'en', direction: 'ltr' }], roles: [], limits: {}, features: {} } });
    }
    return route.fulfill({ status: 404, json: { code: 'errors.not_found' } });
  });
  // The workspace's host, which this walk does not run: whatever arrives there is answered blank.
  await context.route(`${WORKSPACE}/**`, (route) => route.fulfill({ contentType: 'text/html', body: '<p>workspace</p>' }));
  const page = await context.newPage();
  return { origin, page, asked, close: async () => { await browser.close(); await close(); } };
}

test('a calendar return on the installation host goes to the workspace host whole', async () => {
  const query = `?code=c1&state=${STATE}`;
  const { origin, page, asked, close } = await walk(() => ({ json: { base_url: WORKSPACE } }));
  try {
    await page.goto(`${origin}/profile/calendars${query}`);
    await page.waitForURL(`${WORKSPACE}/**`);
    assert.equal(page.url(), `${WORKSPACE}/profile/calendars${query}`);
    assert.deepEqual(asked, [{ state: STATE, kind: 'CALENDAR_PUSH' }]);
  } finally {
    await close();
  }
});

test('a return that belongs here, or is refused, stays and draws the page', async () => {
  for (const answer of [
    (origin) => ({ json: { base_url: origin } }),
    () => ({ status: 404, json: { code: 'errors.not_found', detail_code: 'auth.connector_return_not_found', status: 404 } }),
  ]) {
    const { origin, page, asked, close } = await walk(answer);
    try {
      await page.goto(`${origin}/administration/mail/oauth?code=c1&state=${STATE}`);
      // Signed out here, so the page is the sign-in card - drawn only once the question is answered.
      await page.waitForSelector('input[type="password"], input[autocomplete="username"], input[type="email"]');
      assert.equal(new URL(page.url()).origin, origin);
      assert.deepEqual(asked, [{ state: STATE, kind: 'MAILBOX' }]);
    } finally {
      await close();
    }
  }
});
