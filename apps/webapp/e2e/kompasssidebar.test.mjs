// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The person's sidebar (UC-WRK-36 checks 19-21; the owner's review R-3, R-4, R-12): a product or a
// hub hidden from its ⋯ - or its long press on a phone - and shown again from *More*, written to
// the account; a product whose module is off nowhere; hubs moved while ordered by name; every
// section that folds remembered on the device, and none that does not draws a chevron.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { chromium } from 'playwright';

import { ALL, HOME, HUBS, IT, PD, open, served, side, texts } from './kompassstub.mjs';

test.after(() => served.close());

const tree = (page, name) => side(page).getByRole('tree', { name });
const rowsOf = (page, name) => texts(tree(page, name).locator('[role="treeitem"] .name'));
const more = (page, section) => side(page).locator('section', { has: page.getByRole('heading', { name: section }) }).locator('[data-hidden-rows]');
/** Opens a row's ⋯ and chooses an item. */
async function choose(page, row, item) {
  await row.hover();
  await row.getByRole('button', { name: /^Manage / }).click();
  await page.getByRole('menuitem', { name: item }).click();
}

test('UC-WRK-36 check 19: a product hidden from its ⋯ goes behind More, folded, and comes back from there; one switched off is nowhere', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close } = await open(browser, { modulesOn: ALL.filter((each) => each !== 'GOALS'), prefs: { nav_products_hidden: ['GOALS'] } });
  t.after(close);
  await tree(page, 'Products').getByRole('treeitem', { name: /^Time/ }).waitFor({ timeout: 10_000 });
  // Goals is hidden, but its module is off: neither a row nor behind More.
  assert.equal(await more(page, 'Products').count(), 0, 'More for a product whose module is off');
  await choose(page, tree(page, 'Products').getByRole('treeitem', { name: /^Time/ }), 'Hide from the sidebar');
  await page.waitForFunction(() => !document.querySelector('aside.sidenav [role="tree"][aria-label="Products"]')?.textContent?.includes('Time'));
  assert.deepEqual(written.preferences.at(-1), { nav_products_hidden: ['GOALS', 'TIME'] });
  assert.deepEqual(await rowsOf(page, 'Products'), ['Plans', 'Pages', 'Whiteboards', 'Service desk', 'Assets', 'Agents'], 'the others kept their places');
  // More: folded at first, its count; opened in place, the row with its tile and ⋯ -> Show.
  const hidden = more(page, 'Products');
  const toggle = hidden.getByRole('button', { name: /^More/ });
  assert.equal(await toggle.getAttribute('aria-expanded'), 'false');
  assert.ok((await toggle.innerText()).includes('1'));
  await toggle.click();
  assert.equal(await toggle.getAttribute('aria-expanded'), 'true');
  const time = hidden.locator('li', { hasText: 'Time' });
  assert.equal(await time.locator('.tile').count(), 1, 'the hidden row keeps its coloured tile');
  await choose(page, time, 'Show in the sidebar');
  await tree(page, 'Products').getByRole('treeitem', { name: /^Time/ }).waitFor();
  assert.deepEqual(written.preferences.at(-1), { nav_products_hidden: ['GOALS'] });
  assert.deepEqual(await rowsOf(page, 'Products'), ['Plans', 'Pages', 'Whiteboards', 'Service desk', 'Assets', 'Time', 'Agents'], 'shown again where it stood');
  // The section's ⋯ list of products is gone: hiding is the row's.
  assert.equal(await side(page).getByRole('button', { name: 'Customise the sidebar' }).count(), 0);
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 20: a hub hidden from its ⋯ goes behind More; Change position only on the workspace row', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close } = await open(browser, { modulesOn: ALL, owner: true });
  t.after(close);
  const hubs = tree(page, 'Hubs');
  await hubs.getByRole('treeitem', { name: /^Household/ }).waitFor({ timeout: 10_000 });
  await hubs.getByRole('treeitem', { name: /^Household/ }).hover();
  await hubs.getByRole('treeitem', { name: /^Household/ }).getByRole('button', { name: /^Manage / }).click();
  const items = await texts(page.getByRole('menuitem'));
  assert.equal(items[0], 'Hide from the sidebar');
  assert.ok(items.some((each) => each.startsWith('Change position')), JSON.stringify(items));
  assert.equal(items.at(-1), 'Move to the trash', 'the trash stays last');
  await page.getByRole('menuitem', { name: 'Hide from the sidebar' }).click();
  await page.waitForFunction(() => !document.querySelector('aside.sidenav [role="tree"][aria-label="Hubs"]')?.textContent?.includes('Household'));
  assert.deepEqual(written.preferences.at(-1), { nav_hubs_hidden: [HOME.id] });
  const hidden = more(page, 'Hubs');
  await hidden.getByRole('button', { name: /^More/ }).click();
  await choose(page, hidden.locator('li', { hasText: 'Household' }), 'Show in the sidebar');
  await hubs.getByRole('treeitem', { name: /^Household/ }).waitFor();
  assert.deepEqual(written.preferences.at(-1), { nav_hubs_hidden: [] });
  // Inside the hub its head's ⋯ has no Change position.
  await hubs.getByRole('treeitem', { name: /^Product development/ }).click();
  const head = side(page).getByRole('region', { name: PD.name }).getByRole('button', { name: `Manage ${PD.name}` });
  await head.click();
  await page.getByRole('menuitem', { name: 'Copy link' }).waitFor();
  assert.ok(!(await texts(page.getByRole('menuitem'))).some((each) => each.startsWith('Change position')));
  assert.deepEqual(failures, []);
});

