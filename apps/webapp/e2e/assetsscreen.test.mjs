// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A hub's Assets as user-experience.md §7.3 draws it: the inventory first under a head that says
// how it stands, the operator's sections behind *Manage* as a second strip that a reader is never
// shown - the register and the software, which only list, in the reader's first strip -, a create in the head's list that opens its section's own dialog, connectors as cards with
// one act and a menu, and alerts that say where they come from when there are none - with no
// sideways scroll on a phone, full or empty; and a CI's relations as a small graph beside the list
// (UC-AST-01 check 8, UC-AST-07 check 7, UC-AST-08 check 7, UC-AST-12 check 5).
//
// Chromium only: what this asserts is the rendered screen.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, HUB, MANIFEST, PAGE, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const SERVER_CLASS = {
  key: 'server', name: 'Server', parent: 'ci', system: false, lineage: ['ci', 'server'], version: 1,
  attributes: [], identifier_rules: [], required_relations: [], staleness_days: 0, all_attributes: [],
};
const ci = (n) => ({
  id: `01a0e2e0-0000-7000-8000-0000000c${String(n).padStart(4, '0')}`, hub_id: HUB.id, class: 'server',
  name: n === 1 ? 'a-server-with-a-very-long-host-name-that-must-not-push-the-row-sideways.example.org' : `web-${n}`,
  stage: n % 7 === 0 ? 'MAINTENANCE' : 'IN_USE', status: n % 5 === 0 ? 'PENDING' : 'OK',
  attributes: {}, provenance: {}, protection: {}, quantity: 1, created_at: '2026-10-01T08:00:00Z', updated_at: '2026-10-01T08:00:00Z', version: 1,
});
const MANY = Array.from({ length: 40 }, (_, index) => ci(index + 1));
const health = (total, wanting) => ({
  total, completeness: 0.8, correctness: 0.9, stale: 0, orphans: 0, duplicates: 0, without_owner: 0, review_overdue: 0,
  classes: [], problems: Array.from({ length: wanting }, (_, index) => ({ ci_id: MANY[index].id, class: 'server', kinds: ['STALE'] })),
  stale_relations: 0, services: [], sources: [],
});
const CONNECTOR_ID = '01a0e2e0-0000-7000-8000-0000000cc001';
const CONNECTORS = [
  {
    id: CONNECTOR_ID, hub_id: HUB.id, key: 'netbox-main', kind: 'NETBOX', source: 'netbox', push: false, inventory: false,
    base_url: 'https://netbox.example.org', settings: {}, allowed_hosts: [], hosts: [], every_minutes: 60, enabled: true, collection_id: null,
    has_credential: true, next_run_at: '2026-11-04T12:00:00Z', last_run_at: '2026-11-04T11:00:00Z', last_outcome: 'FAILED',
    last_error: 'connector.unreachable', failures: 3, created_at: '2026-10-01T08:00:00Z', updated_at: '2026-10-01T08:00:00Z', version: 1,
  },
  {
    id: '01a0e2e0-0000-7000-8000-0000000cc002', hub_id: HUB.id, key: 'alerts', kind: 'ALERTMANAGER', source: 'alertmanager', push: true,
    inventory: false, base_url: '', settings: {}, allowed_hosts: [], hosts: [], every_minutes: 0, enabled: true, collection_id: null,
    has_credential: false, last_outcome: '', failures: 0, created_at: '2026-10-01T08:00:00Z', updated_at: '2026-10-01T08:00:00Z', version: 1,
  },
];
const ALERTS = [
  { id: '01a0e2e0-0000-7000-8000-0000000ca001', hub_id: HUB.id, connector_id: CONNECTORS[1].id, key: 'disk_full', status: 'FIRING', severity: 'critical', title: 'Disk almost full on web-2', first_at: '2026-11-04T08:00:00Z', last_at: '2026-11-04T10:00:00Z', count: 4, ci_id: MANY[1].id },
  { id: '01a0e2e0-0000-7000-8000-0000000ca002', hub_id: HUB.id, connector_id: CONNECTORS[1].id, key: 'cpu', status: 'RESOLVED', severity: 'warning', title: 'CPU high', first_at: '2026-11-03T08:00:00Z', last_at: '2026-11-03T09:00:00Z', resolved_at: '2026-11-03T09:30:00Z', count: 1 },
];

