// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Search from the bar, and a search that narrows (F10-05; ADR-0063 decision 4, ADR-0064).
//
// Three things this proves that no unit test can. The bar's field **leads** rather than searches:
// Enter navigates to `/search` and the words arrive in the field there, having travelled in memory
// — `assert(search === '')` is the whole of the deviation from the decision, in one line, because
// `/search` is a POST with no GET so that a term never reaches an access log. The chips **are** in
// the address, because a narrowing is structural and linkable. And a narrowing with no words at
// all is a question the server answers, which is what ADR-0064 changed.
//
// Chromium only: what is walked here is a form, a navigation and a query string, none of which
// has an engine-specific part; `engines.test.mjs` loads the bundle in all three.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const ACCOUNT = { id: '01a0e2e1-0000-7000-8000-000000000001', kind: 'USER', display_name: 'Search Walker', email: 'search@example.invalid', status: 'ACTIVE', locale: 'en', onboarding_completed_at: '2026-09-01T00:00:00Z' };
const HUB = { id: '01a0e2e1-0000-7000-8000-000000000002', type: 'HUB', parent_id: null, name: 'House', order_key: 'a0', version: 1 };
const COLLECTION = { id: '01a0e2e1-0000-7000-8000-000000000003', type: 'COLLECTION', parent_id: HUB.id, name: 'Kitchen', order_key: 'a0', version: 1 };
const HIT = { id: '01a0e2e1-0000-7000-8000-000000000004', type: 'TASK', container_id: COLLECTION.id, parent_id: null, title: 'Buy milk', is_completed: false, order_key: 'a0', version: 1 };
const PAGE = { data: [], items: [], page: { next_cursor: null, has_more: false } };

/** Every search this walk makes, in the order the client made them. */
const asked = [];

async function stub(route) {
  const url = new URL(route.request().url());
  const path = url.pathname;
  if (path.endsWith('/api/v1/stream')) return route.abort();
  if (path.endsWith('/api/v1/sync:snapshot')) {
    const record = JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: HUB.id, container_id: null, payload: HUB });
    return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${record}\n{"cursor":"c-e2e"}\n` });
  }
  if (path.endsWith('/api/v1/sync:pull')) return route.fulfill({ json: { changes: [], cursor: 'c-e2e', has_more: false, tombstone_window_days: 90 } });
  if (path.endsWith('/api/v1/accounts/me')) return route.fulfill({ json: ACCOUNT });
  if (path.endsWith('/api/v1/meta/capabilities')) {
    return route.fulfill({
      json: {
        product_version: '0.9.0',
        api_version: 'v1',
        tenancy_mode: 'single',
        item_types: [],
        supported_locales: [{ locale: 'en', direction: 'ltr' }],
        text_languages: ['en'],
        // What the chips are offered from: a chip whose field the installation does not report is
        // not drawn, so this list is what the walk below expects to see.
        query_fields: [
          { field: 'type', operators: ['IN'] },
          { field: 'is_completed', operators: ['EQ'] },
          { field: 'due_at', operators: ['LTE', 'IS_NULL'] },
          { field: 'collection_id', operators: ['IN'] },
        ],
      },
    });
  }
  if (path.endsWith('/api/v1/search')) {
    asked.push(route.request().postDataJSON());
    return route.fulfill({ json: { ...PAGE, data: [HIT] } });
  }
  if (path.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: [HUB] } });
  if (path.endsWith('/api/v1/containers')) return route.fulfill({ json: { ...PAGE, data: [COLLECTION] } });
  if (path.endsWith(`/api/v1/containers/${COLLECTION.id}`)) return route.fulfill({ json: COLLECTION });
  if (/\/(labels|buckets|views|templates|custom-fields|policies|members)$/.test(path)) return route.fulfill({ json: [] });
  return route.fulfill({ json: PAGE });
}

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, width) {
  const context = await browser.newContext({ viewport: { width, height: 800 } });
  // The copy control is offered only where the clipboard is, and a headless context is not asked.
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await context.route('**/api/v1/**', stub);
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/`);
  await page.getByRole('button', { name: ACCOUNT.display_name }).first().waitFor({ timeout: 15_000 });
  return { page, failures, close: () => context.close() };
}

test('chromium: the bar leads to the search, and the words are not in the address', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, 1280);
  t.after(close);
  asked.length = 0;

  const field = page.locator('header form[role="search"] input');
  await field.fill('milk');
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => location.pathname === '/search', null, { timeout: 5_000 });

  // The words arrived, and the address carries a handle rather than them (issue 997).
  assert.equal(new URL(page.url()).searchParams.get('q'), null, 'the term reached the address bar');
  assert.equal(page.url().includes('milk'), false, `the term is in the address: ${page.url()}`);
  await page.getByRole('link', { name: HIT.title }).waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('main input[type="search"]').inputValue(), 'milk', 'the screen did not take the words');
  // And the bar's own field emptied itself, so one question is not shown in two places.
  assert.equal(await field.inputValue(), '', 'the bar kept the words as well');
  assert.equal(asked.at(0)?.q, 'milk', `the server was asked ${JSON.stringify(asked.at(0))}`);

  // Searching again from the bar while already on the screen is a second search, not nothing.
  await field.fill('bread');
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => document.querySelector('main input[type="search"]')?.value === 'bread', null, { timeout: 5_000 });
  await page.waitForFunction(() => true);
  assert.equal(page.url().includes('bread'), false, `the second term is in the address: ${page.url()}`);

  assert.deepEqual(failures, []);
});

