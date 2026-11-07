// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The page system on *People & access*, as the administrator meets it at 1280×800 and on a phone
// at 390×844 (page-system §10, §11): one page kind per screen (UC-WRK-38 check 1), the create act
// in the head opening a dialog of its size that holds no table, tabs or second dialog and does not
// scroll (checks 2 and 3), a row opening its record page (check 4), the destructive act in the
// row's menu (check 5); a list with its search, filters and count and nothing else open on arrival,
// its foot saying what is shown, one verb per row, and nine people within 1.5 screens on a phone
// (UC-WRK-37 checks 1-5); the sign-in settings with their sections and states in the first
// viewport, a section at its own address, as a detail with *Back* on a phone, short enough, with
// one save model and the column and section list still in view at the section's end (UC-WRK-36
// checks 1-5). Heights are read from `getBoundingClientRect`, never from a screenshot. Against the
// stubbed API; Chromium, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, OTHER, OWNER_GRANTS, SERVICE_ACCOUNT, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const DESKTOP = { width: 1280, height: 800 };
const PHONE = { width: 390, height: 844 };

// Nine people at the workspace, as the seeded demo holds them: the reader, Mara and seven more.
const NAMES = ['Lukas Brandt', 'Amelia Hughes', 'Mehmet Yilmaz', 'Priya Natarajan', 'Sofia Rossi', 'Jonas Weber', 'Clara Neumann'];
const MORE = NAMES.map((name, index) => ({
  id: `01a0e2e0-0000-7000-8000-0000000009${String(index + 10)}`,
  kind: 'USER',
  display_name: name,
  email: `${name.split(' ')[0].toLowerCase()}@example.invalid`,
  status: 'ACTIVE',
}));
const NINE = [
  { id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'OWNER' },
  { id: 'm9', scope_type: 'TENANT', account_id: OTHER.id, role: 'MEMBER' },
  ...MORE.map((account, index) => ({ id: `m${index + 10}`, scope_type: 'TENANT', account_id: account.id, role: index === 0 ? 'ADMIN' : 'MEMBER' })),
];

