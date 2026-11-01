// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A template is chosen, then put (UC-CNV-01, SC 3.2.2): a closed native select changes on every
// arrow key, and a template put on each change filled the shared board with stacked copies for
// everyone drawing. Looking through the templates from the keyboard puts nothing; the button does.

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

test('chromium: arrow keys through the template select put nothing; the button puts one', async (t) => {
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
  const select = page.getByLabel('Template');
  await select.waitFor({ timeout: 20_000 });
  const objects = () => canvas.locator('g.object').count();
  assert.equal(await objects(), 0);

  // What an arrow key does to a closed select on Windows: the value changes and `change` fires,
  // once per press. Chromium on other systems opens the list instead, so the changes are made as
  // the select would make them.
  const offered = (await select.locator('option').evaluateAll((options) => options.map((each) => each.value))).filter(Boolean);
  assert.ok(offered.length >= 3, `the select offers ${offered.length} templates`);
  for (const each of offered.slice(0, 3)) await select.selectOption(each);
  await page.waitForTimeout(300);
  assert.equal(await select.inputValue(), offered[2]);
  assert.equal(await objects(), 0, 'looking through the templates put one on the board');

  await page.locator('[data-put-template]').click();
  await canvas.locator('g.object').first().waitFor({ timeout: 5_000 });
  const put = await objects();
  assert.ok(put > 0, 'the button put nothing');
  // The choice is spent: a second press needs a template chosen again, and says so.
  assert.equal(await page.locator('[data-put-template]').getAttribute('aria-disabled'), 'true');
  await page.locator('[data-put-template]').click({ force: true });
  await page.waitForTimeout(300);
  assert.equal(await objects(), put, 'one choice put two templates');
});
