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
