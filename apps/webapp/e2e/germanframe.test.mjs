// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The frame and its navigation in a German session: every word the frame draws is German, the
// tree marks the place the reader is in - an entry's page its collection, not *Today* - and the
// column's create control has one name on every page. And the installation's door asks for its
// proof once, reading nothing before it is given.
//
// Against a stubbed API. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, COLLECTION, HUB, ITEMS, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const ENTRY = ITEMS[1];

const served = await serve(DIST);
test.after(() => served.close());

/** A signed-in tab whose account reads German; `answer` may take a path before the fixture does. */
async function german(browser, width = 1280, answer = () => undefined) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  const requests = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    requests.push({ method: request.method(), path });
    if (path === '/accounts/me' && request.method() === 'GET') return route.fulfill({ json: { ...ACCOUNT, locale: 'de' } });
    // The answer's own fulfilment, not awaited: a promise is what says it was taken.
    const answered = answer(route, path, request);
    if (answered !== undefined) return answered;
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  return { context, page, failures, requests };
}

const isGerman = (page) => page.waitForFunction(() => document.documentElement.lang === 'de', null, { timeout: 15_000 });

test('chromium: 1280 px — the frame of a collection speaks German: the twist, the sync, the pane, *Neu*', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await german(browser);
  t.after(() => context.close());

  await page.goto(`${served.origin}/collections/${COLLECTION.id}?item=${ENTRY.id}`);
  await isGerman(page);
  const pane = page.getByRole('complementary', { name: ENTRY.title });
  await pane.waitFor({ timeout: 15_000 });

  // The tree's twist names the hub in German.
  const nav = page.getByRole('navigation', { name: 'Arbeitsbereich' });
  await nav.getByRole('button', { name: `Verbergen, was in ${HUB.name} liegt` }).or(nav.getByRole('button', { name: `Zeigen, was in ${HUB.name} liegt` })).first().waitFor({ timeout: 5_000 });
  const twists = await nav.getByRole('button').evaluateAll((buttons) => buttons.map((b) => b.getAttribute('aria-label') ?? b.textContent?.trim() ?? ''));
  assert.equal(twists.some((name) => /what is in/.test(name)), false, `an English twist: ${twists.join(' | ')}`);

  // The pane's head names the entry's level as the collection does - not the contract's "Task".
  const head = await pane.evaluate((node) => node.textContent ?? '');
  assert.equal(/\bTask\b/.test(head), false, `the pane's head is English: ${head.slice(0, 120)}`);
  assert.match(head, /Aufgabe/);

  // One name for the column's create control: *Neu*, never a "Hub anlegen" of its own.
  const column = page.locator('aside.sidenav');
  assert.equal(await column.getByRole('button', { name: 'Neu', exact: true }).count(), 1, 'the column has no *Neu*');
  assert.equal(await column.getByRole('button', { name: 'Hub anlegen', exact: true }).count(), 0, 'the column names its create control twice');

  // Nothing the frame says is English: the bar's sync mark and the live region included.
  await page.waitForTimeout(800);
  const said = await page.evaluate(() =>
    [...document.querySelectorAll('[role="status"], [aria-live], header button')].map((node) => (node.textContent ?? '').trim()).filter(Boolean));
  for (const english of ['Connected', 'Reconnecting', 'Now showing changes as they happen.']) {
    assert.equal(said.some((text) => text.includes(english)), false, `"${english}" in a German frame: ${said.join(' | ')}`);
  }
  assert.ok(said.includes('Verbunden'), `the sync mark is not German: ${said.join(' | ')}`);
  // The page's first connection is no return, so the live region says nothing about it.
  assert.equal(said.some((text) => text.startsWith('Änderungen werden jetzt gezeigt')), false, said.join(' | '));
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — on an entry\'s page the tree marks its collection, and *Neu* stays *Neu*', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await german(browser);
  t.after(() => context.close());

  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await isGerman(page);
  await page.getByRole('heading', { level: 1 }).waitFor({ timeout: 15_000 });
  const nav = page.getByRole('navigation', { name: 'Arbeitsbereich' });
  const collection = nav.getByRole('treeitem', { name: COLLECTION.name });
  await collection.waitFor({ timeout: 10_000 });
  await page.waitForFunction((name) => [...document.querySelectorAll('[role="treeitem"]')]
    .some((row) => row.getAttribute('aria-current') !== null && (row.textContent ?? '').includes(name)), COLLECTION.name, { timeout: 10_000 })
    .catch(() => assert.fail('the entry\'s collection is not the current place'));
  const today = nav.getByRole('treeitem', { name: 'Heute', exact: true });
  assert.equal(await today.getAttribute('aria-current'), null, '*Heute* is marked as the place of an entry');
  assert.equal(await page.locator('aside.sidenav').getByRole('button', { name: 'Neu', exact: true }).count(), 1);
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — *Du* offers the workspace\'s language and zone in grammatical German', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await german(browser);
  t.after(() => context.close());

  await page.goto(`${served.origin}/profile`);
  await isGerman(page);
  const language = page.getByRole('combobox', { name: 'Sprache' });
  await language.waitFor({ timeout: 15_000 });
  const options = await page.locator('select option').allTextContents();
  assert.equal(options.some((text) => /^Das des/.test(text.trim())), false, `a wrong article: ${options.join(' | ')}`);
  assert.ok(options.some((text) => text.trim() === 'Wie im Arbeitsbereich'), options.join(' | '));
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — the installation asks for its proof once and reads nothing before it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  let proven = false;
  const { context, page, failures, requests } = await german(browser, 1280, (route, path) => {
    if (path === '/auth/sessions:elevate' && !proven) {
      return route.fulfill({
        status: 403,
        json: { code: 'errors.forbidden', detail_code: 'auth.step_up_required', status: 403, params: { methods: 'PASSWORD' }, request_id: 'req_g' },
      });
    }
    return undefined;
  });
  t.after(() => context.close());

  await page.goto(`${served.origin}/instance`);
  await isGerman(page);
  const prompt = page.getByRole('dialog', { name: 'Beweise, dass du es bist' });
  await prompt.waitFor({ timeout: 15_000 });
  await page.waitForTimeout(500);
  // One prompt: the door's own button is not standing, let alone spinning, behind it.
  assert.equal(await page.getByRole('button', { name: 'Nachweisen und weiter' }).count(), 0, 'a second prompt behind the dialog');
  // Nothing the door guards was read before the proof.
  const early = requests.filter((each) => each.path.startsWith('/admin/') || each.path.startsWith('/instance/'));
  assert.deepEqual(early, [], `read before the proof: ${JSON.stringify(early)}`);

  // Leaving the prompt leaves the door with its one way in.
  await page.keyboard.press('Escape');
  await prompt.waitFor({ state: 'hidden', timeout: 5_000 });
  await page.getByRole('button', { name: 'Nachweisen und weiter' }).waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
});
