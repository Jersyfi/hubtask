// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// How the sidebar's parts fit and stack (the owner's review of 2026-11-08, R-6, R-8, R-9 and the
// two found beside it): the hub quick switch drops from the hub's head, its section heads are the
// panel's own surface and fold nowhere; the drawer's heads match the drawer and fold; no two
// actions share a mark; the rail's peek is drawn above every sticky bar of the page; the thin
// scrollbar shows a track while hovered; at the narrowest sidebar nothing overlaps and a row's name
// keeps its room beside a long figure. Desktop, a touch tablet at 820 and a phone at 390.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { chromium } from 'playwright';

import { ALL, HOME, IT, MOBILE, PD, open, served, side } from './kompassstub.mjs';

test.after(() => served.close());

const ON = { [PD.id]: ['WORKFLOWS', 'PLANNING', 'TIME', 'GOALS', 'KNOWLEDGE', 'WHITEBOARD'] };
const go = (browser, options) => open(browser, { modulesOn: ALL, hubModules: ON, owner: true, ...options });
const tablet = { width: 820, height: 1180, hasTouch: true };
const phone = { width: 390, height: 844, hasTouch: true, ready: (page) => page.locator('[data-phone-bar]') };

/** Opens two other hubs first, so the quick switch lists them as recently visited. */
async function visited(page) {
  for (const each of [IT, HOME, PD]) {
    await page.goto(new URL(`/hubs/${each.id}`, page.url()).href);
    await page.getByRole('region', { name: each.name }).first().waitFor({ timeout: 10_000 });
  }
}

/** The quick switch's panel against the hub head it drops from. */
async function dropsFromHead(page, scope) {
  const head = scope.locator('.hub').first();
  await scope.getByRole('button', { name: 'Switch hub' }).click();
  const panel = page.getByRole('dialog', { name: 'Switch hub' });
  await panel.getByRole('searchbox', { name: 'Find a hub' }).waitFor();
  const [h, p] = [await head.boundingBox(), await panel.boundingBox()];
  assert.ok(Math.abs(p.y - (h.y + h.height)) <= 8, `the panel starts at ${p.y}, the head ends at ${h.y + h.height}`);
  assert.ok(Math.abs(p.x - h.x) <= 2, `the panel starts at x ${p.x}, the head at ${h.x}`);
  return panel;
}

const background = (locator) => locator.evaluate((node) => getComputedStyle(node).backgroundColor);

test('R-6: the quick switch drops from the hub head; Recently visited is a head of the panel, aligned with the search, folding nothing', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  for (const theme of ['light', 'dark']) {
    const { page, failures, close } = await go(browser, { path: `/hubs/${IT.id}`, storage: { 'hubtask.theme': theme } });
    t.after(close);
    await visited(page);
    const panel = await dropsFromHead(page, side(page));
    const group = panel.locator('section', { has: page.getByRole('heading', { name: 'Recently visited' }) });
    const head = group.locator('.head');
    assert.equal(await background(head), await background(panel), `${theme}: the head is a band of another colour`);
    assert.equal(await head.getByRole('button').count(), 0, 'a chevron that folds nothing');
    assert.notEqual(await head.evaluate((node) => getComputedStyle(node).cursor), 'pointer');
    const [h, s] = [await head.boundingBox(), await panel.locator('.search').boundingBox()];
    assert.ok(Math.abs(h.x - s.x) <= 1 && Math.abs(h.width - s.width) <= 1, `the head spans ${h.x}+${h.width}, the search ${s.x}+${s.width}`);
    assert.deepEqual(failures, []);
  }
});

test('R-6 on a touch tablet: the drawer\'s quick switch drops from its head; its section heads are the drawer\'s surface and fold', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  for (const theme of ['light', 'dark']) {
    const { page, failures, close } = await go(browser, { path: `/hubs/${PD.id}`, storage: { 'hubtask.theme': theme }, ...tablet });
    t.after(close);
    await page.getByRole('button', { name: 'Open the navigation' }).click();
    const drawer = page.getByRole('dialog', { name: 'Navigation' });
    const level = drawer.getByRole('region', { name: PD.name });
    await level.waitFor();
    const heads = drawer.locator('section > .head');
    const surface = await background(drawer);
    for (const each of await heads.all()) assert.equal(await background(each), surface, `${theme}: a grey band in the drawer`);
    const fold = drawer.getByRole('button', { name: 'Fold Collections' });
    await fold.click();
    assert.equal(await fold.getAttribute('aria-expanded'), 'false');
    assert.equal(await drawer.getByRole('tree', { name: /Collections/ }).count(), 0, 'Collections did not fold in the drawer');
    await fold.click();
    await dropsFromHead(page, level);
    assert.deepEqual(failures, []);
  }
});

test('R-6 on a phone: the Browse step\'s section heads are quiet labels - no band, no chevron', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await go(browser, { path: `/browse/hubs/${PD.id}`, ...phone });
  t.after(close);
  const step = page.locator('[data-browse-step]');
  await step.locator('section > .head').first().waitFor({ timeout: 10_000 });
  for (const each of await step.locator('section > .head').all()) {
    assert.equal(await background(each), 'rgba(0, 0, 0, 0)');
    assert.equal(await each.locator('.twist').evaluate((node) => getComputedStyle(node).display), 'none');
  }
  assert.deepEqual(failures, []);
});
