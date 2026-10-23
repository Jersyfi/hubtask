// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The administration's screens that became tabs (ux-inventory §E8): *People* with its groups and
// what each role means, *Machines and apps* with the service accounts and the third-party apps,
// *AI* in three. Each tab has an address, each old address replaces itself with its tab, and
// *People* has one primary action with the three ways somebody is added - offered to an owner and
// an administrator, absent for an auditor (P-05). Against a stubbed API. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, ADMIN_GRANTS, MANIFEST, OTHER, OWNER_GRANTS, PAGE, PERMISSIONS, SIGN_IN_POLICY, WORKSPACE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';
import { GRANT, INVITE, WITHOUT_ADDRESS, addPeople, waysOffered } from './addpeople.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const allowed = { value: true, installation: true, lock: null, source: 'DEFAULT', installation_source: 'DEFAULT' };

/** A signed-in tab as somebody holding `permissions`, whom the server lets give `grants`. */
async function as(browser, permissions, grants, width = 1280) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: permissions } });
    if (path === '/tenant' && request.method() === 'GET') {
      // Accounts without an address allowed, so all three ways are there to be offered.
      return route.fulfill({ json: { ...WORKSPACE, sign_in_policy: { ...SIGN_IN_POLICY, managed_accounts: allowed } } });
    }
    if (path === '/memberships' && request.method() === 'GET' && url.searchParams.get('scope_type') === 'TENANT') {
      const rows = [
        { id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'OWNER' },
        { id: 'm9', scope_type: 'TENANT', account_id: OTHER.id, role: 'MEMBER' },
      ];
      return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: grants } });
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

/** Where the address is now, path and query. */
const here = (page) => page.evaluate(() => location.pathname + location.search);

/** Waits until the address is `target`, and says which it was otherwise. */
async function landsOn(page, target) {
  await page.waitForFunction((want) => location.pathname + location.search === want, target, { timeout: 10_000 })
    .catch(async () => assert.fail(`expected ${target}, the address is ${await here(page)}`));
}

const MOVED = [
  ['/administration/groups', '/administration/people?tab=groups', 'Groups'],
  ['/administration/permissions', '/administration/people?tab=permissions', 'Roles and permissions'],
  ['/administration/service-accounts', '/administration/machines', 'Service accounts'],
  ['/administration/apps', '/administration/machines?tab=apps', 'Third-party apps'],
];

test('chromium: each old address lands on its tab, and a tab chosen is an address', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await as(browser, PERMISSIONS.OWNER, OWNER_GRANTS);
  t.after(() => context.close());

  for (const [old, now, tab] of MOVED) {
    await page.goto(`${served.origin}${old}`);
    await landsOn(page, now);
    const selected = page.getByRole('tab', { name: tab, exact: true });
    await selected.waitFor({ timeout: 15_000 });
    assert.equal(await selected.getAttribute('aria-selected'), 'true', `${tab} is not the open tab at ${now}`);
    assert.equal(await page.locator('main h1').count(), 1, `${now} has not exactly one heading`);
  }

  // Choosing a tab replaces the address, so a reload keeps it.
  await page.goto(`${served.origin}/administration/people`);
  await page.getByRole('tab', { name: 'Groups', exact: true }).click();
  await landsOn(page, '/administration/people?tab=groups');
  await page.getByRole('tab', { name: 'People', exact: true }).click();
  await landsOn(page, '/administration/people');
  await page.goto(`${served.origin}/administration/machines`);
  await page.getByRole('tab', { name: 'Third-party apps', exact: true }).click();
  await landsOn(page, '/administration/machines?tab=apps');

  // The section's column marks *People* on its tabs and *Machines and apps* on its own.
  const section = page.getByRole('navigation', { name: 'Administration', exact: true });
  assert.equal(await section.getByRole('treeitem', { name: 'Machines and apps', exact: true }).getAttribute('aria-current'), 'page');
  await page.goto(`${served.origin}/administration/people?tab=permissions`);
  await page.getByRole('tab', { name: 'Roles and permissions', exact: true }).waitFor({ timeout: 15_000 });
  assert.equal(await section.getByRole('treeitem', { name: 'People', exact: true }).getAttribute('aria-current'), 'page');

  assert.deepEqual(failures, []);
});

