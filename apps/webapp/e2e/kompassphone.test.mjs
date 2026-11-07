// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The navigation concept on a phone and a tablet (§9): the bottom bar, the Browse stack
// (workspace → hub → queues, contents), each tab's own stack, the bar's *‹ the level above*, ☆ and
// ⋯, a collection's view as a chip and its sheet, *New* on *Contents*, the announcement of every
// stack change, 44-pixel targets, a heading and *Close* on every sheet, the drawer between 600 and
// 904 pixels, a module off at a hub, and no sideways scroll at 390 (UC-WRK-36 checks 13-16,
// UC-INS-21 check 11).

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { chromium } from 'playwright';

import { ACCOUNT, ALL, IT, KB, PD, VPN, WEB, id, open, served, texts } from './kompassstub.mjs';

test.after(() => served.close());

const ON = {
  [PD.id]: ['WORKFLOWS', 'PLANNING', 'TIME', 'GOALS', 'KNOWLEDGE', 'WHITEBOARD'],
  [IT.id]: ['WORKFLOWS', 'SERVICE_DESK', 'ASSETS', 'GOALS', 'KNOWLEDGE', 'WHITEBOARD'],
};
const phone = (browser, path, options = {}) =>
  open(browser, { path, modulesOn: ALL, hubModules: ON, owner: true, width: 390, height: 844, hasTouch: true, ready: (page) => page.locator('[data-phone-bar]'), ...options });
const bar = (page) => page.locator('[data-phone-bar]');
const tabs = (page) => page.getByRole('navigation', { name: 'Sections' });
const step = (page) => page.locator('[data-browse-step]');
/** Section heads by their words, not by how they are set (they are drawn in capitals). */
const heads = (locator) => locator.evaluateAll((nodes) => nodes.map((node) => node.textContent.trim()));
const at = (page, path) => page.waitForFunction((want) => location.pathname + location.search === want, path, { timeout: 10_000 });

