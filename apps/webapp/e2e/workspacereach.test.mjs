// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the reader holds on the workspace decides what the frame offers and what it asks for (P-05),
// walked once per role against the manifest's `workspace_permissions`:
//
// * The administration is offered to `STRUCTURE` or `READ_CONFIGURATION` and to nobody else, and an
//   address under `/administration` typed by anybody else answers as one that reaches nothing - no
//   form, no *Save* (the finding: a member typing `/administration/workspace` got both).
// * The frame reads `/quotas` and `/meta/health` only for a reader the server answers them for. Each
//   refused read is a DENIED entry in the trail, and a member's every page load wrote two to four.
//
// Against a stubbed API, so what is asserted is what the client asks for; that the question it asks
// instead (the manifest) records nothing is the server's test. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, MANIFEST, PERMISSIONS, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const NOTHING_HERE = 'There is nothing at this address.';

/** A signed-in tab as somebody holding `permissions` on the workspace, with every API path it asked. */
async function as(browser, permissions) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const asked = [];
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    asked.push(path);
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: permissions } });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  return { context, page, asked, failures };
}

/** The account menu's rows, opened from the bar and closed again. */
async function menuRows(page) {
  await page.getByRole('button', { name: new RegExp(ACCOUNT.display_name) }).first().click();
  const menu = page.getByRole('menu', { name: 'You' });
  await menu.waitFor({ timeout: 10_000 });
  const rows = (await menu.getByRole('menuitem').allTextContents()).map((each) => each.trim());
  await page.keyboard.press('Escape');
  return rows;
}

for (const role of ['MEMBER', 'CONTRIBUTOR', 'VIEWER', 'BELOW']) {
  test(`chromium: ${role} is offered no administration, asks for no quota or health, and a typed address reaches nothing`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, asked, failures } = await as(browser, PERMISSIONS[role]);
    t.after(() => context.close());

    await page.goto(`${served.origin}/`);
    const rows = await menuRows(page);
    assert.ok(rows.includes('Your settings'), `the menu did not open: ${rows}`);
    assert.ok(!rows.includes('Workspace administration'), `the administration is offered: ${rows}`);

    await page.goto(`${served.origin}/administration/workspace`);
    await page.getByRole('heading', { name: NOTHING_HERE }).waitFor();
    assert.equal(await page.getByRole('button', { name: 'Save' }).count(), 0, 'the workspace form is drawn');
    assert.equal(await page.getByText('Sign-in provider').count(), 0, 'the administration column is drawn');

    // The front door reaches nothing either, rather than sending the reader on to a screen that does not.
    await page.goto(`${served.origin}/administration`);
    await page.getByRole('heading', { name: NOTHING_HERE }).waitFor();
    assert.equal(new URL(page.url()).pathname, '/administration');

    assert.deepEqual(asked.filter((path) => path === '/quotas' || path === '/meta/health'), [], 'the frame asked what the server refuses');
    assert.equal(asked.filter((path) => path === '/private-hubs').length, 0, 'the administration column read the private hubs');
    assert.deepEqual(failures, []);
  });
}

for (const role of ['OWNER', 'AUDITOR']) {
  test(`chromium: ${role} is offered the administration and the frame reads the health report`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, asked, failures } = await as(browser, PERMISSIONS[role]);
    t.after(() => context.close());

    await page.goto(`${served.origin}/`);
    assert.ok((await menuRows(page)).includes('Workspace administration'));

    await page.goto(`${served.origin}/administration/workspace`);
    await page.getByRole('navigation').getByText('Sign-in provider').waitFor();
    assert.equal(await page.getByRole('heading', { name: NOTHING_HERE }).count(), 0);
    for (let waited = 0; !asked.includes('/meta/health') && waited < 50; waited++) await page.waitForTimeout(100);
    assert.ok(asked.includes('/meta/health'), 'the health report was not read for somebody who may read it');
    assert.deepEqual(failures, []);
  });
}
