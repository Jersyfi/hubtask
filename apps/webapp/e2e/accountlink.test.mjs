// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The provider's account link (UC-INS-13 check 11): where the manifest answers `account_link` - the
// server does to the workspace's owners and administrators only - "Your provider's account page"
// stands beside "Not included in your plan"; where it answers none, nothing does. Against the
// stubbed API: the walk proves what the client draws from an answer, the server tests who is
// answered what. Chromium only, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { MANIFEST, PAGE, signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const ACCOUNT_PAGE = 'https://shop.example/account';

/** A signed-in page whose manifest answers what `answer` adds, and the stubbed reads beside it. */
async function withManifest(browser, answer, extra = () => undefined) {
  const { page, failures, context } = await signedIn(browser, 1280, 1000);
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, ...answer } });
    if (path === '/automation/rules') return route.fulfill({ json: { ...PAGE, data: [] } });
    if (path === '/automation/rules:check') return route.fulfill({ json: { data: [] } });
    const answered = extra(route, request, path);
    if (answered !== undefined) return answered;
    return stub(route);
  });
  return { page, failures, context };
}

test('chromium: an owner finds the provider\'s account page beside "Not included in your plan"', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await withManifest(browser, { not_included: ['AUTOMATION_RULES'], account_link: ACCOUNT_PAGE });
  t.after(() => context.close());

  await page.goto(`${served.origin}/administration/rules`);
  const link = page.getByRole('link', { name: 'Your provider\'s account page' });
  await link.waitFor({ timeout: 5_000 });
  assert.equal(await link.getAttribute('href'), ACCOUNT_PAGE);
  assert.equal(await link.getAttribute('rel'), 'noopener noreferrer');
  assert.deepEqual(failures, []);
});

test('chromium: where no link is answered, the notice stands alone', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await withManifest(browser, { not_included: ['AUTOMATION_RULES'] });
  t.after(() => context.close());

  await page.goto(`${served.origin}/administration/rules`);
  await page.getByText('Rules don\'t run in this plan.').waitFor({ timeout: 5_000 });
  assert.equal(await page.getByRole('link', { name: 'Your provider\'s account page' }).count(), 0);
  assert.deepEqual(failures, []);
});

const STANDINGS = [
  { quota: 'items', limit: 100, used: 100, ratio: 1, configured: false, locked: false },
  { quota: 'webhook_targets', limit: 5, used: 7, ratio: 1.4, configured: false, locked: false },
  { quota: 'export_jobs', limit: 0, used: 5, configured: false, locked: false },
  { quota: 'api_requests_per_minute', limit: 600, configured: false, locked: false },
  { quota: 'automation_runs_per_hour', limit: 500, used: 34, ratio: 0.068, configured: false, locked: false },
];

test('chromium: a reached limit says what still works, with the provider\'s account page for an owner', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const quotas = (route, _request, path) => (path === '/quotas' ? route.fulfill({ json: STANDINGS }) : undefined);
  const { page, failures, context } = await withManifest(browser, { account_link: ACCOUNT_PAGE }, quotas);
  t.after(() => context.close());

  await page.goto(`${served.origin}/administration/quotas`);
  await page.getByText('The limit for Entries is reached.').waitFor({ timeout: 5_000 });
  // Reached at and above the limit; never where there is no ceiling or no live count.
  assert.equal(await page.getByText(/The limit for .* is reached\./).count(), 2);
  assert.ok(await page.getByText('Everything that exists stays as it is.').first().isVisible());
  assert.equal(await page.getByRole('link', { name: 'Your provider\'s account page' }).count(), 2);
  assert.deepEqual(failures, []);

  // Without an answered link - an auditor, or no address set - the notice stands alone.
  const second = await withManifest(browser, { workspace_permissions: ['READ_CONFIGURATION', 'AUDIT_READ'] }, quotas);
  t.after(() => second.context.close());
  await second.page.goto(`${served.origin}/administration/quotas`);
  await second.page.getByText('The limit for Entries is reached.').waitFor({ timeout: 5_000 });
  assert.equal(await second.page.getByRole('link', { name: 'Your provider\'s account page' }).count(), 0);
});
