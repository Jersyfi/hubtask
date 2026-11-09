// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The role ceiling on the screens, walked as an administrator (domain-model.md §3.2, P-05): on
// *People* an owner's row offers no *Revoke* while a member's does; on *Groups* a group the server
// says the reader may not change shows no member controls and no delete, says why, and keeps
// *Edit*, while a group within the reader's roles keeps them all.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, OTHER, PAGE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const ADMIN_GRANTS = ['ADMIN', 'MEMBER', 'CONTRIBUTOR', 'VIEWER', 'GUEST', 'AUDITOR'];
const OWNERS = { id: '01a0e2e0-0000-7000-8000-0000000000f1', name: 'Owners', version: 1 };
const HELPERS = { id: '01a0e2e0-0000-7000-8000-0000000000f2', name: 'Helpers', version: 1 };

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/memberships' && url.searchParams.get('scope_type') === 'TENANT') {
      return route.fulfill({
        json: {
          ...PAGE,
          data: [
            { id: 'm-owner', scope_type: 'TENANT', account_id: OTHER.id, role: 'OWNER' },
            { id: 'm-me', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'ADMIN' },
          ],
          grantable_roles: ADMIN_GRANTS,
        },
      });
    }
    if (path === `/accounts/${OTHER.id}`) return route.fulfill({ json: OTHER });
    if (path === '/groups') return route.fulfill({ json: { ...PAGE, data: [HELPERS, OWNERS] } });
    if (path === `/groups/${OWNERS.id}`) {
      return route.fulfill({ json: { ...OWNERS, members: [OTHER.id], members_changeable: false } });
    }
    if (path === `/groups/${HELPERS.id}`) {
      return route.fulfill({ json: { ...HELPERS, members: [OTHER.id], members_changeable: true } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  return { page, close: () => context.close() };
}

const rowOf = (page, name) => page.getByRole('row').filter({ has: page.getByRole('link', { name, exact: true }) });

/** What a member's row on a group's page offers in its ⋯. */
async function memberMenu(page, name) {
  const row = page.getByRole('row').filter({ has: page.getByRole('link', { name, exact: true }) });
  await row.getByRole('button', { name: `More for ${name}` }).click();
  await page.getByRole('menu').waitFor();
  const items = (await page.getByRole('menu').getByRole('menuitem').allTextContents()).map((each) => each.trim());
  await page.keyboard.press('Escape');
  await page.getByRole('menu').waitFor({ state: 'hidden' });
  return items;
}

/** What a person's row offers in its ⋯ (page-system PS-6). */
async function menuOf(page, row, name) {
  await row.getByRole('button', { name: `More for ${name}` }).click();
  await page.getByRole('menu').waitFor();
  const items = (await page.getByRole('menu').getByRole('menuitem').allTextContents()).map((each) => each.trim());
  await page.keyboard.press('Escape');
  await page.getByRole('menu').waitFor({ state: 'hidden' });
  return items;
}

test('an administrator is not offered to revoke an owner', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/administration/people`);
  const me = rowOf(page, ACCOUNT.display_name);
  await me.getByRole('button', { name: `More for ${ACCOUNT.display_name}` }).waitFor({ timeout: 15_000 });
  assert.ok((await menuOf(page, me, ACCOUNT.display_name)).includes('Revoke access…'), 'the reader\'s own row offers no Revoke');
  const owner = rowOf(page, OTHER.display_name);
  await owner.waitFor();
  assert.equal((await menuOf(page, owner, OTHER.display_name)).includes('Revoke access…'), false, 'an owner row offers Revoke');
});

test('a group above the reader keeps its name editable and nothing else, and says why', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser);
  t.after(close);

  // A failure while drawing stops the whole screen; it is caught here rather than read as a timeout.
  const failures = [];
  page.on('pageerror', (error) => failures.push(error.message));
  // Each group's own page holds its members and its delete (page-system §7.2).
  await page.goto(`${served.origin}/administration/groups/${HELPERS.id}`);
  await page.getByRole('button', { name: 'Delete group…' }).first().waitFor({ timeout: 15_000 });
  assert.equal(await page.getByRole('button', { name: 'Add people' }).count(), 1);
  assert.equal(await page.getByRole('button', { name: 'Edit', exact: true }).count(), 1);
  assert.ok((await memberMenu(page, OTHER.display_name)).includes('Take out'), 'a changeable group offers no Take out');

  await page.goto(`${served.origin}/administration/groups/${OWNERS.id}`);
  await page.getByText('gives a role you do not hold yourself').waitFor({ timeout: 15_000 });
  await page.waitForLoadState('networkidle');
  assert.equal(await page.getByRole('button', { name: 'Add people' }).count(), 0);
  assert.equal(await page.getByRole('button', { name: 'Delete group…' }).count(), 0);
  assert.equal(await page.getByRole('button', { name: `More for Owners` }).count(), 0, 'the head offers the delete in its menu');
  assert.equal(await page.getByRole('button', { name: 'Edit', exact: true }).count(), 1);
  assert.equal((await memberMenu(page, OTHER.display_name)).includes('Take out'), false, 'a group above the reader offers Take out');
  assert.deepEqual(failures, [], 'the screen failed while drawing');
});
