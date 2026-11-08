// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// On a phone a list of records is a column of list rows, and nothing scrolls sideways
// (user-experience.md §5.12, page-system §4.4): the sessions' DataTable at 375 px, against a
// stubbed API. At a desktop width the same table is a table with its head drawn.

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

const SESSIONS = [
  { id: '01a0e2e0-0000-7000-8000-0000000005e1', created_at: '2026-10-30T08:00:00Z', last_used_at: '2026-11-02T09:00:00Z', user_agent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15', ip_class: '203.0.113.0/24', current: true, signed_in_with: 'PASSWORD_TOTP' },
  { id: '01a0e2e0-0000-7000-8000-0000000005e2', created_at: '2026-10-01T08:00:00Z', last_used_at: null, user_agent: 'Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Mobile Safari/537.36', ip_class: '198.51.100.0/24', current: false, signed_in_with: 'OIDC' },
];

for (const [width, isPhone] of [[375, true], [1280, false]]) {
  test(`chromium: ${width} px — the sessions table ${isPhone ? 'becomes list rows' : 'stays a table'}`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const context = await browser.newContext({ viewport: { width, height: 900 } });
    t.after(() => context.close());
    await context.route('**/api/v1/**', async (route) => {
      const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
      if (path === '/auth/sessions' && route.request().method() === 'GET') return route.fulfill({ json: SESSIONS });
      return stub(route);
    });
    await context.addInitScript(() => {
      sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
      sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    });
    const page = await context.newPage();
    await page.goto(`${served.origin}/profile/sessions`);
    const table = page.locator('main table');
    await table.locator('tbody tr').nth(1).waitFor({ timeout: 15_000 });

    const shape = await table.evaluate((node) => {
      const scroll = node.closest('[role="region"]');
      const row = node.querySelector('tbody tr');
      return {
        sideways: scroll.scrollWidth - scroll.clientWidth,
        pageSideways: document.documentElement.scrollWidth - document.documentElement.clientWidth,
        headDrawn: node.querySelector('thead').getBoundingClientRect().height > 2,
        rowDisplay: getComputedStyle(row).display,
      };
    });
    assert.ok(shape.pageSideways <= 0, `the page scrolls sideways by ${shape.pageSideways}px`);
    if (isPhone) {
      assert.ok(shape.sideways <= 0, `the table scrolls sideways by ${shape.sideways}px`);
      assert.equal(shape.headDrawn, false, 'the column head is drawn above the list rows');
      assert.equal(shape.rowDisplay, 'grid');
    } else {
      assert.equal(shape.headDrawn, true);
      assert.equal(shape.rowDisplay, 'table-row');
    }
  });
}

// The role × permission matrix is the widest table in the administration: on a phone one card per
// role, never a matrix cut at its edge.
test('chromium: 375 px — who may do what stacks one card per role', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 375, height: 900 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', (route) => stub(route));
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/administration/people?tab=permissions`);
  const table = page.locator('main table').first();
  await table.locator('tbody tr').nth(1).waitFor({ timeout: 15_000 });

  const shape = await table.evaluate((node) => {
    const scroll = node.closest('[role="region"]');
    const row = node.querySelector('tbody tr');
    return {
      sideways: scroll.scrollWidth - scroll.clientWidth,
      pageSideways: document.documentElement.scrollWidth - document.documentElement.clientWidth,
      headDrawn: node.querySelector('thead').getBoundingClientRect().height > 2,
      rowDisplay: getComputedStyle(row).display,
    };
  });
  assert.ok(shape.pageSideways <= 0, `the page scrolls sideways by ${shape.pageSideways}px`);
  assert.ok(shape.sideways <= 0, `the matrix scrolls sideways by ${shape.sideways}px`);
  assert.equal(shape.headDrawn, false, 'the column head is drawn above the cards');
  assert.equal(shape.rowDisplay, 'flex');
});
