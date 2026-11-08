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

/** The glyph a control draws, as its drawing: two controls with the same mark draw the same paths. */
const glyph = (locator) => locator.locator('svg').first().innerHTML();

test('R-6: Switch hub and Change position draw different marks, and no menu of the sidebar repeats one', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await go(browser, { path: '/' });
  t.after(close);
  const hubs = side(page).getByRole('tree', { name: 'Hubs' });
  const row = hubs.getByRole('treeitem', { name: /^Household/ });
  await row.waitFor({ timeout: 10_000 });
  /** Opens a menu and returns its items' marks, closing it again. */
  const marks = async (trigger) => {
    await trigger.click();
    await page.getByRole('menuitem').first().waitFor();
    const drawn = (await Promise.all((await page.getByRole('menuitem').all()).map(async (item) => ((await item.locator('svg').count()) ? glyph(item) : null)))).filter(Boolean);
    await page.keyboard.press('Escape');
    return drawn;
  };
  await row.hover();
  const rowMenu = await marks(row.getByRole('button', { name: /^Manage / }));
  await row.hover();
  await row.getByRole('button', { name: /^Manage / }).click();
  const position = await glyph(page.getByRole('menuitem', { name: /^Change position/ }));
  await page.keyboard.press('Escape');
  assert.equal(new Set(rowMenu).size, rowMenu.length, 'a hub row\'s ⋯ repeats a mark');

  await hubs.getByRole('treeitem', { name: /^Product development/ }).click();
  const level = side(page).getByRole('region', { name: PD.name });
  const switcher = level.getByRole('button', { name: 'Switch hub' });
  await switcher.waitFor();
  assert.notEqual(await glyph(switcher), position, 'Switch hub draws Change position\'s mark');
  const head = level.locator('.hub');
  const headMarks = await Promise.all((await head.getByRole('button').all()).map(glyph));
  assert.equal(new Set(headMarks).size, headMarks.length, 'the hub head repeats a mark');
  const hubMenu = await marks(level.getByRole('button', { name: `Manage ${PD.name}` }));
  assert.equal(new Set(hubMenu).size, hubMenu.length, 'the hub\'s ⋯ repeats a mark');
  const contents = side(page).locator('section', { has: page.getByRole('heading', { name: /^Contents/ }) });
  await contents.locator('.head').hover();
  const create = await marks(contents.getByRole('button', { name: 'Create in Contents' }));
  assert.equal(new Set(create).size, create.length, 'Page and From a template… share a mark');
  assert.deepEqual(failures, []);
});

test('R-8: the rail\'s peek is drawn above the view tabs and the board; a touch tablet\'s drawer above the page\'s bars', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const path = `/collections/${MOBILE.id}?layout=board`;
  {
    const { page, failures, close } = await go(browser, { path, storage: { 'hubtask.nav': 'rail' } });
    t.after(close);
    const tabs = page.getByRole('main').getByRole('link', { name: /^Board/ }).first();
    await tabs.waitFor({ timeout: 10_000 });
    const column = page.getByRole('main').getByRole('link', { name: 'Push notifications' });
    await side(page).hover({ position: { x: 20, y: 400 } });
    await page.locator('aside.sidenav[data-peek]').waitFor();
    for (const under of [tabs, column]) {
      const box = await under.boundingBox();
      const top = await page.evaluate(([x, y]) => !!document.elementFromPoint(x, y)?.closest('aside.sidenav'), [box.x + box.width / 2, box.y + box.height / 2]);
      assert.ok(top, `${await under.innerText()} is drawn above the peek`);
    }
    assert.deepEqual(failures, []);
  }
  {
    const { page, failures, close } = await go(browser, { path, ...tablet });
    t.after(close);
    const tabs = page.getByRole('main').getByRole('link', { name: /^Board/ }).first();
    await tabs.waitFor({ timeout: 10_000 });
    const box = await tabs.boundingBox();
    await page.getByRole('button', { name: 'Open the navigation' }).click();
    await page.getByRole('dialog', { name: 'Navigation' }).getByRole('region').first().waitFor();
    const top = await page.evaluate(([x, y]) => !!document.elementFromPoint(x, y)?.closest('dialog'), [box.x + box.width / 2, box.y + box.height / 2]);
    assert.ok(top, 'the view tabs are drawn above the drawer');
    assert.deepEqual(failures, []);
  }
});

/** The colour of one pixel of a screenshot, read on a blank page. */
async function pixel(browser, png, x, y) {
  const reader = await browser.newPage();
  try {
    return await reader.evaluate(async ([data, px, py]) => {
      const image = new Image();
      image.src = `data:image/png;base64,${data}`;
      await image.decode();
      const canvas = document.createElement('canvas');
      [canvas.width, canvas.height] = [image.width, image.height];
      const context = canvas.getContext('2d');
      context.drawImage(image, 0, 0);
      return [...context.getImageData(px, py, 1, 1).data.slice(0, 3)];
    }, [png.toString('base64'), x, y]);
  } finally {
    await reader.close();
  }
}

test('R-9: the sidebar\'s thin scrollbar keeps its room, shows nothing idle and a track while hovered', async (t) => {
  // Headless Chromium hides every scrollbar unless told otherwise.
  const browser = await chromium.launch({ ignoreDefaultArgs: ['--hide-scrollbars'] });
  t.after(() => browser.close());
  const { page, failures, close } = await go(browser, { path: `/hubs/${PD.id}`, height: 520 });
  t.after(close);
  const level = side(page).getByRole('region', { name: PD.name });
  await level.waitFor({ timeout: 10_000 });
  const style = await level.evaluate((node) => {
    const computed = getComputedStyle(node);
    return { thin: node.dataset.scrollbar, gutter: computed.scrollbarGutter, width: computed.scrollbarWidth, color: computed.scrollbarColor, overflows: node.scrollHeight > node.clientHeight, gutterRoom: node.offsetWidth - node.clientWidth };
  });
  // Chromium draws the pseudo-elements only where neither standard property is set.
  assert.deepEqual({ ...style, gutterRoom: undefined }, { thin: 'thin', gutter: 'stable', width: 'auto', color: 'auto', overflows: true, gutterRoom: undefined });
  const room = Number.parseFloat(await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue('--layout-nav-scrollbar')));
  assert.equal(style.gutterRoom, room, 'the room reserved is the token\'s');
  const box = await level.boundingBox();
  // The track's middle, below the thumb: the level is scrolled to its top.
  const [x, y] = [Math.floor(box.x + box.width - room / 2), Math.floor(box.y + box.height - 12)];
  await page.mouse.move(box.x + box.width / 2, box.y + 4);
  await page.mouse.move(1000, 600);
  const idle = await pixel(browser, await page.screenshot(), x, y);
  await level.hover({ position: { x: box.width / 2, y: box.height / 2 } });
  const hovered = await pixel(browser, await page.screenshot(), x, y);
  const plane = await level.evaluate((node) => {
    const probe = document.createElement('span');
    probe.style.color = 'var(--bg-surface-hover)';
    node.append(probe);
    const colour = getComputedStyle(probe).color;
    probe.remove();
    return colour.match(/\d+/g).slice(0, 3).map(Number);
  });
  assert.notDeepEqual(idle, hovered, 'the scrollbar looks the same hovered');
  assert.ok(hovered.every((value, i) => Math.abs(value - plane[i]) <= 2), `the track is ${hovered}, not the hover surface ${plane}`);
  assert.deepEqual(failures, []);
});