const ASSET = {
  ci_id: MANY[1].id, hub_id: HUB.id, asset_tag: 'LT-0042', cost_minor: 129900, currency: 'EUR', purchased_on: '2026-01-15',
  supplier: 'A supplier', warranty_until: '2029-01-15', warranty_ended: false, contract_id: null, stockroom_id: null,
  assigned_to: null, request_id: null, state: 'IN_STOCK', disposal: null, updated_at: '2026-10-01T08:00:00Z', version: 1,
};

async function open(browser, { width = 1280, height = 900, path, full = true, role = 'OWNER', scheme = 'light', classes = [SERVER_CLASS], modulesOn }) {
  const { page, context, failures } = await signedIn(browser, width, height);
  await page.emulateMedia({ colorScheme: scheme });
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const at = url.pathname.replace('/api/v1', '');
    if (role !== 'OWNER') {
      if (at === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: ['READ'] } });
      if (at === '/memberships') {
        const scope = url.searchParams.get('scope_type');
        const rows = scope === 'HUB' ? [{ id: 'm2', scope_type: 'HUB', scope_id: HUB.id, account_id: ACCOUNT.id, role }] : scope === 'TENANT' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role }] : [];
        return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: [] } });
      }
    }
    if (modulesOn && at === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, modules_on: modulesOn } });
    if (modulesOn && /^\/containers\/[^/]+\/modules$/.test(at)) return route.fulfill({ json: { modules_on: modulesOn } });
    if (at === '/cmdb/classes') return route.fulfill({ json: { data: classes } });
    if (at === '/cmdb/sources') return route.fulfill({ json: { data: [{ key: 'netbox', kind: 'CONNECTOR', priority: 50 }] } });
    if (at === `/hubs/${HUB.id}/cis`) return route.fulfill({ json: { data: full ? MANY : [], next: null } });
    if (at === `/hubs/${HUB.id}/cmdb-health`) return route.fulfill({ json: full ? health(MANY.length, 6) : health(0, 0) });
    if (at === `/hubs/${HUB.id}/assets`) return route.fulfill({ json: { data: full ? [ASSET] : [], next: null } });
    if (at === `/hubs/${HUB.id}/cmdb-connectors`) return route.fulfill({ json: { data: full ? CONNECTORS : [] } });
    if (at === `/hubs/${HUB.id}/ci-alerts`) {
      const status = url.searchParams.get('status');
      return route.fulfill({ json: { data: full ? ALERTS.filter((each) => !status || each.status === status) : [] } });
    }
    return stub(route);
  });
  await page.goto(`${served.origin}${path}`);
  return { page, context, failures };
}

const strips = (page) => page.getByRole('tablist');
const tabNames = async (list) => (await list.getByRole('tab').allTextContents()).map((each) => each.trim());
async function noSidewaysScroll(page, what) {
  const wide = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(wide <= 1, `${what} scrolls sideways by ${wide} px`);
}

test('chromium: the inventory comes first under its figure, the operator\'s sections behind Manage', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, { path: `/hubs/${HUB.id}/assets` });
  t.after(() => context.close());

  const figure = page.locator('[data-assets-figure]');
  await figure.waitFor({ timeout: 20_000 });
  await page.getByText('alert firing').waitFor({ timeout: 5_000 });
  assert.match(await figure.innerText(), /40 configuration items[\s\S]*6 need attention[\s\S]*1 alert firing/);
  // The count opens the health, where each one says why, and its title says what counts.
  const attention = figure.getByRole('link', { name: '6 need attention' });
  assert.equal(await attention.getAttribute('href'), `/hubs/${HUB.id}/assets/health`);
  assert.match(await attention.getAttribute('title') ?? '', /incomplete, stale, orphaned/);
  assert.deepEqual(await tabNames(strips(page).first()), ['Inventory', 'Health', 'Alerts', 'Groups and map', 'Manage']);
  assert.equal(await strips(page).count(), 1, 'the second strip shows before Manage is chosen');
  await page.getByRole('link', { name: 'web-2', exact: true }).waitFor({ timeout: 5_000 });

  await strips(page).first().getByRole('tab', { name: 'Manage' }).click();
  await page.waitForFunction(() => location.pathname.endsWith('/assets/runs'), null, { timeout: 5_000 });
  assert.deepEqual(await tabNames(strips(page).nth(1)), ['Runs', 'Duplicates', 'Connectors', 'Campaigns', 'Register', 'Software', 'Import']);
  assert.deepEqual(failures, []);
});

