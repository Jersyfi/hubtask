// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The members dialog draws the invitation's two acts only where the server accepts them (P-05):
// inviting and renewing an invitation both ask `MANAGE_MEMBERS` on the workspace (identity.md
// §12), so a reader who manages one hub only is offered neither. Against the shared fixture.
// Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, ADMIN_GRANTS, HUB, ITEMS, MANIFEST, OWNER_GRANTS, PAGE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

/**
 * The reader's standing: `workspace` manages the workspace's members (the workspace answers
 * grantable roles), `hub` manages the hub only (the workspace answers none).
 */
async function open(browser, standing) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, roles: OWNER_GRANTS.map((role) => ({ role, permissions: ['READ'] })) } });
    if (path === '/memberships' && request.method() === 'GET') {
      const scope = url.searchParams.get('scope_type');
      if (scope === 'TENANT') {
        return route.fulfill({
          json: {
            ...PAGE,
            data: standing === 'workspace' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'OWNER' }] : [],
            grantable_roles: standing === 'workspace' ? OWNER_GRANTS : [],
          },
        });
      }
      const rows = scope === 'HUB' && url.searchParams.get('scope_id') === HUB.id && standing === 'hub'
        ? [{ id: 'm-hub', scope_type: 'HUB', scope_id: HUB.id, account_id: ACCOUNT.id, role: 'ADMIN' }]
        : [];
      return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: ADMIN_GRANTS } });
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

async function share(page) {
  await page.goto(`${served.origin}/items/${ITEMS[2].id}`);
  await page.getByRole('button', { name: `Actions for ${ITEMS[2].title}` }).first().click();
  await page.getByRole('menuitem', { name: 'Share this entry' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('heading', { name: 'Give somebody a role' }).waitFor({ timeout: 15_000 });
  return dialog;
}

test('chromium: a reader managing one hub may give roles there, and is not offered an invitation the server refuses', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, 'hub');
  t.after(close);

  const dialog = await share(page);
  assert.equal(await dialog.getByLabel('Email address').count(), 0, 'the invitation is offered to a hub manager');
  assert.deepEqual(failures, []);
});

test('chromium: a reader managing the workspace is offered the invitation here', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, 'workspace');
  t.after(close);

  const dialog = await share(page);
  await dialog.getByLabel('Email address').waitFor();
  assert.deepEqual(failures, []);
});
