// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The writes the administration offered to readers the server refuses (ux-inventory §E8, P-05):
//
// * renaming a group asks MANAGE_MEMBERS - an auditor reads the groups and is not offered *Rename*;
// * emptying the trash asks the owner's DELETE_CONTAINER - an administrator is not offered it;
// * a service account's tokens are listed, minted and revoked under MANAGE_MEMBERS - an auditor
//   reads the accounts and is not offered their tokens.
//
// Each refusal itself is the application layer's (rule 2) and tested there; this proves the screen
// does not offer what would be refused. Against a stubbed API. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, COLLECTION, HUB, MANIFEST, PAGE, PERMISSIONS, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const GROUP = { id: '01a0e2e0-0000-7000-8000-00000000c501', name: 'Gate keepers', version: 1 };
const MACHINE = { id: '01a0e2e0-0000-7000-8000-00000000c502', display_name: 'Gate nightly export', kind: 'SERVICE' };
const TRASHED = {
  kind: 'ITEM', id: '01a0e2e0-0000-7000-8000-00000000c503', trash_batch_id: '01a0e2e0-0000-7000-8000-00000000c504',
  deleted_at: new Date().toISOString(), purge_after: new Date(Date.now() + 30 * 864e5).toISOString(),
  deleted_by: { type: 'USER', id: ACCOUNT.id }, title: 'Gate old tiles', subtype: 'TASK',
  hub_id: HUB.id, collection_id: COLLECTION.id, parent_id: null, version: 1, held: false,
};

async function as(browser, permissions) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: permissions } });
    if (path === '/groups') return route.fulfill({ json: { ...PAGE, data: [GROUP] } });
    if (path === `/groups/${GROUP.id}`) return route.fulfill({ json: { ...GROUP, members: [], members_changeable: true } });
    if (path === '/auth/service-accounts') return route.fulfill({ json: [MACHINE] });
    if (path === '/trash') return route.fulfill({ json: { data: [TRASHED], page: { next_cursor: null, has_more: false } } });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  return { context, page: await context.newPage() };
}

async function offered(browser, permissions, address, waitFor, button) {
  const { context, page } = await as(browser, permissions);
  try {
    await page.goto(`${served.origin}${address}`);
    await page.getByText(waitFor).first().waitFor({ timeout: 15_000 });
    // The manifest decides; give it its answer before counting.
    await page.waitForLoadState('networkidle');
    return await page.getByRole('button', { name: button, exact: true }).count();
  } finally {
    await context.close();
  }
}

test('chromium: renaming a group is offered only to whom may manage members', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  assert.equal(await offered(browser, PERMISSIONS.ADMIN, '/administration/groups', GROUP.name, 'Rename'), 1);
  assert.equal(await offered(browser, PERMISSIONS.AUDITOR, '/administration/groups', GROUP.name, 'Rename'), 0);
});

test('chromium: emptying the trash is offered only to whom may delete a hub', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  assert.equal(await offered(browser, PERMISSIONS.OWNER, '/trash', TRASHED.title, 'Empty the trash'), 1);
  assert.equal(await offered(browser, PERMISSIONS.ADMIN, '/trash', TRASHED.title, 'Empty the trash'), 0);
});

test('chromium: a service account\'s tokens are offered only to whom may manage members', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  assert.equal(await offered(browser, PERMISSIONS.ADMIN, '/administration/service-accounts', MACHINE.display_name, 'Its tokens'), 1);
  assert.equal(await offered(browser, PERMISSIONS.AUDITOR, '/administration/service-accounts', MACHINE.display_name, 'Its tokens'), 0);
});
