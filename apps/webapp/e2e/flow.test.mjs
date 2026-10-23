// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A collection run as a flow (UC-WRK-31): it opens on the board, its states are the columns with
// their limits and the column that completes; an entry refused past the first column says which
// field it lacks; a move into a full column goes ahead and is announced; the head and the
// navigation row show the flow figure and mark the full column - at three widths, light and dark.
// Chromium, over the shared fixture's figure hub; the server's own refusal is
// test/integration/flow_gate_test.go.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { BUCKETS, FIGURES, ITEMS, showFigures, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());
showFigures();

const FLOW = FIGURES.flow;
/** The board's columns, with *Doing* holding as many as its limit allows. */
const COLUMNS = BUCKETS.map((bucket) => ({ ...bucket, collection_id: FLOW.id, ...(bucket.id === BUCKETS[1].id ? { wip_limit: 1 } : {}) }));
const REFUSED = ITEMS[2];
const ACCEPTED = ITEMS[0];

/** The flow, opened, with every write recorded and the gate's refusal answered for one entry. */
async function flow(browser, width, colorScheme = 'light') {
  const written = [];
  const tab = await signedIn(browser, width, 900);
  await tab.context.unroute('**/api/v1/**');
  await tab.context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() !== 'GET') written.push({ method: request.method(), path, body: request.postDataJSON() });
    if (path.endsWith('/buckets')) return route.fulfill({ json: COLUMNS });
    if (request.method() === 'PATCH' && path.endsWith(`/items/${REFUSED.id}`)) {
      return route.fulfill({
        status: 422,
        contentType: 'application/problem+json',
        json: {
          type: 'https://hubtask.eu/problems/validation_failed', title: 'validation_failed', status: 422,
          code: 'validation_failed', detail_code: 'items.required_fields_missing',
          params: { keys: 'customer', count: '1' },
          field_errors: [{ path: '/custom_fields/customer', code: 'fields.value_required', params: { key: 'customer' } }],
        },
      });
    }
    return stub(route);
  });
  await tab.page.emulateMedia({ colorScheme });
  await tab.page.goto(`${served.origin}/collections/${FLOW.id}`);
  await tab.page.getByRole('heading', { name: FLOW.name, level: 1 }).waitFor({ state: 'attached', timeout: 15_000 });
  return { ...tab, written };
}

async function moveByMenu(page, card, column) {
  await page.locator(`[data-card="${card.id}"]`).click({ button: 'right' });
  await page.getByRole('menuitem', { name: `Move ${card.title} to ${column}`, exact: true }).click();
}

test('chromium: UC-WRK-31 check 1 — a flow opens on the board, its states the columns with a limit and a done column', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await flow(browser, 1280);
  t.after(() => context.close());
  // No layout was chosen: the board is the collection's default.
  await page.locator('[data-column-zone]').first().waitFor({ timeout: 10_000 });
  assert.equal(await page.getByRole('radio', { name: 'Board' }).isChecked(), true);
  for (const column of COLUMNS) {
    assert.equal(await page.locator(`[data-column-zone="${column.id}"]`).count(), 1, column.name);
  }
  // *Doing* shows its count against its limit; *Done* is the column that completes.
  const doing = page.locator(`[data-column-zone="${BUCKETS[1].id}"] .count`);
  assert.equal((await doing.textContent())?.replace(/\s+/g, ''), '1/1');
  const done = page.locator(`[data-column-zone="${BUCKETS[2].id}"] .done`);
  assert.equal(await done.getAttribute('title'), 'Entries moved here are marked as done.');
  assert.equal(await page.locator(`[data-column-zone="${BUCKETS[0].id}"] .done`).count(), 0);
  // And an entry put in it is completed.
  const completed = page.waitForRequest((request) => request.method() === 'POST' && request.url().endsWith(`/items/${ACCEPTED.id}:complete`));
  await moveByMenu(page, ACCEPTED, BUCKETS[2].name);
  await completed;
  assert.deepEqual(failures, []);
});

