// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *Stock* of a hub's inventory as the work surface draws it (§6.10, §6.11, scene invlist): five tabs
// with the firing alerts counted, every former sub-page's address landing on its tab, the query line
// with the `asset.` keys narrowing the hub's objects on the device, the object-type tree with its
// counts and the saved groups beside it, the table's columns with the open entries, a line saved as
// a group, nothing to write for a reader, and rows that stack read-only on a phone
// (UC-AST-01 checks 8 and 10, UC-AST-11 check 5).
//
// Chromium only: what this asserts is the rendered screen.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, HUB, MANIFEST, OTHER, PAGE, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const ASSET_KEYS = [
  { key: 'asset.class', field: 'asset.class', value: 'choice', product: 'asset', objects: ['asset'], examples: ['asset.class: server, laptop'], values: [] },
  { key: 'asset.stage', field: 'asset.stage', value: 'choice', product: 'asset', objects: ['asset'], examples: ['asset.stage: in_use'], values: ['planned', 'ordered', 'in_stock', 'in_use', 'maintenance', 'retired', 'disposed'] },
  { key: 'asset.owner', field: 'asset.owner', value: 'person', product: 'asset', objects: ['asset'], examples: ['asset.owner: none'], values: ['me', 'none'] },
  { key: 'asset.location', field: 'asset.location', value: 'relation', product: 'asset', objects: ['asset'], examples: ['asset.location: none'], values: ['none'] },
  { key: 'asset.tag', field: 'asset.tag', value: 'text', product: 'asset', objects: ['asset'], examples: ['asset.tag: INV-0042'], values: ['none'] },
];
const WITH_KEYS = { ...MANIFEST, query_keys: [...MANIFEST.query_keys, ...ASSET_KEYS] };

const cls = (key, name, lineage, extra = {}) => ({
  key, name, parent: lineage[1] ?? '', system: false, lineage, version: 1, attributes: [], identifier_rules: [], required_relations: [],
  staleness_days: 0, all_attributes: [], ...extra,
});
const CLASSES = [
  cls('ci', 'Object', ['ci'], { abstract: true }),
  cls('hardware', 'Hardware', ['hardware', 'ci'], { abstract: true }),
  cls('server', 'Server', ['server', 'hardware', 'ci']),
  cls('laptop', 'Laptop', ['laptop', 'hardware', 'ci']),
  cls('service', 'Services', ['service', 'ci'], { abstract: true }),
  cls('application_service', 'Application service', ['application_service', 'service', 'ci']),
  cls('location', 'Locations', ['location', 'ci'], { abstract: true }),
  cls('room', 'Room', ['room', 'location', 'ci']),
  // A type with nothing in it, and nothing beneath it, stays out of the tree.
  cls('printer', 'Printer', ['printer', 'hardware', 'ci']),
];
const id = (n) => `01a0e2e0-0000-7000-8000-0000000d${String(n).padStart(4, '0')}`;
const ci = (n, klass, name, extra = {}) => ({
  id: id(n), hub_id: HUB.id, class: klass, name, stage: 'IN_USE', status: 'OK', attributes: {}, provenance: {}, protection: {}, quantity: 1,
  created_at: '2026-10-01T08:00:00Z', updated_at: '2026-10-01T08:00:00Z', version: 1, ...extra,
});
const ROOM = ci(8, 'room', 'RZ Hamburg', { owner_account_id: ACCOUNT.id });
const CIS = [
  ci(1, 'application_service', 'Portal-API', { owner_account_id: OTHER.id, protection: { c: 'HIGH', a: 'NORMAL' } }),
  ci(2, 'application_service', 'Kundenportal', { owner_account_id: ACCOUNT.id, protection: { i: 'VERY_HIGH' } }),
  ci(3, 'server', 'db-01.hh.intern', { owner_account_id: ACCOUNT.id, location_id: ROOM.id }),
  ci(4, 'server', 'app-01.hh.intern', { stage: 'MAINTENANCE', owner_account_id: OTHER.id, location_id: ROOM.id }),
  ci(5, 'laptop', 'NB-0417', { owner_account_id: ACCOUNT.id }),
  ci(6, 'laptop', 'NB-0422'),
  ci(7, 'laptop', 'NB-0233', { stage: 'DISPOSED' }),
  ROOM,
];
const byName = (name) => CIS.find((each) => each.name === name);
const HEALTH = {
  total: CIS.length, completeness: 0.9, correctness: 1, stale: 0, orphans: 0, duplicates: 0, without_owner: 2, review_overdue: 1,
  classes: [], problems: [{ ci_id: byName('NB-0422').id, class: 'laptop', kinds: ['NO_OWNER'] }],
  stale_relations: 0, services: [], sources: [],
};
const ALERTS = [
  { id: id(901), hub_id: HUB.id, connector_id: id(900), key: 'latency', status: 'FIRING', severity: 'critical', title: 'Latency high', first_at: '2026-11-04T08:00:00Z', last_at: '2026-11-04T10:00:00Z', count: 2, ci_id: byName('Portal-API').id },
  { id: id(902), hub_id: HUB.id, connector_id: id(900), key: 'cpu', status: 'RESOLVED', severity: 'warning', title: 'CPU high', first_at: '2026-11-03T08:00:00Z', last_at: '2026-11-03T09:00:00Z', resolved_at: '2026-11-03T09:30:00Z', count: 1 },
];
const WORK = { data: [{ ci_id: byName('Portal-API').id, open: 2 }, { ci_id: byName('db-01.hh.intern').id, open: 1 }], truncated: false };
const NO_OWNER_GROUP = {
  id: id(950), hub_id: HUB.id, key: 'no_owner', name: 'Without an owner', filter: { no_owner: true }, service_id: null,
  updated_at: '2026-10-01T08:00:00Z', version: 1, members: 2,
};
const REGISTERED = [
  { ci_id: byName('NB-0417').id, hub_id: HUB.id, asset_tag: 'NB-0417', cost_minor: 148000, currency: 'EUR', purchased_on: null, supplier: '', warranty_until: null, warranty_ended: false, contract_id: null, stockroom_id: null, assigned_to: ACCOUNT.id, request_id: null, state: 'DEPLOYED', disposal: null, updated_at: '2026-10-01T08:00:00Z', version: 1 },
];

