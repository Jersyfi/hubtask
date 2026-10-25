// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The hub tree ranks by drag and by key (ux-inventory.md §E1): a hub among the hubs, a collection
// among its hub's collections, with the same writes the page menu's *Move up/down* sends and the
// same sentence after them. A reader whose role may not shape the workspace gets no grip and no
// key. Chromium only, as the shell walk.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, PAGE, signedIn } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const id = (n) => `01a0e2e0-0000-7000-8000-0000001729${n}`;
const hub = (n, name) => ({ id: id(n), type: 'HUB', parent_id: null, name, order_key: `a${n}`, version: 1 });
const collection = (n, name, parent) => ({ id: id(n), type: 'COLLECTION', parent_id: parent, name, order_key: `a${n}`, version: 1 });

/**
 * A workspace of three hubs, the first holding three collections, whose order the walk's own
 * server keeps: a rank write changes what the next read answers, so the tree is seen to move only
 * when the server says so. Every write is recorded as it was sent.
 */
async function workspace(page, { role = 'OWNER' } = {}) {
  const garden = hub('01', 'Garden');
  let hubs = [garden, hub('02', 'Studio'), hub('03', 'Travel')];
  let beds = [collection('11', 'Beds', garden.id), collection('12', 'Tools', garden.id), collection('13', 'Seeds', garden.id)];
  const writes = [];
  const placed = (level, moving, before) => {
    const rest = level.filter((each) => each.id !== moving);
    const at = before === null ? rest.length : rest.findIndex((each) => each.id === before);
    rest.splice(at, 0, level.find((each) => each.id === moving));
    return rest;
  };

  await page.route('**/api/v1/**', (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    if (path.endsWith('/api/v1/containers') && request.method() === 'GET') {
      if (url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: hubs } });
      if (url.searchParams.get('parent_id') === garden.id) return route.fulfill({ json: { ...PAGE, data: beds } });
      return route.fulfill({ json: { ...PAGE, data: [] } });
    }
    const ranked = path.match(/\/api\/v1\/containers\/([^/:]+):(reorder|move)$/);
    if (ranked && request.method() === 'POST') {
      const [, moving, verb] = ranked;
      const body = request.postDataJSON();
      writes.push({ moving, verb, ...body });
      if (verb === 'reorder') hubs = placed(hubs, moving, body.before_container_id);
      else beds = placed(beds, moving, body.before_container_id);
      return route.fulfill({ json: [...hubs, ...beds].find((each) => each.id === moving) });
    }
    if (path.endsWith('/api/v1/memberships') && role !== 'OWNER') {
      const rows = url.searchParams.get('scope_type') === 'TENANT' ? [{ id: 'm-1729', scope_type: 'TENANT', account_id: ACCOUNT.id, role }] : [];
      return route.fulfill({ json: { ...PAGE, data: rows } });
    }
    return route.fallback();
  });

  await page.goto(`${served.origin}/`);
  const tree = page.getByRole('navigation', { name: 'Workspace' }).first().getByRole('tree');
  await tree.getByRole('treeitem', { name: 'Travel' }).waitFor({ timeout: 15_000 });
  return {
    tree,
    writes,
    order: async (names) => {
      await page.waitForFunction(
        (wanted) => {
          const drawn = [...document.querySelectorAll('[role="tree"] [role="treeitem"]')].map((row) => row.textContent.trim());
          return wanted.every((name, index) => index === 0 || drawn.indexOf(name) > drawn.indexOf(wanted[index - 1]));
        },
        names,
        { timeout: 5_000 },
      ).catch(async () => assert.fail(`the tree is not ${names.join(', ')}`));
    },
  };
}

const said = (page) => page.locator('.announcement[role="status"]');

