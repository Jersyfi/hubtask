// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// An inventory object's page as the work surface draws it (§6.10, IN-2, IN-3, scene `ci`): the head
// with the stage as a step bar and *Change stage*, the attributes with the source of each, the
// relations as a picture or a list, the impact both ways, the work on it with its reasons and
// *Report an incident* naming the object on the new ticket, the rail's responsibility, protection,
// origin and groups, the register record's life with its moves - and nothing to write for a reader
// or on a phone (UC-AST-01 checks 8 and 10, UC-AST-05 check 5, UC-AST-09 check 8, UC-AST-12 check 5).
//
// Chromium only: what this asserts is the rendered screen.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, COLLECTION, CREATED_ID, HUB, MANIFEST, OTHER, PAGE, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const MODULES = ['ASSETS', 'SERVICE_DESK'];
const WITH_MODULES = { ...MANIFEST, modules_on: MODULES };

const attribute = (key, need = 'OPTIONAL') => ({ key, kind: 'TEXT', need });
const cls = (key, name, lineage, extra = {}) => ({
  key, name, parent: lineage[1] ?? '', system: false, lineage, version: 1, attributes: [], identifier_rules: [], required_relations: [],
  staleness_days: 0, all_attributes: [], ...extra,
});
const CLASSES = [
  cls('ci', 'Object', ['ci'], { abstract: true }),
  cls('application_service', 'Application service', ['application_service', 'ci'], {
    all_attributes: [attribute('url', 'RECOMMENDED'), attribute('version', 'RECOMMENDED'), attribute('runtime'), attribute('hours'),
      attribute('runbook', 'RECOMMENDED'), attribute('sla', 'RECOMMENDED'), attribute('contact', 'RECOMMENDED')],
  }),
  cls('server', 'Server', ['server', 'ci']),
  cls('laptop', 'Laptop', ['laptop', 'ci']),
  cls('room', 'Room', ['room', 'location', 'ci']),
];
const TYPES = [
  { key: 'depends_on', name: 'depends on', reverse_name: 'is needed by', pairs: [], system: true, archived: false, version: 1 },
  { key: 'runs_on', name: 'runs on', reverse_name: 'runs', pairs: [], system: true, archived: false, version: 1 },
];

const id = (n) => `01a0e2e0-0000-7000-8000-0000000e${String(n).padStart(4, '0')}`;
const ci = (n, klass, name, extra = {}) => ({
  id: id(n), hub_id: HUB.id, class: klass, name, stage: 'IN_USE', status: 'OK', attributes: {}, provenance: {}, protection: {}, quantity: 1,
  created_at: '2026-10-01T08:00:00Z', updated_at: '2026-10-01T08:00:00Z', version: 1, has_asset: false, ...extra,
});
const relation = (n, type, from, to, source = 'manual') => ({ id: id(500 + n), type, source_ci_id: from, target_ci_id: to, data_source: source, last_seen: '2026-11-01T08:00:00Z' });

const ROOM = ci(9, 'room', 'RZ Hamburg');
const PORTAL_ID = id(1);
const SHOP = ci(2, 'application_service', 'Kundenportal', { protection: { c: 'HIGH' }, owner_account_id: OTHER.id });
const DB = ci(3, 'server', 'db-01.hh.intern');
const APP1 = ci(4, 'server', 'app-01.hh.intern', { stage: 'MAINTENANCE' });
const APP2 = ci(5, 'server', 'app-02.hh.intern');
const PORTAL = ci(1, 'application_service', 'Portal-API', {
  attributes: { url: 'https://api.example.invalid/v3', version: '3.14.2', runtime: 'Go 1.24', hours: 'Mon-Fri 6-22' },
  provenance: { url: { source: 'manual', at: '2026-11-01T08:00:00Z' }, version: { source: 'glpi', at: '2026-11-01T08:00:00Z' }, runtime: { source: 'glpi', at: '2026-11-01T08:00:00Z' } },
  protection: { a: 'VERY_HIGH', fixed: ['a'], reason: 'Checkout depends on it' },
  effective_protection: { c: 'HIGH', i: null, a: 'VERY_HIGH' },
  location_id: ROOM.id, review_due: '2027-02-09', last_seen_at: '2026-11-14T06:00:00Z', quantity: 1,
  relations: [
    relation(1, 'depends_on', SHOP.id, PORTAL_ID),
    relation(2, 'depends_on', PORTAL_ID, DB.id, 'glpi'),
    relation(3, 'runs_on', PORTAL_ID, APP1.id),
    relation(4, 'runs_on', PORTAL_ID, APP2.id),
  ],
});
const LAPTOP = ci(6, 'laptop', 'NB-0417', { has_asset: true, owner_account_id: ACCOUNT.id });
const CIS = [PORTAL, SHOP, DB, APP1, APP2, LAPTOP, ROOM];