async function open(browser, { width = 1280, height = 900, path, role = 'OWNER' } = {}) {
  const { page, context, failures } = await signedIn(browser, width, height);
  const written = [];
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const at = url.pathname.replace('/api/v1', '');
    if (at === '/meta/capabilities') return route.fulfill({ json: role === 'OWNER' ? WITH_KEYS : { ...WITH_KEYS, workspace_permissions: ['READ'] } });
    if (role !== 'OWNER' && at === '/memberships') {
      const scope = url.searchParams.get('scope_type');
      const rows = scope === 'HUB' ? [{ id: 'm2', scope_type: 'HUB', scope_id: HUB.id, account_id: ACCOUNT.id, role }] : scope === 'TENANT' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role }] : [];
      return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: [] } });
    }
    if (at === '/cmdb/classes') return route.fulfill({ json: { data: CLASSES } });
    if (at === `/hubs/${HUB.id}/cis`) return route.fulfill({ json: { data: CIS, next: null } });
    if (at === `/hubs/${HUB.id}/cmdb-health`) return route.fulfill({ json: HEALTH });
    if (at === `/hubs/${HUB.id}/ci-alerts`) {
      const status = url.searchParams.get('status');
      return route.fulfill({ json: { data: ALERTS.filter((each) => !status || each.status === status) } });
    }
    if (at === `/hubs/${HUB.id}/ci-work`) return route.fulfill({ json: WORK });
    if (at === `/hubs/${HUB.id}/ci-groups`) return route.fulfill({ json: { data: [NO_OWNER_GROUP] } });
    if (at.startsWith(`/hubs/${HUB.id}/ci-groups/`) && request.method() === 'PUT') {
      const body = request.postDataJSON();
      written.push({ at, body });
      return route.fulfill({ json: { ...NO_OWNER_GROUP, id: id(951), key: decodeURIComponent(at.split('/').pop()), name: body.name, filter: body.filter, members: 1 } });
    }
    if (at === `/hubs/${HUB.id}/assets`) return route.fulfill({ json: { data: REGISTERED, next: null } });
    if (at === `/hubs/${HUB.id}/software`) return route.fulfill({ json: { data: [] } });
    return stub(route);
  });
  await page.goto(`${served.origin}${path}`);
  return { page, context, failures, written };
}

