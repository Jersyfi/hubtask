// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A template is chosen, then put (UC-CNV-04 check 5, SC 3.2.2): a template put on each change of a
// choice filled the shared board with stacked copies for everyone drawing. In the gallery of
// *Put a template on the board…* (user-experience.md §7.2) choosing cards puts nothing; the
// dialog's own button puts the one chosen, once.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, HUB, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const BOARD_ID = '01a0e2e0-0000-7000-8000-00000000b0c1';
const BOARD = {
  id: BOARD_ID, hub_id: HUB.id, title: 'Retro', element_count: 0, elements: [], writable: true, trashed_at: null,
  created_by: ACCOUNT.id, created_at: '2026-10-30T09:00:00Z', updated_by: ACCOUNT.id, updated_at: '2026-10-30T09:00:00Z', version: 1,
};

test('chromium: choosing templates in the gallery puts nothing; the button puts the one chosen, once', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context } = await signedIn(browser, 1440, 900);
  t.after(() => context.close());
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith(`/whiteboards/${BOARD_ID}`)) return route.fulfill({ json: BOARD });
    if (path.endsWith(`/whiteboards/${BOARD_ID}/comments`)) return route.fulfill({ json: { data: [] } });
    if (path.endsWith(`/hubs/${HUB.id}/whiteboards`)) return route.fulfill({ json: { data: [] } });
    return stub(route);
  });
  await page.goto(`${served.origin}/whiteboards/${BOARD_ID}`);
  const canvas = page.getByRole('application');
  await canvas.waitFor({ timeout: 20_000 });
  const objects = () => canvas.locator('g.object, g.frame').count();
  assert.equal(await objects(), 0);

  await page.locator('[data-board-insert]').click();
  await page.getByRole('menuitem', { name: 'Put a template on the board…' }).click();
  const dialog = page.getByRole('dialog', { name: 'Put a template on the board…' });
  await dialog.waitFor({ timeout: 5_000 });
  assert.equal(await dialog.locator('[data-board-start=BLANK]').count(), 0, 'a board already open offers no blank start');
  for (const each of ['BRAINSTORM', 'FLOWCHART', 'USER_STORY_MAP']) {
    await dialog.locator(`[data-board-start=${each}] button`).click();
    assert.equal(await dialog.locator(`[data-board-start=${each}] button`).getAttribute('aria-pressed'), 'true');
  }
  await page.waitForTimeout(300);
  assert.equal(await objects(), 0, 'choosing in the gallery put a template on the board');

  await dialog.locator('[data-put-template]').click();
  await canvas.locator('g.frame').first().waitFor({ timeout: 5_000 });
  assert.equal(await dialog.count(), 0, 'the dialog stays open after putting');
  const put = await objects();
  assert.ok(put > 0, 'the button put nothing');
  await page.waitForTimeout(300);
  assert.equal(await objects(), put, 'one choice put two templates');
});