const DESK_ENTRY = (key, title, extra = {}) => ({
  item_id: id(700 + Number(key.replace(/\D/gu, ''))), key, title, entry_type: 'TASK', collection_id: COLLECTION.id, hub_id: HUB.id,
  status: { key: 'open', name: 'In progress', category: 'IN_PROGRESS' }, completed: false, start_at: null, due_at: null, reason: 'NAMES', via: null, ...extra,
});
const WORK = {
  ci_id: PORTAL_ID,
  data: [
    DESK_ENTRY('SA-12', 'Customer portal: sign-in times out'),
    DESK_ENTRY('SA-15', 'Database disk almost full', { reason: 'BENEATH', via: { ci_id: DB.id, name: DB.name, class: 'server' }, status: { key: 'new', name: 'New', category: 'TODO' } }),
    DESK_ENTRY('SA-3', 'Certificate renewed', { completed: true, status: { key: 'done', name: 'Done', category: 'DONE' } }),
  ],
};
const IMPACT = {
  DOWNSTREAM: { ci_id: PORTAL_ID, direction: 'DOWNSTREAM', reached: [{ ci_id: SHOP.id, class: 'application_service', name: SHOP.name, stage: 'IN_USE', depth: 1 }], services: [SHOP.id] },
  UPSTREAM: { ci_id: PORTAL_ID, direction: 'UPSTREAM', reached: [
    { ci_id: DB.id, class: 'server', name: DB.name, stage: 'IN_USE', depth: 1 },
    { ci_id: APP1.id, class: 'server', name: APP1.name, stage: 'MAINTENANCE', depth: 1 },
  ], services: [] },
};
const GROUPS = [
  { id: id(950), hub_id: HUB.id, key: 'services', name: 'Customer-facing services', filter: { classes: ['application_service'] }, service_id: null, updated_at: '2026-10-01T08:00:00Z', version: 1, members: 2 },
  { id: id(951), hub_id: HUB.id, key: 'laptops', name: 'Laptops', filter: { classes: ['laptop'] }, service_id: null, updated_at: '2026-10-01T08:00:00Z', version: 1, members: 1 },
];
const BACKLINKS = { data: [{ source_kind: 'PAGE', source_id: id(800), page_id: id(800), entry_id: null, whiteboard_id: null, title: 'Portal runbook', strength: 'MENTION' }] };
const ASSET = {
  ci_id: LAPTOP.id, hub_id: HUB.id, asset_tag: 'NB-0417', cost_minor: 148000, currency: 'EUR', purchased_on: '2026-01-05', supplier: 'Bechtle', warranty_until: '2029-01-05',
  warranty_ended: false, contract_id: null, stockroom_id: null, assigned_to: ACCOUNT.id, request_id: null, state: 'DEPLOYED', disposal: null,
  updated_at: '2026-03-05T08:00:00Z', version: 4,
  moves: [
    { state: 'ORDERED', at: '2026-01-02T08:00:00Z', by: ACCOUNT.id, assigned_to: null, stockroom_id: null },
    { state: 'IN_STOCK', at: '2026-01-10T08:00:00Z', by: ACCOUNT.id, assigned_to: null, stockroom_id: ROOM.id },
    { state: 'DEPLOYED', at: '2026-01-12T08:00:00Z', by: ACCOUNT.id, assigned_to: ACCOUNT.id, stockroom_id: null },
  ],
};

