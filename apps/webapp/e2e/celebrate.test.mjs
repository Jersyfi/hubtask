// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Milestones and kudos (UC-WRK-29 checks 4-7): a finished collection and hub carry their milestone
// with its date in the head; the reader gives and takes back kudos on an entry somebody else
// completed, and reads who gave kudos on one they completed, by name and never as a number; with
// celebrations off nothing of either is read or drawn. Chromium, over the shared fixture's finished
// hub, at three widths and in both modes.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { DONE_BY_ME, DONE_BY_OTHER, FINISHED, FINISHED_HUB, OTHER, showCelebrations, signedIn } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const DAY = 'Everything done on Oct 20, 2026';

async function open(browser, width, path, heading) {
  // UTC, so that the milestone's day reads the same on every machine.
  const tab = await signedIn(browser, width, 900, { timezoneId: 'UTC' });
  const kudosReads = [];
  tab.page.on('request', (request) => {
    if (request.url().includes('/kudos')) kudosReads.push(`${request.method()} ${new URL(request.url()).pathname}`);
  });
  await tab.page.goto(`${served.origin}${path}`);
  await tab.page.getByRole('heading', { name: heading, level: 1 }).waitFor({ state: 'attached', timeout: 15_000 });
  return { ...tab, kudosReads };
}

for (const width of [390, 700, 1280]) {
  test(`chromium: ${width} px — a finished collection and hub carry their milestone`, async (t) => {
    showCelebrations();
    const browser = await chromium.launch();
    t.after(() => browser.close());
    for (const [path, name] of [[`/collections/${FINISHED.id}`, FINISHED.name], [`/hubs/${FINISHED_HUB.id}`, FINISHED_HUB.name]]) {
      const { page, context, failures } = await open(browser, width, path, name);
      const milestone = page.locator('[data-milestone]');
      await milestone.waitFor({ timeout: 5_000 });
      assert.equal((await milestone.textContent())?.trim(), DAY, name);
      assert.deepEqual(failures, []);
      await context.close();
    }
  });
}

test('chromium: 1280 px — kudos on somebody else\'s finished entry are given and taken back', async (t) => {
  showCelebrations();
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures, kudosReads } = await open(browser, 1280, `/items/${DONE_BY_OTHER.id}`, DONE_BY_OTHER.title);
  const toggle = page.getByRole('button', { name: `Give kudos to ${OTHER.display_name} for finishing this` });
  await toggle.waitFor({ timeout: 5_000 });
  assert.equal(await toggle.getAttribute('aria-pressed'), 'false');
  await toggle.click();
  await page.getByText(`Kudos given to ${OTHER.display_name}.`).first().waitFor({ timeout: 5_000 });
  await assert.doesNotReject(() => page.waitForFunction(
    () => document.querySelector('[data-kudos="give"] button')?.getAttribute('aria-pressed') === 'true', undefined, { timeout: 5_000 },
  ));
  await toggle.click();
  await page.getByText('Kudos taken back.').first().waitFor({ timeout: 5_000 });
  assert.equal(await toggle.getAttribute('aria-pressed'), 'false');
  assert.ok(kudosReads.includes(`PUT /api/v1/items/${DONE_BY_OTHER.id}/kudos`), kudosReads.join(', '));
  assert.ok(kudosReads.includes(`DELETE /api/v1/items/${DONE_BY_OTHER.id}/kudos`), kudosReads.join(', '));
  assert.deepEqual(failures, []);
  await context.close();
});

test('chromium: 1280 px — the person who finished reads who gave kudos, never a count', async (t) => {
  showCelebrations();
  const browser = await chromium.launch();
  t.after(() => browser.close());
  for (const colorScheme of ['light', 'dark']) {
    const { page, context, failures } = await open(browser, 1280, `/items/${DONE_BY_ME.id}`, DONE_BY_ME.title);
    await page.emulateMedia({ colorScheme });
    const received = page.locator('[data-kudos="received"]');
    await received.waitFor({ timeout: 5_000 });
    const text = (await received.textContent())?.trim() ?? '';
    assert.equal(text, `Kudos from ${OTHER.display_name}`);
    assert.doesNotMatch(text, /\d/);
    // One's own completion offers nothing to give.
    assert.equal(await page.locator('[data-kudos="give"]').count(), 0);
    assert.deepEqual(failures, []);
    await context.close();
  }
});

test('chromium: 1280 px — with celebrations off, no milestone and no kudos are read or drawn', async (t) => {
  showCelebrations({ celebrating: false });
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const visits = [
    [`/collections/${FINISHED.id}`, FINISHED.name],
    [`/hubs/${FINISHED_HUB.id}`, FINISHED_HUB.name],
    [`/items/${DONE_BY_OTHER.id}`, DONE_BY_OTHER.title],
    [`/items/${DONE_BY_ME.id}`, DONE_BY_ME.title],
  ];
  for (const [path, name] of visits) {
    const { page, context, failures, kudosReads } = await open(browser, 1280, path, name);
    // The account is read before anything is drawn; give the page the time a milestone would take.
    await page.waitForLoadState('networkidle');
    assert.equal(await page.locator('[data-milestone]').count(), 0, `${name} draws a milestone`);
    assert.equal(await page.locator('[data-kudos]').count(), 0, `${name} draws kudos`);
    assert.deepEqual(kudosReads, [], `${name} read kudos`);
    assert.deepEqual(failures, []);
    await context.close();
  }
});
