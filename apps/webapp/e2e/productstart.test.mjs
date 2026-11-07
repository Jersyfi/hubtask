// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A product's first visit (user-experience.md §4): with no hub using it yet, its empty home offers
// the catalogue's templates that switch it on - and no other - beside *Create hub*. Pressing one
// makes the hub and opens its *Products* section with that template's plan, so the newcomer takes
// it in one more press.
//
// Chromium only; what the server plans and applies is its own tests'.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, DEFAULT_STRUCTURE, MANIFEST, OTHER, PAGE, PERMISSIONS, forgetUnstubbed, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const NEW_HUB = '01a0e2e0-0000-7000-8000-0000000004d1';
const template = (key, modules) => ({
  key, version: 1, name: key, origin: 'HUBTASK', provenance: `${key}@1`, created_at: '2026-09-01T00:00:00Z',
  update_available: false, taken_version: null, hub_document: {},
  document: { modules: Object.fromEntries(modules.map((name) => [name, { switch: 'ON' }])) },
});
const SCRUM = template('scrum_team', ['workflows', 'planning', 'knowledge']);
const KANBAN = template('kanban_team', ['workflows']);
const planned = {
  template: SCRUM.key, upgrade: [], document: {}, mode: 'MERGE', base_hash: 'b1', plan_hash: 'p1', refusals: [], operations: [],
  flags: { hides_data: false, changes_permissions: false }, counts: { added: 1, changed: 0, removed: 0 },
  changes: [{ path: 'workflows.scrum', section: 'workflows', key: 'scrum', op: 'ADD', fields: [] }],
};

