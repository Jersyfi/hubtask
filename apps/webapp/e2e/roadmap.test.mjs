// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A portfolio's roadmap (UC-GOL-03 checks 10, 12 and 14) as a person opens it: the head says how
// far it reaches, how many projects it holds and which need a look; the chart opens on its whole
// reach, as the plan does (user-experience.md §7.5); an empty roadmap says what fills it. The
// server answers the roadmap; the walk proves what the screen draws. Chromium only, as the other
// walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { COLLECTION, HUB, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const PORTFOLIO = '01a0e2e0-0000-7000-8000-0000000e6a01';
const bar = (n, title, start, finish, extra = {}) => ({
  item_id: `01a0e2e0-0000-7000-8000-0000000e6b0${n}`, title, collection_id: COLLECTION.id, start, finish, milestone: false, done: false, critical: false, ...extra,
});
const FULL = {
  portfolio_id: PORTFOLIO, name: 'Programme', status: 'NONE', start: '2027-01-04', finish: '2027-05-28',
  lanes: [{
    hub_id: HUB.id, name: HUB.name, projects: [
      {
        container_id: COLLECTION.id, kind: 'COLLECTION', name: 'Kitchen', start_on: null, end_on: null, status: 'AT_RISK', finish: '2027-03-26',
        bars: [bar(1, 'Worktop', '2027-01-04', '2027-02-12', { critical: true }), bar(2, 'Handover', '2027-03-26', '2027-03-26', { milestone: true })],
      },
      { container_id: HUB.id, kind: 'HUB', name: 'Garden', start_on: '2027-03-01', end_on: '2027-05-28', status: 'ON_TRACK', finish: null, bars: [] },
    ],
  }],
};
const EMPTY = { portfolio_id: PORTFOLIO, name: 'Programme', status: 'NONE', start: null, finish: null, lanes: [] };

async function open(browser, roadmap) {
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 900, { timezoneId: 'UTC' });
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === `/portfolios/${PORTFOLIO}/roadmap`) return route.fulfill({ json: roadmap });
    if (path === `/portfolios/${PORTFOLIO}/public-link`) return route.fulfill({ json: { portfolio_id: PORTFOLIO, link: null } });
    return stub(route);
  });
  await page.goto(`${served.origin}/portfolios/${PORTFOLIO}/roadmap`);
  return { page, failures, close: () => context.close(), unstubbed };
}

test('chromium: 1280 px — the roadmap says its reach and its projects at risk, and opens on all of it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close, unstubbed } = await open(browser, FULL);
  t.after(close);

  await page.getByText(/^From Jan 4, 2027 to May 28, 2027 · 2 projects · 1 at risk$/).waitFor({ timeout: 15_000 });

  // The window holds the whole reach, months away from today, without a press.
  // A little room before and after, the start snapped to its month's line.
  await page.getByText('Dec 1, 2026 to Jun 5, 2027', { exact: true }).waitFor({ timeout: 5_000 });
  await page.getByRole('table', { name: 'Roadmap as a table' }).waitFor({ timeout: 5_000 });
  assert.ok(await page.getByRole('button', { name: 'Fit roadmap' }).isVisible());

  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

test('chromium: 1280 px — a roadmap without projects says what fills it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close, unstubbed } = await open(browser, EMPTY);
  t.after(close);

  await page.getByText('No project is on this roadmap yet.').waitFor({ timeout: 15_000 });
  assert.equal(await page.getByRole('button', { name: 'Open the portfolio' }).count(), 1);
  assert.equal(await page.getByRole('table', { name: 'Roadmap as a table' }).count(), 0, 'an empty roadmap draws an empty table');

  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});
