// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Your settings save in place (UC-WRK-32 checks 3, 6 and 7 for the eight screens of the section):
// a form with changes shows one bar with their count, *Discard* and *Save*; saving is busy in the
// control's own place and confirms as a passing message; nothing is switched off for want of a
// change. Read in the built app over the shared fixture, at the three widths a person meets.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, MANIFEST, signedIn } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

for (const width of [375, 768, 1280]) {
  test(`chromium: ${width} px — the profile shows a save bar with its changes, sends only them, and confirms`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, context, failures } = await signedIn(browser, width, 900);
    const sent = [];
    await context.route(`**/api/v1/accounts/${ACCOUNT.id}/preferences`, (route) => {
      sent.push(route.request().postDataJSON());
      return route.fulfill({ json: { ...ACCOUNT, ...route.request().postDataJSON() } });
    });

    await page.goto(`${served.origin}/profile`);
    const language = page.getByLabel('Language', { exact: true });
    await language.waitFor({ timeout: 15_000 });

    // No bar and no switched-off save while nothing has changed.
    const bar = page.getByRole('region', { name: 'Unsaved changes' });
    assert.equal(await bar.count(), 0, 'the bar is there without a change');
    assert.equal(await page.locator('main [aria-disabled="true"]').count(), 0, 'something on the profile is switched off');

    await page.getByLabel('A week starts on', { exact: true }).selectOption('SUNDAY');
    await bar.waitFor();
    assert.match(await bar.innerText(), /1 change/);
    await language.selectOption('de');
    assert.match(await bar.innerText(), /2 changes/);

    // Discard puts the account's values back and the bar goes.
    await bar.getByRole('button', { name: 'Discard' }).click();
    await bar.waitFor({ state: 'detached' });
    assert.equal(sent.length, 0, 'discarding wrote something');

    // One change, saved: only that field travels, and a passing message says it landed.
    await page.getByLabel('A week starts on', { exact: true }).selectOption('SUNDAY');
    await bar.getByRole('button', { name: 'Save' }).click();
    // Drawn once as a toast with no live region of its own, and said once, by the frame's region.
    const shown = page.locator('[data-tone="success"]', { hasText: 'Saved.' });
    await shown.waitFor({ timeout: 5_000 });
    assert.equal(await shown.getAttribute('role'), null, 'the toast announces itself beside the frame');
    await page.waitForFunction(() => [...document.querySelectorAll('[role="status"]')].filter((node) => node.textContent?.trim() === 'Saved.').length === 1);
    assert.deepEqual(sent, [{ week_start: 'SUNDAY' }], 'the save sent more than what changed');
    await bar.waitFor({ state: 'detached' });
    assert.deepEqual(failures, []);
  });
}

const CATEGORIES = ['ASSIGNMENT', 'COMMENT', 'INVITATION'];

/** Serves the notification categories and the reader's rows, every one sent by mail. */
async function withNotifications(context) {
  await context.route('**/api/v1/meta/capabilities', (route) => route.fulfill({ json: { ...MANIFEST, notification_categories: CATEGORIES, notification_channels: ['EMAIL'] } }));
  await context.route(`**/api/v1/accounts/${ACCOUNT.id}/notification-preferences**`, (route) => {
    if (route.request().method() !== 'GET') return route.fulfill({ json: {} });
    return route.fulfill({ json: { data: CATEGORIES.map((category) => ({ category, channel: 'EMAIL', enabled: true, include_title: false, is_default: true })) } });
  });
}

