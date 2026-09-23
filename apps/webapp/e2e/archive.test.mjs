// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The archive (F10-03, ADR-0063 decision 1): what has been put aside, and the way back. It exists
// because archiving a container is otherwise a one-way door — the tree never asks for archived
// rows, so the container leaves the navigation and nothing shows it again (issue 933).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ARCHIVED, COLLECTION, HUB, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

test('chromium: 1280 px — the archive holds what the tree does not, and brings it back', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const written = [];
  const { page, failures, context, close, unstubbed } = await signedIn(browser, 1280, 1000);
  t.after(close);
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    if (request.method() !== 'GET') written.push(`${request.method()} ${new URL(request.url()).pathname}`);
    return stub(route);
  });

  await page.goto(`${served.origin}/archive`);
  await page.getByRole('heading', { level: 1, name: 'Archive' }).waitFor({ timeout: 15_000 });

  // The row is here, and it is not in the tree — which is the whole point of the screen.
  const row = page.locator('main').getByText(ARCHIVED.name, { exact: true });
  await row.waitFor({ timeout: 10_000 });
  assert.equal(await page.getByRole('navigation', { name: 'Workspace' }).getByRole('treeitem', { name: ARCHIVED.name }).count(), 0, 'the archived collection is in the tree');
  // And it says where it lives, so a name that means nothing on its own is placed.
  assert.equal(await page.locator('main').getByText(`In ${HUB.name}`).count(), 1);
  assert.equal(await page.getByRole('navigation', { name: 'Workspace' }).getByRole('treeitem', { name: COLLECTION.name }).count(), 0, 'the tree drew a collection without its hub open');

  // The way back, from here.
  await page.getByRole('button', { name: `Bring ${ARCHIVED.name} back` }).click();
  await page.waitForTimeout(400);
  assert.ok(
    written.includes(`POST /api/v1/containers/${ARCHIVED.id}:unarchive`),
    `nothing was unarchived: ${JSON.stringify(written)}`,
  );

  // Entries are named rather than left to be wondered about: they stay where they are.
  assert.equal(await page.locator('main').getByText(/Archived entries stay in the list/).count(), 1);
  assert.deepEqual(failures, []);
  // Nothing was answered by a guess: a shape this fixture never prepared is a shape the walk
  // cannot claim to have exercised (`fixture.mjs`).
  assert.deepEqual(unstubbed(), []);
});

test('chromium: 375 px — the archive is reached from the drawer, at its foot', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close, unstubbed } = await signedIn(browser, 375, 812);
  t.after(close);
  await page.goto(`${served.origin}/`);
  await page.getByRole('button', { name: 'Open the navigation' }).click();
  const drawer = page.locator('dialog[open]');
  await drawer.waitFor({ timeout: 10_000 });
  const keeping = drawer.getByRole('navigation', { name: 'What is kept' });
  assert.deepEqual((await keeping.getByRole('treeitem').allTextContents()).map((row) => row.trim()), ['Archive', 'Trash']);
  await keeping.getByRole('treeitem', { name: 'Archive' }).click();
  await page.waitForFunction(() => location.pathname === '/archive', undefined, { timeout: 5_000 });
  assert.deepEqual(failures, []);
  // Nothing was answered by a guess: a shape this fixture never prepared is a shape the walk
  // cannot claim to have exercised (`fixture.mjs`).
  assert.deepEqual(unstubbed(), []);
});
