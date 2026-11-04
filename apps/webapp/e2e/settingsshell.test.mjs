// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The one settings shell (user-experience.md §5.4), walked on the mail screen:
//
// * the sections are listed at the start and a press moves to one;
// * a changed setting brings the save bar with its count, *Discard* takes it away;
// * a move away - a link, or the browser's back - asks first while a change is unsaved: *Stay*
//   keeps it on the screen, *Leave without saving* goes;
// * at a phone's width the list is a row of its own and the page never scrolls sideways.
//
// Chromium, large and compact.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const SETTINGS = {
  enabled: false, name: 'familie', senders: { level: 'MEMBERS', allowed: [], blocked: [] }, sender_routes: [], domain: 'in.hubtask.test',
};

async function routed(context) {
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/mail/settings') return route.fulfill({ json: SETTINGS });
    if (path === '/mail/receipts') return route.fulfill({ json: { receipts: [] } });
    return stub(route);
  });
}

/** A same-origin link pressed in the page, as the column's rows and any link in a screen are. */
async function follow(page, href) {
  await page.evaluate((target) => {
    const link = document.createElement('a');
    link.href = target;
    document.body.append(link);
    link.click();
    link.remove();
  }, href);
}

test('chromium: the shell lists the sections, counts a change and asks before a move drops it', { timeout: 120_000 }, async () => {
  const browser = await chromium.launch();
  const { context, page, failures } = await signedIn(browser, 1280, 900);
  await routed(context);
  try {
    await page.goto(`${served.origin}/administration`);
    await follow(page, '/administration/mail');
    const shell = page.locator('[data-settings-page]');
    await shell.waitFor({ timeout: 15_000 });
    const list = shell.getByRole('navigation', { name: 'Sections of E-mail' });
    await list.getByRole('link', { name: 'Who may write' }).waitFor();

    // Nothing changed: no bar, and a move goes without a question.
    assert.equal(await page.locator('[data-save-bar][data-open]').count(), 0);

    await page.locator('[data-mail-on]').getByRole('switch').click();
    const bar = page.getByRole('region', { name: 'Unsaved changes' });
    await bar.getByText('1 change').waitFor({ timeout: 5_000 });

    // A link away asks; staying keeps the screen and the change.
    await follow(page, '/administration/voice');
    const question = page.getByRole('dialog', { name: 'Leave without saving?' });
    await question.getByText('1 change on this screen is not saved yet.', { exact: false }).waitFor({ timeout: 5_000 });
    await question.getByRole('button', { name: 'Stay' }).last().click();
    await question.waitFor({ state: 'detached', timeout: 5_000 });
    assert.equal(new URL(page.url()).pathname, '/administration/mail');
    await bar.getByText('1 change').waitFor();

    // The browser's back asks too, and the address stays the screen's while it does.
    await page.goBack();
    await question.waitFor({ timeout: 5_000 });
    assert.equal(new URL(page.url()).pathname, '/administration/mail');
    await question.getByRole('button', { name: 'Stay' }).last().click();

    // Discarded, the bar is gone and a move goes at once.
    await bar.getByRole('button', { name: 'Discard' }).click();
    await bar.waitFor({ state: 'detached', timeout: 5_000 });
    await follow(page, '/administration/voice');
    await page.waitForURL('**/administration/voice', { timeout: 5_000 });
    assert.equal(await question.count(), 0);

    // Changed again, leaving goes where the reader asked to.
    await page.goBack();
    await page.locator('[data-mail-on]').getByRole('switch').click();
    await bar.getByText('1 change').waitFor({ timeout: 5_000 });
    await follow(page, '/administration/voice');
    await question.getByRole('button', { name: 'Leave without saving' }).click();
    await page.waitForURL('**/administration/voice', { timeout: 5_000 });
    assert.deepEqual(failures, []);
  } finally {
    await browser.close();
  }
});

test('chromium: at a phone width the sections are a row of their own and nothing scrolls sideways', { timeout: 120_000 }, async () => {
  const browser = await chromium.launch();
  const { context, page, failures } = await signedIn(browser, 375, 812);
  await routed(context);
  try {
    await page.goto(`${served.origin}/administration/mail`);
    const list = page.locator('[data-settings-sections]');
    await list.waitFor({ timeout: 15_000 });
    await page.locator('[data-mail-log]').waitFor();
    const wide = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
    assert.equal(wide, false, 'the page is no wider than the screen');
    // The list sits above the sections rather than beside them.
    const listBox = await list.boundingBox();
    const first = await page.locator('[data-mail-mailboxes]').boundingBox();
    assert.ok(listBox && first && listBox.y + listBox.height <= first.y, 'the list is above the first section');
    // A press moves to the section and puts the keyboard there.
    await list.getByRole('link', { name: 'Recently arrived' }).click();
    await page.waitForFunction(() => document.activeElement?.id === 'mail-part-log', null, { timeout: 5_000 });
    assert.deepEqual(failures, []);
  } finally {
    await browser.close();
  }
});