async function open(browser, { width = 1280, height = 900, path = `/cis/${PORTAL_ID}`, role = 'OWNER' } = {}) {
  const { page, context, failures } = await signedIn(browser, width, height);
  const written = [];
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const at = url.pathname.replace('/api/v1', '');
    const method = request.method();
    if (at === '/meta/capabilities') return route.fulfill({ json: role === 'OWNER' ? WITH_MODULES : { ...WITH_MODULES, workspace_permissions: ['READ'] } });
    if (role !== 'OWNER' && at === '/memberships') {
      const scope = url.searchParams.get('scope_type');
      const rows = scope === 'HUB' ? [{ id: 'm2', scope_type: 'HUB', scope_id: HUB.id, account_id: ACCOUNT.id, role }] : scope === 'TENANT' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role }] : [];
      return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: [] } });
    }
    if (/^\/containers\/[^/]+\/modules$/.test(at)) return route.fulfill({ json: { modules_on: MODULES } });
    if (at === `/hubs/${HUB.id}/queues`) return route.fulfill({ json: { data: [], desks: [COLLECTION.id] } });
    if (at === '/cmdb/classes') return route.fulfill({ json: { data: CLASSES } });
    if (at === '/cmdb/relation-types') return route.fulfill({ json: { data: TYPES } });
    if (at === `/hubs/${HUB.id}/cis`) return route.fulfill({ json: { data: CIS, next: null } });
    if (at === `/hubs/${HUB.id}/ci-groups`) return route.fulfill({ json: { data: GROUPS } });
    if (at === `/cis/${PORTAL_ID}/work`) return route.fulfill({ json: WORK });
    if (at === `/cis/${PORTAL_ID}/backlinks`) return route.fulfill({ json: BACKLINKS });
    if (at === `/cis/${PORTAL_ID}/impact`) return route.fulfill({ json: IMPACT[url.searchParams.get('direction') ?? 'DOWNSTREAM'] });
    if (at === `/cis/${LAPTOP.id}/asset`) return route.fulfill({ json: ASSET });
    const compliance = /^\/cis\/([^/]+)\/compliance$/.exec(at);
    if (compliance && method === 'PUT') {
      const body = request.postDataJSON();
      written.push({ call: `PUT ${at}`, body });
      const was = CIS.find((each) => each.id === compliance[1]);
      return route.fulfill({ json: { ...was, ...body, version: was.version + 1 } });
    }
    const affect = /^\/items\/([^/]+)\/cis\/([^/]+)$/.exec(at);
    if (affect && method === 'PUT') {
      written.push({ call: `PUT ${at}` });
      return route.fulfill({ json: { item_id: affect[1], cis: [], services: [], related: [], collisions: [] } });
    }
    const one = CIS.find((each) => at === `/cis/${each.id}`);
    if (one && method === 'GET') return route.fulfill({ json: one });
    if (method !== 'GET' && !at.startsWith('/stream') && !at.startsWith('/sync')) written.push({ call: `${method} ${at}`, body: request.postDataJSON?.() ?? null });
    return stub(route);
  });
  await page.goto(`${served.origin}${path}`);
  return { page, context, failures, written };
}

const head = (page) => page.locator('header.object-head');
const marks = (page) => page.getByRole('navigation', { name: /^Sections of / });
async function waitFor(page, written, call) {
  for (let waited = 0; waited < 50; waited++) {
    const found = written.find((each) => each.call === call);
    if (found) return found;
    await page.waitForTimeout(100);
  }
  return assert.fail(`${call} was not sent: ${written.map((each) => each.call).join(', ')}`);
}

test('chromium: the head says the stage as a step bar, and a writer changes it through the compliance write', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures, written } = await open(browser);
  t.after(() => context.close());

  const bar = page.getByRole('list', { name: 'Stage of Portal-API' });
  await bar.waitFor({ timeout: 20_000 });
  assert.equal((await head(page).getByRole('heading', { level: 1 }).textContent()).trim(), 'Portal-API');
  // The trail: hub › Inventory › the object type, which opens the stock with its line, › the object.
  const trail = head(page).getByRole('navigation', { name: 'Where it sits in the inventory' });
  assert.match((await trail.innerText()).replace(/\s+/gu, ' '), /House.*Application service.*Portal-API/u);
  assert.match(await trail.getByRole('link', { name: 'Application service' }).getAttribute('href'), /\/inventory\?line=asset\.class%3A/u);
  // The meta line: type, stage, when confirmed and who keeps it.
  assert.match((await head(page).innerText()).replace(/\s+/gu, ' '), /Application service In use confirmed .* · from glpi/u);
  assert.equal(await head(page).getByRole('button', { name: 'Copy the reference' }).count(), 1);
  // Done steps carry a check, the current one is marked, the later ones quiet.
  const steps = await bar.locator('li').evaluateAll((all) => all.map((each) => [each.textContent.replace(/\s+/gu, ' ').trim(), each.dataset.state, each.getAttribute('aria-current')]));
  assert.deepEqual(steps, [
    ['Planned done', 'done', null], ['Ordered done', 'done', null], ['In stock done', 'done', null], ['In use', 'current', 'step'],
    ['In maintenance', 'later', null], ['Retired', 'later', null], ['Disposed of', 'later', null],
  ]);
  assert.equal(await bar.locator('li[data-state=done] svg').count(), 3, 'a done step says it by a glyph, not by colour alone');

  await page.getByRole('button', { name: 'Change stage' }).click();
  const menu = page.getByRole('menu', { name: 'Change stage' });
  await menu.getByRole('menuitem', { name: 'In maintenance' }).click();
  const put = await waitFor(page, written, `PUT /cis/${PORTAL_ID}/compliance`);
  assert.deepEqual(put.body, { stage: 'MAINTENANCE' });
  assert.deepEqual(failures, []);
});

