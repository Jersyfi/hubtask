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
import { ACCOUNT, signedIn } from './fixture.mjs';

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