test('UC-WRK-36 check 13: Today · Browse · ＋ · Inbox · Search; Browse holds the levels as steps, each tab keeps its own stack', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await phone(browser, '/');
  t.after(close);
  assert.deepEqual(await texts(tabs(page).locator('.word')), ['Today', 'Browse', 'Capture', 'Inbox', 'Search']);
  assert.equal(await page.getByRole('button', { name: 'Open the navigation' }).count(), 0, 'a drawer beside the stack');

  await tabs(page).getByRole('link', { name: 'Browse' }).click();
  await at(page, '/browse');
  await step(page).getByRole('heading', { name: 'Hubs' }).waitFor();
  assert.ok((await heads(step(page).locator('h3'))).includes('Products'), 'the workspace level has no products');
  await step(page).getByRole('link', { name: IT.name }).click();
  await at(page, `/browse/hubs/${IT.id}`);
  // The hub's three parts: its tools, its collections, its contents.
  await step(page).getByRole('list', { name: `Tools of ${IT.name}` }).waitFor();
  assert.deepEqual(await texts(step(page).getByRole('list', { name: `Tools of ${IT.name}` }).getByRole('link')), ['Overview', 'Queues', 'Inventory', 'Goals']);
  assert.deepEqual(await heads(step(page).locator('h3')), ['Collections', 'Contents']);
  await step(page).getByRole('link', { name: /^Queues/ }).click();
  await at(page, `/browse/hubs/${IT.id}/queues`);
  await step(page).getByRole('link', { name: /^About to breach/ }).waitFor();
  assert.deepEqual((await texts(step(page).getByRole('link'))).slice(0, 3), ['About to breach', 'Mine', 'Unassigned'], 'the starred queue is not first');

  // Another tab and back: Browse is where it was left; Browse again: its root.
  await tabs(page).getByRole('link', { name: 'Today' }).click();
  await at(page, '/');
  await tabs(page).getByRole('link', { name: 'Browse' }).click();
  await at(page, `/browse/hubs/${IT.id}/queues`);
  await tabs(page).getByRole('link', { name: 'Browse' }).click();
  await at(page, '/browse');
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 14: ‹ the level above, the title, ☆ and ⋯; a content row opens by its name and descends by its ›; the view is a chip with the same views', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await phone(browser, `/browse/hubs/${IT.id}`);
  t.after(close);
  await bar(page).getByRole('button', { name: `Star ${IT.name}` }).waitFor({ timeout: 10_000 });
  assert.equal(await bar(page).locator('[data-back]').innerText(), 'Hubs');
  assert.equal(await bar(page).locator('[data-bar="title"]').innerText(), IT.name);
  assert.equal(await bar(page).getByRole('button', { name: `Manage ${IT.name}` }).count(), 1, 'the hub has no ⋯');
  const contents = step(page).getByRole('region', { name: 'Contents' });
  assert.equal(await contents.getByRole('link', { name: KB.title }).getAttribute('href'), `/pages/${KB.id}`, 'the name does not open the page');
  await contents.getByRole('button', { name: `Open what is under ${KB.title}` }).click();
  await at(page, `/browse/hubs/${IT.id}/contents?under=${KB.id}`);
  await step(page).getByRole('link', { name: VPN.title }).waitFor();
  assert.equal(await bar(page).locator('[data-back]').innerText(), 'Contents');
  await bar(page).locator('[data-back]').click();
  await at(page, `/browse/hubs/${IT.id}/contents`);

  // A collection: ‹ its hub, its ☆ and ⋯, the view as a chip whose sheet lists the bar's views.
  await page.goto(`${served.origin}/collections/${WEB.id}`);
  await bar(page).getByRole('button', { name: `Star ${WEB.name}` }).waitFor({ timeout: 10_000 });
  assert.equal(await bar(page).locator('[data-back]').innerText(), PD.name);
  assert.equal(await bar(page).locator('[data-bar="title"]').innerText(), WEB.name);
  await page.getByRole('main').getByRole('button', { name: 'List' }).click();
  const sheet = page.getByRole('dialog', { name: 'View' });
  await sheet.waitFor();
  const phoneViews = await texts(sheet.getByRole('link'));
  const desk = await open(browser, { path: `/collections/${WEB.id}`, modulesOn: ALL, hubModules: ON, owner: true });
  t.after(desk.close);
  const deskBar = desk.page.getByRole('main').getByRole('navigation', { name: WEB.name });
  await deskBar.getByRole('link', { name: 'Board' }).waitFor({ timeout: 10_000 });
  assert.deepEqual(phoneViews, await texts(deskBar.getByRole('link')), 'the sheet holds other views than the bar');
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 15: every stack change is announced, every target is 44 pixels, every sheet has a heading and Close', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await phone(browser, '/browse');
  t.after(close);
  await step(page).getByRole('link', { name: IT.name }).click();
  await page.waitForFunction((want) => document.querySelector('.announcement')?.textContent?.trim() === want, `${IT.name}, level 2`, { timeout: 10_000 });
  await step(page).getByRole('link', { name: /^Queues/ }).click();
  await page.waitForFunction(() => document.querySelector('.announcement')?.textContent?.trim() === 'Queues, level 3', null, { timeout: 10_000 });

  const small = async () => page.evaluate(() => {
    const seen = [...document.querySelectorAll('[data-phone-bar] a, [data-phone-bar] button, nav[aria-label="Sections"] a, nav[aria-label="Sections"] button, [data-browse-step] a, [data-browse-step] button, dialog[open] a, dialog[open] button')];
    return seen
      .filter((each) => each.getClientRects().length > 0)
      .map((each) => ({ name: each.getAttribute('aria-label') ?? each.textContent?.trim(), box: each.getBoundingClientRect() }))
      .filter(({ box }) => box.height < 44 || box.width < 44)
      .map(({ name, box }) => `${name}: ${Math.round(box.width)}×${Math.round(box.height)}`);
  });
  for (const path of ['/browse', `/browse/hubs/${IT.id}`, `/browse/hubs/${IT.id}/queues`, `/browse/hubs/${IT.id}/contents`]) {
    await page.goto(`${served.origin}${path}`);
    await step(page).locator('a').first().waitFor({ timeout: 10_000 });
    assert.deepEqual(await small(), [], `${path}: targets under 44 pixels`);
  }

  // *New* on *Contents*, the account, a collection's views: each sheet with its heading and Close.
  await page.getByRole('button', { name: 'New', exact: true }).click();
  const made = page.getByRole('dialog', { name: `New in “${IT.name}”` });
  await made.getByRole('heading', { name: `New in “${IT.name}”` }).waitFor();
  assert.deepEqual(await texts(made.getByRole('listitem')), ['Page', 'Page from a template…', 'Whiteboard', 'Collection']);
  assert.deepEqual(await small(), [], 'the New sheet: targets under 44 pixels');
  await made.getByRole('button', { name: 'Close' }).click();
  await made.waitFor({ state: 'hidden' });
  await page.goto(`${served.origin}/`);
  await bar(page).getByRole('button', { name: ACCOUNT.display_name }).click();
  const you = page.getByRole('dialog', { name: 'You' });
  await you.getByRole('heading', { name: 'You' }).waitFor();
  assert.equal(await you.getByRole('button', { name: 'Close' }).count(), 1);
  assert.deepEqual(await small(), [], 'the account sheet: targets under 44 pixels');
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 16: between 600 and 904 pixels the sidebar is a drawer with both levels, and no bottom bar', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { path: '/', modulesOn: ALL, hubModules: ON, owner: true, width: 768, height: 1024 });
  t.after(close);
  assert.equal(await tabs(page).count(), 0, 'a bottom bar on a tablet');
  await page.getByRole('button', { name: 'Open the navigation' }).click();
  const drawer = page.getByRole('dialog');
  await drawer.getByRole('tree', { name: 'Hubs' }).waitFor();
  assert.ok((await heads(drawer.locator('section h3'))).includes('Products'));
  await drawer.getByRole('treeitem', { name: IT.name }).click();
  await at(page, `/hubs/${IT.id}`);
  await page.getByRole('button', { name: 'Open the navigation' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'All hubs' }).waitFor();
  assert.equal(await page.getByRole('dialog').getByRole('link', { name: 'Overview' }).count(), 1, 'the drawer does not hold the hub level');
  // A step of the phone's stack opens the place the drawer draws.
  await page.goto(`${served.origin}/browse/hubs/${IT.id}`);
  await at(page, `/hubs/${IT.id}`);
  assert.deepEqual(failures, []);
});