test('chromium: an empty product home offers its templates, and one makes the hub with its parts at one press', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  forgetUnstubbed();
  const hubs = [];
  const planFor = [];
  const made = [];
  const answer = async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, modules_on: ['WORKFLOWS', 'KNOWLEDGE'] } });
    // The replica carries the hub once it is made.
    if (path === '/sync:snapshot') {
      const records = hubs.map((c) => JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: c.id, container_id: null, payload: c }));
      return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${[...records, '{"cursor":"c-e2e"}'].join('\n')}\n` });
    }
    if ((path === '/containers' || path === '/containers:from-template') && method === 'POST') {
      made.push({ path, body: request.postDataJSON() });
      const { template: _template, answers: _answers, samples: _samples, acknowledge: _acknowledge, ...look } = request.postDataJSON();
      const hub = { id: NEW_HUB, type: 'HUB', parent_id: null, order_key: 'a1', version: 1, structure: DEFAULT_STRUCTURE, ...look };
      hubs.push(hub);
      return route.fulfill({ status: 201, json: hub });
    }
    if (path === '/containers' && method === 'GET') return route.fulfill({ json: { ...PAGE, data: url.searchParams.get('type') === 'HUB' ? hubs : [] } });
    if (path === `/containers/${NEW_HUB}` && method === 'GET' && hubs[0]) return route.fulfill({ json: hubs[0] });
    // Whoever makes a hub owns it, as the server answers it.
    if (path === '/memberships' && url.searchParams.get('scope_type') === 'HUB') {
      return route.fulfill({ json: { ...PAGE, data: hubs.map((hub) => ({ id: 'm-new', scope_type: 'HUB', scope_id: hub.id, account_id: ACCOUNT.id, role: 'OWNER' })), grantable_roles: [] } });
    }
    if (path === '/config/templates' && method === 'GET') return route.fulfill({ json: { data: [KANBAN, SCRUM] } });
    if (path === `/config/templates/${SCRUM.key}:plan`) {
      planFor.push(url.searchParams.get('hub_id') ?? request.postDataJSON()?.hub_id ?? '');
      return route.fulfill({ json: planned });
    }
    if (path === `/config/templates/${KANBAN.key}:plan`) return route.fulfill({ json: { ...planned, template: KANBAN.key, changes: [] } });
    return stub(route);
  };
  const failures = [];
  async function open(into) {
    await into.route('**/api/v1/**', answer);
    await into.addInitScript(() => {
      sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
      sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    });
    const opened = await into.newPage();
    opened.on('pageerror', (error) => failures.push(String(error)));
    return opened;
  }
  const page = await open(context);

  await page.goto(`${served.origin}/pages`);
  const start = page.getByRole('button', { name: 'Scrum team', exact: true });
  await start.waitFor({ timeout: 15_000 });
  await page.locator('#main').getByRole('button', { name: 'Create hub', exact: true }).first().waitFor();
  assert.equal(await page.getByRole('button', { name: 'Kanban team', exact: true }).count(), 0, 'a template that does not switch Pages on is offered');

  await start.click();
  const dialog = page.getByRole('dialog');
  // The dialog's gallery (§5.2) holds the template pressed as the one chosen, among the families.
  await dialog.locator('[data-hub-template="scrum_team"] button[aria-pressed="true"]').waitFor({ timeout: 10_000 });
  assert.equal(await dialog.locator('[data-family="teams"] [data-hub-template]').count(), 2, 'the teams family does not hold both team templates');
  assert.equal(await dialog.locator('[data-family="simple"]').count(), 1, 'the presets are not the Simple family');
  // §8.11: the chosen template names the hub until the reader types a name; another choice
  // renames it, a typed name stays.
  const hubName = dialog.getByLabel('Hub name');
  assert.equal(await hubName.inputValue(), 'Scrum team');
  await dialog.locator('[data-hub-template="kanban_team"] button').click();
  assert.equal(await hubName.inputValue(), 'Kanban team');
  await hubName.fill('Delivery');
  await dialog.locator('[data-hub-template="scrum_team"] button').click();
  assert.equal(await hubName.inputValue(), 'Delivery');
  // §5.2: what the template sets up is said before the press - the workspace part, which the
  // workspace has not taken - and nothing is made yet.
  await dialog.getByText('Adds the workflow “scrum”').waitFor({ timeout: 10_000 });
  assert.ok(planFor.length > 0 && planFor.every((hub) => hub === ''), 'the plan before the press was not the workspace part\'s');
  assert.equal(made.length, 0, 'a hub was made before the press');
  await dialog.getByRole('button', { name: 'Create hub', exact: true }).click();
  // One request makes the hub with the template's parts, and the app opens its main product.
  await page.waitForURL(`**/hubs/${NEW_HUB}/plan`, { timeout: 10_000 });
  assert.deepEqual(made.map((each) => each.path), ['/containers:from-template'], 'the hub was not made with its template in one request');
  assert.equal(made[0].body.template, SCRUM.key);
  assert.equal(made[0].body.name, 'Delivery');
  assert.deepEqual(failures, []);
});

// UC-SVC-05, §4: a service template taken by the workspace leaves its collections as they were, so
// the desk has hubs but no desk - and offers the templates that start one, not only *Create hub*.
test('chromium: the desk without a desk offers its templates though hubs exist', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  forgetUnstubbed();
  const HUB = '01a0e2e0-0000-7000-8000-0000000004e1';
  const COLLECTION = '01a0e2e0-0000-7000-8000-0000000004e2';
  const hub = { id: HUB, type: 'HUB', parent_id: null, name: 'Team', order_key: 'a1', version: 1, structure: DEFAULT_STRUCTURE };
  const collection = { id: COLLECTION, type: 'COLLECTION', parent_id: HUB, name: 'Backlog', order_key: 'a1', version: 1, structure: DEFAULT_STRUCTURE };
  const ITIL = template('itil_service_desk', ['workflows', 'service_desk', 'knowledge']);
  const on = ['WORKFLOWS', 'SERVICE_DESK', 'KNOWLEDGE'];
  const incident = { key: 'incident', level: 'TASK', system: false, position: 1, archived: false, name: 'Incident', icon: 'circle-alert', color_token: 'red', form: { channels: [{ channel: 'PORTAL' }] } };
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, modules_on: on } });
    if (path === `/containers/${HUB}/modules` || path === `/containers/${COLLECTION}/modules`) return route.fulfill({ json: { modules_on: on } });
    if (path === '/sync:snapshot') {
      const records = [hub, collection].map((c) => JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: c.id, container_id: c.parent_id, payload: c }));
      return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${[...records, '{"cursor":"c-e2e"}'].join('\n')}\n` });
    }
    if (path === '/containers' && route.request().method() === 'GET') {
      return route.fulfill({ json: { ...PAGE, data: url.searchParams.get('type') === 'HUB' ? [hub] : [collection] } });
    }
    if (path === '/entry-types') return route.fulfill({ json: { ...PAGE, data: [incident] } });
    if (path === '/config/templates' && route.request().method() === 'GET') return route.fulfill({ json: { data: [ITIL, SCRUM] } });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/desk`);
  await page.getByText('No desk takes requests yet').waitFor({ timeout: 15_000 });
  await page.getByRole('button', { name: 'ITIL service desk', exact: true }).waitFor({ timeout: 10_000 });
  assert.equal(await page.getByRole('button', { name: 'Scrum team', exact: true }).count(), 0, 'a template that does not switch the desk on is offered');
  // The product's trail reads as a hub's does: a chevron between the levels, one current level.
  const trail = page.getByRole('navigation', { name: 'Where you are among the products' }).first();
  assert.equal(await trail.locator('li').count(), 2);
  assert.equal(await trail.locator('li').first().locator('.separator').count(), 1, 'no separator after Products');
  assert.equal(await trail.locator('[aria-current="page"]').count(), 1);
  assert.deepEqual(failures, []);
});

// §4: a product switched on at the workspace is on in its hubs at once, so the first visit is
// decided by whether anything of it exists - an existing hub without a page still starts Pages.
test('chromium: a product on in a hub that holds nothing of it offers its create act and its templates', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  forgetUnstubbed();
  const HUB = '01a0e2e0-0000-7000-8000-0000000004f1';
  const hub = { id: HUB, type: 'HUB', parent_id: null, name: 'Allgemein', order_key: 'a1', version: 1, structure: DEFAULT_STRUCTURE };
  const on = ['WORKFLOWS', 'KNOWLEDGE'];
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, modules_on: on } });
    if (path === `/containers/${HUB}/modules`) return route.fulfill({ json: { modules_on: on } });
    if (path === '/sync:snapshot') {
      const records = [JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: HUB, container_id: null, payload: hub })];
      return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${[...records, '{"cursor":"c-e2e"}'].join('\n')}\n` });
    }
    if (path === '/containers' && route.request().method() === 'GET') return route.fulfill({ json: { ...PAGE, data: url.searchParams.get('type') === 'HUB' ? [hub] : [] } });
    if (path === `/hubs/${HUB}/pages`) return route.fulfill({ json: { ...PAGE, data: [] } });
    if (path === '/config/templates' && route.request().method() === 'GET') return route.fulfill({ json: { data: [KANBAN, SCRUM] } });
    if (path === '/memberships' && url.searchParams.get('scope_type') === 'HUB') {
      return route.fulfill({ json: { ...PAGE, data: [{ id: 'm-a', scope_type: 'HUB', scope_id: HUB, account_id: ACCOUNT.id, role: 'OWNER' }], grantable_roles: [] } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/pages`);
  await page.getByText('Nothing in Pages yet').waitFor({ timeout: 15_000 });
  const main = page.locator('#main');
  await main.getByRole('button', { name: 'Scrum team', exact: true }).waitFor({ timeout: 10_000 });
  assert.equal(await main.getByRole('button', { name: 'New page', exact: true }).count(), 1, 'the create act is not offered once');
  // §5.2: the act creates - the place's create dialog opens, not only its list.
  await main.getByRole('button', { name: 'New page', exact: true }).click();
  await page.waitForURL(`**/hubs/${HUB}/pages?new=1`, { timeout: 10_000 });
  await page.getByRole('dialog', { name: 'New page' }).waitFor({ timeout: 10_000 });
  assert.deepEqual(failures, []);
});

// §5.2: a create dialog names where it makes the object as its *In:* chip, one press to change -
// among the hubs where the product is on and the reader may write, never one where it is off.
test('chromium: the create dialog makes the page in the hub its place chip names', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  forgetUnstubbed();
  const HUB = '01a0e2e0-0000-7000-8000-0000000004f1';
  const IT = '01a0e2e0-0000-7000-8000-0000000004f2';
  const OFF = '01a0e2e0-0000-7000-8000-0000000004f3';
  const hubOf = (id, name, key) => ({ id, type: 'HUB', parent_id: null, name, order_key: key, version: 1, structure: DEFAULT_STRUCTURE });
  const hubs = [hubOf(HUB, 'Allgemein', 'a1'), hubOf(IT, 'IT-Service-Desk', 'a2'), hubOf(OFF, 'Vertrieb', 'a3')];
  const on = ['WORKFLOWS', 'KNOWLEDGE'];
  const made = [];
  const sent = [];
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    const method = route.request().method();
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, modules_on: on } });
    if (path === `/containers/${OFF}/modules`) return route.fulfill({ json: { modules_on: ['WORKFLOWS'] } });
    if (/^\/containers\/[^/]+\/modules$/.test(path)) return route.fulfill({ json: { modules_on: on } });
    if (path === '/sync:snapshot') {
      const records = hubs.map((hub) => JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: hub.id, container_id: null, payload: hub }));
      return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${[...records, '{"cursor":"c-e2e"}'].join('\n')}\n` });
    }
    if (path === '/containers' && method === 'GET') return route.fulfill({ json: { ...PAGE, data: url.searchParams.get('type') === 'HUB' ? hubs : [] } });
    const pagesAt = /^\/hubs\/([^/]+)\/pages$/.exec(path);
    if (pagesAt && method === 'POST') {
      made.push(pagesAt[1]);
      sent.push(route.request().postDataJSON());
      return route.fulfill({ status: 201, json: { id: '01a0e2e0-0000-7000-8000-0000000004f9', hub_id: pagesAt[1], title: 'Runbook', parent_id: null } });
    }
    if (pagesAt) return route.fulfill({ json: { ...PAGE, data: [] } });
    if (path === '/memberships' && url.searchParams.get('scope_type') === 'HUB') {
      const scope = url.searchParams.get('scope_id');
      return route.fulfill({ json: { ...PAGE, data: [{ id: `m-${scope}`, scope_type: 'HUB', scope_id: scope, account_id: ACCOUNT.id, role: 'OWNER' }], grantable_roles: [] } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/hubs/${HUB}/pages?new=1`);
  const dialog = page.getByRole('dialog', { name: 'New page' });
  await dialog.waitFor({ timeout: 15_000 });
  const chip = dialog.locator('[data-chip="place"]');
  await chip.filter({ hasText: 'Allgemein' }).waitFor({ timeout: 10_000 });
  await chip.click();
  const choices = page.getByRole('menuitem');
  await choices.first().waitFor({ timeout: 10_000 });
  assert.deepEqual(await choices.allInnerTexts(), ['Allgemein', 'IT-Service-Desk'], 'the chip offers a hub where pages are off');
  await page.getByRole('menuitem', { name: 'IT-Service-Desk' }).click();
  await chip.filter({ hasText: 'IT-Service-Desk' }).waitFor({ timeout: 10_000 });
  // One anatomy (§5.2): the title first, the gallery with Blank chosen, the chip, one primary.
  const order = await dialog.evaluate((root) => ['input', '[data-page-start]', '[data-chip="place"]']
    .map((selector) => root.querySelector(selector))
    .every((each, i, all) => each && (i === 0 || all[i - 1].compareDocumentPosition(each) & Node.DOCUMENT_POSITION_FOLLOWING)));
  assert.ok(order, 'the dialog does not run title, gallery, place');
  assert.equal(await dialog.locator('[data-page-start="blank"] button').getAttribute('aria-pressed'), 'true');
  assert.equal(await dialog.locator('select').count(), 0, 'the templates are still a select');
  await dialog.locator('[data-page-start="shipped:meeting"] button').click();
  assert.equal(await dialog.locator('[data-page-start="shipped:meeting"] button').getAttribute('aria-pressed'), 'true');
  await dialog.getByLabel('Title').fill('Runbook');
  await dialog.getByRole('button', { name: 'Write the page', exact: true }).click();
  await page.waitForURL('**/pages/01a0e2e0-0000-7000-8000-0000000004f9', { timeout: 10_000 });
  assert.deepEqual(made, [IT], 'the page was not made in the hub the chip names');
  assert.equal(sent[0]?.title, 'Runbook');
  assert.ok(typeof sent[0]?.body === 'string' && sent[0].body.length > 0, 'the chosen template\'s body was not sent');
  assert.deepEqual(failures, []);
});

// §5.2: a hub takes its part once the workspace took the template's own. Whoever may change the
// workspace is shown the workspace's part first in the same dialog, then the hub's - no refusal.
test('chromium: a template the workspace has not taken is planned for the workspace first, then for the hub', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  forgetUnstubbed();
  const hub = { id: NEW_HUB, type: 'HUB', parent_id: null, name: 'Delivery', order_key: 'a1', version: 1, structure: DEFAULT_STRUCTURE };
  let workspaceTook = false;
  const plans = [];
  const applied = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, modules_on: ['WORKFLOWS', 'KNOWLEDGE', 'PLANNING'] } });
    if (path === '/sync:snapshot') {
      const record = JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: hub.id, container_id: null, payload: hub });
      return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${record}\n{"cursor":"c-e2e"}\n` });
    }
    if (path === '/containers' && method === 'GET') return route.fulfill({ json: { ...PAGE, data: url.searchParams.get('type') === 'HUB' ? [hub] : [] } });
    if (path === `/containers/${NEW_HUB}` && method === 'GET') return route.fulfill({ json: hub });
    if (path === '/memberships' && url.searchParams.get('scope_type') === 'HUB') {
      return route.fulfill({ json: { ...PAGE, data: [{ id: 'm-new', scope_type: 'HUB', scope_id: hub.id, account_id: ACCOUNT.id, role: 'OWNER' }], grantable_roles: [] } });
    }
    if (path === '/config/templates' && method === 'GET') return route.fulfill({ json: { data: [SCRUM] } });
    if (path === `/config/templates/${SCRUM.key}:plan`) {
      const forHub = request.postDataJSON()?.hub_id ?? '';
      plans.push(forHub);
      if (forHub && !workspaceTook) {
        // As the server answers it: a field of config.refused, not the problem's own code.
        return route.fulfill({ status: 422, contentType: 'application/problem+json',
          body: JSON.stringify({ status: 422, code: 'errors.validation', detail_code: 'config.refused', field_errors: [{ path: '/key', code: 'config.template_workspace_first', params: { template: SCRUM.key } }] }) });
      }
      return route.fulfill({ json: { ...planned, hub_id: forHub || null } });
    }
    if (path === '/config:apply') {
      const body = request.postDataJSON();
      applied.push(body.hub_id ?? '');
      if (!body.hub_id) workspaceTook = true;
      return route.fulfill({ json: { version: 2, applied: true } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/hubs/${NEW_HUB}/settings/modules?template=${SCRUM.key}`);
  const dialog = page.getByRole('dialog');
  await dialog.locator('[data-template-workspace-first]').waitFor({ timeout: 15_000 });
  await dialog.getByRole('button', { name: 'Take template' }).click();
  await dialog.locator('[data-template-workspace-first]').waitFor({ state: 'detached', timeout: 10_000 });
  await dialog.getByText('Adds the workflow “scrum”').waitFor({ timeout: 10_000 });
  assert.deepEqual(applied, [''], 'the workspace part was not applied first, alone');
  assert.deepEqual(plans, [NEW_HUB, '', NEW_HUB], 'the plans were not hub, workspace, hub');
  assert.deepEqual(failures, []);
});

// The list says the workspace has not taken the template: no refusal is asked for first.
test('chromium: a template the workspace has not taken is planned for the workspace first, as the list says, before any refusal', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  forgetUnstubbed();
  const hub = { id: NEW_HUB, type: 'HUB', parent_id: null, name: 'Delivery', order_key: 'a1', version: 1, structure: DEFAULT_STRUCTURE };
  let workspaceTook = false;
  const plans = [];
  const applied = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, modules_on: ['WORKFLOWS', 'KNOWLEDGE', 'PLANNING'] } });
    if (path === '/sync:snapshot') {
      const record = JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: hub.id, container_id: null, payload: hub });
      return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${record}\n{"cursor":"c-e2e"}\n` });
    }
    if (path === '/containers' && method === 'GET') return route.fulfill({ json: { ...PAGE, data: url.searchParams.get('type') === 'HUB' ? [hub] : [] } });
    if (path === `/containers/${NEW_HUB}` && method === 'GET') return route.fulfill({ json: hub });
    if (path === '/memberships' && url.searchParams.get('scope_type') === 'HUB') {
      return route.fulfill({ json: { ...PAGE, data: [{ id: 'm-new', scope_type: 'HUB', scope_id: hub.id, account_id: ACCOUNT.id, role: 'OWNER' }], grantable_roles: [] } });
    }
    if (path === '/config/templates' && method === 'GET') return route.fulfill({ json: { data: [{ ...SCRUM, workspace_first: !workspaceTook }] } });
    if (path === `/config/templates/${SCRUM.key}:plan`) {
      const forHub = request.postDataJSON()?.hub_id ?? '';
      plans.push(forHub);
      if (forHub && !workspaceTook) {
        // As the server answers it: a field of config.refused, not the problem's own code.
        return route.fulfill({ status: 422, contentType: 'application/problem+json',
          body: JSON.stringify({ status: 422, code: 'errors.validation', detail_code: 'config.refused', field_errors: [{ path: '/key', code: 'config.template_workspace_first', params: { template: SCRUM.key } }] }) });
      }
      return route.fulfill({ json: { ...planned, hub_id: forHub || null } });
    }
    if (path === '/config:apply') {
      const body = request.postDataJSON();
      applied.push(body.hub_id ?? '');
      if (!body.hub_id) workspaceTook = true;
      return route.fulfill({ json: { version: 2, applied: true } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/hubs/${NEW_HUB}/settings/modules?template=${SCRUM.key}`);
  const dialog = page.getByRole('dialog');
  await dialog.locator('[data-template-workspace-first]').waitFor({ timeout: 15_000 });
  await dialog.getByRole('button', { name: 'Take template' }).click();
  await dialog.locator('[data-template-workspace-first]').waitFor({ state: 'detached', timeout: 10_000 });
  await dialog.getByText('Adds the workflow “scrum”').waitFor({ timeout: 10_000 });
  assert.deepEqual(applied, [''], 'the workspace part was not applied first, alone');
  assert.deepEqual(plans, ['', NEW_HUB], 'the plans were not workspace, then hub');
  assert.deepEqual(failures, []);
});

// §5.2: a hub's owner who may not change the workspace is told before choosing that the workspace
// takes the template first, and by whom - no take is offered, so no refusal follows a choice.
test('chromium: a template the workspace has not taken names its administrators to a hub owner who may not take it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  forgetUnstubbed();
  const hub = { id: NEW_HUB, type: 'HUB', parent_id: null, name: 'Delivery', order_key: 'a1', version: 1, structure: DEFAULT_STRUCTURE };
  const plans = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: PERMISSIONS.MEMBER, modules_on: ['WORKFLOWS', 'KNOWLEDGE', 'PLANNING'] } });
    if (path === '/sync:snapshot') {
      const record = JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: hub.id, container_id: null, payload: hub });
      return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${record}\n{"cursor":"c-e2e"}\n` });
    }
    if (path === '/containers' && method === 'GET') return route.fulfill({ json: { ...PAGE, data: url.searchParams.get('type') === 'HUB' ? [hub] : [] } });
    if (path === `/containers/${NEW_HUB}` && method === 'GET') return route.fulfill({ json: hub });
    if (path === '/memberships' && url.searchParams.get('scope_type') === 'HUB') {
      return route.fulfill({ json: { ...PAGE, data: [{ id: 'm-hub', scope_type: 'HUB', scope_id: hub.id, account_id: ACCOUNT.id, role: 'OWNER' }], grantable_roles: [] } });
    }
    if (path === '/memberships' && url.searchParams.get('scope_type') === 'TENANT') {
      return route.fulfill({ json: { ...PAGE, data: [
        { id: 'm-admin', scope_type: 'TENANT', scope_id: null, account_id: OTHER.id, role: 'ADMIN' },
        { id: 'm-me', scope_type: 'TENANT', scope_id: null, account_id: ACCOUNT.id, role: 'MEMBER' },
      ], grantable_roles: [] } });
    }
    if (path === '/config/templates' && method === 'GET') {
      return route.fulfill({ json: { data: [{ ...SCRUM, workspace_first: true }, { ...KANBAN, workspace_first: false }] } });
    }
    if (path.endsWith(':plan')) {
      plans.push(path);
      return route.fulfill({ json: { ...planned, hub_id: NEW_HUB } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/hubs/${NEW_HUB}/settings/modules`);
  const asked = page.locator(`[data-template="${SCRUM.key}"][data-ask-admin]`);
  await asked.waitFor({ timeout: 15_000 });
  await asked.getByText(`Ask an administrator to add it: ${OTHER.display_name}.`).waitFor({ timeout: 10_000 });
  assert.equal(await asked.locator(`[data-take]`).count(), 0, 'a take is offered that the server would refuse');
  const kanban = page.locator(`[data-template="${KANBAN.key}"]`);
  assert.equal(await kanban.getAttribute('data-ask-admin'), null, 'a template the workspace took sends to an administrator');
  assert.equal(await kanban.locator(`[data-take="${KANBAN.key}"]`).count(), 1, 'a template the workspace took is not offered');
  assert.deepEqual(plans, []);
  assert.deepEqual(failures, []);
});

// §3.3: *Try it* in More in Hubtask takes the product's template through the hub dialog - not to
// the product's switched-off page - and a product no template starts still goes to its home,
// whose switch then says what the product still needs (§2.8).
test('chromium: Try it opens the hub dialog on the product\'s template', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  forgetUnstubbed();
  const ITIL = template('itil_service_desk', ['workflows', 'service_desk', 'knowledge']);
  let on = ['WORKFLOWS'];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, modules_on: on } });
    if (path === '/config/templates' && request.method() === 'GET') return route.fulfill({ json: { data: [KANBAN, ITIL] } });
    if (path === '/tenant/modules/TIME' && request.method() === 'PUT') {
      on = [...on, 'TIME'];
      return route.fulfill({ json: { modules: [] } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/`);
  const more = page.getByRole('region', { name: 'More in Hubtask' });
  await more.waitFor({ timeout: 15_000 });
  await more.getByRole('listitem').filter({ hasText: 'Service desk' }).getByRole('button', { name: 'Try it' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.locator('[data-hub-template="itil_service_desk"] button[aria-pressed="true"]').waitFor({ timeout: 10_000 });
  assert.equal(new URL(page.url()).pathname, '/', 'Try it left Today instead of opening the dialog');
  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'detached', timeout: 5_000 });

  // No template switches Time on here: Try it goes to its home and its switch.
  await more.getByRole('listitem').filter({ has: page.locator('.offer-name', { hasText: /^Time$/ }) }).getByRole('button', { name: 'Try it' }).click();
  await page.waitForFunction(() => location.pathname === '/time', null, { timeout: 5_000 });
  await page.getByRole('button', { name: 'Switch Time on' }).click();
  // The page gives way to the product; the message keeps what it still needs.
  await page.getByRole('link', { name: 'Set up time: the time policy and kinds of work' }).waitFor({ timeout: 10_000 });
  assert.deepEqual(failures, []);
});
