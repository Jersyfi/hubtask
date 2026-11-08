// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A detail screen stays in the level of the place that owns it (UC-WRK-36 check 2; the owner's
// review R-18, R-19): page → whiteboard → page from the hub's *Contents* without a reload keeps
// the hub's level standing - the same element, never torn down and slid in again - and draws no
// boundary; a goal of a hub opens with that hub's level and *Goals* current, a workspace goal with
// the workspace level and the *Goals* product current.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { chromium } from 'playwright';

import { ALL, GOALS, PD, PD_BOARDS, PD_PAGES, WORKSPACE_GOAL, open, served, side } from './kompassstub.mjs';

test.after(() => served.close());

const ON = { [PD.id]: ['PLANNING', 'TIME', 'GOALS', 'KNOWLEDGE', 'WHITEBOARD'] };
const at = (page, path) => page.waitForFunction((want) => location.pathname === want, path, { timeout: 10_000 });
/** Marks the hub level's sliding part; `isSame` answers whether that very element still stands. */
const markLevel = (page) => page.evaluate(() => { document.querySelector('aside.sidenav .level .slide').dataset.kept = 'yes'; });
const isSame = (page) => page.evaluate(() => document.querySelector('aside.sidenav .level .slide')?.dataset.kept === 'yes');
/** Counts every hub level the sidebar puts in: one that is torn down and drawn again counts twice. */
const countLevels = (page) => page.evaluate(() => {
  window.__levels = 0;
  new MutationObserver((records) => {
    for (const record of records) for (const node of record.addedNodes) if (node instanceof Element && node.matches('.slide, .level')) window.__levels++;
  }).observe(document.querySelector('aside.sidenav'), { childList: true, subtree: true });
});

const WALK = () => {
  const handbook = PD_PAGES.find((each) => each.title === 'Product handbook');
  const onboarding = PD_PAGES.find((each) => each.title === 'Onboarding');
  const retro = PD_BOARDS.find((each) => each.title === 'Retro board');
  return [['Product handbook', `/pages/${handbook.id}`], ['Retro board', `/whiteboards/${retro.id}`], ['Onboarding', `/pages/${onboarding.id}`], ['Retro board', `/whiteboards/${retro.id}`]];
};
/** The screen is drawn, by its heading, and no boundary stands in its place. */
async function drawn(page, title, path) {
  await at(page, path);
  await page.getByRole('main').getByRole('heading', { name: title }).first().waitFor({ timeout: 10_000 });
  await page.waitForTimeout(300);
  assert.equal(await page.locator('[data-crash]').count(), 0, `a boundary on ${title}`);
}

test('R-18: page → whiteboard → page from Contents keeps the hub\'s level standing and draws no boundary', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { path: `/hubs/${PD.id}`, modulesOn: ALL, hubModules: ON, owner: true });
  t.after(close);
  const contents = side(page).getByRole('region', { name: PD.name }).getByRole('tree', { name: `Contents of ${PD.name}` });
  await contents.waitFor({ timeout: 10_000 });
  await markLevel(page);
  await countLevels(page);
  for (const [title, path] of WALK()) {
    await contents.getByRole('treeitem', { name: title }).first().click();
    await drawn(page, title, path);
    assert.equal(await isSame(page), true, `the hub's level was drawn again on ${title}`);
    assert.equal(await contents.getByRole('treeitem', { name: title }).first().getAttribute('aria-current'), 'page', `${title} is not current`);
  }
  assert.equal(await page.evaluate(() => window.__levels), 0, 'a level was put in again');
  assert.deepEqual(failures, []);
});

test('R-18 on a touch tablet: page → whiteboard → page from the drawer, each time at the hub\'s level, no boundary', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { path: `/hubs/${PD.id}`, width: 820, height: 1180, hasTouch: true, modulesOn: ALL, hubModules: ON, owner: true });
  t.after(close);
  for (const [title, path] of WALK()) {
    await page.getByRole('button', { name: 'Open the navigation' }).click();
    const drawer = page.getByRole('dialog');
    await drawer.getByRole('button', { name: 'All hubs' }).waitFor({ timeout: 10_000 });
    await drawer.getByRole('tree', { name: `Contents of ${PD.name}` }).getByRole('treeitem', { name: title }).first().tap();
    await drawn(page, title, path);
  }
  await page.getByRole('button', { name: 'Open the navigation' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'All hubs' }).waitFor({ timeout: 10_000 });
  assert.deepEqual(failures, []);
});

test('R-19: a goal of a hub stays at the hub level with Goals current; a workspace goal is at the workspace level', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { path: `/hubs/${PD.id}/goals`, modulesOn: ALL, hubModules: ON, owner: true });
  t.after(close);
  const level = side(page).getByRole('region', { name: PD.name });
  await level.getByRole('link', { name: 'Goals' }).waitFor({ timeout: 10_000 });
  await markLevel(page);
  const goal = GOALS[0];
  await page.getByRole('main').getByRole('link', { name: goal.title }).first().click();
  await at(page, `/goals/${goal.id}`);
  await page.getByRole('main').getByRole('heading', { name: goal.title }).first().waitFor({ timeout: 10_000 });
  await page.waitForTimeout(300);
  assert.equal(await level.getByRole('button', { name: 'All hubs' }).count(), 1, 'the goal left its hub\'s level');
  assert.equal(await isSame(page), true, 'the hub\'s level was drawn again');
  assert.equal(await level.getByRole('link', { name: 'Goals' }).getAttribute('aria-current'), 'page', 'Goals is not current');
  // A workspace goal, reached from inside the hub: the workspace level, the Goals product current.
  await page.evaluate((path) => { history.pushState(null, '', path); dispatchEvent(new PopStateEvent('popstate')); }, `/goals/${WORKSPACE_GOAL.id}`);
  await page.getByRole('main').getByRole('heading', { name: WORKSPACE_GOAL.title }).first().waitFor({ timeout: 10_000 });
  const products = side(page).getByRole('tree', { name: 'Products' });
  await products.waitFor({ timeout: 5_000 });
  assert.equal(await side(page).getByRole('button', { name: 'All hubs' }).count(), 0, 'a workspace goal shows a hub\'s level');
  assert.equal(await products.getByRole('treeitem', { name: /^Goals/ }).getAttribute('aria-current'), 'page', 'the Goals product is not current');
  assert.deepEqual(failures, []);
});