test('UC-INS-21 check 11: on a phone a module off at a hub takes its tool and its part of the hub out of the stack', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await phone(browser, `/browse/hubs/${IT.id}`, { hubModules: { [IT.id]: ['WORKFLOWS', 'SERVICE_DESK'] } });
  t.after(close);
  const tools = step(page).getByRole('list', { name: `Tools of ${IT.name}` });
  await tools.getByRole('link', { name: /^Queues/ }).waitFor({ timeout: 10_000 });
  assert.deepEqual(await texts(tools.getByRole('link')), ['Overview', 'Queues']);
  assert.deepEqual(await heads(step(page).locator('h3')), ['Collections'], 'Contents stays with knowledge and whiteboards off');
  assert.deepEqual(failures, []);
});

test('no sideways scroll at 390 pixels on the main screens', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await phone(browser, '/');
  t.after(close);
  const screens = ['/', '/browse', `/browse/hubs/${IT.id}`, `/browse/hubs/${IT.id}/queues`, `/browse/hubs/${IT.id}/contents`, `/hubs/${IT.id}`, `/hubs/${PD.id}/plan`,
    `/collections/${WEB.id}`, `/hubs/${IT.id}/desk?queue=${id(500)}`, `/pages/${KB.id}`, '/jumble', '/plans'];
  for (const path of screens) {
    await page.goto(`${served.origin}${path}`);
    await bar(page).waitFor({ timeout: 10_000 });
    await page.waitForTimeout(1200);
    const over = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    assert.equal(over, 0, `${path} scrolls sideways by ${over} pixels`);
  }
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 14: an edge swipe goes one level up; one from the middle of the screen goes nowhere', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await phone(browser, `/browse/hubs/${IT.id}/queues`);
  t.after(close);
  await step(page).getByRole('link', { name: /^About to breach/ }).waitFor({ timeout: 10_000 });
  // A finger, through the browser's own touch input: the pointer events a phone sends.
  const touch = await page.context().newCDPSession(page);
  const swipe = async (fromX, toX, y = 500) => {
    await touch.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: fromX, y }] });
    for (let step = 1; step <= 8; step++) await touch.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: fromX + ((toX - fromX) * step) / 8, y }] });
    await touch.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  };
  await swipe(150, 340);
  await page.waitForTimeout(300);
  assert.equal(await page.evaluate(() => location.pathname), `/browse/hubs/${IT.id}/queues`, 'a swipe from the middle went up');
  await swipe(4, 220);
  await at(page, `/browse/hubs/${IT.id}`);
  await swipe(4, 220);
  await at(page, '/browse');
  assert.deepEqual(failures, []);
});
