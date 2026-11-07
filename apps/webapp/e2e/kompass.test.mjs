// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The navigation concept's sidebar (UC-WRK-36 checks 1-8 and 17, UC-WRK-37 check 5): the fixed top
// and foot, the workspace level, a hub's level with its tools, queues, collections and one content
// tree, the quick switch, the width, the rail and its peek. Walked at desk width; `SHOTS=<dir>`
// also writes the screenshots the mockup is compared with.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { chromium } from 'playwright';

import {
  id, ACCOUNT, hub, IT, PD, HOME, collection, WEB, sprint, GOALS, VPN, served, open, ALL, side, texts, heads,
} from './kompassstub.mjs';

const SHOTS = process.env.SHOTS;

test.after(() => served.close());

test('UC-WRK-36 check 1: the workspace level from the top, the foot pinned', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { modulesOn: ALL, hubModules: { [HOME.id]: [] }, stars: [{ kind: 'HUB', id: IT.id }] });
  t.after(close);
  await side(page).getByRole('treeitem', { name: /IT operations/ }).first().waitFor({ timeout: 10_000 });
  assert.deepEqual(await texts(side(page).getByRole('list', { name: 'For you' }).getByRole('link')), ['Today', 'Inbox', 'Waiting for your yes']);
  assert.deepEqual(await texts(side(page).getByRole('list', { name: 'What is kept' }).getByRole('link')), ['Administration', 'Archive', 'Trash']);
  // Stars, products and hubs are trees whose rows are dragged into order (UC-WRK-38 check 7).
  await side(page).locator('section').first().getByRole('treeitem', { name: 'IT operations', exact: true }).waitFor({ timeout: 10_000 });
  const order = await heads(page);
  assert.deepEqual(order.map((each) => each.toLowerCase()), ['starred', 'products', 'hubs'], JSON.stringify(order));
  const productRows = await texts(side(page).locator('section').nth(1).getByRole('treeitem'));
  assert.deepEqual(productRows, ['Plans', 'Pages', 'Whiteboards', 'Service desk', 'Assets', 'Time', 'Goals', 'Agents']);
  // 14 hubs: 12 shown, then *n more*.
  assert.equal(await side(page).locator('section').nth(2).getByRole('treeitem').count(), 12);
  await side(page).getByText('2 more hubs').waitFor();
  assert.equal(await side(page).getByRole('link', { name: 'Today' }).getAttribute('aria-current'), 'page');
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 1: no star, no Starred; no administration without the right; no product while none is on', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { modulesOn: [], administration: false });
  t.after(close);
  await side(page).getByRole('treeitem', { name: /IT operations/ }).first().waitFor({ timeout: 10_000 });
  assert.deepEqual((await heads(page)).map((each) => each.toLowerCase()), ['hubs']);
  assert.deepEqual(await texts(side(page).getByRole('list', { name: 'What is kept' }).getByRole('link')), ['Archive', 'Trash']);
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 checks 2-6: a hub\'s level - head, tools in order, queues, collections, one content tree', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  // Planning is off at the hub: no Plan; the desk collection brings Queues.
  const { page, failures, close } = await open(browser, {
    path: `/hubs/${IT.id}`,
    modulesOn: ALL,
    hubModules: { [IT.id]: ['WORKFLOWS', 'SERVICE_DESK', 'ASSETS', 'GOALS', 'KNOWLEDGE', 'WHITEBOARD'] },
    stars: [{ kind: 'VIEW', id: id(501) }],
  });
  t.after(close);
  const level = side(page).getByRole('region', { name: 'IT operations' });
  await level.getByRole('link', { name: 'Queues' }).waitFor({ timeout: 10_000 });
  await level.getByRole('button', { name: 'All hubs' }).waitFor();
  await level.getByRole('heading', { name: 'IT operations' }).waitFor();
  await level.getByRole('button', { name: 'Star IT operations' }).waitFor();
  await level.getByRole('button', { name: 'Switch hub' }).waitFor();
  const tools = await texts(level.locator('ul.tools > li > a'));
  // Check 3: the fixed order, Overview first; Mine is starred and leads the queues (check 4).
  assert.deepEqual(tools, ['Overview', 'Queues', 'Mine', 'About to breach', 'Unassigned', 'Inventory', 'Goals'], JSON.stringify(tools));
  assert.equal(await level.getByRole('link', { name: 'Overview' }).getAttribute('aria-current'), 'page');
  // Check 4: a breaching queue's count is on the danger surface and says so in words.
  const breach = level.locator('ul.tools > li', { hasText: 'About to breach' });
  assert.ok((await breach.innerText()).includes('1'));
  assert.ok(await breach.locator('[data-urgent], .urgent, [aria-label*="about to breach"], [title*="about to breach"]').count() >= 1, 'the urgent count has no words');
  // Parts in order: Collections, then Contents with its count.
  await level.getByRole('tree', { name: 'Contents of IT operations' }).waitFor();
  assert.deepEqual((await heads(page)).map((each) => each.toLowerCase().replace(/\s+/g, ' ')), ['collections', 'contents · 4']);
  // Check 6: the desk carries its mark.
  const desk = level.getByRole('treeitem', { name: /Service desk/ });
  assert.ok((await desk.innerHTML()).includes('Desk'), 'the desk mark is missing');
  // Check 5: pages and the board in one tree, the template left out, the board without a twist.
  const contents = level.getByRole('tree', { name: 'Contents of IT operations' });
  assert.deepEqual(await texts(contents.getByRole('treeitem')), ['Knowledge base', 'Runbooks', 'Network map']);
  assert.equal(await contents.getByRole('treeitem', { name: 'Network map' }).getAttribute('aria-expanded'), null);
  // Check 2: *‹ All hubs* returns to the workspace level; the page stays.
  await level.getByRole('button', { name: 'All hubs' }).click();
  await side(page).getByRole('tree', { name: 'Hubs' }).waitFor({ timeout: 5_000 });
  assert.equal(new URL(page.url()).pathname, `/hubs/${IT.id}`);
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 2: a collection, a page and a tool show their hub\'s level; focus lands on All hubs', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { path: '/', modulesOn: ALL, hubModules: { [PD.id]: ['PLANNING', 'TIME', 'GOALS'] } });
  t.after(close);
  await side(page).getByRole('treeitem', { name: /Product development/ }).click();
  const level = side(page).getByRole('region', { name: 'Product development' });
  await level.getByRole('link', { name: 'Plan' }).waitFor({ timeout: 10_000 });
  assert.deepEqual(await texts(level.locator('ul.tools > li > a')), ['Overview', 'Plan', 'Time', 'Goals']);
  await page.waitForFunction(() => document.activeElement?.textContent?.includes('All hubs'), undefined, { timeout: 5_000 });
  await level.getByRole('treeitem', { name: /Web app/ }).click();
  await page.waitForFunction((at) => location.pathname === `/collections/${at}`, WEB.id, { timeout: 5_000 });
  assert.equal(await level.getByRole('treeitem', { name: /Web app/ }).getAttribute('aria-current'), 'page');
  // The sprint mark after a collection that runs sprints (check 6).
  assert.ok((await level.getByRole('treeitem', { name: /Web app/ }).innerHTML()).includes('Runs sprints'));
  await level.getByRole('link', { name: 'Time' }).click();
  await page.waitForFunction((at) => location.pathname === `/hubs/${at}/time`, PD.id, { timeout: 5_000 });
  assert.equal(await side(page).getByRole('link', { name: 'Time' }).getAttribute('aria-current'), 'page');
  // No hub tool is also a tab on the screen (check 3): the screen's bar holds the tool's own views.
  const bar = page.getByRole('main').getByRole('navigation', { name: 'Product development' });
  await bar.waitFor({ timeout: 5_000 });
  assert.deepEqual(await texts(bar.getByRole('link')), ['Actuals', 'Workload', 'Budget']);
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 7: the quick switch keeps the tool where the other hub has it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, {
    path: `/hubs/${PD.id}/plan`, modulesOn: ALL,
    hubModules: { [PD.id]: ['PLANNING'], [IT.id]: ['PLANNING'], [HOME.id]: [] },
    stars: [{ kind: 'HUB', id: IT.id }],
  });
  t.after(close);
  const level = side(page).getByRole('region', { name: 'Product development' });
  await level.getByRole('button', { name: 'Switch hub' }).click({ timeout: 10_000 });
  const flyout = page.getByRole('dialog', { name: 'Switch hub' });
  await flyout.getByRole('link', { name: 'IT operations' }).click();
  await page.waitForFunction((at) => location.pathname === `/hubs/${at}/plan`, IT.id, { timeout: 5_000 });
  await side(page).getByRole('region', { name: 'IT operations' }).getByRole('button', { name: 'Switch hub' }).click();
  await page.getByRole('dialog', { name: 'Switch hub' }).getByRole('searchbox').fill('house');
  await page.getByRole('dialog', { name: 'Switch hub' }).getByRole('link', { name: 'Household' }).click();
  await page.waitForFunction((at) => location.pathname === `/hubs/${at}`, HOME.id, { timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 17: a workspace with no module on shows no tool and no Contents in a hub (D1)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { path: `/hubs/${HOME.id}`, modulesOn: [] });
  t.after(close);
  const level = side(page).getByRole('region', { name: 'Household' });
  await level.getByRole('treeitem', { name: /To do/ }).waitFor({ timeout: 10_000 });
  assert.deepEqual(await texts(level.locator('ul.tools > li > a')), ['Overview']);
  assert.deepEqual((await heads(page)).map((each) => each.toLowerCase()), ['collections']);
  if (SHOTS) await side(page).screenshot({ path: `${SHOTS}/live-hub-household.png` });
  assert.deepEqual(failures, []);
});

