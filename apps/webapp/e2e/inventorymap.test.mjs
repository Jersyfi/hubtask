// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *Map* of a hub's inventory as the work surface draws it (§6.10, scene invmap, UC-AST-11 check 5):
// (1) one service at a time - the worst-standing first -, how far below it, *only problems* and the
// legend; (2) the incident and the change on the map, the way of the disruption dashed, a stage
// beside its object, the picture as a list; (3) what is open on the service and the changes planned
// on it with their collisions; *All services* keeps the hub's layered map; read only and in its own
// frame on a phone. `SHOTS=<dir>` writes the screenshots the mockup is compared with.
//
// Chromium only: what this asserts is the rendered screen.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { serve } from './serve.mjs';
import { HUB } from './fixture.mjs';
import { CIS, bar, id, noSidewaysScroll, openInventory } from './inventoryseed.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());
const SHOTS = process.env.SHOTS;

const names = (page, selector) => page.locator(selector).evaluateAll((all) => all.map((each) => each.querySelector('a')?.textContent?.trim()));
const column = (page, depth) => names(page, `[data-map-depth="${depth}"] [data-map-node]`);

test('chromium: scene invmap - a service by depth, its disruption and its change, open and planned', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures } = await openInventory(browser, served.origin, { path: `/hubs/${HUB.id}/inventory/map` });
  t.after(() => context.close());

  // (1) The worst-standing service first, three steps deep, the legend at the end of the tools.
  const graph = page.locator('[data-service-graph]');
  await graph.waitFor({ timeout: 20_000 });
  assert.equal(await bar(page).getByRole('link', { name: 'Map' }).getAttribute('aria-current'), 'page');
  const service = page.getByRole('combobox', { name: 'Service' });
  assert.equal(await service.locator('option:checked').textContent(), 'Service: Kundenportal');
  assert.deepEqual((await service.locator('option').allTextContents()), ['Service: Kundenportal', 'Service: E-Mail & Kalender', 'All services']);
  assert.equal(await page.getByRole('radio', { name: '3' }).getAttribute('aria-checked'), 'true');
  assert.match(await page.locator('[data-map-legend]').innerText(), /red = open disruption · dashed = path of the disruption/);
  // The scene's warning mark is the icon, not a typed exclamation mark.
  assert.equal(await page.locator('[data-map-legend] svg').count(), 1);
  assert.doesNotMatch(await page.locator('[data-map-legend]').innerText(), /!/u);

  // (2) The service at the start, what it rests on by steps below it.
  await page.locator('[data-map-line]').first().waitFor({ state: 'attached', timeout: 5_000 });
  assert.deepEqual(await column(page, 0), ['Kundenportal']);
  assert.deepEqual(await column(page, 1), ['Portal-API', 'Portal-Frontend']);
  assert.deepEqual(await column(page, 2), ['app-01', 'db-01.hh.intern', 'lb-01.hh.intern']);
  assert.deepEqual(await column(page, 3), ['app-02.hh.intern', 'RZ Hamburg · Rack 4', 'VPN-Gateway Frankfurt']);
  // The incident at db-01 and at the service itself, the change at the gateway, the maintenance as a lozenge.
  const db = page.locator(`[data-map-node="${CIS.db.id}"]`);
  assert.equal(await db.getAttribute('data-struck'), '');
  assert.equal((await db.locator('[data-map-struck]').innerText()).trim().split('\n').pop(), 'SA-12');
  assert.equal(await db.locator('[data-map-struck]').getAttribute('href'), `/items/${id(100)}`);
  assert.equal(await page.locator(`[data-map-node="${CIS.portal.id}"]`).getAttribute('data-struck'), '');
  assert.match(await page.locator(`[data-map-node="${CIS.vpn.id}"] [data-map-planned]`).innerText(), /CHA-1/);
  assert.match(await page.locator(`[data-map-node="${CIS.app.id}"]`).innerText(), /In maintenance/);
  // The way of the disruption: service → Portal-API → db-01, dashed.
  assert.deepEqual((await page.locator('[data-map-line][data-dashed]').evaluateAll((all) => all.map((each) => each.getAttribute('data-map-line')))).sort(),
    [`${CIS.api.id}>${CIS.db.id}:depends_on`, `${CIS.portal.id}>${CIS.api.id}:depends_on`].sort());
  // Each object opens its page; the map is reached by the keyboard.
  assert.equal(await db.getByRole('link', { name: 'db-01.hh.intern' }).getAttribute('href'), `/cis/${CIS.db.id}`);
  await db.getByRole('link', { name: 'db-01.hh.intern' }).focus();
  assert.equal(await page.evaluate(() => document.activeElement?.textContent?.trim()), 'db-01.hh.intern');

  // (3) What is open on the service, through what, and what is planned with its collisions.
  const open = page.locator('[data-map-open]');
  assert.match(await open.locator('[data-map-entry="SA-12"]').innerText(), /SA-12\s+Anmeldung bricht mit Gateway Timeout ab[\s\S]*via db-01\.hh\.intern/);
  // Each card heads with its product's tile; an open entry is one line - tile, key, title, through what.
  assert.equal(await open.locator('h2 [data-product="desk"]').count(), 1);
  assert.equal(await page.locator('[data-map-changes] h2 [data-product="plans"]').count(), 1);
  const sa12 = open.locator('[data-map-entry="SA-12"]');
  assert.equal(await sa12.locator('[data-product="desk"]').count(), 1);
  const [rowBox, keyBox] = await Promise.all([sa12.boundingBox(), sa12.getByText('SA-12', { exact: true }).boundingBox()]);
  assert.ok(rowBox.height < keyBox.height * 2.2, `the open entry takes ${Math.round(rowBox.height)} px, more than one line`);
  assert.match(await open.locator('[data-map-entry="SA-8"]').innerText(), /direct/);
  assert.equal(await open.locator('[data-map-entry="SA-3"]').count(), 0, 'a completed entry is listed');
  const changes = page.locator('[data-map-changes]');
  await changes.locator('[data-collides]').nth(1).waitFor({ timeout: 5_000 });
  assert.deepEqual(await changes.locator('[data-map-entry]').evaluateAll((all) => all.map((each) => each.getAttribute('data-map-entry'))), ['CHA-1', 'CHA-2']);
  assert.match(await changes.locator('[data-map-entry="CHA-1"]').innerText(), /Collides with no other change\./);
  assert.match(await changes.locator('[data-map-entry="CHA-2"]').innerText(), /Collides with CHA-9 Speicher erweitern/);
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/invmap.1280.png`, fullPage: true });

  // Depth: one step shows the service and what it rests on directly.
  await page.getByRole('radio', { name: '1 step' }).click();
  await page.waitForFunction(() => document.querySelectorAll('[data-map-depth]').length === 2, null, { timeout: 5_000 });
  assert.deepEqual(await column(page, 1), ['Portal-API', 'Portal-Frontend']);
  await page.getByRole('radio', { name: '3' }).click();

  // Only problems: only the ways to the struck, the planned and the wanting.
  await page.getByRole('switch', { name: 'Only problems' }).check();
  await page.waitForFunction((gone) => !document.querySelector(`[data-map-node="${gone}"]`), CIS.app2.id, { timeout: 5_000 });
  assert.deepEqual(await column(page, 1), ['Portal-API', 'Portal-Frontend']);
  assert.deepEqual(await column(page, 2), ['db-01.hh.intern', 'lb-01.hh.intern']);
  assert.deepEqual(await column(page, 3), ['VPN-Gateway Frankfurt']);
  await page.getByRole('switch', { name: 'Only problems' }).uncheck();

  // The picture as a list (§8: every chart has a table).
  await page.getByRole('switch', { name: 'As a list' }).check();
  const list = page.getByRole('table', { name: 'What Kundenportal rests on' });
  await list.waitFor({ timeout: 5_000 });
  assert.equal(await list.getByRole('row').filter({ hasText: 'db-01.hh.intern' }).filter({ hasText: 'SA-12' }).count(), 1);
  assert.equal(await page.locator('[data-service-graph]').count(), 0);
  await page.getByRole('switch', { name: 'As a list' }).uncheck();

  // Another service, and all of them as the hub's layered map.
  await service.selectOption({ label: 'Service: E-Mail & Kalender' });
  await page.locator(`[data-map-node="${CIS.exchange.id}"]`).waitFor({ timeout: 5_000 });
  assert.equal(await page.locator(`[data-map-node="${CIS.db.id}"]`).count(), 0);
  assert.deepEqual(await column(page, 1), ['Exchange Online Connector']);
  assert.match(await page.locator('[data-map-open]').innerText(), /Nothing is open on this service\./);
  await service.selectOption({ label: 'All services' });
  await page.locator('[data-service-map]').waitFor({ timeout: 5_000 });
  assert.equal(await page.getByRole('radio', { name: '1 step' }).count(), 0, 'depth offered for the whole hub');
  assert.deepEqual(failures, []);
});

test('chromium 390 px: scene invmap on a phone - the picture scrolls in its frame, the cards stack, nothing writes', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures } = await openInventory(browser, served.origin, { width: 390, height: 844, path: `/hubs/${HUB.id}/inventory/map`, role: 'VIEWER' });
  t.after(() => context.close());

  const graph = page.locator('[data-service-graph]');
  await graph.waitFor({ timeout: 20_000 });
  await page.locator('[data-map-changes] [data-collides]').first().waitFor({ timeout: 5_000 });
  await noSidewaysScroll(page, 'the map on a phone', assert);
  assert.ok(await graph.evaluate((each) => each.scrollWidth > each.clientWidth), 'the picture does not scroll in its frame');
  const [openBox, changesBox] = await Promise.all([page.locator('[data-map-open]').boundingBox(), page.locator('[data-map-changes]').boundingBox()]);
  assert.ok(changesBox.y >= openBox.y + openBox.height, 'the cards stand side by side');
  assert.equal(await page.locator('[data-inventory-map] button:not([role=radio])').count(), 0, 'the map offers a button');
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/invmap.390.png`, fullPage: true });
  assert.deepEqual(failures, []);
});