test('chromium: a reader is shown neither Manage nor a create, and a managed address shows the inventory', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, { path: `/hubs/${HUB.id}/assets/connectors`, role: 'VIEWER' });
  t.after(() => context.close());

  await page.getByRole('link', { name: 'web-2', exact: true }).waitFor({ timeout: 20_000 });
  assert.deepEqual(await tabNames(strips(page).first()), ['Inventory', 'Health', 'Alerts', 'Groups and map', 'Register', 'Software']);
  assert.equal(await page.getByRole('button', { name: 'New configuration item' }).count(), 0);

  // The register lists for a reader, and offers nothing to write (UC-AST-12 check 5).
  await strips(page).first().getByRole('tab', { name: 'Register' }).click();
  await page.locator(`[data-asset="${ASSET.ci_id}"]`).waitFor({ timeout: 5_000 });
  assert.ok(page.url().endsWith('/assets/register'));
  assert.equal(await page.locator('[data-register] button').count(), 0);
  await strips(page).first().getByRole('tab', { name: 'Software' }).click();
  await page.waitForFunction(() => location.pathname.endsWith('/assets/software'), null, { timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('chromium: New connector in the head\'s list opens the connector dialog on its section', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, { path: `/hubs/${HUB.id}/assets` });
  t.after(() => context.close());

  await page.locator('[data-assets-figure]').waitFor({ timeout: 20_000 });
  await page.getByRole('button', { name: 'More ways to add to the inventory' }).click();
  await page.getByRole('menuitem', { name: 'New connector' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Program').waitFor({ timeout: 5_000 });
  assert.ok(page.url().endsWith('/assets/connectors'));
  await dialog.getByRole('button', { name: 'Cancel' }).last().click();
  // The ask was taken once: coming back to the section opens nothing.
  await strips(page).nth(1).getByRole('tab', { name: 'Runs' }).click();
  await strips(page).nth(1).getByRole('tab', { name: 'Connectors' }).click();
  await page.locator('[data-connector=netbox-main]').waitFor({ timeout: 5_000 });
  assert.equal(await page.getByRole('dialog').count(), 0, 'the dialog opened a second time');
  assert.deepEqual(failures, []);
});

test('chromium: a connector card has one act and the rest in its menu, its failure in words', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, { path: `/hubs/${HUB.id}/assets/connectors` });
  t.after(() => context.close());

  const card = page.locator('[data-connector=netbox-main]');
  await card.waitFor({ timeout: 20_000 });
  assert.deepEqual((await card.getByRole('button').allTextContents()).map((each) => each.trim()).sort(), ['More for netbox-main', 'Pull now']);
  assert.match(await card.innerText(), /3 failures in a row/);
  await card.getByRole('button', { name: 'More for netbox-main' }).click();
  const items = (await page.getByRole('menuitem').allTextContents()).map((each) => each.trim());
  assert.ok(items.includes('Change') && items.includes('Try without writing'), `the menu holds ${items}`);
  assert.deepEqual(failures, []);
});

for (const [width, scheme] of [[1280, 'light'], [375, 'dark']]) {
  test(`chromium ${width} px, ${scheme}: empty alerts say where alerts come from and offer an intake`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, context, failures } = await open(browser, { width, height: 812, scheme, path: `/hubs/${HUB.id}/assets/alerts`, full: false });
    t.after(() => context.close());

    await page.getByText('alert intake', { exact: false }).first().waitFor({ timeout: 20_000 });
    await page.getByRole('button', { name: 'Set up an alert intake' }).click();
    await page.getByRole('dialog').getByLabel('Program').waitFor({ timeout: 5_000 });
    assert.ok(page.url().endsWith('/assets/connectors'));
    await noSidewaysScroll(page, `empty assets ${width}`);
    assert.deepEqual(failures, []);
  });

  test(`chromium ${width} px, ${scheme}: a full inventory, connectors and alerts never scroll sideways`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, context, failures } = await open(browser, { width, height: 812, scheme, path: `/hubs/${HUB.id}/assets` });
    t.after(() => context.close());

    await page.getByRole('link', { name: 'web-40', exact: true }).waitFor({ timeout: 20_000 });
    await noSidewaysScroll(page, `inventory ${width}`);
    await page.goto(`${served.origin}/hubs/${HUB.id}/assets/connectors`);
    await page.locator('[data-connector=alerts]').waitFor({ timeout: 20_000 });
    await noSidewaysScroll(page, `connectors ${width}`);
    await page.goto(`${served.origin}/hubs/${HUB.id}/assets/alerts`);
    await page.locator('[data-alert=disk_full]').waitFor({ timeout: 20_000 });
    await noSidewaysScroll(page, `alerts ${width}`);
    assert.deepEqual(failures, []);
  });
}