test('UC-WRK-37 check 5: ☆ in the hub\'s head stars it, and it stands under Starred', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close } = await open(browser, { path: `/hubs/${PD.id}`, modulesOn: [] });
  t.after(close);
  const star = side(page).getByRole('button', { name: 'Star Product development' });
  await star.click({ timeout: 10_000 });
  await page.waitForFunction(() => true);
  await side(page).getByRole('button', { name: 'Remove the star from Product development' }).waitFor({ timeout: 5_000 });
  assert.deepEqual(written.stars.at(-1), { stars: [{ kind: 'HUB', id: PD.id }] });
  await side(page).getByRole('button', { name: 'All hubs' }).click();
  await side(page).locator('section', { hasText: 'Starred' }).getByRole('treeitem', { name: 'Product development', exact: true }).waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('UC-WRK-37 check 5: S stars the place on screen and unstars it again; in a field it types', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close } = await open(browser, { path: `/collections/${WEB.id}`, modulesOn: ALL });
  t.after(close);
  const head = page.locator('main').getByRole('button', { name: 'Star Web app' });
  await head.waitFor({ timeout: 10_000 });
  await page.locator('main h1').click();
  await page.keyboard.press('s');
  await page.locator('main').getByRole('button', { name: 'Remove the star from Web app' }).waitFor({ timeout: 5_000 });
  assert.deepEqual(written.stars.at(-1), { stars: [{ kind: 'COLLECTION', id: WEB.id }] });
  await page.keyboard.press('s');
  await head.waitFor({ timeout: 5_000 });
  assert.deepEqual(written.stars.at(-1), { stars: [] });
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 5: the contents mode is the person\'s, written to their preferences', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close } = await open(browser, { path: `/hubs/${IT.id}`, modulesOn: ALL, hubModules: { [IT.id]: ['KNOWLEDGE', 'WHITEBOARD'] } });
  t.after(close);
  const contents = side(page).getByRole('tree', { name: 'Contents of IT operations' });
  await contents.waitFor({ timeout: 10_000 });
  await side(page).getByRole('button', { name: 'Contents', exact: true }).click();
  await page.getByRole('menuitem', { name: 'A–Z' }).click();
  await page.waitForFunction(() => true);
  await contents.getByRole('treeitem', { name: 'Set up the VPN' }).waitFor({ timeout: 5_000 });
  assert.deepEqual(written.preferences.at(-1), { nav_contents_mode: 'NAME' });
  assert.deepEqual(await texts(contents.getByRole('treeitem')), ['Knowledge base', 'Network map', 'Runbooks', 'Set up the VPN']);
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 8: the width is dragged and kept, Ctrl+[ folds to the rail, hovering it peeks', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { modulesOn: ['PLANNING'], storage: { 'hubtask.nav.width': '320' } });
  t.after(close);
  await side(page).getByRole('treeitem', { name: /IT operations/ }).first().waitFor({ timeout: 10_000 });
  assert.equal(Math.round((await side(page).boundingBox()).width), 320, 'the kept width is not used');
  const handle = side(page).getByRole('separator', { name: 'Width of the navigation' });
  await handle.dblclick();
  assert.equal(Math.round((await side(page).boundingBox()).width), 264);
  const box = await handle.boundingBox();
  await page.mouse.move(box.x + box.width / 2, box.y + 40);
  await page.mouse.down();
  await page.mouse.move(box.x + 600, box.y + 40, { steps: 5 });
  await page.mouse.up();
  assert.equal(Math.round((await side(page).boundingBox()).width), 400, 'the drag is not bounded at 400');
  assert.equal(await page.evaluate(() => localStorage.getItem('hubtask.nav.width')), '400');
  await page.locator('main').click({ position: { x: 600, y: 400 } });
  await page.keyboard.press('Control+BracketLeft');
  await page.waitForFunction(() => document.querySelector('aside.sidenav')?.hasAttribute('data-rail'), undefined, { timeout: 5_000 });
  assert.ok((await side(page).boundingBox()).width < 80);
  await page.mouse.move(20, 300);
  await page.waitForFunction(() => document.querySelector('aside.sidenav')?.hasAttribute('data-peek'), undefined, { timeout: 5_000 });
  await side(page).getByRole('tree', { name: 'Hubs' }).waitFor({ timeout: 5_000 });
  await page.mouse.move(900, 400);
  await page.waitForFunction(() => !document.querySelector('aside.sidenav')?.hasAttribute('data-peek'), undefined, { timeout: 5_000 });
  assert.deepEqual(failures, []);
});