const bar = (page) => page.getByRole('navigation', { name: 'Parts of the inventory' });
const line = (page) => page.getByRole('combobox', { name: 'Filter the objects' });
const rowNames = (page) => page.locator('[data-ci-row]').evaluateAll((rows) => rows.map((row) => row.querySelector('.title')?.textContent?.trim()));
async function rowsAre(page, names, what) {
  await page.waitForFunction((expected) => {
    const shown = [...document.querySelectorAll('[data-ci-row] .title')].map((each) => each.textContent.trim());
    return JSON.stringify(shown) === JSON.stringify(expected);
  }, names, { timeout: 5_000 }).catch(() => undefined);
  assert.deepEqual(await rowNames(page), names, what);
}
/** Empties the line by its chips' own *Remove*, then types a new one. */
async function typeLine(page, text) {
  for (let left = await page.locator('[data-inventory-line] .remove button').count(); left > 0; left -= 1) {
    await page.locator('[data-inventory-line] .remove button').first().click();
  }
  await line(page).fill(text);
}
const ALL_BUT_DISPOSED = ['Portal-API', 'Kundenportal', 'db-01.hh.intern', 'app-01.hh.intern', 'NB-0417', 'NB-0422', 'RZ Hamburg'];

test('chromium: the inventory is five tabs, the firing alerts counted, the figure from the health', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, { path: `/hubs/${HUB.id}/inventory` });
  t.after(() => context.close());

  await bar(page).getByRole('link').first().waitFor({ timeout: 20_000 });
  const tabs = (await bar(page).getByRole('link').allTextContents()).map((each) => each.replace(/\s+/gu, ' ').trim());
  await bar(page).getByRole('link', { name: /Alerts\s*1/ }).waitFor({ timeout: 5_000 });
  assert.deepEqual((await bar(page).getByRole('link').allTextContents()).map((each) => each.replace(/\s+/gu, ' ').trim().replace(/ ?(\d+)$/u, ' · $1')),
    ['Stock', 'Map', 'Health', 'Alerts · 1', 'Upkeep'], `the tabs are ${tabs}`);
  assert.equal(await page.locator('[data-assets-manage]').count(), 0, 'Manage is still drawn');
  assert.equal(await bar(page).getByRole('link', { name: 'Stock' }).getAttribute('aria-current'), 'page');
  const figure = page.locator('[data-assets-figure]');
  await figure.waitFor({ timeout: 5_000 });
  assert.match((await figure.innerText()).replace(/\s+/gu, ' '), /8 objects · 1 needs attention · 1 review overdue · Completeness 90 %/);
  // The head's two acts (scene invlist): the import beside the create.
  await page.getByRole('button', { name: 'Import', exact: true }).click();
  await page.waitForFunction(() => location.pathname.endsWith('/inventory/import'), null, { timeout: 5_000 });
  assert.equal(await bar(page).getByRole('link', { name: 'Upkeep' }).getAttribute('aria-current'), 'page');
  assert.deepEqual(failures, []);
});

