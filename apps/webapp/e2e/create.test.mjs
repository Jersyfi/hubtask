// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Create anything from one button, UC-WRK-46 checks 1-6, walked against the shared fixture:
//
// * check 1 - *Create* in the bar, `C`, `Q` and the phone's ＋ open one menu; its first line is capture's;
// * check 2 - the kinds the place allows, the kinds made last; a kind whose module is off is absent;
// * check 3 - the place is preset from where one stands and is a chip one changes; the kinds follow it;
// * check 4 - a page, a whiteboard and a dashboard are made at once and opened; an inventory item, a
//   collection and an entry open their own dialog or screen;
// * check 5 - a kind's letter makes it while the menu is open; holding the phone's ＋ makes the kind made last;
// * check 6 - from a template and an import stand at the foot and open capture in that mode.
//
// What capture does with the line is UC-WRK-26's, walked in `capture.test.mjs`. Chromium only: a
// menu of buttons and a native select have no engine-specific part.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, COLLECTION, HUB, ITEMS, MANIFEST, PAGE, PERMISSIONS, signedIn } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const MADE = '01a0e2e0-0000-7000-8000-0000000046a1';
/** Every product whose kind *Create* offers in a hub. */
const ALL_ON = ['KNOWLEDGE', 'WHITEBOARD', 'GOALS', 'ASSETS', 'TIME'];

const createOf = (page) => page.getByRole('group', { name: 'Create' });
const lineOf = (page) => page.getByRole('textbox', { name: 'A new entry in one sentence' });
const captureOf = (page) => page.getByRole('dialog', { name: 'Capture' });
const kindsOf = (page) => createOf(page).locator('[data-kind]').evaluateAll((buttons) => [...new Set(buttons.map((each) => each.getAttribute('data-kind')))]);

/** A signed-in tab with `modulesOn` on, in the workspace and in House; every create answered. */
async function as(browser, { width = 1280, modulesOn = ALL_ON, hubRole, portals } = {}) {
  const { context, page, failures } = await signedIn(browser, width, 800, width < 600 ? { hasTouch: true } : {});
  const writes = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, modules_on: modulesOn, ...(hubRole ? { workspace_permissions: PERMISSIONS.MEMBER } : {}) } });
    }
    if (path === '/accounts/me' && portals) return route.fulfill({ json: { ...ACCOUNT, portals } });
    if (path === '/memberships' && hubRole) {
      const rows = new URL(request.url()).searchParams.get('scope_type') === 'HUB' ? [{ id: 'm2', scope_type: 'HUB', scope_id: HUB.id, account_id: ACCOUNT.id, role: hubRole }] : [];
      return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: [] } });
    }
    if (/^\/containers\/[^/]+\/modules$/.test(path) && request.method() === 'GET') return route.fulfill({ json: { modules_on: modulesOn } });
    if (request.method() === 'POST' && (path === `/hubs/${HUB.id}/pages` || path === `/hubs/${HUB.id}/whiteboards` || path === '/dashboards')) {
      writes.push({ call: `POST ${path}`, body: request.postDataJSON() });
      return route.fulfill({ status: 201, json: { id: MADE, hub_id: HUB.id, version: 1, ...request.postDataJSON() } });
    }
    return route.fallback();
  });
  return { context, page, failures, writes };
}

/** Lands on a page and lets the roles arrive, then puts focus on the page, outside any field. */
async function land(page, path) {
  await page.goto(`${served.origin}${path}`);
  await page.getByRole('main').waitFor();
  await page.waitForTimeout(600);
  await page.locator('body').click({ position: { x: 1, y: 300 } });
}

/** Opens *Create* by its letter and waits for the kinds of the hub one stands in. */
async function summon(page, key = 'c') {
  await page.keyboard.press(key);
  await lineOf(page).waitFor({ timeout: 5_000 });
  await createOf(page).locator('[data-kind="page"]').first().waitFor({ timeout: 5_000 });
}

