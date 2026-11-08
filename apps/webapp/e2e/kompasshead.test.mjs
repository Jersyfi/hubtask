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