test('chromium: the former sub-pages\' addresses land on their tabs', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, { path: `/hubs/${HUB.id}/inventory/runs` });
  t.after(() => context.close());

  const current = async () => (await bar(page).locator('a[aria-current="page"]').textContent()).trim().replace(/\s*\d+$/u, '');
  for (const [section, tab, mark] of [
    ['runs', 'Upkeep', '[data-care-part=runs]'],
    ['duplicates', 'Upkeep', '[data-care-part=duplicates]'],
    ['connectors', 'Upkeep', '[data-care-part=connectors]'],
    ['campaigns', 'Upkeep', '[data-care-part=campaigns]'],
    ['import', 'Upkeep', '[data-care-part=import]'],
    ['care', 'Upkeep', '[data-care-part=runs]'],
    ['groups', 'Stock', '[data-inventory-saved]'],
    ['register', 'Stock', '[data-register]'],
    ['software', 'Stock', '[data-stock-parts]'],
    ['map', 'Map', '[data-inventory-tab=map]'],
    ['health', 'Health', '[data-inventory-tab=health]'],
    ['alerts', 'Alerts', '[data-inventory-tab=alerts]'],
  ]) {
    await page.goto(`${served.origin}/hubs/${HUB.id}/inventory/${section}`);
    await page.locator(mark).first().waitFor({ timeout: 10_000 });
    assert.equal(await current(), tab, `/inventory/${section}`);
  }
  // The register and the software are Stock's switch, each part with its count.
  await page.goto(`${served.origin}/hubs/${HUB.id}/inventory/register`);
  const register = page.getByRole('radio', { name: 'Devices and licences · 1' });
  await register.waitFor({ timeout: 10_000 });
  assert.equal(await register.getAttribute('aria-checked'), 'true');
  await page.getByRole('radio', { name: 'All objects' }).click();
  await page.waitForFunction(() => location.pathname.endsWith('/inventory'), null, { timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('chromium: the line narrows by object type, stage, owner none and words; a saved group fills it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, { path: `/hubs/${HUB.id}/inventory` });
  t.after(() => context.close());

  await page.locator('[data-ci-row]').first().waitFor({ timeout: 20_000 });
  // The line opens without the disposed, its chip in the reader's words.
  assert.match(await page.locator('[data-inventory-line]').innerText(), /Stage[\s\S]*Disposed of/);
  await rowsAre(page, ALL_BUT_DISPOSED, 'the default line');

  await typeLine(page, 'asset.class: hardware ');
  await rowsAre(page, ['db-01.hh.intern', 'app-01.hh.intern', 'NB-0417', 'NB-0422', 'NB-0233'], 'asset.class takes the types beneath');
  await typeLine(page, 'asset.stage: maintenance ');
  await rowsAre(page, ['app-01.hh.intern'], 'asset.stage');
  await typeLine(page, '-asset.stage: in_use asset.class: laptop ');
  await rowsAre(page, ['NB-0233'], 'a key turned round');
  await typeLine(page, 'asset.owner: none ');
  await rowsAre(page, ['NB-0422', 'NB-0233'], 'asset.owner: none');
  await typeLine(page, 'asset.owner: me asset.location: none ');
  await rowsAre(page, ['Kundenportal', 'NB-0417', 'RZ Hamburg'], 'me and asset.location: none');
  // Words narrow while they are typed, before they are a chip (Q-4).
  await typeLine(page, 'hh.intern');
  await rowsAre(page, ['db-01.hh.intern', 'app-01.hh.intern'], 'words in the name');
  await typeLine(page, 'nothing-is-called-so');
  await page.getByText('No object matches the line').waitFor({ timeout: 5_000 });
  await page.getByRole('button', { name: 'Clear the line' }).click();
  await rowsAre(page, [...ALL_BUT_DISPOSED.slice(0, 6), 'NB-0233', 'RZ Hamburg'], 'a cleared line');

  // A saved group puts its filter in the line.
  await page.locator('[data-group=no_owner] .tree-row').click();
  await rowsAre(page, ['NB-0422', 'NB-0233'], 'the saved group');
  assert.match(await page.locator('[data-inventory-line]').innerText(), /Owner[\s\S]*nobody/);
  assert.equal(await page.locator('[data-group=no_owner] .tree-row').getAttribute('aria-current'), 'true');
  assert.deepEqual(failures, []);
});

test('chromium: the object-type tree counts the types beneath within the line, and a type narrows', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, { path: `/hubs/${HUB.id}/inventory` });
  t.after(() => context.close());

  const tree = page.locator('[data-inventory-tree]');
  await tree.locator('[data-tree-class]').first().waitFor({ timeout: 20_000 });
  const counts = async () => tree.locator('[data-tree-class]').evaluateAll((rows) => rows.map((row) => `${row.getAttribute('data-tree-class') || 'all'}=${row.querySelector('.tree-count')?.textContent}`));
  // The disposed laptop is not counted under the default line; the empty printer is not drawn.
  assert.deepEqual(await counts(), ['all=7', 'hardware=4', 'server=2', 'laptop=2', 'service=2', 'application_service=2', 'location=1', 'room=1']);
  await tree.locator('[data-tree-class=hardware]').click();
  await rowsAre(page, ['db-01.hh.intern', 'app-01.hh.intern', 'NB-0417', 'NB-0422'], 'the tree\'s type');
  assert.match(await page.locator('[data-inventory-line]').innerText(), /Object type[\s\S]*Hardware/);
  assert.equal(await tree.locator('[data-tree-class=hardware]').getAttribute('aria-current'), 'true');
  // The tree counts within the rest of the line, its own type aside.
  assert.deepEqual(await counts(), ['all=7', 'hardware=4', 'server=2', 'laptop=2', 'service=2', 'application_service=2', 'location=1', 'room=1']);
  await tree.locator('[data-tree-class=""]').click();
  await rowsAre(page, ALL_BUT_DISPOSED, 'All');
  // Saved groups with their counts.
  assert.match(await page.locator('[data-group=no_owner]').innerText(), /Without an owner\s*2/);
  assert.deepEqual(failures, []);
});