test('chromium: the attributes say their source, two to a row, and count the recommended ones missing', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser);
  t.after(() => context.close());

  const list = page.locator('[data-ci-attributes]');
  await list.waitFor({ timeout: 20_000 });
  const terms = await list.locator('dt').allTextContents();
  assert.deepEqual(terms.map((each) => each.replace(/\s+/gu, ' ').trim()), ['Address on the web · by hand', 'Version · from glpi', 'runtime · from glpi', 'hours']);
  const [first, second] = await list.locator('.attribute').evaluateAll((all) => all.slice(0, 2).map((each) => each.getBoundingClientRect().top));
  assert.equal(first, second, 'two attributes stand side by side on a desk');
  // The marks: attributes, relations, impact, work and the register, each with its count.
  const shown = (await marks(page).getByRole('link').allTextContents()).map((each) => each.replace(/\s+/gu, ' ').trim());
  assert.deepEqual(shown, ['Attributes4', 'Relations4', 'Impact1', 'Work on it3', 'Register record']);
  const missing = page.locator('[data-missing-attributes]');
  assert.equal((await missing.textContent()).trim(), '+ 3 recommended attributes of the object type are missing');
  await missing.click();
  await page.getByLabel('runbook').waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('chromium: the relations are a picture or a list, never both, the choice kept', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser);
  t.after(() => context.close());

  const picture = page.locator('[data-ci-graph]');
  await picture.waitFor({ timeout: 20_000 });
  assert.equal(await page.locator('[data-ci-relation-list]').count(), 0, 'the list is not drawn beside the picture');
  // This object in the middle, what rests on it on one side and what it rests on on the other.
  assert.deepEqual(await picture.locator('[data-side=in] .node-name').allTextContents(), ['Kundenportal']);
  assert.deepEqual(await picture.locator('[data-side=out] .node-name').allTextContents(), ['db-01.hh.intern', 'app-01.hh.intern', 'app-02.hh.intern']);
  assert.deepEqual((await picture.locator('[data-side=out] .edge').allTextContents()).map((each) => each.trim()), ['depends on', 'runs on', 'runs on']);
  // The open entry on a neighbour, in the danger tone; a neighbour in maintenance says it.
  const db = picture.locator('[data-side=out] .node').filter({ hasText: 'db-01' });
  assert.equal(await db.locator('[data-tone=danger]').textContent().then((text) => text.trim()), 'SA-15');
  const app = picture.locator('[data-side=out] .node').filter({ hasText: 'app-01' });
  assert.match(await app.innerText(), /In maintenance/u);

  await page.getByRole('radio', { name: 'List' }).click();
  const list = page.locator('[data-ci-relation-list]');
  await list.waitFor({ timeout: 5_000 });
  assert.equal(await picture.count(), 0, 'the picture is gone when the list shows');
  assert.equal(await list.locator('[data-relation-row]').count(), 4);
  // Removing is in a row's ⋯ only.
  assert.equal(await list.getByRole('button', { name: 'Remove the relation' }).count(), 0);
  await list.getByRole('button', { name: 'Actions for the relation to db-01.hh.intern' }).click();
  await page.getByRole('menuitem', { name: 'Remove the relation' }).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');
  // Kept on the device for this reader.
  await page.reload();
  await page.locator('[data-ci-relation-list]').waitFor({ timeout: 20_000 });
  assert.equal(await page.locator('[data-ci-graph]').count(), 0);
  // *+ Relation* opens a dialog of the ladder rather than a form in the page.
  await page.locator('[data-relate]').click();
  const dialog = page.getByRole('dialog', { name: 'Relate Portal-API to another object' });
  await dialog.waitFor({ timeout: 5_000 });
  assert.equal(await dialog.getByLabel('To').count(), 1);
  assert.deepEqual(failures, []);
});

