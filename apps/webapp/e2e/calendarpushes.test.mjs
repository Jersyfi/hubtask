// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The calendars a reader's feeds are written into (UC-INT-16) on the integrations anatomy
// (user-experience.md §7.10): each push a card with its state in words, a stopped one's failure
// beside *Connect again*, which signs in to the same feed and provider again, the head's figure,
// and the column's figure on *Calendars*; with none, the screen says what a push is for. Against
// the stubbed API; Chromium only, as the other walks.

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

const FEED = '01a0e2e0-0000-7000-8000-000000002101';
const WRITING = {
  id: '01a0e2e0-0000-7000-8000-000000002111', feed_id: FEED, provider: 'GOOGLE', address: 'ada@example.org',
  state: 'ACTIVE', pushed_at: '2026-11-03T09:15:00Z', created_at: '2026-10-30T10:00:00Z',
};
const STOPPED = {
  id: '01a0e2e0-0000-7000-8000-000000002112', feed_id: FEED, provider: 'MICROSOFT', address: 'ada@example.com',
  state: 'FAILED', failure: 'calendar_push.grant_revoked', pushed_at: '2026-11-01T09:15:00Z', created_at: '2026-10-30T10:00:00Z',
};

test('chromium: a stopped calendar push says why beside Connect again, and the column counts it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  const writes = [];
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api\/v1/, '');
    if (path === '/integrations/calendar-pushes' && request.method() === 'GET') {
      return route.fulfill({ json: { data: [WRITING, STOPPED], providers: ['MICROSOFT', 'GOOGLE'] } });
    }
    if (path === '/integrations/calendar-pushes:authorize') {
      writes.push(request.postDataJSON());
      return route.fulfill({ json: { authorize_url: `${served.origin}/profile/calendars`, expires_at: '2026-11-04T10:00:00Z' } });
    }
    return stub(route);
  });

  await page.goto(`${served.origin}/profile/calendars`);
  await page.locator('[data-pushes-figure]').getByText('1 writing · 1 stopped').waitFor({ timeout: 10_000 });

  const column = page.getByRole('navigation', { name: 'Your settings' });
  assert.equal(await column.getByRole('link', { name: /1 stopped/ }).count(), 1, 'the figure is not heard with the row');

  const writing = page.getByRole('row').filter({ has: page.locator('[data-push="GOOGLE"]') });
  await writing.getByText('Writing', { exact: true }).waitFor();
  // UC-INT-16 check 3: each push names its provider, the account, whether it writes and when it
  // last did.
  await writing.getByText('Google Calendar', { exact: true }).waitFor();
  await writing.getByText('ada@example.org', { exact: true }).waitFor();
  await writing.getByText('Last written', { exact: false }).waitFor();
  assert.equal(await writing.getByRole('button', { name: 'Connect again' }).count(), 0, 'a push that writes offers no reconnection');

  const stopped = page.getByRole('row').filter({ has: page.locator('[data-push="MICROSOFT"]') });
  await stopped.getByText('Stopped', { exact: true }).waitFor();
  await stopped.locator('[data-push-failure]').getByText('no longer gives access', { exact: false }).waitFor();
  await stopped.getByRole('button', { name: 'Connect again' }).click();
  for (let tries = 0; tries < 50 && writes.length === 0; tries++) await new Promise((resolve) => setTimeout(resolve, 100));
  assert.deepEqual(writes, [{ feed_id: FEED, provider: 'MICROSOFT' }]);
  assert.deepEqual(failures, []);
});

test('chromium: with no calendar push the screen says what one is for', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 375, 812);
  t.after(() => context.close());

  await page.goto(`${served.origin}/profile/calendars`);
  await page.getByText('No feed is written into a calendar.').waitFor({ timeout: 10_000 });
  await page.getByText('stand beside your appointments', { exact: false }).waitFor();
  assert.equal(await page.locator('[data-pushes-figure]').count(), 0, 'nothing to count, no figure');
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.equal(overflow, 0, 'the screen scrolls sideways at phone width');
  assert.deepEqual(failures, []);
});