test('chromium: the table\'s columns - type, stage, owner, location, protection and the open entries', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, { path: `/hubs/${HUB.id}/inventory` });
  t.after(() => context.close());

  const table = page.getByRole('table', { name: 'Objects' });
  await table.waitFor({ timeout: 20_000 });
  assert.deepEqual((await table.locator('thead th').allTextContents()).map((each) => each.trim()).filter(Boolean),
    ['Name', 'Object type', 'Stage', 'Owner', 'Location', 'Protection', 'Open']);
  const row = (name) => table.locator('tr', { has: page.locator('[data-ci-row]', { hasText: name }) });
  // The open entries from ci-work; none is a dash.
  await row('Portal-API').locator('[data-open-count="2"]').waitFor({ timeout: 5_000 });
  await row('db-01.hh.intern').locator('[data-open-count="1"]').waitFor({ timeout: 5_000 });
  assert.equal(await row('NB-0417').locator('[data-open-count]').count(), 0);
  // Marked where health finds it wanting or an alert fires; the name alone names the link.
  assert.equal(await row('Portal-API').locator('[data-wanting]').count(), 1);
  assert.equal(await row('NB-0422').locator('[data-wanting]').count(), 1);
  assert.equal(await row('NB-0417').locator('[data-wanting]').count(), 0);
  await page.getByRole('link', { name: 'Portal-API', exact: true }).waitFor();
  const text = async (name) => (await row(name).innerText()).replace(/\s+/gu, ' ');
  assert.match(await text('db-01.hh.intern'), /Server In use .* RZ Hamburg –/);
  assert.match(await text('Portal-API'), /Application service In use .* high 2/);
  assert.match(await text('Kundenportal'), /very high/);
  assert.match(await text('NB-0422'), /Laptop In use nobody – –/);
  assert.equal(await row('NB-0422').locator('[data-nobody]').count(), 1);
  // The row opens the object.
  assert.equal(await page.getByRole('link', { name: 'Portal-API', exact: true }).getAttribute('href'), `/cis/${byName('Portal-API').id}`);

  // By object type: the rows grouped under their type.
  await page.getByRole('radio', { name: 'By object type' }).click();
  await page.locator('tr', { hasText: 'Laptop' }).first().waitFor();
  assert.ok((await table.innerText()).indexOf('Application service') < (await table.innerText()).indexOf('Kundenportal'));
  // Columns: a column hidden stays hidden on this device.
  await page.locator('[data-inventory-columns]').click();
  await page.getByRole('menuitem', { name: 'Protection' }).click();
  await page.reload();
  await page.getByRole('table', { name: 'Objects' }).waitFor({ timeout: 20_000 });
  assert.ok(!(await page.getByRole('table', { name: 'Objects' }).locator('thead').innerText()).includes('Protection'), 'the hidden column came back');
  assert.equal(await page.getByRole('radio', { name: 'By object type' }).getAttribute('aria-checked'), 'true');
  assert.deepEqual(failures, []);
});

