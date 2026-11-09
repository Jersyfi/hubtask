// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A workspace's first start (UC-WRK-30 check 5, user-experience.md §3.2), walked on Home: whoever
// `GET /tenant` offers it to is asked what is run here, by the catalogue's families; *Simple* holds
// the four purposes, each one command that makes a hub and its collections on matching presets; a
// template is one command too, with its answers and samples, after its plan in words. Each closes
// the question, retried under the same key after a failure; a skip closes it and creates nothing;
// where it is not offered nobody is asked. Chromium only; who is offered it is the server's tests'.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { HUB, WORKSPACE, signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

/** This walk's own identifiers: no other fixture record uses them. */
const STARTED_HUB = '01a0e2e0-0000-7000-8000-0000000028a1';
const FIRST_LINE = '01a0e2e0-0000-7000-8000-0000000028a2';

/** A template of the catalogue with a hub part, its question and its samples. */
const DESK = {
  key: 'itil_service_desk', version: 11, name: 'itil_service_desk', origin: 'HUBTASK', provenance: 'itil_service_desk@11',
  created_at: '2026-01-01T00:00:00Z', taken_version: null, update_available: false,
  document: { modules: { workflows: { switch: 'ON' }, service_desk: { switch: 'ON' }, knowledge: { switch: 'ON' } }, types: { incident: { level: 'TASK', name: 'Incident' } } },
  hub_document: { samples: { s1: { title: 'Printer offline' } } },
  questions: [{ key: 'first_group', kind: 'GROUP', options: [], required: false }],
};
const PLAN = {
  base_hash: 'b', plan_hash: 'p', changes: [{ op: 'ADD', section: 'types', key: 'incident', path: 'types.incident', after: { name: 'Incident' } }],
  operations: [], refusals: [], flags: { hides_data: false, changes_permissions: false }, step_up: false, reach: [],
  counts: { added: 1, changed: 0, removed: 0 }, template: 'itil_service_desk@11', hub_id: null, collection_id: null,
  from_version: null, upgrade: [], document: DESK.document, mode: 'MERGE',
};

async function as(browser, offered, { failFirstApply = false, skipped = false } = {}) {
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  const sent = [];
  let closed = false;
  let applies = 0;
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/tenant' && request.method() === 'GET') {
      return route.fulfill({ json: { ...WORKSPACE, starter_offered: offered && !closed, starter_skipped: skipped && applies === 0 } });
    }
    if (path === '/tenant/starter:close' && request.method() === 'POST') {
      sent.push({ path });
      closed = true;
      return route.fulfill({ status: 204, body: '' });
    }
    if (path === '/tenant/starter:apply' && request.method() === 'POST') {
      const body = request.postDataJSON();
      sent.push({ path, body, key: request.headers()['idempotency-key'] });
      applies += 1;
      if (failFirstApply && applies === 1) {
        return route.fulfill({ status: 503, contentType: 'application/problem+json', json: { type: 'about:blank', title: 'Unavailable', status: 503, code: 'service.unavailable' } });
      }
      closed = true;
      return route.fulfill({ status: 201, json: { ...HUB, id: STARTED_HUB, type: 'HUB', parent_id: null, name: body.hub_name } });
    }
    if (path === '/config/templates' && request.method() === 'GET') {
      return route.fulfill({ json: { data: [DESK] } });
    }
    if (path === '/config/templates/itil_service_desk:plan' && request.method() === 'POST') {
      return route.fulfill({ json: PLAN });
    }
    if (path === '/groups' && request.method() === 'POST') {
      sent.push({ path, body: request.postDataJSON() });
      return route.fulfill({ status: 201, json: { id: FIRST_LINE, name: request.postDataJSON().name, description: null, member_count: 0, created_at: '2026-01-01T00:00:00Z' } });
    }
    if (path === '/containers' && request.method() === 'POST') {
      sent.push({ path, body: request.postDataJSON() });
      return route.fulfill({ status: 500, json: {} });
    }
    return stub(route);
  });
  return { page, failures, context, sent };
}

const dialogOf = (page) => page.locator('[data-starter]');