test('chromium: the impact reads both ways, each object with its type, steps, need and owner', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser);
  t.after(() => context.close());

  const section = page.locator('#ci-impact');
  await section.locator('[data-impact=DOWNSTREAM] [data-impact-row]').first().waitFor({ timeout: 20_000 });
  assert.match(await section.innerText(), /up to 8 steps/u);
  const down = (await section.locator('[data-impact-row]').allInnerTexts()).map((each) => each.replace(/\s+/gu, ' ').trim());
  assert.equal(down.length, 1);
  assert.match(down[0], /Kundenportal Application service · 1 step · High service Mara Lind/u);
  await section.getByRole('radio', { name: 'What it depends on' }).click();
  await section.locator('[data-impact=UPSTREAM]').waitFor({ timeout: 5_000 });
  const up = await section.locator('[data-impact-row] a').allTextContents();
  assert.deepEqual(up, ['db-01.hh.intern', 'app-01.hh.intern']);
  assert.deepEqual(failures, []);
});

test('chromium: the work on it stands with its reasons, and an incident reported names the object', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures, written } = await open(browser);
  t.after(() => context.close());

  const section = page.locator('#ci-work');
  await section.locator('[data-work=open]').waitFor({ timeout: 20_000 });
  const named = section.locator('[data-work-entry="SA-12"]');
  assert.equal(await named.getAttribute('data-reason'), 'NAMES');
  assert.match((await named.innerText()).replace(/\s+/gu, ' '), /SA-12 Customer portal: sign-in times out names this object In progress/u);
  const beneath = section.locator('[data-work-entry="SA-15"]');
  assert.equal(await beneath.getAttribute('data-reason'), 'BENEATH');
  assert.match((await beneath.innerText()).replace(/\s+/gu, ' '), /on an object beneath: db-01\.hh\.intern/u);
  assert.equal(await beneath.getByRole('link', { name: 'db-01.hh.intern' }).getAttribute('href'), `/cis/${DB.id}`);
  // The completed one folded under its count.
  assert.equal(await section.locator('[data-work=done]').isVisible().catch(() => false), false);
  await section.getByRole('button', { name: '1 done' }).click();
  await section.locator('[data-work-entry="SA-3"]').waitFor({ timeout: 5_000 });
  // What mentions it, as a second list.
  assert.match(await section.locator('[data-mentioned]').innerText(), /Portal runbook/u);

  // *Report an incident* is the capture in the desk's collection; the new entry names the object.
  await section.getByRole('button', { name: 'Report an incident' }).click();
  const capture = page.getByRole('dialog', { name: 'Capture' });
  await capture.waitFor({ timeout: 5_000 });
  assert.match(await capture.innerText(), new RegExp(COLLECTION.name, 'u'));
  await capture.getByRole('textbox', { name: 'What do you want to remember?' }).fill('Portal answers 502');
  await page.keyboard.press('Enter');
  await waitFor(page, written, 'POST /items');
  await waitFor(page, written, `PUT /items/${CREATED_ID}/cis/${PORTAL_ID}`);
  assert.deepEqual(failures, []);
});

test('chromium: the rail says who answers for it, its protection with its origin, its sources and its groups', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures, written } = await open(browser);
  t.after(() => context.close());

  const rail = page.getByRole('complementary', { name: 'Properties of Portal-API' });
  await rail.locator('[data-rail-owner]').waitFor({ timeout: 20_000 });
  const owner = (await rail.locator('[data-rail-owner]').innerText()).replace(/\s+/gu, ' ');
  assert.match(owner, /Person nobody · set/u);
  assert.match(owner, /Location RZ Hamburg/u);
  assert.match(owner, /Next review 9 Feb 2027|Next review Feb 9, 2027/u);
  const protection = (await rail.locator('[data-rail-protection]').innerText()).replace(/\s+/gu, ' ');
  assert.match(protection, /Confidentiality High inherited from Kundenportal/u);
  assert.match(protection, /Availability Very high own · Checkout depends on it/u);
  assert.match((await rail.locator('[data-rail-origin]').innerText()).replace(/\s+/gu, ' '), /Sources by hand, glpi/u);
  const groups = rail.locator('[data-rail-groups]');
  assert.deepEqual(await groups.getByRole('link').allTextContents(), ['Customer-facing services']);
  assert.equal(await groups.getByRole('link').getAttribute('href'), `/hubs/${HUB.id}/inventory?group=${GROUPS[0].id}`);
  // *set* opens the responsibility; it saves through the same compliance write.
  await rail.getByRole('button', { name: 'set', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Change the responsibility' });
  await dialog.getByLabel('Owner, a person').selectOption(OTHER.id);
  await dialog.getByRole('button', { name: 'Save' }).click();
  const put = await waitFor(page, written, `PUT /cis/${PORTAL_ID}/compliance`);
  assert.equal(put.body.owner_account_id, OTHER.id);
  assert.deepEqual(failures, []);
});