test('chromium: UC-WRK-31 checks 2 and 3 — a refusal names the field; a move into a full column goes ahead and is said', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, written } = await flow(browser, 1280);
  t.after(() => context.close());
  await page.locator(`[data-card="${REFUSED.id}"]`).waitFor({ timeout: 10_000 });

  await moveByMenu(page, REFUSED, BUCKETS[1].name);
  const alert = page.getByRole('alert').filter({ hasText: 'customer' });
  await alert.waitFor({ timeout: 5_000 });
  assert.equal((await alert.textContent())?.trim(), 'Fill in customer before this entry moves past the first column.');

  await moveByMenu(page, ACCEPTED, BUCKETS[1].name);
  const said = page.locator('.announcement[role="status"]');
  await page.waitForFunction(() => /holds more than its limit/.test(document.querySelector('.announcement')?.textContent ?? ''), null, { timeout: 5_000 });
  assert.equal(
    (await said.textContent())?.trim(),
    `${ACCEPTED.title} moved to ${BUCKETS[1].name}, which now holds more than its limit of 1.`,
  );
  const move = written.find((each) => each.method === 'PATCH' && each.path.endsWith(`/items/${ACCEPTED.id}`));
  assert.equal(move?.body?.bucket_id, BUCKETS[1].id, 'the move into the full column was not sent');
  assert.deepEqual(failures, []);
});

test('chromium: UC-WRK-31 check 2 — the Structure panel says the flow keeps its required fields, and switches it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, written } = await flow(browser, 1280);
  t.after(() => context.close());
  await page.getByRole('button', { name: `Actions for ${FLOW.name}` }).click();
  await page.getByRole('menuitem', { name: 'Structure' }).click();
  const section = page.locator('dialog[open] [data-setting="automation"]');
  await section.waitFor({ timeout: 5_000 });
  const gate = section.getByRole('checkbox', { name: 'Required fields are filled in before an entry leaves the first column' });
  assert.equal(await gate.isChecked(), true);
  await gate.uncheck();
  const sent = page.waitForRequest((request) => request.method() === 'PATCH' && request.url().endsWith(`/containers/${FLOW.id}/structure`));
  await page.locator('dialog[open]').getByRole('button', { name: 'Save', exact: true }).click();
  assert.deepEqual((await sent).postDataJSON(), { settings: { automation: [] } });
  assert.equal(written.filter((each) => each.path.endsWith('/structure')).length, 1);
  assert.deepEqual(failures, []);
});

for (const width of [390, 700, 1280]) {
  test(`chromium: ${width} px — UC-WRK-31 checks 3 and 4: the flow figure and the full column in the head and the row, light and dark`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    for (const colorScheme of ['light', 'dark']) {
      const { page, failures, context } = await flow(browser, width, colorScheme);
      const figure = page.locator('[data-figure="FLOW"]').first();
      await figure.waitFor({ timeout: 10_000 });
      assert.equal((await figure.locator('.text').textContent())?.trim(), '12 a week · 2.1 days on average');
      assert.equal((await figure.locator('[data-over-limit]').textContent())?.trim(), '1 column over its limit');
      // The mark is a tinted surface, not the page's own background, in either mode.
      const tinted = await figure.locator('[data-over-limit]').evaluate((el) => getComputedStyle(el).backgroundColor);
      assert.notEqual(tinted, 'rgba(0, 0, 0, 0)', `${colorScheme}: the full column is not marked`);
      const sideways = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      assert.ok(sideways <= 0, `${colorScheme}: the page scrolls sideways by ${sideways} px`);
      if (width >= 1280) {
        const row = page.locator(`[data-node="${FLOW.id}"] .meta`);
        await row.waitFor({ timeout: 5_000 });
        assert.equal((await row.textContent())?.trim(), '1 full');
      }
      assert.deepEqual(failures, []);
      await context.close();
    }
  });
}