test('chromium: Create, C, Q and the phone’s ＋ open one menu whose first line is capture’s (check 1)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await as(browser);
  t.after(() => context.close());

  for (const path of [`/collections/${COLLECTION.id}`, `/items/${ITEMS[0].id}`, `/hubs/${HUB.id}`, '/profile']) {
    for (const key of ['c', 'q']) {
      await land(page, path);
      await page.keyboard.press(key);
      await lineOf(page).waitFor({ timeout: 5_000 });
      // The line is the first thing in the menu, and it has the focus.
      assert.equal(await createOf(page).locator('input, button, select').first().getAttribute('aria-label'), 'A new entry in one sentence', `${key} on ${path}`);
      assert.equal(await page.evaluate(() => document.activeElement?.getAttribute('aria-label')), 'A new entry in one sentence', `${key} on ${path}: focus`);
      await page.keyboard.press('Escape');
      await lineOf(page).waitFor({ state: 'hidden', timeout: 5_000 });
    }
    const button = page.getByRole('banner').getByRole('button', { name: 'Create' });
    await button.click();
    await lineOf(page).waitFor({ timeout: 5_000 });
    // Wide, it hangs from the button (create d): its top just under it, its end on the button's
    // end, with no head of its own; Escape gives the focus back to the button.
    const hung = await page.getByRole('dialog', { name: 'Create' }).evaluate(async (dialog) => {
      await Promise.all(dialog.getAnimations({ subtree: true }).map((each) => each.finished));
      return dialog.getBoundingClientRect().toJSON();
    });
    const from = await button.evaluate((each) => each.getBoundingClientRect().toJSON());
    assert.ok(hung.top >= from.bottom && hung.top - from.bottom < 24, `${path}: the panel's top ${hung.top} hangs under the button's foot ${from.bottom}`);
    assert.ok(Math.abs(hung.right - from.right) <= 2, `${path}: the panel's end ${hung.right} is the button's ${from.right}`);
    assert.equal(await page.getByRole('dialog', { name: 'Create' }).getByRole('heading', { name: 'Create', exact: true }).isVisible(), false, `${path}: a head`);
    await page.keyboard.press('Escape');
    await lineOf(page).waitFor({ state: 'hidden', timeout: 5_000 });
    assert.equal(await page.evaluate(() => document.activeElement?.closest('header, [role="banner"]') !== null && document.activeElement?.textContent?.trim()), 'Create', `${path}: focus back`);
  }

  const phone = await as(browser, { width: 375 });
  t.after(() => phone.context.close());
  await phone.page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await phone.page.getByText(ITEMS[2].title).first().waitFor();
  await phone.page.getByRole('navigation', { name: 'Sections' }).getByRole('button', { name: 'Capture' }).tap();
  await lineOf(phone.page).waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
  assert.deepEqual(phone.failures, []);
});

test('chromium: the kinds the place allows, the kinds made last, and none whose module is off (check 2)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  const { context, page, failures } = await as(browser);
  t.after(() => context.close());
  await land(page, `/collections/${COLLECTION.id}`);
  await summon(page);
  assert.deepEqual(await kindsOf(page), ['entry', 'page', 'whiteboard', 'goal', 'dashboard', 'asset', 'time', 'collection']);
  assert.equal(await createOf(page).getByRole('heading', { name: 'Here in House' }).count(), 1, 'the kinds are not named after the hub');
  assert.equal(await createOf(page).locator('section[aria-labelledby="create-last"]').count(), 0, 'nothing was made yet, yet something is "last"');

  // Something made is offered first next time, on this device.
  await createOf(page).locator('[data-kind="goal"]').first().click();
  await page.waitForURL(new RegExp(`/hubs/${HUB.id}/goals`));
  await land(page, `/collections/${COLLECTION.id}`);
  await summon(page);
  const last = createOf(page).locator('section[aria-labelledby="create-last"]');
  assert.deepEqual(await last.locator('[data-kind]').evaluateAll((buttons) => buttons.map((each) => each.getAttribute('data-kind'))), ['goal']);
  assert.deepEqual(failures, []);

  // With no product on, only the entry and the collection remain: absent, not grey.
  const bare = await as(browser, { modulesOn: [] });
  t.after(() => bare.context.close());
  await land(bare.page, `/collections/${COLLECTION.id}`);
  await bare.page.keyboard.press('c');
  await lineOf(bare.page).waitFor({ timeout: 5_000 });
  await createOf(bare.page).locator('[data-kind="collection"]').first().waitFor({ timeout: 5_000 });
  assert.deepEqual(await kindsOf(bare.page), ['entry', 'collection']);
  assert.equal(await createOf(bare.page).locator('[data-kind][disabled]').count(), 0, 'a kind is drawn greyed');
  assert.deepEqual(bare.failures, []);

  // A member may write in House but not shape it: no collection. The departments one asks stand below.
  const member = await as(browser, { hubRole: 'MEMBER', portals: [{ collection_id: COLLECTION.id, name: 'IT help' }] });
  t.after(() => member.context.close());
  await land(member.page, `/collections/${COLLECTION.id}`);
  await summon(member.page);
  assert.deepEqual(await kindsOf(member.page), ['entry', 'page', 'whiteboard', 'goal', 'dashboard', 'asset', 'time']);
  const ask = createOf(member.page).locator('section[aria-labelledby="create-ask"]');
  assert.match(await ask.innerText(), /Request to a department[\s\S]*IT help/iu);
  assert.deepEqual(member.failures, []);
});

