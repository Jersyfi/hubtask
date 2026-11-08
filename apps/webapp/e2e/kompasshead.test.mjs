// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A collection's head and its view bar, and where a page stands after a move (the owner's review of
// 2026-11-08, R-11, R-13, R-15, and Today's Lists card found beside it): every view of a collection
// draws one head - the same title, the same ☆, one figure slot - and its bar at one offset; the
// bar's tools fold to their marks, then behind ⋯, and never overlap the views; a new screen starts
// at its top and *back* returns to where the reader was; a long list name stays in its tile.
// Desktop, a touch tablet at 820 and a phone at 390.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { chromium } from 'playwright';

import { ALL, COLLECTIONS, DESIGN, MOBILE, PD, WEB, open, served, side } from './kompassstub.mjs';

test.after(() => served.close());

const ON = { [PD.id]: ['WORKFLOWS', 'PLANNING', 'TIME', 'GOALS', 'KNOWLEDGE', 'WHITEBOARD'] };
const go = (browser, options) => open(browser, { modulesOn: ALL, hubModules: ON, owner: true, ...options });
const desktop = { width: 1440, height: 900 };
const tablet = { width: 820, height: 1180, hasTouch: true };
const phone = { width: 390, height: 844, hasTouch: true, ready: (page) => page.locator('[data-phone-bar]') };
const box = async (locator) => {
  const { x, y, width, height } = await locator.boundingBox();
  return { x: Math.round(x), y: Math.round(y), width: Math.round(width), height: Math.round(height) };
};

const VIEWS = ['?layout=board', '/sprint', '/backlog', '?layout=list', '/timeline', '/time'];

test('R-11: every view of a collection draws one head - its name, its star, one figure - and the bar at one place', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  for (const [name, size] of [['desktop', desktop], ['tablet', tablet], ['phone', phone]]) {
    const { page, failures, close } = await go(browser, { path: `/collections/${MOBILE.id}`, ...size });
    t.after(close);
    const seen = [];
    for (const view of VIEWS) {
      await page.goto(new URL(`/collections/${MOBILE.id}${view}`, page.url()).href);
      const head = page.locator('main [data-collection-head]');
      await head.getByRole('heading', { level: 1, name: MOBILE.name }).waitFor();
      const bar = size === phone ? page.locator('main .views') : page.getByRole('navigation', { name: MOBILE.name });
      await bar.waitFor();
      await page.waitForTimeout(300);
      const figure = head.locator('.head-figure');
      seen.push({
        view,
        bar: await box(bar),
        figure: (await figure.count()) > 0 ? (await box(figure)).y : undefined,
        stars: await head.getByRole('button', { name: `Star ${MOBILE.name}` }).count(),
        heads: await page.locator('main h1').count(),
      });
    }
    const first = seen[0];
    for (const each of seen) {
      assert.deepEqual(each.bar, first.bar, `${name}: the bar of ${each.view} stands at ${JSON.stringify(each.bar)}, the board's at ${JSON.stringify(first.bar)}`);
      assert.equal(each.figure, first.figure, `${name}: the figure of ${each.view} moved`);
      assert.equal(each.heads, 1, `${name}: ${each.view} draws one title`);
      // A phone draws the star in its app bar, for every view alike.
      assert.equal(each.stars, size === phone ? 0 : 1, `${name}: ${each.view} stars the collection from its head`);
    }
    assert.deepEqual(failures, []);
  }
});

test('R-11: the sprint\'s days stand in the figure\'s place, the collection\'s count where a view has none', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await go(browser, { path: `/collections/${MOBILE.id}/sprint`, ...desktop });
  t.after(close);
  const figure = page.locator('main [data-collection-head] .head-figure');
  await figure.locator('[data-sprint-figure]').waitFor();
  assert.match(await figure.innerText(), /days left/);
  await page.goto(new URL(`/collections/${MOBILE.id}/time`, page.url()).href);
  await page.locator('main [data-collection-head]').getByRole('heading', { level: 1, name: MOBILE.name }).waitFor();
  assert.match(await figure.innerText(), /12 open/);
});