test('UC-WRK-38 check 7: a hub dragged while the hubs read A-Z turns them to the shared order and lands after the hub above it; Alt+arrow turns it first', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close } = await open(browser, { modulesOn: ALL, owner: true, prefs: { nav_hubs_order: 'NAME' } });
  t.after(close);
  const hubs = tree(page, 'Hubs');
  const hub = (name) => hubs.getByRole('treeitem', { name: new RegExp(`^${name}`) });
  await hub('Data').waitFor({ timeout: 10_000 });
  assert.deepEqual((await rowsOf(page, 'Hubs')).slice(0, 3), ['Data', 'Design', 'Facilities']);
  // Alt+↓ while A-Z: the order turns, nothing moves yet.
  await hub('Design').focus();
  await page.keyboard.press('Alt+ArrowDown');
  await page.waitForFunction(() => document.body.innerText.includes('The hubs are in the shared order now'));
  assert.deepEqual(written.preferences.at(-1), { nav_hubs_order: null });
  assert.deepEqual((await rowsOf(page, 'Hubs')).slice(0, 3), HUBS.slice(0, 3).map((each) => each.name));
  // Back to A-Z, then drag Legal onto the line above IT operations, under Household: the shared
  // order, Legal right after Household.
  await side(page).getByRole('button', { name: 'Order hubs' }).click();
  await page.getByRole('menuitem', { name: 'A–Z' }).click();
  await page.waitForFunction(() => document.querySelector('aside.sidenav [role="tree"][aria-label="Hubs"] [role="treeitem"]')?.textContent?.includes('Data'));
  const from = await hub('Legal').boundingBox();
  const to = await hub('IT operations').boundingBox();
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
  await page.mouse.down();
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2 - 6, { steps: 3 });
  await page.mouse.move(to.x + to.width / 2, to.y + to.height * 0.1, { steps: 8 });
  await page.mouse.up();
  await page.waitForFunction(() => document.body.innerText.includes('Position'));
  assert.deepEqual(written.preferences.at(-1), { nav_hubs_order: null });
  const shared = await rowsOf(page, 'Hubs');
  assert.equal(shared[shared.indexOf('Household') + 1], 'Legal', JSON.stringify(shared));
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 21: Starred, Products and Hubs fold by their chevron and stay folded on this device; a head that does not fold draws none', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { modulesOn: ALL, stars: [{ kind: 'HUB', id: IT.id }] });
  t.after(close);
  await tree(page, 'Hubs').getByRole('treeitem').first().waitFor({ timeout: 10_000 });
  for (const name of ['Starred', 'Products', 'Hubs']) {
    const fold = side(page).getByRole('button', { name: `Fold ${name}` });
    assert.equal(await fold.getAttribute('aria-expanded'), 'true');
    await fold.click();
    assert.equal(await fold.getAttribute('aria-expanded'), 'false');
    assert.equal(await tree(page, name).count(), 0, `${name} still shows its rows`);
  }
  await page.reload();
  await side(page).getByRole('button', { name: 'Fold Hubs' }).waitFor({ timeout: 10_000 });
  assert.equal(await side(page).getByRole('tree').count(), 0, 'the folds were not kept');
  await side(page).getByRole('button', { name: 'Fold Hubs' }).click();
  await tree(page, 'Hubs').getByRole('treeitem').first().waitFor();
  // Every head: a chevron only on a button that folds; the pointer only there.
  const heads = await side(page).locator('section .head').evaluateAll((nodes) => nodes.map((node) => {
    const twist = node.querySelector('.twist');
    return { button: twist?.tagName === 'BUTTON', chevron: !!twist?.querySelector('svg'), pointer: twist ? getComputedStyle(twist).cursor : 'none' };
  }));
  for (const head of heads) assert.ok(head.button ? head.chevron && head.pointer === 'pointer' : !head.chevron && head.pointer !== 'pointer', JSON.stringify(heads));
  assert.deepEqual(failures, []);
});

