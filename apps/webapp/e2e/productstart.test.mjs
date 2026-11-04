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

import { ACCOUNT, DEFAULT_STRUCTURE, MANIFEST, PAGE, forgetUnstubbed, stub } from './fixture.mjs';
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

test('chromium: an empty product home offers its templates, and one makes a hub that opens on its plan', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  forgetUnstubbed();
  const hubs = [];
  const planFor = [];
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
    if (path === '/containers' && method === 'POST') {
      const made = { id: NEW_HUB, type: 'HUB', parent_id: null, order_key: 'a1', version: 1, structure: DEFAULT_STRUCTURE, ...request.postDataJSON() };
      hubs.push(made);
      return route.fulfill({ status: 201, json: made });
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
  await dialog.getByLabel('Hub name').fill('Delivery');
  await dialog.getByRole('button', { name: 'Create hub', exact: true }).click();
  await page.waitForURL(`**/hubs/${NEW_HUB}/settings/modules?template=${SCRUM.key}`, { timeout: 10_000 });
  // The live stream brings the new hub to a running client; this fixture has none, so the address
  // is opened afresh, where the replica's first read carries the hub.
  const fresh = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => fresh.close());
  const again = await open(fresh);
  await again.goto(page.url());
  await again.getByRole('dialog').getByText('Adds the workflow “scrum”').waitFor({ timeout: 10_000 });
  assert.ok(planFor.length > 0 && planFor.every((hub) => hub === NEW_HUB), 'the template\'s plan was not asked for the new hub');
  assert.deepEqual(failures, []);
});