/** The bar's views and its tools, side by side and not over each other. */
async function fits(page, label) {
  const bar = page.getByRole('navigation', { name: DESIGN.name });
  await bar.waitFor();
  await page.waitForTimeout(400);
  const views = await box(bar.locator('ul'));
  const tools = await box(bar.locator('.tools, .tools-more'));
  const whole = await box(bar);
  assert.ok(tools.x >= views.x + views.width - 1, `${label}: the tools start at ${tools.x}, the views end at ${views.x + views.width}`);
  assert.ok(tools.x + tools.width <= whole.x + whole.width + 1, `${label}: the tools run past the bar`);
  for (const button of await bar.locator('.tools button').all()) {
    const each = await box(button);
    assert.ok(each.y >= whole.y && each.y + each.height <= whole.y + whole.height + 1, `${label}: a tool stands outside the bar's line`);
  }
  return bar;
}

test('R-13: the tools fold to their marks - named, with their words as tooltips - and never overlap the views', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  for (const width of [1440, 1024, 820, 600]) {
    const { page, failures, close } = await go(browser, { path: `/collections/${DESIGN.id}?layout=list`, width, height: 900, hasTouch: width < 905 });
    t.after(close);
    const bar = await fits(page, `${width}`);
    const tools = bar.locator('.tools');
    const fit = await tools.getAttribute('data-fit');
    const filter = bar.getByRole('button', { name: 'Filter' });
    assert.equal(await filter.count(), 1, `${width}: Filter is named`);
    if (fit === 'marks') {
      assert.equal(await filter.getAttribute('title'), 'Filter', `${width}: a mark carries its words as tooltip`);
      const mark = await box(filter);
      assert.ok(Math.abs(mark.width - mark.height) <= 1, `${width}: a mark stands square, ${mark.width} by ${mark.height}`);
    }
    if (width === 1440) assert.equal(fit, 'whole', 'the widest window shows the tools whole');
    if (width === 1024) assert.equal(fit, 'marks', 'the views keep their room, the tools fold to marks');
    assert.deepEqual(failures, []);
  }
});

test('R-13: narrower than the marks and the views, the tools move behind one ⋯ and work from there', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await go(browser, { path: `/collections/${DESIGN.id}?layout=list`, width: 905, height: 900, storage: { 'hubtask.nav.width': '400' } });
  t.after(close);
  const bar = await fits(page, '905 beside a wide sidebar');
  await bar.getByRole('button', { name: 'Tools' }).click();
  const panel = page.getByRole('dialog', { name: 'Tools' });
  for (const name of ['Filter', 'Fields', 'Group', 'Sort', 'Views', 'Select entries']) {
    assert.equal(await panel.getByRole('button', { name }).count(), 1, `${name} behind ⋯`);
  }
  await panel.getByRole('button', { name: 'Sort' }).click();
  await page.getByRole('menuitem', { name: 'The order you arranged' }).waitFor();
  assert.deepEqual(failures, []);
});

test('R-15: a new screen starts at its top, back returns to where the reader was', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  for (const [name, size] of [['phone', phone], ['desktop', { ...desktop, height: 600 }]]) {
    const { page, failures, close } = await go(browser, { path: '/', ...size });
    t.after(close);
    const tile = page.locator('section[aria-labelledby="today-lists"] a', { hasText: WEB.name });
    await tile.waitFor();
    const reach = await page.evaluate(() => document.documentElement.scrollHeight - innerHeight);
    assert.ok(reach > 120, `${name}: Today is taller than the screen`);
    await tile.scrollIntoViewIfNeeded();
    await page.evaluate(() => scrollBy(0, 60));
    const at = await page.evaluate(() => scrollY);
    assert.ok(at > 100, `${name}: Today scrolled to ${at}`);
    await tile.click();
    await page.locator('main').getByRole('heading', { level: 1, name: WEB.name }).waitFor({ state: 'attached' });
    await page.waitForTimeout(300);
    assert.equal(await page.evaluate(() => scrollY), 0, `${name}: the collection opened where Today was scrolled to`);
    await page.goBack();
    await tile.waitFor();
    await page.waitForFunction((y) => Math.abs(scrollY - y) <= 2, at, { timeout: 5_000 });
    assert.deepEqual(failures, []);
  }
});

