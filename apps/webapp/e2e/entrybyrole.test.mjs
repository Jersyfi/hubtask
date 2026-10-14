// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// An entry offers exactly what the reader's role lets them do to it (UC-WRK-22 checks 1-3,
// UC-WRK-19 check 4), walked per role against the shared fixture's matrix:
//
// * a member changes everything and comments;
// * a contributor changes and comments on what is assigned to them, and on anything else sees no
//   control that changes it and one line saying it is not theirs - and may still add inside it;
// * a viewer changes nothing and does not comment, told so in one line;
// * a guest shared on the one entry changes nothing and comments.
//
// The role is the one the server decides the entry by: the strongest along its path. The guest's
// reads of the workspace's and the collection's memberships are refused, as the server refuses
// them, and count as holding nothing there. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, CHILDREN, COLLECTION, HUB, ITEMS, OWNER_GRANTS, PAGE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const ENTRY = ITEMS[0];
const CHILD = CHILDREN[ENTRY.id][0];

/**
 * A signed-in tab whose reader holds `role`: on the workspace, or - for a guest - on the one entry,
 * with every other scope's memberships refused. `assigned` makes the entry theirs.
 */
async function as(browser, role, { assigned = false } = {}) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const isGuest = role === 'GUEST';
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/memberships') {
      const scope = url.searchParams.get('scope_type');
      if (isGuest && scope !== 'ITEM') return route.fulfill({ status: 403, json: { code: 'access.not_permitted' } });
      const rows = isGuest
        ? [{ id: 'g1', scope_type: 'ITEM', scope_id: url.searchParams.get('scope_id'), account_id: ACCOUNT.id, role: 'GUEST' }]
        : scope === 'TENANT' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role }] : [];
      return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: role === 'OWNER' ? OWNER_GRANTS : [] } });
    }
    if (assigned && path === `/items/${ENTRY.id}` && route.request().method() === 'GET') {
      return route.fulfill({ json: { ...ENTRY, assignee_id: ACCOUNT.id } });
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
  return { context, page, failures };
}

/** What the entry page offers, once the role has been read. */
async function offered(page) {
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByText(CHILD.title).first().waitFor();
  // The role arrives with the memberships; give the screen the moment to draw it.
  await page.waitForTimeout(600);
  const count = (locator) => locator.count();
  return {
    complete: (await count(page.getByRole('checkbox', { name: `Mark ${ENTRY.title} as done` }))) === 1,
    comment: (await count(page.getByLabel('Say something'))) === 1,
    addInside: (await count(page.getByRole('button', { name: `Add an activity inside ${CHILD.title}`, exact: true }))) === 1,
    childMenu: (await count(page.getByRole('button', { name: `Move ${CHILD.title}` }))) === 1,
    assigneeEditor: (await count(page.locator('button[data-detail="assignee"]'))) === 1,
    notYours: (await count(page.locator('.role-reason', { hasText: 'Your role changes only the entries assigned to you.' }))) > 0,
    readsOnly: (await count(page.locator('.role-reason', { hasText: 'Your role can read entries here but not change them.' }))) > 0,
  };
}

const EXPECTED = {
  MEMBER: { complete: true, comment: true, addInside: true, childMenu: true, assigneeEditor: true, notYours: false, readsOnly: false },
  CONTRIBUTOR: { complete: false, comment: false, addInside: true, childMenu: true, assigneeEditor: false, notYours: true, readsOnly: false },
  VIEWER: { complete: false, comment: false, addInside: false, childMenu: false, assigneeEditor: false, notYours: false, readsOnly: true },
  GUEST: { complete: false, comment: true, addInside: false, childMenu: false, assigneeEditor: false, notYours: false, readsOnly: true },
};

for (const [role, expected] of Object.entries(EXPECTED)) {
  test(`chromium: ${role} is offered on an entry exactly what the role lets them do`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await as(browser, role);
    t.after(() => context.close());

    const seen = await offered(page);
    assert.deepEqual(seen, expected);
    assert.deepEqual(failures, []);
  });
}

test('chromium: a contributor changes and comments on the entry assigned to them', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await as(browser, 'CONTRIBUTOR', { assigned: true });
  t.after(() => context.close());

  const seen = await offered(page);
  assert.equal(seen.complete, true);
  assert.equal(seen.comment, true);
  assert.equal(seen.assigneeEditor, true);
  assert.equal(seen.notYours, false);
  assert.deepEqual(failures, []);
});

/** The container page's menu, opened and closed again. */
async function pageMenu(page, name) {
  await page.getByRole('button', { name: `Actions for ${name}` }).first().click();
  const rows = (await page.getByRole('menuitem').allTextContents()).map((each) => each.trim());
  await page.keyboard.press('Escape');
  return rows;
}

// UC-WRK-22 check 4, UC-WRK-01 check 6, UC-WRK-09 check 7, UC-WRK-05: what shapes the workspace is
// for a role that may shape it. A member works in the collection and is offered none of it.
for (const [role, shapes] of [['OWNER', true], ['MEMBER', false]]) {
  test(`chromium: ${role} is ${shapes ? '' : 'not '}offered what changes the shape of a hub and a collection`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await as(browser, role);
    t.after(() => context.close());

    await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
    await page.getByText(ITEMS[2].title).first().waitFor();
    await page.waitForTimeout(600);
    const rows = await pageMenu(page, COLLECTION.name);
    for (const shaping of ['Rename', 'Move to another hub…', 'Labels', 'Custom fields', 'Policies']) {
      assert.equal(rows.includes(shaping), shapes, `${shaping} in ${rows}`);
    }
    // Used by everybody: the views, the templates, who is here.
    for (const used of ['Saved views', 'Templates', 'People']) assert.ok(rows.includes(used), `${used} missing from ${rows}`);
    // The trash takes a subtree with it: the owner's alone.
    assert.equal(rows.includes('Move to the trash'), role === 'OWNER');

    await page.getByRole('radio', { name: 'Board' }).click();
    await page.getByText(ITEMS[2].title).first().waitFor();
    assert.equal(await page.getByRole('button', { name: 'Create column' }).count() > 0, shapes, 'Create column');

    await page.goto(`${served.origin}/hubs/${HUB.id}`);
    await page.getByRole('heading', { name: HUB.name, level: 1 }).waitFor();
    await page.waitForTimeout(600);
    assert.equal(await page.getByRole('button', { name: 'Create collection' }).count() > 0, shapes, 'Create collection');
    assert.equal(await page.getByRole('button', { name: 'Import…' }).count() > 0, shapes, 'Import');
    assert.deepEqual(failures, []);
  });
}

test('chromium: a viewer reads the collection with no control that changes an entry, and nothing to add', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await as(browser, 'VIEWER');
  t.after(() => context.close());

  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.getByText(ITEMS[2].title).first().waitFor();
  await page.waitForTimeout(600);
  assert.equal(await page.getByRole('button', { name: `Move ${ITEMS[2].title}` }).count(), 0, 'a row menu that moves');
  assert.equal(await page.getByRole('button', { name: 'Labels' }).count(), 0, 'a label picker on a row');
  assert.equal(await page.locator('[data-opener="add-entry"]').count(), 0, 'an add control');
  assert.deepEqual(failures, []);
});
