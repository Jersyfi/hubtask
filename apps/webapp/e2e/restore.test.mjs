// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The restore screen, walked as the owner and as an administrator: replacing the workspace is the
// owner's (DELETE_CONTAINER, backup-restore.md §8.2), so an administrator is not offered it, and the
// owner is (P-05).
//
// Chromium only: what this asserts is the rendered form.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, MANIFEST, OTHER, stub, forgetUnstubbed } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const ROLES = [
  { role: 'OWNER', permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS', 'DELETE_CONTAINER'] },
  { role: 'ADMIN', permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS', 'READ_CONFIGURATION'] },
];

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, role) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, roles: ROLES } });
    if (path === '/memberships' && url.searchParams.get('scope_type') === 'TENANT') {
      return route.fulfill({ json: { data: [
        { id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role },
        { id: 'm9', scope_type: 'TENANT', account_id: OTHER.id, role: 'MEMBER' },
      ], page: { next_cursor: null, has_more: false } } });
    }
    if (path === '/backup-targets') return route.fulfill({ json: [] });
    if (path === '/legal-holds') return route.fulfill({ json: [] });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  forgetUnstubbed();
  await page.goto(`${served.origin}/administration/restore`);
  await page.getByLabel('How').waitFor();
  return { page, failures, close: () => context.close() };
}

async function offeredModes(page) {
  // The membership arrives after the first paint; the offer follows it.
  await page.waitForTimeout(300);
  return page.getByLabel('How').locator('option').allTextContents();
}

test('chromium: the owner is offered replacing the workspace', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await open(browser, 'OWNER');
    assert.ok((await offeredModes(page)).includes('Replace this workspace'));
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

test('chromium: an administrator is not offered replacing the workspace', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await open(browser, 'ADMIN');
    const modes = await offeredModes(page);
    assert.equal(modes.length, 3, modes.join(', '));
    assert.ok(!modes.includes('Replace this workspace'));
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});
