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

import { ACCOUNT, ADMIN_GRANTS, HUB, INVITED, ITEMS, MANIFEST, OWNER_GRANTS, PAGE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const RENEWED = 'https://hubtask.example/invitation#token=hbt_inv_renewed';
const bertRow = (dialog) => dialog.getByRole('listitem').filter({ hasText: INVITED.display_name });

/**
 * The reader's standing: `workspace` manages the workspace's members (the workspace answers
 * grantable roles), `hub` manages the hub only (the workspace answers none).
 */
async function open(browser, standing, { mail = true, sent = {} } = {}) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') {
      return route.fulfill({
        json: { ...MANIFEST, roles: OWNER_GRANTS.map((role) => ({ role, permissions: ['READ'] })), features: { ...MANIFEST.features, mail } },
      });
    }
    if (path === `/accounts/${INVITED.id}:renew-invitation`) {
      sent.renewed = (sent.renewed ?? 0) + 1;
      return route.fulfill({ headers: { 'cache-control': 'no-store' }, json: { ...INVITED, ...(mail ? {} : { invitation_link: RENEWED }) } });
    }
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
      // Bert was invited to the hub alone, so the entry's dialog shows him as inherited from it.
      const rows = scope === 'HUB' && url.searchParams.get('scope_id') === HUB.id
        ? [
          ...(standing === 'hub' ? [{ id: 'm-hub', scope_type: 'HUB', scope_id: HUB.id, account_id: ACCOUNT.id, role: 'ADMIN' }] : []),
          { id: 'm-bert', scope_type: 'HUB', scope_id: HUB.id, account_id: INVITED.id, role: 'GUEST' },
        ]
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
  await bertRow(dialog).waitFor();
  assert.equal(await bertRow(dialog).getByRole('button', { name: /invitation/ }).count(), 0, 'a renewal is offered to a hub manager');
  assert.deepEqual(failures, []);
});

// #1412: somebody invited to one hub is renewed where they are shown - here, by mail and by link.
test('chromium: an invitation limited to a hub is sent again from the dialog', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const sent = {};
  const { page, failures, close } = await open(browser, 'workspace', { mail: true, sent });
  t.after(close);

  const dialog = await share(page);
  await bertRow(dialog).getByRole('button', { name: 'Send the invitation again' }).click();
  await dialog.getByText('The invitation to Bert went out again').waitFor();
  assert.equal(sent.renewed, 1);
  assert.deepEqual(failures, []);
});

test('chromium: without mail, the renewed invitation is a new link shown once', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const sent = {};
  const { page, failures, close } = await open(browser, 'workspace', { mail: false, sent });
  t.after(close);

  const dialog = await share(page);
  await bertRow(dialog).getByRole('button', { name: 'New invitation link' }).click();
  await dialog.getByText('Invitation link for Bert').waitFor();
  assert.equal(sent.renewed, 1);
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
