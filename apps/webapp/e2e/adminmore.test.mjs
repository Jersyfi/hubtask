// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The administration's column draws every row, advanced or not, in a workspace of one as of several
// (user-experience.md §8.9): no row hides under a *More*.
//
// Against a stubbed API. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { WORKSPACE, stub, forgetUnstubbed } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

async function column(browser, { severalPeople, path }) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const read = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (read === '/tenant' && route.request().method() === 'GET') {
      return route.fulfill({ json: { ...WORKSPACE, several_people: severalPeople } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  forgetUnstubbed();
  await page.goto(`${served.origin}${path}`);
  const nav = page.getByRole('navigation', { name: 'Administration', exact: true });
  await nav.getByRole('treeitem', { name: 'People' }).or(nav.getByRole('link', { name: 'People' })).first().waitFor({ timeout: 15_000 });
  return { page, nav, close: () => context.close() };
}

const visible = async (nav, name) => (await nav.getByText(name, { exact: true }).filter({ visible: true }).count()) > 0;

for (const severalPeople of [false, true]) {
  test(`chromium: a workspace of ${severalPeople ? 'several' : 'one'} draws every row, with no More`, async (t) => {
    const browser = await sharedChromium(t);
    const { nav, close } = await column(browser, { severalPeople, path: '/administration' });
    t.after(close);
    await nav.getByText('Voice assistants', { exact: true }).waitFor({ timeout: 10_000 });
    for (const name of ['Limits', 'Link types', 'Templates']) assert.equal(await visible(nav, name), true, name);
    assert.equal(await nav.getByText('More', { exact: true }).count(), 0);
  });
}