test('chromium: a reload keeps the words, and the address never held them', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, 1280);
  t.after(close);
  asked.length = 0;

  await page.locator('header form[role="search"] input').fill('milk');
  await page.keyboard.press('Enter');
  await page.getByRole('link', { name: HIT.title }).waitFor({ timeout: 10_000 });

  // The address names a handle and nothing else about the search's content.
  await page.waitForFunction(() => new URL(location.href).searchParams.get('s') !== null, null, { timeout: 5_000 });
  const handle = new URL(page.url()).searchParams.get('s');
  assert.match(handle, /^[0-9a-f]{8}$/, `the handle is ${handle}`);
  assert.equal(page.url().includes('milk'), false, 'the term is in the address');

  // What a reload is for: the words come back, and so do the results.
  await page.reload();
  await page.getByRole('link', { name: HIT.title }).waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('main input[type="search"]').inputValue(), 'milk', 'the reload lost the words');
  assert.equal(new URL(page.url()).searchParams.get('s'), handle, 'the reload minted a second handle');

  // Going away and coming back is the same promise through the other door.
  await page.getByRole('link', { name: HIT.title }).click();
  await page.goBack();
  await page.waitForFunction(() => document.querySelector('main input[type="search"]')?.value === 'milk', null, { timeout: 10_000 });

  assert.deepEqual(failures, []);
});

test('chromium: a link carries the narrowing and none of the words', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, 1280);
  t.after(close);
  asked.length = 0;

  // A search with both: words typed, and a chip chosen.
  await page.goto(`${served.origin}/search?type=TASK`);
  await page.locator('main input[type="search"]').fill('milk');
  await page.getByRole('link', { name: HIT.title }).waitFor({ timeout: 10_000 });
  await page.waitForFunction(() => new URL(location.href).searchParams.get('s') !== null, null, { timeout: 5_000 });

  // The link the screen offers: the chip, and no handle - a handle is this tab's, not a link's.
  const link = await page.evaluate(async () => {
    const control = [...document.querySelectorAll('button')].find((each) => each.textContent?.includes('Copy a link'));
    control?.click();
    await new Promise((resolve) => setTimeout(resolve, 200));
    return navigator.clipboard.readText();
  });
  assert.equal(link.includes('type=TASK'), true, `the link lost the narrowing: ${link}`);
  assert.equal(link.includes('milk'), false, `the link carries the term: ${link}`);
  assert.equal(link.includes('s='), false, `the link carries a handle: ${link}`);

  // And opening it in a tab that never saw the words shows the narrowing with an empty field.
  const second = await page.context().newPage();
  await second.goto(link);
  await second.getByRole('link', { name: HIT.title }).waitFor({ timeout: 15_000 });
  assert.equal(await second.locator('main input[type="search"]').inputValue(), '', 'the link carried the words after all');
  await second.close();

  assert.deepEqual(failures, []);
});

test('chromium: a chip is in the address, and a narrowing with no words is a search', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, 1280);
  t.after(close);
  asked.length = 0;

  // Straight to the screen, with nothing typed: the empty state, and no request yet.
  await page.goto(`${served.origin}/search`);
  const chip = page.getByRole('button', { name: /^Kind/ });
  await chip.waitFor({ timeout: 10_000 });
  assert.equal(asked.length, 0, 'an empty screen searched');

  // A chip with no words at all — what ADR-0064 made askable.
  await chip.click();
  await page.getByRole('checkbox', { name: 'Task' }).check();
  await page.keyboard.press('Escape');
  await page.waitForFunction(() => new URL(location.href).searchParams.get('type') === 'TASK', null, { timeout: 5_000 });
  await page.getByRole('link', { name: HIT.title }).waitFor({ timeout: 10_000 });

  const wordless = asked.at(-1);
  assert.equal(wordless.q, undefined, `a wordless search sent ${JSON.stringify(wordless)}`);
  assert.deepEqual(wordless.filter, { op: 'IN', field: 'type', value: ['TASK'] }, `the filter sent was ${JSON.stringify(wordless.filter)}`);

  // The narrowing is a link: the address alone brings the same screen back.
  const linked = page.url();
  await page.goto(`${served.origin}/`);
  await page.goto(linked);
  await page.getByRole('link', { name: HIT.title }).waitFor({ timeout: 10_000 });
  assert.equal(await page.getByRole('button', { name: /^Kind/ }).textContent().then((text) => text.includes('1')), true, 'the chip does not say how many are chosen');

  assert.deepEqual(failures, []);
});