/** The figure at a row's end: its last line, where the name is the first. */
const figureOf = async (row) => (await row.innerText()).split('\n').map((each) => each.trim()).filter(Boolean).slice(1).join(' ');

test('UC-WRK-36 checks 1-6: the rows carry their figures, the hub head its ⋯, the hub its contents', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const recent = JSON.stringify(Array.from({ length: 20 }, (_, i) => ({ kind: 'collection', id: id(1000 + i), title: `Place ${i + 1}`, at: Date.now() - i * 60_000, placeId: PD.id })));
  const { page, failures, close } = await open(browser, {
    height: 1300,
    path: `/hubs/${PD.id}`,
    modulesOn: ALL,
    hubModules: { [PD.id]: ['PLANNING', 'TIME', 'GOALS', 'KNOWLEDGE', 'WHITEBOARD'], [IT.id]: ['WORKFLOWS', 'SERVICE_DESK', 'ASSETS', 'GOALS', 'KNOWLEDGE', 'WHITEBOARD'] },
    stars: [{ kind: 'COLLECTION', id: WEB.id }, { kind: 'HUB', id: IT.id }],
    storage: { [`hubtask.recent.${ACCOUNT.id}`]: recent },
  });
  t.after(close);
  const top = side(page).getByRole('list', { name: 'For you' });
  const level = side(page).getByRole('region', { name: 'Product development' });
  await level.getByRole('tree', { name: 'Contents of Product development' }).waitFor({ timeout: 10_000 });
  // For you: what is due by tonight, the undecided arrivals, what waits for the reader's yes (§2.2).
  await page.waitForFunction(() => /Today\s*5/.test(document.querySelector('aside.sidenav ul.foryou')?.textContent ?? ''), undefined, { timeout: 5_000 });
  assert.equal(await figureOf(top.getByRole('listitem').filter({ hasText: 'Today' })), '5');
  assert.equal(await figureOf(top.getByRole('listitem').filter({ hasText: 'Inbox' })), '3');
  assert.equal(await figureOf(top.getByRole('listitem').filter({ hasText: 'Waiting for your yes' })), '2');
  // The hub head: the name, ▾, ☆ and ⋯ - the ⋯ there for every reader, its link first.
  const more = level.getByRole('button', { name: 'Manage Product development' });
  await more.waitFor();
  // The tools' figures: the running sprint, the goals at risk (§2.4 item 2).
  const tool = (name) => level.locator('ul.tools > li', { hasText: name });
  await page.waitForFunction(() => document.querySelector('aside.sidenav ul.tools')?.textContent?.includes('at risk'), undefined, { timeout: 5_000 });
  assert.equal(await figureOf(tool('Plan')), 'Sprint 42 · 6 d');
  assert.equal(await figureOf(tool('Goals')), '1 at risk');
  // The collections' figures (§2.4 part 3): the sprint's days left, open, due.
  const row = (name) => level.getByRole('treeitem', { name: new RegExp(name) });
  await page.waitForFunction(() => document.querySelector('aside.sidenav [role="tree"]')?.textContent?.includes('9 d'), undefined, { timeout: 5_000 });
  assert.deepEqual(await Promise.all(['Web app', 'Mobile', 'Design', 'Bugs'].map((name) => figureOf(row(name)))), ['6 d', '9 d', '14 open', '3 due']);
  // Contents: the hub's pages and boards in one tree, with its count.
  assert.ok((await heads(page)).some((each) => /^contents · 11$/i.test(each.replace(/\s+/g, ' '))), JSON.stringify(await heads(page)));
  if (SHOTS) await side(page).screenshot({ path: `${SHOTS}/live-hub-pd.png` });
  await more.click();
  await page.getByRole('menuitem', { name: 'Copy link' }).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');

  // IT operations: a desk's requests about to breach, the inventory's attention.
  await page.goto(`${served.origin}/hubs/${IT.id}`);
  const it = side(page).getByRole('region', { name: 'IT operations' });
  await it.getByRole('link', { name: 'Queues' }).waitFor({ timeout: 10_000 });
  await page.waitForFunction(() => document.querySelector('aside.sidenav [role="tree"]')?.textContent?.includes('SLA'), undefined, { timeout: 5_000 });
  assert.equal(await figureOf(it.getByRole('treeitem', { name: /Service desk/ })), '1 SLA');
  assert.equal(await figureOf(it.locator('ul.tools > li', { hasText: 'Inventory' })), '6 !');
  if (SHOTS) await side(page).screenshot({ path: `${SHOTS}/live-hub-it.png` });

  // The workspace level: Recent opens the last twenty places (§2.3).
  await page.goto(`${served.origin}/`);
  await side(page).getByRole('treeitem', { name: /IT operations/ }).first().waitFor({ timeout: 10_000 });
  if (SHOTS) await side(page).screenshot({ path: `${SHOTS}/live-today.png` });
  await side(page).getByRole('button', { name: 'Recent' }).click();
  assert.equal(await page.getByRole('dialog', { name: 'Recent' }).getByRole('link').count(), 20);
  assert.deepEqual(failures, []);
});