/** A finger's long press (ADR-0088): the hold, then the lift. */
async function longPress(page, locator) {
  const box = await locator.boundingBox();
  const at = { x: box.x + box.width / 3, y: box.y + box.height / 2 };
  await locator.dispatchEvent('pointerdown', { pointerType: 'touch', button: 0, clientX: at.x, clientY: at.y, isPrimary: true });
  await page.waitForTimeout(450);
  await page.evaluate((point) => window.dispatchEvent(new PointerEvent('pointerup', { pointerType: 'touch', clientX: point.x, clientY: point.y, bubbles: true })), at);
  await page.waitForTimeout(400);
}

test('UC-WRK-36 checks 19-20 on a phone: a long press hides a product or a hub, More opens in place and shows it again', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close } = await open(browser, { path: '/browse', modulesOn: ALL, owner: true, width: 390, height: 844, hasTouch: true, ready: (p) => p.locator('[data-phone-bar]') });
  t.after(close);
  const stack = page.locator('[data-browse-step]');
  await stack.getByRole('link', { name: 'Time' }).waitFor({ timeout: 10_000 });
  await longPress(page, stack.getByRole('link', { name: 'Time' }));
  const sheet = page.getByRole('dialog', { name: 'Time' });
  await sheet.getByRole('button', { name: 'Hide from the sidebar' }).click();
  await page.waitForFunction(() => !document.querySelector('[data-browse-step]')?.querySelector('a[href="/time"]'));
  assert.deepEqual(written.preferences.at(-1), { nav_products_hidden: ['TIME'] });
  const toggle = stack.locator('[data-hidden-rows]').first().getByRole('button', { name: /^More/ });
  assert.equal(await toggle.getAttribute('aria-expanded'), 'false');
  const box = await toggle.boundingBox();
  assert.ok(box.height >= 44, `More is ${box.height} px high`);
  await toggle.click();
  await longPress(page, stack.getByRole('link', { name: 'Time' }));
  await page.getByRole('dialog', { name: 'Time' }).getByRole('button', { name: 'Show in the sidebar' }).click();
  await page.waitForFunction(() => document.querySelectorAll('[data-browse-step] [data-hidden-rows]').length === 0);
  assert.deepEqual(written.preferences.at(-1), { nav_products_hidden: [] });
  // A hub's sheet: its star and Hide.
  await longPress(page, stack.getByRole('link', { name: IT.name }));
  const hubSheet = page.getByRole('dialog', { name: IT.name });
  assert.deepEqual(await texts(hubSheet.getByRole('button').filter({ hasNotText: 'Close' })), ['Star', 'Hide from the sidebar']);
  await hubSheet.getByRole('button', { name: 'Hide from the sidebar' }).click();
  await page.waitForFunction(() => document.querySelectorAll('[data-browse-step] [data-hidden-rows]').length === 1);
  assert.deepEqual(written.preferences.at(-1), { nav_hubs_hidden: [IT.id] });
  assert.deepEqual(failures, []);
});