test('chromium: the register record shows the device\'s life with each move', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, { path: `/cis/${LAPTOP.id}` });
  t.after(() => context.close());

  const life = page.locator('[data-asset-life]');
  await life.waitFor({ timeout: 20_000 });
  const steps = await life.locator('[data-life-step]').evaluateAll((all) => all.map((each) => [each.dataset.lifeStep, 'reached' in each.dataset, 'current' in each.dataset]));
  assert.deepEqual(steps, [
    ['REQUESTED', true, false], ['ORDERED', true, false], ['IN_STOCK', true, false], ['DEPLOYED', true, true], ['RETIRED', false, false], ['DISPOSED', false, false],
  ]);
  assert.match(await life.locator('[data-life-step=IN_STOCK]').innerText(), /in RZ Hamburg/u);
  assert.match(await life.locator('[data-life-step=DEPLOYED]').innerText(), /with Jérôme Winkel/u);
  assert.match(await life.locator('[data-life-step=DISPOSED]').innerText(), /Proof needed/u);
  assert.deepEqual(failures, []);
});

test('chromium: an object without a register record says what one is for, in a dashed line', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser);
  t.after(() => context.close());

  const empty = page.locator('[data-register-empty]');
  await empty.waitFor({ timeout: 20_000 });
  assert.match(await empty.innerText(), /No register record\. For devices and licences/u);
  assert.equal(await empty.getByRole('button', { name: 'Add the register record' }).count(), 1);
  assert.deepEqual(failures, []);
});

test('chromium: at 390 px the object is read only - the step bar scrolls, the marks switch, nothing writes', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, { width: 390, height: 844 });
  t.after(() => context.close());

  await page.getByRole('list', { name: 'Stage of Portal-API' }).waitFor({ timeout: 20_000 });
  // The marks are the switch, the relations first; one section shows.
  const first = (await marks(page).getByRole('link').first().textContent()).replace(/\s+/gu, ' ').trim();
  assert.equal(first, 'Relations4');
  assert.equal(await page.locator('#ci-relations').isVisible(), true);
  assert.equal(await page.locator('#ci-attributes').isVisible(), false);
  await marks(page).getByRole('link', { name: /Work on it/u }).click();
  await page.locator('#ci-work').waitFor({ state: 'visible', timeout: 5_000 });
  for (const name of ['Change stage', 'Report an incident', 'Start work on it', 'Relation', 'Change', 'set', 'Add the register record']) {
    assert.equal(await page.getByRole('button', { name, exact: true }).count(), 0, `${name} is drawn on a phone`);
  }
  // The step bar scrolls sideways inside itself; the page does not.
  const sideways = await page.evaluate(() => document.scrollingElement.scrollWidth > window.innerWidth);
  assert.equal(sideways, false, 'the page scrolls sideways');
  assert.deepEqual(failures, []);
});

test('chromium: a reader without write sees no write anywhere', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, { role: 'VIEWER' });
  t.after(() => context.close());

  await page.locator('[data-ci-graph]').waitFor({ timeout: 20_000 });
  await page.locator('#ci-work [data-work=open]').waitFor({ timeout: 10_000 });
  await page.locator('[data-register-empty]').waitFor({ timeout: 10_000 });
  for (const name of ['Change stage', 'Report an incident', 'Start work on it', 'Relation', 'Change', 'set', 'Add the register record']) {
    assert.equal(await page.getByRole('button', { name, exact: true }).count(), 0, `${name} is drawn for a reader`);
  }
  assert.equal(await page.locator('[data-missing-attributes]').evaluate((each) => each.tagName), 'P', 'the missing attributes are said, not offered');
  assert.equal(await head(page).getByRole('button', { name: /^Actions for/u }).count(), 0, 'the head offers a ⋯ to a reader');
  await page.getByRole('radio', { name: 'List' }).click();
  assert.equal(await page.locator('[data-ci-relation-list]').getByRole('button').count(), 0, 'a relation row offers a ⋯ to a reader');
  assert.deepEqual(failures, []);
});