const RELATED = {
  ...MANY[1],
  relations: [
    ...MANY.slice(2, 10).map((other, at) => ({ id: `r-out-${at}`, type: 'runs_on', source_ci_id: MANY[1].id, target_ci_id: other.id, data_source: 'manual', last_seen: '2026-11-01T08:00:00Z' })),
    { id: 'r-in-0', type: 'runs_on', source_ci_id: MANY[0].id, target_ci_id: MANY[1].id, data_source: 'netbox', last_seen: '2026-11-01T08:00:00Z' },
  ],
};

for (const width of [1280, 375]) {
  test(`chromium ${width} px: a CI's relations are a small graph with the list beside it`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, context, failures } = await open(browser, { width, height: 900, path: `/cis/${RELATED.id}` });
    t.after(() => context.close());
    await context.route(`**/api/v1/cis/${RELATED.id}`, (route) => route.fulfill({ json: RELATED }));
    await context.route(`**/api/v1/cis/${RELATED.id}/impact**`, (route) => route.fulfill({ json: { ci_id: RELATED.id, direction: 'DOWNSTREAM', reached: [], services: [] } }));
    await page.reload();

    const graph = page.locator('[data-ci-graph]');
    await graph.waitFor({ timeout: 20_000 });
    assert.equal(await graph.getAttribute('aria-hidden'), 'true', 'the graph is read a second time');
    // Six of the eight it points at, the rest counted; the one pointing at it on the other side.
    assert.equal(await graph.locator('[data-side=out] .node').count(), 6);
    assert.match(await graph.locator('[data-side=out]').innerText(), /and 2 more/);
    assert.equal(await graph.locator('[data-side=in] .node').count(), 1);
    // The list carries every relation with its link.
    assert.equal(await page.locator('section[aria-labelledby=ci-relations] .rows a').count(), 9);
    await noSidewaysScroll(page, `ci ${width}`);
    assert.deepEqual(failures, []);
  });
}

// A product's settings are a settings page of their own (page-system §9.6, PS-2): the section list
// in place of the home's tabs, the home's create act not shown, *Assets* in the trail; the class
// tree shows its families with each deeper branch folded until opened.
test('chromium: the assets settings are a settings page without the home\'s act, and fold the class tree', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const shipped = (key, name, lineage) => ({ ...SERVER_CLASS, key, name, parent: lineage[1] ?? '', system: true, lineage });
  const classes = [
    shipped('ci', 'Configuration item', ['ci']),
    shipped('hardware', 'Hardware', ['hardware', 'ci']),
    shipped('server', 'Server', ['server', 'hardware', 'ci']),
    shipped('virtual', 'Virtual machine', ['virtual', 'server', 'hardware', 'ci']),
    shipped('software', 'Software', ['software', 'ci']),
  ];
  const { page, context, failures } = await open(browser, { path: '/assets/settings', classes, modulesOn: ['ASSETS'] });
  t.after(() => context.close());

  await page.locator('[data-class-row="hardware"]').waitFor({ timeout: 20_000 });
  const rows = async () => page.locator('[data-class-row]').evaluateAll((all) => all.map((row) => row.getAttribute('data-class-row')));
  assert.deepEqual(await rows(), ['ci', 'hardware', 'software'], 'the deeper classes are not folded');
  await page.locator('[data-class-row="hardware"] [data-class-toggle]').click();
  assert.deepEqual(await rows(), ['ci', 'hardware', 'server', 'software']);
  assert.equal(await page.locator('[data-class-row="hardware"] [data-class-toggle]').getAttribute('aria-expanded'), 'true');
  assert.equal(await page.locator('[data-page-kind="settings"]').count(), 1, 'the assets settings are no settings page');
  await page.getByRole('navigation', { name: 'Sections of Assets settings' }).waitFor({ timeout: 10_000 });
  assert.equal(await page.getByRole('navigation', { name: 'Assets', exact: true }).count(), 0, 'the home\'s tabs stand beside the section list');
  assert.equal(await page.getByRole('button', { name: 'New configuration item' }).count(), 0, 'the home\'s create act is shown on its settings');
  assert.equal(await page.locator('header').getByRole('link', { name: 'Assets', exact: true }).getAttribute('href'), '/assets');
  assert.deepEqual(failures, []);
});
