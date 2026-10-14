// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Limiting a person to part of the workspace from the web app (UC-WRK-21 checks 4 and 5,
// UC-ID-14 check 2, UC-ID-15): an invitation from *People* with a role on one hub only, an
// invitation from an entry's *Share* with a role on that entry only, and the members dialog
// offering somebody who holds a role only on another hub. Against the shared fixture; the writes
// are recorded, and the grant each makes is the assertion. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, HUB, ITEMS, MANIFEST, OWNER_GRANTS, PAGE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const NEWCOMER = '01a0e2e0-0000-7000-8000-0000000000e1';
/** A second hub, and somebody who holds a role on it and nowhere else. */
const GARDEN = { id: '01a0e2e0-0000-7000-8000-0000000000e2', type: 'HUB', parent_id: null, name: 'Garden', order_key: 'a1', version: 1 };
const KIM = { id: '01a0e2e0-0000-7000-8000-0000000000e3', kind: 'USER', display_name: 'Kim Gardener', email: 'kim@example.invalid', status: 'ACTIVE', locale: 'en' };

async function open(browser, sent) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, roles: OWNER_GRANTS.map((role) => ({ role, permissions: ['READ'] })) } });
    if (path === '/containers' && url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: [HUB, GARDEN] } });
    if (path === `/containers/${GARDEN.id}`) return route.fulfill({ json: GARDEN });
    if (path === `/accounts/${KIM.id}`) return route.fulfill({ json: KIM });
    if (path === '/memberships' && request.method() === 'GET') {
      const scope = url.searchParams.get('scope_type');
      const id = url.searchParams.get('scope_id');
      const rows = scope === 'TENANT'
        ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'OWNER' }]
        : scope === 'HUB' && id === GARDEN.id
          ? [{ id: 'm-kim', scope_type: 'HUB', scope_id: GARDEN.id, account_id: KIM.id, role: 'VIEWER' }]
          : [];
      return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: OWNER_GRANTS } });
    }
    if (path === '/memberships' && request.method() === 'POST') {
      sent.grants.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: { id: 'm-new', ...request.postDataJSON() } });
    }
    if (path === '/accounts:invite') {
      sent.invites.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: { id: NEWCOMER, kind: 'USER', display_name: 'anna', email: 'anna@example.invalid', status: 'INVITED' } });
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

async function settle(sent, count) {
  for (let waited = 0; sent.grants.length < count && waited < 50; waited++) await new Promise((done) => setTimeout(done, 100));
}

test('chromium: People invites somebody with a role on one hub, not on the workspace', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const sent = { invites: [], grants: [] };
  const { page, failures, close } = await open(browser, sent);
  t.after(close);

  await page.goto(`${served.origin}/administration/people`);
  const form = page.locator('form').filter({ has: page.getByLabel('Email address') });
  await form.getByLabel('Email address').fill('anna@example.invalid');
  await form.getByLabel('Where').selectOption(HUB.id);
  await form.getByLabel('Role').selectOption('GUEST');
  await form.getByRole('button', { name: 'Send the invitation' }).click();
  await settle(sent, 1);

  assert.equal(sent.invites.length, 1);
  assert.deepEqual(
    { scope_type: sent.grants[0]?.scope_type, scope_id: sent.grants[0]?.scope_id, role: sent.grants[0]?.role, account_id: sent.grants[0]?.account_id },
    { scope_type: 'HUB', scope_id: HUB.id, role: 'GUEST', account_id: NEWCOMER },
  );
  assert.deepEqual(failures, []);
});

test('chromium: an entry’s Share invites somebody to that entry alone, and offers a person from another hub', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const sent = { invites: [], grants: [] };
  const { page, failures, close } = await open(browser, sent);
  t.after(close);

  await page.goto(`${served.origin}/items/${ITEMS[2].id}`);
  await page.getByRole('button', { name: `Actions for ${ITEMS[2].title}` }).first().click();
  await page.getByRole('menuitem', { name: 'Share this entry' }).click();
  const dialog = page.getByRole('dialog');

  // UC-WRK-21 check 4: somebody holding a role only on another hub can be chosen here.
  const who = dialog.getByLabel('Who');
  await who.locator('option', { hasText: KIM.display_name }).waitFor({ state: 'attached', timeout: 10_000 });

  // UC-WRK-21 check 5: invited with a role on this entry and nowhere else.
  const form = dialog.locator('form').filter({ has: page.getByLabel('Email address') });
  await form.getByLabel('Email address').fill('anna@example.invalid');
  await form.getByLabel('Role').selectOption('GUEST');
  await form.getByRole('button', { name: 'Send the invitation' }).click();
  await settle(sent, 1);

  assert.equal(sent.invites.length, 1);
  assert.deepEqual(
    { scope_type: sent.grants[0]?.scope_type, scope_id: sent.grants[0]?.scope_id, role: sent.grants[0]?.role },
    { scope_type: 'ITEM', scope_id: ITEMS[2].id, role: 'GUEST' },
  );
  assert.deepEqual(failures, []);
});
