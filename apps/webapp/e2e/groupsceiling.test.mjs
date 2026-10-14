// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The role ceiling on the screens, walked as an administrator (domain-model.md §3.2, P-05): on
// *People* an owner's row offers no *Revoke* while a member's does; on *Groups* a group the server
// says the reader may not change shows no member controls and no delete, says why, and keeps
// *Rename*, while a group within the reader's roles keeps them all.

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

const rowOf = (page, name) => page.getByRole('row').filter({ has: page.getByRole('rowheader', { name, exact: true }) });

test('an administrator is not offered to revoke an owner', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/administration/people`);
  const me = rowOf(page, ACCOUNT.display_name);
  await me.getByRole('button', { name: 'Revoke' }).waitFor({ timeout: 15_000 });
  const owner = rowOf(page, OTHER.display_name);
  await owner.waitFor();
  assert.equal(await owner.getByRole('button', { name: 'Revoke' }).count(), 0, 'an owner row offers Revoke');
});

test('a group above the reader keeps its name editable and nothing else, and says why', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser);
  t.after(close);

  // A failure while drawing stops the whole screen; it is caught here rather than read as a timeout.
  const failures = [];
  page.on('pageerror', (error) => failures.push(error.message));
  await page.goto(`${served.origin}/administration/groups`);
  const owners = page.locator('section').filter({ has: page.getByRole('heading', { name: 'Owners' }) });
  const helpers = page.locator('section').filter({ has: page.getByRole('heading', { name: 'Helpers' }) });
  await helpers.getByRole('button', { name: 'Delete the group' }).waitFor({ timeout: 15_000 });
  await owners.getByText('gives a role you do not hold yourself').waitFor();

  assert.equal(await owners.getByRole('button', { name: 'Take out' }).count(), 0);
  assert.equal(await owners.getByRole('button', { name: 'Add', exact: true }).count(), 0);
  assert.equal(await owners.getByRole('button', { name: 'Delete the group' }).count(), 0);
  assert.equal(await owners.getByRole('button', { name: 'Rename' }).count(), 1);

  assert.equal(await helpers.getByRole('button', { name: 'Take out' }).count(), 1);
  assert.equal(await helpers.getByRole('button', { name: 'Add', exact: true }).count(), 1);
  assert.deepEqual(failures, [], 'the screen failed while drawing');
});
