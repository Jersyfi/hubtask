// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The administration's advanced rows fold under *More* while the workspace holds one person
// (user-experience.md §8.2): the column a newcomer meets is short, a folded row opens from *More*,
// and a reader standing on one finds *More* open. In a workspace of several, nothing folds.
//
// Against a stubbed API. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

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
  const nav = page.getByRole('navigation', { name: 'Administration' });
  await nav.getByRole('treeitem', { name: 'People' }).or(nav.getByRole('link', { name: 'People' })).first().waitFor({ timeout: 15_000 });
  return { page, nav, close: () => context.close() };
}

const visible = async (nav, name) => (await nav.getByText(name, { exact: true }).filter({ visible: true }).count()) > 0;

test('chromium: a workspace of one folds the advanced rows under More, and More opens them', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { nav, close } = await column(browser, { severalPeople: false, path: '/administration' });
  t.after(close);

  await nav.getByText('More', { exact: true }).first().waitFor();
  assert.equal(await nav.getByText('More', { exact: true }).count(), 2, 'how we work and connections fold');
  assert.equal(await visible(nav, 'Limits'), true, 'a group folds two rows or none');
  assert.equal(await visible(nav, 'Voice assistants'), false);
  assert.equal(await visible(nav, 'Link types'), false);
  assert.equal(await visible(nav, 'Templates'), true, 'Templates stays where a newcomer starts');
  await nav.getByText('More', { exact: true }).last().click();  // the connections' More
  await nav.getByText('Voice assistants', { exact: true }).waitFor();
});

test('chromium: standing on a folded row finds its More open', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { nav, close } = await column(browser, { severalPeople: false, path: '/administration/voice' });
  t.after(close);
  await nav.getByText('Voice assistants', { exact: true }).waitFor({ timeout: 10_000 });
});

test('chromium: a workspace of several folds nothing', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { nav, close } = await column(browser, { severalPeople: true, path: '/administration' });
  t.after(close);
  await nav.getByText('Voice assistants', { exact: true }).waitFor({ timeout: 10_000 });
  assert.equal(await nav.getByText('More', { exact: true }).count(), 0);
});