test('chromium: the first start - asked once, answered by one command making a hub and collections on presets', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, sent } = await as(browser, true);
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  const dialog = dialogOf(page);
  await dialog.waitFor({ timeout: 10_000 });
  await dialog.locator('[data-starter-family="service"]').waitFor({ timeout: 10_000 });
  await dialog.locator('[data-starter-family="simple"] button').click();
  for (const purpose of ['MYSELF', 'HOUSEHOLD', 'CLIENTS', 'TEAM']) {
    assert.equal(await dialog.locator(`[data-starter-purpose="${purpose}"]`).count(), 1, `${purpose} is not offered`);
  }
  assert.match(await dialog.locator('[data-starter-samples-note]').innerText(), /sample entries/, 'the sets do not say they bring sample entries');
  await dialog.locator('[data-starter-purpose="HOUSEHOLD"] button').click();
  await page.waitForURL(`**/hubs/${STARTED_HUB}`, { timeout: 10_000 });

  assert.equal(sent.length, 1, `one command makes the set, not ${sent.length} writes`);
  assert.equal(sent[0].path, '/tenant/starter:apply');
  assert.match(sent[0].key ?? '', /^[0-9a-f-]{36}$/, 'the set is sent without an Idempotency-Key');
  assert.deepEqual(sent[0].body, {
    hub_name: 'Home',
    hub_preset: 'TASKS',
    collections: [
      { name: 'Shopping', preset: 'LIST', samples: ['Milk', 'Detergent'] },
      { name: 'Chores', samples: ['Take out the bins', 'Water the plants'] },
      { name: 'Projects', preset: 'PROJECT', samples: ['Paint the hallway'] },
    ],
  });
  // Not asked again.
  await page.goto(`${served.origin}/`);
  await page.waitForTimeout(1200);
  assert.equal(await dialogOf(page).isVisible(), false, 'asked a second time');
  assert.deepEqual(failures, []);
});

test('chromium: the first start failed once - the retry sends the same key and makes one set', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, sent } = await as(browser, true, { failFirstApply: true });
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  const dialog = dialogOf(page);
  await dialog.waitFor({ timeout: 10_000 });
  await dialog.locator('[data-starter-family="simple"] button').click();
  await dialog.locator('[data-starter-purpose="TEAM"] button').click();
  await dialog.getByRole('alert').waitFor({ timeout: 10_000 });
  await dialog.locator('[data-starter-purpose="TEAM"] button').click();
  await page.waitForURL(`**/hubs/${STARTED_HUB}`, { timeout: 10_000 });

  const applied = sent.filter((write) => write.path === '/tenant/starter:apply');
  assert.equal(applied.length, 2);
  assert.equal(applied[0].key, applied[1].key, 'the retry was sent under a new key, which could make a second set');
  assert.deepEqual(sent.filter((write) => write.path !== '/tenant/starter:apply'), [], 'something besides the one command was written');
});

test('chromium: the first start skipped - closed, nothing made', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, sent } = await as(browser, true);
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  await dialogOf(page).waitFor({ timeout: 10_000 });
  await page.getByRole('button', { name: 'Skip' }).last().click();
  await page.waitForTimeout(600);
  assert.deepEqual(sent, [{ path: '/tenant/starter:close' }]);
  assert.equal(await dialogOf(page).isVisible(), false);
  assert.deepEqual(failures, []);
});

test('chromium: where the first start is not offered, nobody is asked', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, sent } = await as(browser, false);
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  await page.locator('aside.sidenav').waitFor({ timeout: 10_000 });
  await page.waitForTimeout(1200);
  assert.equal(await dialogOf(page).count() > 0 && (await dialogOf(page).isVisible()), false, 'asked where it is not offered');
  assert.deepEqual(sent, []);
  assert.deepEqual(failures, []);
});