test('chromium: the place is preset from where one stands, a chip one changes, and the kinds follow it (check 3)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await as(browser);
  t.after(() => context.close());

  // Standing in Kitchen: the chip names it.
  await land(page, `/collections/${COLLECTION.id}`);
  await summon(page);
  const place = createOf(page).getByRole('combobox', { name: 'Where it is made' });
  assert.equal(await place.inputValue(), `c:${COLLECTION.id}`);
  assert.match(await createOf(page).locator('label.place').innerText(), /in Kitchen/u);
  await page.keyboard.press('Escape');

  // Standing nowhere and having made nothing: no hub, so only what needs none.
  const fresh = await as(browser);
  t.after(() => fresh.context.close());
  await land(fresh.page, '/profile');
  await fresh.page.keyboard.press('c');
  await lineOf(fresh.page).waitFor({ timeout: 5_000 });
  assert.deepEqual(await kindsOf(fresh.page), ['entry', 'time']);
  // Choosing House in the chip brings the hub's kinds.
  await createOf(fresh.page).getByRole('combobox', { name: 'Where it is made' }).selectOption(`h:${HUB.id}`);
  await createOf(fresh.page).locator('[data-kind="page"]').first().waitFor({ timeout: 5_000 });
  assert.deepEqual(await kindsOf(fresh.page), ['entry', 'page', 'whiteboard', 'goal', 'dashboard', 'asset', 'time', 'collection']);
  assert.match(await createOf(fresh.page).locator('label.place').innerText(), /in House/u);
  assert.deepEqual(failures, []);
  assert.deepEqual(fresh.failures, []);
});

test('chromium: a page, a whiteboard and a dashboard are made at once; the others open their own place (checks 4, 5)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, writes } = await as(browser);
  t.after(() => context.close());

  // A kind's letter makes it while the menu is open (check 5), and it is made at once (check 4).
  for (const [letter, call, path, title] of [
    ['s', `POST /hubs/${HUB.id}/pages`, `/pages/${MADE}`, 'Untitled page'],
    ['w', `POST /hubs/${HUB.id}/whiteboards`, `/whiteboards/${MADE}`, 'Untitled whiteboard'],
    ['d', 'POST /dashboards', `/dashboards/${MADE}`, 'Untitled dashboard'],
  ]) {
    await land(page, `/collections/${COLLECTION.id}`);
    await summon(page);
    await page.keyboard.press(letter);
    await page.waitForURL((url) => url.pathname === path, { timeout: 5_000 });
    const made = writes.find((each) => each.call === call);
    assert.ok(made, `${letter}: ${call} was not sent`);
    assert.equal(made.body.title ?? made.body.name, title);
  }

  // The others open their own place to be made in.
  await land(page, `/collections/${COLLECTION.id}`);
  await summon(page);
  await page.keyboard.press('k');
  await page.waitForURL((url) => url.pathname === `/hubs/${HUB.id}/inventory` && url.searchParams.get('new') === '1', { timeout: 5_000 });

  await land(page, `/collections/${COLLECTION.id}`);
  await summon(page);
  await createOf(page).locator('[data-kind="collection"]').first().click();
  await page.getByRole('dialog').filter({ hasText: /collection/iu }).first().waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');

  await land(page, `/collections/${COLLECTION.id}`);
  await summon(page);
  await page.keyboard.press('e');
  await captureOf(page).waitFor({ timeout: 5_000 });
  assert.match(await captureOf(page).locator('[data-chip="place"]').innerText(), /Kitchen/u, 'the entry is not made where one stands');
});

test('chromium: 375 px — holding the ＋ makes the kind made last (check 5)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await as(browser, { width: 375 });
  t.after(() => context.close());

  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.getByText(ITEMS[2].title).first().waitFor();
  const plus = page.getByRole('navigation', { name: 'Sections' }).getByRole('button', { name: 'Capture' });
  await plus.tap();
  await lineOf(page).waitFor({ timeout: 5_000 });
  await page.locator('dialog[open] .tilebutton[data-kind="entry"]').tap();
  await captureOf(page).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');
  await captureOf(page).waitFor({ state: 'hidden', timeout: 5_000 });

  // The context menu's way of holding (a long press, a right click, Shift F10): capture at once.
  assert.equal(await plus.getAttribute('aria-keyshortcuts'), 'Shift+F10');
  await plus.click({ button: 'right' });
  await captureOf(page).waitFor({ timeout: 5_000 });
  assert.match(await captureOf(page).locator('[data-chip="place"]').innerText(), /Kitchen/u);
  assert.deepEqual(failures, []);
});

test('chromium: from a template and an import stand at the foot and open capture in that mode (check 6)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await as(browser);
  t.after(() => context.close());

  for (const [foot, mode] of [['From template …', 'From a template'], ['Import …', 'Import a file']]) {
    await land(page, `/collections/${COLLECTION.id}`);
    await summon(page);
    await createOf(page).locator('footer').getByRole('button', { name: foot }).click();
    await captureOf(page).waitFor({ timeout: 5_000 });
    const tab = captureOf(page).getByRole('tablist', { name: 'How to add' }).getByRole('tab', { name: mode });
    assert.equal(await tab.getAttribute('aria-selected'), 'true', `${foot} opened capture in another mode`);
    await page.keyboard.press('Escape');
    await captureOf(page).waitFor({ state: 'hidden', timeout: 5_000 });
  }
  assert.deepEqual(failures, []);
});
