// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A refusal says the reason in words and keeps the reference behind *Details for support*
// (user-experience.md §5.8): the audit trail refused for a missing permission names that
// permission, and the reference and the code are one press away, with Copy. Against a stubbed API.

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

test('chromium: a refusal names the permission and folds its reference', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/audit') {
      return route.fulfill({
        status: 403, contentType: 'application/problem+json',
        json: { status: 403, code: 'forbidden', detail_code: 'access.not_permitted', params: { permission: 'AUDIT_READ' }, request_id: 'req-1ed9ffdb' },
      });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/administration/audit`);

  const toggle = page.locator('main').getByRole('button', { name: 'Details for support' });
  await toggle.waitFor({ timeout: 15_000 });
  const text = await page.locator('main').innerText();
  assert.match(text, /Read the audit trail/, 'the permission is named as the role editor names it');
  assert.doesNotMatch(text, /AUDIT_READ|req-1ed9ffdb/, 'a key or the reference is in the sentence');
  assert.equal(await toggle.getAttribute('aria-expanded'), 'false');

  await toggle.click();
  assert.equal(await toggle.getAttribute('aria-expanded'), 'true');
  const region = page.locator('main').getByRole('region', { name: 'Details for support' });
  assert.match(await region.innerText(), /req-1ed9ffdb/);
  assert.match(await region.innerText(), /access\.not_permitted/);
  await region.getByRole('button', { name: 'Copy reference' }).click();
  assert.equal(await page.evaluate(() => navigator.clipboard.readText()), 'req-1ed9ffdb');
});

// A list that could not be read draws the same fold (FailureState): the reason, *Try again*, and
// the reference and the code behind *Details for support*, never the bare reference line.
test('chromium: a list that could not be read folds its reference the same way, and asks again', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  let asked = 0;
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/approvals') {
      asked += 1;
      return route.fulfill({
        status: 403, contentType: 'application/problem+json',
        json: { status: 403, code: 'forbidden', detail_code: 'access.not_permitted', params: { permission: 'AUDIT_READ' }, request_id: 'req-2b7c0a11' },
      });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/approvals`);

  const failure = page.locator('main [role="alert"]');
  const toggle = failure.getByRole('button', { name: 'Details for support' });
  await toggle.waitFor({ timeout: 15_000 });
  assert.doesNotMatch(await failure.innerText(), /req-2b7c0a11/, 'the reference is in the sentence');
  await toggle.click();
  const region = failure.getByRole('region', { name: 'Details for support' });
  assert.match(await region.innerText(), /req-2b7c0a11/);
  assert.match(await region.innerText(), /access\.not_permitted/);
  const before = asked;
  await Promise.all([
    page.waitForRequest((request) => new URL(request.url()).pathname.endsWith('/api/v1/approvals'), { timeout: 5_000 }),
    failure.getByRole('button', { name: 'Try again' }).click(),
  ]);
  assert.ok(asked > before, 'Try again did not ask the list again');
});