for (const [who, permissions, grants] of [['an owner', PERMISSIONS.OWNER, OWNER_GRANTS], ['an administrator', PERMISSIONS.ADMIN, ADMIN_GRANTS]]) {
  test(`chromium: ${who} adds people through one primary action with the three ways`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await as(browser, permissions, grants);
    t.after(() => context.close());

    await page.goto(`${served.origin}/administration/people`);
    await page.getByRole('button', { name: 'Add people', exact: true }).waitFor({ timeout: 15_000 });
    // The list first: no form is open until a way is chosen.
    assert.equal(await page.getByRole('heading', { name: INVITE }).count(), 0, 'a form is open before anything was chosen');
    assert.deepEqual(await waysOffered(page), [INVITE, WITHOUT_ADDRESS, GRANT]);

    // The button itself is the invitation, and the reader is taken to its first field.
    await addPeople(page);
    assert.equal(await page.evaluate(() => document.activeElement?.getAttribute('type')), 'email', 'the address field is not focused');

    // One way at a time: choosing another replaces the open one.
    await addPeople(page, GRANT);
    assert.equal(await page.getByRole('heading', { name: INVITE }).count(), 0, 'two ways are open at once');
    await addPeople(page, WITHOUT_ADDRESS);
    assert.equal(await page.getByRole('heading', { name: GRANT }).count(), 0, 'two ways are open at once');

    // Closed, the list is alone again and focus is back on the action that opened it.
    await page.getByRole('button', { name: 'Close', exact: true }).click();
    assert.equal(await page.getByRole('heading', { name: WITHOUT_ADDRESS }).count(), 0, 'the form stayed open');
    assert.equal(await page.evaluate(() => document.activeElement?.textContent?.trim()), 'Add people');

    // The action belongs to the people: the other tabs carry none.
    await page.getByRole('tab', { name: 'Groups', exact: true }).click();
    await landsOn(page, '/administration/people?tab=groups');
    assert.equal(await page.getByRole('button', { name: 'Add people', exact: true }).count(), 0, 'the groups offer Add people');

    assert.deepEqual(failures, []);
  });
}

test('chromium: an auditor reads the people, the groups and the roles, and is offered no way to add anybody', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await as(browser, PERMISSIONS.AUDITOR, []);
  t.after(() => context.close());

  await page.goto(`${served.origin}/administration/people`);
  await page.getByRole('row').filter({ hasText: OTHER.display_name }).waitFor({ timeout: 15_000 });
  assert.equal(await page.getByRole('button', { name: 'Add people', exact: true }).count(), 0, 'an auditor is offered Add people');
  assert.equal(await page.getByRole('button', { name: 'Ways to add somebody' }).count(), 0, 'an auditor is offered the ways');
  for (const tab of ['Groups', 'Roles and permissions']) {
    await page.getByRole('tab', { name: tab, exact: true }).click();
    assert.equal(await page.getByRole('tab', { name: tab, exact: true }).getAttribute('aria-selected'), 'true');
  }

  assert.deepEqual(failures, []);
});

test('chromium: 375 px — the three ways are in the head\'s menu under the same name', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await as(browser, PERMISSIONS.OWNER, OWNER_GRANTS, 375);
  t.after(() => context.close());

  await page.goto(`${served.origin}/administration/people`);
  await page.getByRole('button', { name: 'Add people', exact: true }).waitFor({ timeout: 15_000 });
  assert.deepEqual(await waysOffered(page), [INVITE, WITHOUT_ADDRESS, GRANT]);
  await addPeople(page, GRANT);

  assert.deepEqual(failures, []);
});

for (const [who, permissions] of [['an owner', PERMISSIONS.OWNER], ['an auditor', PERMISSIONS.AUDITOR]]) {
  test(`chromium: ${who} finds AI in three tabs, each with an address of its own`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await as(browser, permissions, []);
    t.after(() => context.close());

    await page.goto(`${served.origin}/administration/ai`);
    const tabs = page.getByRole('tablist', { name: 'AI' }).getByRole('tab');
    await tabs.first().waitFor({ timeout: 15_000 });
    assert.deepEqual((await tabs.allTextContents()).map((name) => name.trim()), ['Source', 'Consent and people', 'Budget']);

    // Each tab holds its own part and nothing of the others'.
    await page.getByRole('tab', { name: 'Consent and people', exact: true }).click();
    await landsOn(page, '/administration/ai?tab=consent');
    await page.locator('[data-person-opt-out]').waitFor();
    assert.equal(await page.getByRole('heading', { name: 'The budget' }).count(), 0, 'the budget is on the consent tab');
    await page.getByRole('tab', { name: 'Budget', exact: true }).click();
    await landsOn(page, '/administration/ai?tab=budget');
    await page.getByRole('heading', { name: 'The budget' }).waitFor();
    assert.equal(await page.locator('[data-person-opt-out]').count(), 0, 'the people are on the budget tab');
    await page.getByRole('tab', { name: 'Source', exact: true }).click();
    await landsOn(page, '/administration/ai');
    await page.getByRole('heading', { name: 'The budget' }).waitFor({ state: 'detached', timeout: 5_000 })
      .catch(() => assert.fail('the budget is on the source tab'));

    assert.deepEqual(failures, []);
  });
}
