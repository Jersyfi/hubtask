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
// The narrowing travels as one parameter, `?f=`, written in the language the chips and the text
// surface both speak (`data/searchquery.ts`) — one parameter per chip is what it was before, and
// it could say only the six things the chips had controls for.
//
// Chromium only: what is walked here is a form, a navigation and a query string, none of which
// has an engine-specific part; `engines.test.mjs` loads the bundle in all three.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { fallback, unstubbedSoFar } from './fixture.mjs';
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
          { field: 'assignee_id', operators: ['EQ', 'IS_NULL'] },
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
  // The entry itself, because this walk opens one: the back button is half of what it proves. A
  // page envelope here would put `undefined` where the screen reads a type, and the entry screen
  // throws while rendering - on a slow machine, where it gets far enough to try.
  if (path.endsWith(`/api/v1/items/${HIT.id}`)) return route.fulfill({ json: HIT });
  if (path.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: [HUB] } });
  if (path.endsWith('/api/v1/containers')) return route.fulfill({ json: { ...PAGE, data: [COLLECTION] } });
  if (path.endsWith(`/api/v1/containers/${COLLECTION.id}`)) return route.fulfill({ json: COLLECTION });
  if (/\/(labels|buckets|views|templates|custom-fields|policies|members)$/.test(path)) return route.fulfill({ json: [] });
  // The frame's own reads, in the shapes the API answers them, and a record of anything this
  // walk never prepared: a guess nobody notices is what `fallback` exists to prevent.
  return fallback(route, route.request(), path);
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

/** Waits for the screen to have asked at least this many searches, or gives up saying so. */
async function waitForAsked(count) {
  for (let tries = 0; tries < 100; tries += 1) {
    if (asked.length >= count) return;
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  assert.fail(`only ${asked.length} searches were asked, wanted ${count}`);
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

  // Going away and coming back is the same promise through the other door. The entry is waited
  // for rather than raced past: a `goBack()` issued while the entry is still rendering passes on a
  // fast machine and fails on a slow one, which is how this walk first went red only in CI.
  await page.getByRole('link', { name: HIT.title }).click();
  await page.waitForFunction(() => location.pathname.startsWith('/items/'), null, { timeout: 10_000 });
  await page.getByRole('heading', { name: HIT.title }).first().waitFor({ timeout: 10_000 });
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
  await page.goto(`${served.origin}/search?f=type%3Atask`);
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
  assert.equal(decodeURIComponent(link).includes('f=type:task'), true, `the link lost the narrowing: ${link}`);
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
  await page.waitForFunction(() => new URL(location.href).searchParams.get('f') === 'type:task', null, { timeout: 5_000 });
  await page.getByRole('link', { name: HIT.title }).waitFor({ timeout: 10_000 });

  const wordless = asked.at(-1);
  assert.equal(wordless.q, undefined, `a wordless search sent ${JSON.stringify(wordless)}`);
  assert.deepEqual(wordless.filter, { op: 'IN', field: 'type', value: ['TASK'] }, `the filter sent was ${JSON.stringify(wordless.filter)}`);

  // The narrowing is a link: the address alone brings the same screen back.
  const linked = page.url();
  await page.goto(`${served.origin}/`);
  await page.goto(linked);
  await page.getByRole('link', { name: HIT.title }).waitFor({ timeout: 10_000 });
  // The chip says *what* is chosen, not how many (ADR-0066 decision 3): a count is a pill inside a
  // pill, and it makes a reader open a chip to find out what it holds.
  assert.match(
    await page.getByRole('button', { name: /^Kind/ }).textContent().then((text) => text.replace(/\s+/g, ' ').trim()),
    /^Kind\s*Task$/,
    'the chip does not name what is chosen',
  );

  assert.deepEqual(failures, []);
});

test('chromium: a narrowing pressed in the bar reaches a screen already standing on one', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, 1280);
  t.after(close);
  asked.length = 0;

  // Standing on a narrowed search already, which is what made this go wrong: the screen is
  // mounted, so nothing remounts, and a narrowing pressed in the bar arrives as a changed address
  // and nothing else.
  await page.goto(`${served.origin}/search?f=type%3Atask`);
  await page.getByRole('link', { name: HIT.title }).waitFor({ timeout: 10_000 });
  await waitForAsked(1);
  assert.deepEqual(asked.at(-1).filter, { op: 'IN', field: 'type', value: ['TASK'] });

  // The bar's menu, and one of the narrowings in it.
  const answered = asked.length;
  await page.getByRole('combobox', { name: 'Search everything' }).click();
  await page.locator('#search-menu').getByRole('option', { name: 'Mine, open' }).click();
  await page.waitForFunction(() => new URL(location.href).searchParams.get('f') === 'who:me is:open', null, { timeout: 5_000 });

  // The address moved, and so did the question. It used to move alone: the URL changed, the chips
  // kept their old answers and the same filter was asked again, which reads as a screen that did
  // not load.
  await waitForAsked(answered + 1);
  assert.deepEqual(
    asked.at(-1).filter,
    { op: 'AND', nodes: [{ op: 'EQ', field: 'assignee_id', value: '@me' }, { op: 'EQ', field: 'is_completed', value: false }] },
    `the screen kept asking ${JSON.stringify(asked.at(-1).filter)}`,
  );
  // And the chips say the new narrowing rather than the old one.
  const reads = async (name) =>
    (await page.getByRole('button', { name }).textContent()).replace(/\s+/g, ' ').trim();
  assert.equal(await reads(/^Kind/), 'Kind', 'the old narrowing is still on the chips');
  assert.equal(await reads(/^Status/), 'Status Open', 'the new narrowing is not on the chips');

  assert.deepEqual(failures, []);
});

test('chromium: Enter with nothing typed does what the menu says it does', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, 1280);
  t.after(close);

  // The menu draws the word "Enter" against "Open search" before a single character is typed, so
  // that is what the key has to mean there. It used to only open the menu - the one thing it
  // cannot mean, because the menu is already open: it is where the reader read the word.
  await page.getByRole('combobox', { name: 'Search everything' }).click();
  await page.locator('#search-menu').waitFor({ timeout: 10_000 });
  await page.keyboard.press('Enter');
  await page.waitForURL(/\/search$/, { timeout: 5_000 });
  await page.getByRole('heading', { name: 'Search' }).waitFor({ timeout: 10_000 });

  assert.deepEqual(failures, []);
});