test('chromium: Alt+arrows rank a hub, the focus stays with it, and the move is said', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  const { tree, writes, order } = await workspace(page);

  const studio = tree.getByRole('treeitem', { name: 'Studio' });
  assert.equal(await studio.getAttribute('aria-keyshortcuts'), 'Alt+ArrowUp Alt+ArrowDown');
  await studio.focus();
  await page.keyboard.press('Alt+ArrowUp');
  await order(['Studio', 'Garden', 'Travel']);
  assert.deepEqual(writes, [{ moving: id('02'), verb: 'reorder', before_container_id: id('01') }]);
  await said(page).filter({ hasText: 'Studio is now number 1 of 3.' }).waitFor({ timeout: 5_000 });
  await page.waitForFunction((wanted) => document.activeElement?.getAttribute('data-node') === wanted, id('02'), { timeout: 5_000 })
    .catch(() => assert.fail('the focus did not stay with the hub it ranked'));

  // Already first: nothing is sent, and the focus does not wander off to the row above.
  await page.keyboard.press('Alt+ArrowUp');
  await page.waitForTimeout(300);
  assert.equal(writes.length, 1);
  assert.equal(await page.evaluate(() => document.activeElement?.getAttribute('data-node')), id('02'));

  // Shift goes to the end of the level.
  await page.keyboard.press('Alt+Shift+ArrowDown');
  await order(['Garden', 'Travel', 'Studio']);
  assert.deepEqual(writes[1], { moving: id('02'), verb: 'reorder', before_container_id: null });
  await said(page).filter({ hasText: 'Studio is now number 3 of 3.' }).waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('chromium: a hub dragged by its row lands where it is let go, and letting go opens nothing', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  const { tree, writes, order } = await workspace(page);

  const garden = tree.getByRole('treeitem', { name: 'Garden' });
  // The row is the handle: no grip beside it to aim for.
  assert.equal(await garden.locator('[data-grip]').count(), 0);

  const from = await garden.locator('.label').boundingBox();
  const travel = await tree.getByRole('treeitem', { name: 'Travel' }).boundingBox();
  const path = page.url();
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
  await page.mouse.down();
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2 + 10, { steps: 3 });
  await page.mouse.move(from.x + from.width / 2, travel.y + travel.height + 4, { steps: 8 });
  // Carried, and the slot it would take outlined.
  assert.equal(await garden.getAttribute('data-dragging'), '');
  assert.equal(await tree.getByRole('treeitem', { name: 'Travel' }).getAttribute('data-drop'), '');
  await page.mouse.up();

  await order(['Studio', 'Travel', 'Garden']);
  assert.deepEqual(writes, [{ moving: id('01'), verb: 'reorder', before_container_id: null }]);
  await said(page).filter({ hasText: 'Garden is now number 3 of 3.' }).waitFor({ timeout: 5_000 });
  assert.equal(page.url(), path, 'letting go of the drag opened the hub');

  // A press on the row that does not travel is the row's own click, and writes nothing.
  await tree.getByRole('treeitem', { name: 'Studio' }).locator('.label').click();
  await page.waitForURL(`**/hubs/${id('02')}`, { timeout: 5_000 });
  assert.equal(writes.length, 1);
  assert.deepEqual(failures, []);
});

test('chromium: a collection ranks among its hub’s collections by the same move the page menu sends', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  const { tree, writes, order } = await workspace(page);

  const twist = tree.getByRole('button', { name: 'Show what is in Garden' });
  await twist.waitFor({ timeout: 5_000 });
  await twist.click();
  const beds = tree.getByRole('treeitem', { name: 'Beds' });
  await beds.waitFor({ timeout: 5_000 });
  await beds.focus();
  await page.keyboard.press('Alt+ArrowDown');
  await order(['Tools', 'Beds', 'Seeds']);
  assert.deepEqual(writes, [{ moving: id('11'), verb: 'move', target_parent_id: id('01'), before_container_id: id('13') }]);
  await said(page).filter({ hasText: 'Beds is now number 2 of 3.' }).waitFor({ timeout: 5_000 });

  // The level is the hub's: dragging a collection far past the last hub still lands it last among
  // the collections, and never among the hubs.
  const seeds = tree.getByRole('treeitem', { name: 'Seeds' });
  const from = await seeds.locator('.label').boundingBox();
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
  await page.mouse.down();
  await page.mouse.move(from.x + from.width / 2, from.y - 60, { steps: 10 });
  await page.mouse.up();
  await order(['Seeds', 'Tools', 'Beds']);
  assert.deepEqual(writes[1], { moving: id('13'), verb: 'move', target_parent_id: id('01'), before_container_id: id('12') });
  assert.deepEqual(failures, []);
});

test('chromium: a reader whose role may not shape the workspace has no grip and no rank key (P-05)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  const { tree, writes } = await workspace(page, { role: 'VIEWER' });

  await page.waitForTimeout(500);
  assert.equal(await tree.locator('[data-grip]').count(), 0);
  const studio = tree.getByRole('treeitem', { name: 'Studio' });
  assert.equal(await studio.getAttribute('aria-keyshortcuts'), null);
  await studio.focus();
  await page.keyboard.press('Alt+ArrowUp');
  await page.waitForTimeout(300);
  assert.deepEqual(writes, []);
  assert.deepEqual(failures, []);
});