test('chromium: the first start on a template - its family, its plan in words, then one command with the template and its samples', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, sent } = await as(browser, true);
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  const dialog = dialogOf(page);
  await dialog.locator('[data-starter-family="service"] button').click({ timeout: 10_000 });
  const card = dialog.locator('[data-starter-template="itil_service_desk"]');
  assert.match(await card.innerText(), /Service desk|Pages/i, 'the card names neither the products it switches on nor itself');
  assert.doesNotMatch(await card.innerText(), /itil_service_desk/, 'the card shows a machine key');
  await card.locator('button').last().click();
  await dialog.locator('[data-plan-changes] li').first().waitFor({ timeout: 10_000 });
  assert.match(await dialog.locator('[data-plan-changes]').innerText(), /Adds the .*Incident/, 'the plan is not said in words');
  // What it creates stands open in the form, not behind a fold (§3.2).
  assert.equal(await dialog.locator('[data-starter-plan] [data-template-preview]').isVisible(), true, 'what the template creates is folded away');
  assert.match(await dialog.locator('[data-starter-plan] [data-template-preview]').innerText(), /Entry types/);
  assert.equal(await dialog.locator('input[data-template-samples]').isChecked(), true, 'sample entries are not on by default');
  // A GROUP question makes its group in place, and the new group is the answer (§3.2).
  const makeGroup = dialog.locator('[data-create-group="first_group"]');
  await makeGroup.locator('input').fill('First line');
  await makeGroup.locator('button').click();
  await page.waitForFunction(() => document.querySelector('[data-create-group="first_group"] input')?.value === '', undefined, { timeout: 10_000 });
  await page.getByRole('dialog').getByRole('button', { name: 'Set up', exact: true }).last().click();
  await page.waitForURL(new RegExp(STARTED_HUB), { timeout: 10_000 });

  assert.deepEqual(sent.find((write) => write.path === '/groups')?.body, { name: 'First line' });
  assert.equal(sent.find((write) => write.path === '/tenant/starter:apply').body.answers?.first_group, FIRST_LINE, 'the group made is not the answer');
  sent.splice(0, sent.length, ...sent.filter((write) => write.path !== '/groups'));
  assert.deepEqual(sent.map((write) => write.path), ['/tenant/starter:apply'], 'more than the one command was written');
  assert.equal(sent[0].body.template, 'itil_service_desk');
  assert.equal(sent[0].body.samples, true);
  assert.equal(sent[0].body.hub_preset, undefined, 'a template set chose a preset of its own');
  assert.ok(sent[0].body.hub_name.length > 0);
  assert.deepEqual(failures, []);
});

// After a skip, Today's first steps carry *Choose what you use Hubtask for* (user-experience.md
// §3.3), which opens the same dialog; dismissing it sends nothing, choosing makes the one set.
test('chromium: a skipped first start is offered on Today and opens the same dialog', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, sent } = await as(browser, false, { skipped: true });
  t.after(() => context.close());
  await context.route('**/api/v1/items:query', async (route) => {
    const body = route.request().postDataJSON();
    if (body?.scope?.container_id === HUB.id && body.page?.size === 1) return route.fulfill({ json: { data: [], groups: [], page: { next_cursor: null, has_more: false } } });
    return route.fallback();
  });

  await page.goto(`${served.origin}/`);
  const steps = page.getByRole('region', { name: 'First steps' });
  await steps.waitFor({ timeout: 15_000 });
  await page.waitForTimeout(800);
  assert.equal(await dialogOf(page).isVisible(), false, 'a skipped first start opened by itself');
  const choose = steps.locator('[data-step-choose]');
  assert.match((await choose.textContent()) ?? '', /Choose what you use Hubtask for/u);

  await choose.getByRole('button', { name: 'Choose' }).click();
  const dialog = dialogOf(page);
  await dialog.waitFor({ timeout: 10_000 });
  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'hidden', timeout: 10_000 });
  assert.deepEqual(sent, [], 'dismissing the dialog again sent something');

  await choose.getByRole('button', { name: 'Choose' }).click();
  await dialog.waitFor({ timeout: 10_000 });
  await dialog.locator('[data-starter-family="simple"] button').click();
  await dialog.locator('[data-starter-purpose="HOUSEHOLD"] button').click();
  await page.waitForURL(`**/hubs/${STARTED_HUB}`, { timeout: 10_000 });
  assert.deepEqual(sent.map((write) => write.path), ['/tenant/starter:apply']);
  assert.deepEqual(failures, []);
});