test('R-15 on desktop: a row of the sidebar opens its screen at the top, and the sidebar keeps its own scroll', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await go(browser, { path: `/collections/${WEB.id}?layout=list`, width: 1440, height: 520 });
  t.after(close);
  await page.locator('main').getByRole('heading', { level: 1, name: WEB.name }).waitFor();
  const middle = side(page).locator('[data-scrollbar="thin"]').first();
  await middle.evaluate((node) => { node.scrollTop = 30; });
  await page.evaluate(() => scrollTo(0, document.documentElement.scrollHeight));
  assert.ok((await page.evaluate(() => scrollY)) > 0, 'the collection is taller than the window');
  // The pointer brings the row into view first, as a reader's would; from there nothing moves it.
  const row = side(page).getByRole('treeitem', { name: /^Design\b/ });
  await row.hover();
  const kept = await middle.evaluate((node) => node.scrollTop);
  assert.ok(kept > 0, 'the hub level scrolls at this height');
  await row.click();
  await page.locator('main').getByRole('heading', { level: 1, name: DESIGN.name }).waitFor();
  await page.waitForTimeout(300);
  assert.equal(await page.evaluate(() => scrollY), 0, 'the next collection opened scrolled down');
  assert.equal(await middle.evaluate((node) => node.scrollTop), kept, 'the sidebar jumped');
  assert.deepEqual(failures, []);
});

test('Today\'s Lists: a long name stays in its tile, and the card in its column', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const long = 'Team Boreal – Platform & API and everything around it';
  const renamed = COLLECTIONS.map((each) => (each.id === WEB.id ? { ...each, name: long } : each));
  const extra = async (route, request, url) => {
    if (!url.pathname.endsWith('/api/v1/containers') || url.searchParams.get('type') === 'HUB') return false;
    await route.fulfill({ json: { data: renamed.filter((each) => each.parent_id === url.searchParams.get('parent_id')), page: { next_cursor: null, has_more: false } } });
    return true;
  };
  for (const size of [desktop, { width: 1024, height: 900 }, phone]) {
    const { page, failures, close } = await go(browser, { path: '/', extra, ...size });
    t.after(close);
    const card = page.locator('section[aria-labelledby="today-lists"]');
    await card.locator('a', { hasText: 'Team Boreal' }).waitFor();
    const edge = await box(card);
    for (const tile of await card.locator('a').all()) {
      const each = await box(tile);
      assert.ok(each.x + each.width <= edge.x + edge.width, `${size.width}: a tile runs to ${each.x + each.width}, the card ends at ${edge.x + edge.width}`);
    }
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `${size.width}: the page scrolls sideways`);
    assert.deepEqual(failures, []);
  }
});

test('The list\'s column heads stand over their columns where they fit, and are gone where they do not - never over each other', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  for (const width of [1440, 1024, 905, 820, 600, 390]) {
    const { page, failures, close } = await go(browser, { path: `/collections/${DESIGN.id}?layout=list`, width, height: 900, hasTouch: width < 905, ready: width < 600 ? phone.ready : undefined });
    t.after(close);
    const level = page.locator('main .level').first();
    await level.locator('.task-row').first().waitFor();
    await page.waitForTimeout(300);
    const head = page.locator('main .fields-head').first();
    const shown = (await head.count()) > 0 && (await head.isVisible()) && (await head.evaluate((node) => getComputedStyle(node).visibility === 'visible'));
    if (width === 1440) assert.ok(shown, 'the widest window shows the column heads');
    if (shown) {
      const heads = [];
      for (const each of await head.locator('.col-head').all()) heads.push({ text: (await each.innerText()).trim(), ...(await box(each)) });
      for (let index = 1; index < heads.length; index += 1) {
        const before = heads[index - 1];
        assert.ok(heads[index].x >= before.x + before.width, `${width}: "${heads[index].text}" starts at ${heads[index].x}, "${before.text}" ends at ${before.x + before.width}`);
      }
      // Every row's fields stand in the row, under their heads.
      for (const row of (await level.locator('.cells:visible').all()).slice(0, 4)) {
        const cells = await box(row);
        const edge = await box(level);
        assert.ok(cells.x + cells.width <= edge.x + edge.width + 1, `${width}: a row's fields run to ${cells.x + cells.width}, the list ends at ${edge.x + edge.width}`);
      }
    }
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `${width}: the page scrolls sideways`);
    assert.deepEqual(failures, []);
  }
});