test('chromium: a line is saved as a group, and a line a group cannot hold says why', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures, written } = await open(browser, { path: `/hubs/${HUB.id}/inventory` });
  t.after(() => context.close());

  await page.locator('[data-ci-row]').first().waitFor({ timeout: 20_000 });
  // The default line holds a negated key: the dialog says it is left out.
  await page.locator('[data-save-group]').click();
  const dialog = page.locator('dialog[open]');
  await dialog.locator('[data-group-note]').waitFor({ timeout: 5_000 });
  assert.match(await dialog.locator('[data-group-note]').innerText(), /minus/);
  assert.equal(await dialog.getAttribute('data-size'), 'l');
  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });

  await typeLine(page, 'asset.class: laptop asset.owner: none ');
  await rowsAre(page, ['NB-0422', 'NB-0233'], 'the line to save');
  await page.locator('[data-save-group]').click();
  await dialog.waitFor({ timeout: 5_000 });
  assert.equal(await dialog.locator('[data-group-note]').count(), 0);
  assert.equal(await dialog.getByRole('checkbox', { name: 'Laptop' }).isChecked(), true);
  assert.match(await dialog.locator('[data-group-kept]').innerText(), /Owner: nobody/);
  await dialog.getByLabel('Key', { exact: false }).first().fill('laptops_unowned');
  await dialog.getByRole('textbox', { name: /^Name/ }).fill('Laptops without an owner');
  await dialog.getByRole('button', { name: 'Save' }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.equal(written.length, 1);
  assert.equal(written[0].at, `/hubs/${HUB.id}/ci-groups/laptops_unowned`);
  assert.deepEqual(written[0].body, { name: 'Laptops without an owner', filter: { classes: ['laptop'], no_owner: true }, service_id: null });
  assert.deepEqual(failures, []);
});

test('chromium: a reader is offered no create, no import and no group to save', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, { path: `/hubs/${HUB.id}/inventory`, role: 'VIEWER' });
  t.after(() => context.close());

  await page.locator('[data-ci-row]').first().waitFor({ timeout: 20_000 });
  await page.locator('[data-group=no_owner]').waitFor({ timeout: 5_000 });
  assert.equal(await page.getByRole('button', { name: 'New object', exact: true }).count(), 0);
  assert.equal(await page.getByRole('button', { name: 'Import', exact: true }).count(), 0);
  assert.equal(await page.locator('[data-save-group]').count(), 0);
  assert.equal(await page.getByRole('button', { name: /^More for / }).count(), 0);
  assert.equal(await bar(page).getByRole('link', { name: 'Upkeep' }).count(), 0);
  // Reading and narrowing stay theirs.
  await page.locator('[data-group=no_owner] .tree-row').click();
  await rowsAre(page, ['NB-0422', 'NB-0233'], 'the reader\'s saved group');
  assert.deepEqual(failures, []);
});

test('chromium 390 px: rows stack with their meta line, and nothing writes', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, { width: 390, height: 844, path: `/hubs/${HUB.id}/inventory` });
  t.after(() => context.close());

  const meta = page.locator('[data-ci-meta]').first();
  await meta.waitFor({ timeout: 20_000 });
  assert.match((await meta.innerText()).replace(/\s+/gu, ' '), /Application service · In use · .+ · 2 open/);
  assert.equal(await page.locator('[data-inventory-tree]').count(), 0, 'the tree takes a column on a phone');
  // The tree folds into one choice.
  await page.getByLabel('Object type or saved group').selectOption({ label: '· Laptop · 2' });
  await rowsAre(page, ['NB-0417', 'NB-0422'], 'the phone\'s choice');
  assert.equal(await page.getByRole('button', { name: 'New object', exact: true }).count(), 0);
  assert.equal(await page.getByRole('button', { name: 'Import', exact: true }).count(), 0);
  assert.equal(await page.locator('[data-save-group]').count(), 0);
  const wide = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(wide <= 1, `the stock scrolls sideways by ${wide} px`);
  assert.deepEqual(failures, []);
});