/** Signed in as the fixture's owner, at a width; below `medium` as a phone. */
async function open(browser, { width, height }, { isNine = false } = {}) {
  const isPhone = width < 768;
  const context = await browser.newContext({ viewport: { width, height }, ...(isPhone ? { isMobile: true, hasTouch: true } : {}) });
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (isNine && path === '/memberships' && url.searchParams.get('scope_type') === 'TENANT') {
      return route.fulfill({ json: { data: NINE, page: { next_cursor: null, has_more: false }, grantable_roles: OWNER_GRANTS } });
    }
    const account = MORE.find((each) => path === `/accounts/${each.id}`);
    if (account) return route.fulfill({ json: account });
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

async function visit(page, path) {
  await page.goto(`${served.origin}${path}`);
  await page.locator('main h1').first().waitFor({ state: 'attached', timeout: 15_000 });
  await page.locator('[data-page-kind]').first().waitFor({ state: 'attached', timeout: 15_000 });
}

const SCREENS = [
  ['/administration/people', 'list'],
  ['/administration/people?tab=groups', 'list'],
  ['/administration/people?tab=roles', 'list'],
  ['/administration/people?tab=permissions', 'list'],
  ['/administration/sign-in', 'settings'],
  ['/administration/sign-in/passwords', 'settings'],
  ['/administration/legal', 'settings'],
  ['/administration/machines', 'list'],
  ['/administration/machines?tab=apps', 'list'],
  [`/administration/people/${OTHER.id}`, 'record'],
  [`/administration/machines/${SERVICE_ACCOUNT.id}`, 'record'],
];

for (const size of [DESKTOP, PHONE]) {
  const at = `${size.width}×${size.height}`;

  test(`chromium ${at}: every People & access screen is exactly one page kind (UC-WRK-38 check 1)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size);
    t.after(() => context.close());
    for (const [path, kind] of SCREENS) {
      await visit(page, path);
      const kinds = await page.locator('[data-page-kind]').evaluateAll((nodes) => nodes.map((node) => node.getAttribute('data-page-kind')));
      assert.deepEqual(kinds, [kind], `${path} draws ${kinds.join(', ') || 'no page kind'}`);
    }
    assert.deepEqual(failures, []);
  });

  test(`chromium ${at}: People narrows, counts and acts by row; nothing else is open on arrival (UC-WRK-37 checks 1-5)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size, { isNine: true });
    t.after(() => context.close());
    await visit(page, '/administration/people');
    const main = page.getByRole('main');
    await main.getByRole('link', { name: 'Lukas Brandt' }).first().waitFor({ timeout: 15_000 });

    // Check 1: the search, the filters - behind *Filters* on a phone - and the count.
    await main.getByRole('searchbox', { name: 'Search people' }).waitFor();
    await main.getByText('9 of 9', { exact: true }).first().waitFor();
    if (size === PHONE) await main.getByRole('button', { name: /^Filters/ }).waitFor();
    else assert.ok((await main.locator('.filter-bar button').count()) >= 2, 'the filters are chips beside the search');

    // Check 2: no form and no field but the search is open on arrival.
    const fields = await main.locator('input:not([type="search"]), select, textarea, form').evaluateAll((nodes) => nodes.filter((node) => node.getClientRects().length > 0).length);
    assert.equal(fields, 0, 'a field is open on arrival');

    // Check 3: the foot says what is shown.
    await main.getByText('Showing 9', { exact: true }).waitFor();

    // Check 4: a row carries at most one verb beside its menu.
    const verbs = await main.locator('tbody tr').evaluateAll((rows) =>
      rows.map((row) => [...row.querySelectorAll('button')].filter((button) => button.getClientRects().length > 0 && !button.getAttribute('aria-haspopup')).length),
    );
    assert.ok(verbs.length >= 9, `${verbs.length} rows drawn`);
    assert.ok(verbs.every((count) => count <= 1), `a row has more than one verb: ${verbs.join(', ')}`);

    // Narrowing: "pri" leaves Priya.
    await main.getByRole('searchbox', { name: 'Search people' }).fill('pri');
    await main.getByText('1 of 9', { exact: true }).first().waitFor();

    // Check 5: on a phone the nine people and the head fit in 1.5 screens.
    if (size === PHONE) {
      await main.getByRole('searchbox', { name: 'Search people' }).fill('');
      await main.getByText('9 of 9', { exact: true }).first().waitFor();
      const height = await page.evaluate(() => document.documentElement.scrollHeight);
      assert.ok(height <= 1.5 * size.height, `the People page is ${height} px high, ${(height / size.height).toFixed(2)} screens`);
    }
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    assert.equal(overflow, 0, 'the page scrolls sideways');
    assert.deepEqual(failures, []);
  });

  test(`chromium ${at}: the head's primary opens a dialog of its size, a row its record page, revoking sits in the row's menu (UC-WRK-38 checks 2-5)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size);
    t.after(() => context.close());
    await visit(page, '/administration/people');
    const main = page.getByRole('main');
    await main.getByRole('link', { name: 'Mara Lind' }).first().waitFor({ timeout: 15_000 });

    // Check 2 and 3: *Add people* opens a sized dialog without a table, tabs or a second dialog,
    // whose body does not scroll unless it is size L.
    await main.getByRole('button', { name: 'Add people', exact: true }).first().click();
    const dialog = page.locator('dialog[open]');
    await dialog.waitFor({ timeout: 5_000 });
    const shape = await dialog.evaluate((node) => {
      const body = node.querySelector('.body');
      return {
        size: node.getAttribute('data-size'),
        tables: node.querySelectorAll('table, [role="table"], [role="grid"]').length,
        tabs: node.querySelectorAll('[role="tablist"]').length,
        dialogs: node.querySelectorAll('dialog').length,
        scrolls: body ? body.scrollHeight > body.clientHeight + 1 : false,
      };
    });
    assert.ok(['s', 'm', 'l'].includes(shape.size ?? ''), `the dialog has no size: ${shape.size}`);
    assert.equal(shape.tables, 0, 'the dialog holds a table');
    assert.equal(shape.tabs, 0, 'the dialog holds tabs');
    assert.equal(shape.dialogs, 0, 'the dialog holds a second dialog');
    if (size === DESKTOP && shape.size !== 'l') assert.equal(shape.scrolls, false, 'the dialog scrolls at 1280×800');
    await page.keyboard.press('Escape');
    await dialog.waitFor({ state: 'detached', timeout: 5_000 }).catch(async () => {
      // A dialog holding changes asks first; nothing was typed, so it closes.
      assert.equal(await page.locator('dialog[open]').count(), 0, 'the dialog stayed open');
    });

    // Check 5: the destructive act is in the row's menu, not a button on the row.
    const row = main.locator('tbody tr').filter({ hasText: 'Mara Lind' });
    assert.equal(await row.getByRole('button', { name: /revoke/i }).count(), 0, 'revoking is a button on the row');
    await row.getByRole('button', { name: /More for Mara Lind/ }).click();
    const menu = page.getByRole('menu').or(page.locator('dialog[open]'));
    await menu.getByText('Revoke access…').first().waitFor({ timeout: 5_000 });
    await page.keyboard.press('Escape');

    // Check 4: the person has lists of their own, so the row opens their page.
    await main.getByRole('link', { name: 'Mara Lind' }).first().click();
    await page.waitForFunction((id) => location.pathname === `/administration/people/${id}`, OTHER.id, { timeout: 10_000 });
    await page.locator('[data-page-kind="record"]').waitFor({ timeout: 10_000 });
    assert.deepEqual(failures, []);
  });

  test(`chromium ${at}: Sign-in shows its sections with their state and opens one at its address (UC-WRK-36 checks 1-5)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size);
    t.after(() => context.close());

    // Check 1: every section and its state within the first viewport.
    await visit(page, '/administration/sign-in');
    const nav = page.locator('nav.section-nav');
    await nav.getByRole('link').first().waitFor({ timeout: 15_000 });
    const links = await nav.getByRole('link').evaluateAll((nodes) => nodes.map((node) => ({ text: node.textContent?.trim() ?? '', bottom: node.getBoundingClientRect().bottom })));
    assert.ok(links.length >= 5, `${links.length} sections listed`);
    for (const link of links) assert.ok(link.bottom <= size.height, `${link.text} is below the first viewport`);
    const statuses = await nav.locator('a .status, a [class*="status"]').count();
    assert.ok(statuses >= links.length, `${statuses} states for ${links.length} sections`);

    // Check 2: a section's address opens it; on a phone alone, with the way back.
    await visit(page, '/administration/sign-in/passwords');
    await page.getByRole('heading', { name: 'Passwords' }).first().waitFor({ timeout: 10_000 });
    if (size === PHONE) {
      await page.getByRole('button', { name: 'Sign-in' }).first().waitFor();
      assert.equal(await nav.evaluate((node) => node.getClientRects().length), 0, 'the section list stands beside the section on a phone');
    } else {
      assert.ok((await nav.evaluate((node) => node.getClientRects().length)) > 0, 'the section list is not beside the section');
    }

    // Check 3: the section is no taller than two viewports, four on a phone.
    const height = await page.evaluate(() => document.documentElement.scrollHeight);
    assert.ok(height <= (size === PHONE ? 4 : 2) * size.height, `the section is ${height} px high`);

    // Check 4: one save model - one save bar at most, its one *Save*.
    const saves = await page.getByRole('main').getByRole('button', { name: 'Save', exact: true }).count();
    assert.ok(saves <= 1, `${saves} save buttons in one section`);

    // Check 5: at the section's end the column and the section list are still in view.
    if (size === DESKTOP) {
      await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
      for (const [what, locator] of [['the column', page.getByRole('navigation', { name: 'Administration', exact: true })], ['the section list', nav]]) {
        const box = await locator.evaluate((node) => node.getBoundingClientRect().toJSON());
        assert.ok(box.bottom > 0 && box.top < size.height && box.height > 0, `${what} left the viewport`);
      }
    }
    assert.deepEqual(failures, []);
  });
}