for (const width of [375, 768, 1280]) {
  test(`chromium: ${width} px — the always-on row says it once, and its box does not flip`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, context, failures } = await signedIn(browser, width, 900);
    await withNotifications(context);
    await page.goto(`${served.origin}/profile/notifications`);
    const row = page.locator('tbody tr', { hasText: 'I am invited to the workspace' });
    await row.waitFor({ timeout: 15_000 });
    assert.equal(await page.getByText('You are always told about this one').count(), 1, 'the always-on sentence is said more than once');
    const box = row.getByRole('checkbox', { name: 'Tell me' });
    assert.equal(await box.getAttribute('aria-disabled'), 'true');
    const described = await box.evaluate((node) => document.getElementById(node.getAttribute('aria-describedby') ?? '')?.textContent ?? '');
    assert.match(described, /You are always told about this one/);
    await box.focus();
    await page.keyboard.press(' ');
    assert.equal(await box.isChecked(), true, 'the always-on box was switched off');
    assert.equal(await page.locator('[role="tooltip"]:visible').count(), 0, 'the box drew a bubble beside the note');
    assert.deepEqual(failures, []);
  });

  test(`chromium: ${width} px — tokens and passkeys mark what is missing when pressed`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, context, failures } = await signedIn(browser, width, 900);
    const sent = [];
    context.on('request', (request) => {
      if (request.method() === 'POST' && request.url().includes('/api/v1/')) sent.push(new URL(request.url()).pathname);
    });
    await page.goto(`${served.origin}/profile/tokens`);
    const create = page.getByRole('button', { name: 'Create the token' });
    await create.waitFor({ timeout: 15_000 });
    assert.equal(await create.getAttribute('aria-disabled'), null, 'Create is switched off before anything is filled');
    await create.click();
    await page.waitForFunction(() => document.activeElement?.getAttribute('aria-invalid') === 'true');
    assert.equal(await page.getByLabel('What it is for').getAttribute('aria-invalid'), 'true');

    // A virtual authenticator, as passkeys.test.mjs has: without one this browser offers no
    // passkey at all and the panel says so in the form's place.
    const cdp = await context.newCDPSession(page);
    await cdp.send('WebAuthn.enable');
    await cdp.send('WebAuthn.addVirtualAuthenticator', {
      options: { protocol: 'ctap2', transport: 'internal', hasResidentKey: true, hasUserVerification: true, isUserVerified: true, automaticPresenceSimulation: true },
    });
    // At `localhost`: an address is no relying party, so the add form is drawn only there.
    await page.goto(`${served.origin.replace('127.0.0.1', 'localhost')}/profile/security`);
    const name = page.getByTestId('passkeys').getByRole('textbox', { name: /^Name/ });
    await name.waitFor({ timeout: 15_000 });
    await name.fill('');
    await page.getByRole('button', { name: 'Add a passkey' }).click();
    await page.waitForFunction(() => document.activeElement?.getAttribute('aria-invalid') === 'true');
    assert.deepEqual(sent.filter((path) => path.includes('token') || path.includes('passkey')), [], `something was sent: ${sent}`);
    assert.deepEqual(failures, []);
  });

  test(`chromium: ${width} px — a switch saves busy in place, and a toast confirms above the bars`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, context, failures } = await signedIn(browser, width, 900);
    let release;
    const held = new Promise((resolve) => (release = resolve));
    await context.route(`**/api/v1/accounts/${ACCOUNT.id}/preferences`, async (route) => {
      await held;
      return route.fulfill({ json: { ...ACCOUNT, ...route.request().postDataJSON() } });
    });
    await page.goto(`${served.origin}/profile`);
    const moments = page.getByRole('switch', { name: 'Mark the moments' }).or(page.getByLabel('Mark the moments'));
    await moments.first().waitFor({ timeout: 15_000 });
    await moments.first().click();
    await page.waitForFunction(() => document.querySelector('[aria-busy="true"]') !== null);
    assert.equal(await moments.first().getAttribute('aria-disabled'), null, 'the switch is switched off for "Saving"');
    release();
    const shown = page.locator('[data-tone="success"]').first();
    await shown.waitFor({ timeout: 5_000 });
    // Nothing covers it, and it covers no bar: it sits above the phone's bottom bar.
    const bottom = await shown.evaluate((node) => node.getBoundingClientRect().bottom);
    const bars = await page.locator('nav[aria-label] >> visible=true').evaluateAll((nodes) => nodes.map((node) => node.getBoundingClientRect()).filter((box) => box.bottom >= window.innerHeight - 1).map((box) => box.top));
    for (const top of bars) assert.ok(bottom <= top + 1, `the toast covers a bar at the foot (${bottom} > ${top})`);
    // Held while the pointer is on it.
    await shown.hover();
    await page.waitForTimeout(200);
    assert.equal(await shown.isVisible(), true);
    assert.deepEqual(failures, []);
  });
}
