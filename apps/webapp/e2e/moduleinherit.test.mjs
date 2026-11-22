// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A product switched on at the workspace is offered to the hubs, not switched on in them
// (UC-INS-21 checks 2, 3 and 13): the switch asks in which existing hubs, none chosen; the hub
// chosen has it on, the other off and free to switch it on itself, a hub made afterwards on; off at
// the workspace locks every hub's switch with its reason, and on again gives each hub the state it
// had. The backend below keeps the switches as the server does (setting/ModuleHubs_test.go proves
// the server's half). Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { HUB, PAGE, chooseNew } from './fixture.mjs';
import { serve } from './serve.mjs';
import { ids, productWalk } from './productwalk.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const GARDEN = { ...HUB, id: '01a0e2e0-0000-7000-8000-0000001867b1', name: 'Garden', order_key: 'a1' };
const MODULE = 'PLANNING';

/**
 * One workspace's Planning switch and its hubs', kept as the server keeps them: a hub without a
 * value of its own follows the workspace, and a workspace switch that turns writes each such hub
 * the state it had. `on` is the manifest's `modules_on`, which the walk hands to `productWalk`.
 */
function workspace(hubs) {
  const on = [];
  const own = new Map();
  const newId = ids('1867c');
  let stored = false;
  const isOn = () => on.includes(MODULE);
  const state = (hubId) => {
    if (!hubId) return { module: MODULE, on: isOn(), source: stored ? 'WORKSPACE' : 'DEFAULT' };
    const mine = own.get(hubId);
    return { module: MODULE, on: isOn() && (mine ?? true), source: mine === undefined ? (stored ? 'WORKSPACE' : 'DEFAULT') : 'HUB' };
  };
  const answer = (method, path, query, body) => {
    if (path === '/containers' && method === 'GET') {
      return { json: { ...PAGE, data: query.get('type') === 'HUB' ? hubs : [] } };
    }
    if (path === '/containers' && method === 'POST') {
      const made = { ...HUB, id: newId(), name: body.name, order_key: `a${hubs.length + 1}` };
      hubs.push(made);
      return { status: 201, json: made };
    }
    const one = hubs.find((each) => path === `/containers/${each.id}`);
    if (one && method === 'GET') return { json: one };
    if (path === '/config/templates' && method === 'GET') return { json: { data: [] } };
    if (path === '/tenant/modules' && method === 'GET') return { json: { modules: [state(query.get('hub_id') ?? undefined)] } };
    if (path === `/tenant/modules/${MODULE}/reach` && method === 'GET') return { json: { module: MODULE, on: isOn(), people: 0 } };
    if (path === `/tenant/modules/${MODULE}` && method === 'PUT') {
      const next = body.switch === 'ON';
      if (body.hub_id) {
        if (next && !isOn()) return { status: 403, json: { code: 'forbidden', detail_code: 'setting.not_allowed', status: 403, request_id: 'req_e2e' } };
        if (body.switch === 'INHERIT') own.delete(body.hub_id); else own.set(body.hub_id, next);
        return { json: { modules: [state(body.hub_id)] } };
      }
      const was = isOn();
      stored = body.switch !== 'INHERIT';
      if (was !== next) {
        for (const hub of hubs) if (!own.has(hub.id)) own.set(hub.id, was);
        if (next) on.push(MODULE); else on.splice(on.indexOf(MODULE), 1);
      }
      for (const id of body.hub_ids ?? []) own.set(id, true);
      return { json: { modules: [state()] } };
    }
    return undefined;
  };
  return { on, answer };
}

/** The hub's own switch in its settings, once it is drawn. */
async function hubSwitch(page, hub) {
  await page.goto(`${served.origin}/hubs/${hub.id}/settings/modules`);
  const row = page.locator(`[data-module="${MODULE}"]`);
  await row.getByRole('switch').waitFor({ timeout: 10_000 });
  return row;
}

/** The workspace's switch on *Modules*, once it is drawn. */
async function workspaceSwitch(page) {
  await page.goto(`${served.origin}/administration/modules`);
  const row = page.locator(`[data-module="${MODULE}"]`);
  await row.getByRole('switch').waitFor({ timeout: 10_000 });
  return row;
}

test('chromium: switching a product on asks in which existing hubs, and each hub keeps its state (UC-INS-21 checks 3, 13)', async (t) => {
  const browser = await sharedChromium(t);
  const hubs = [HUB, GARDEN];
  const shared = workspace(hubs);
  const walk = await productWalk(browser, { modules: shared.on, answer: shared.answer });
  t.after(walk.close);
  const { page } = walk;
  const switches = () => walk.written.filter((each) => each.method === 'PUT' && each.path === `/tenant/modules/${MODULE}`);

  // The question: every hub the person may shape, none chosen, and nothing sent before it is answered.
  let row = await workspaceSwitch(page);
  await row.getByRole('switch').click();
  const dialog = page.getByRole('dialog');
  const question = dialog.getByRole('group', { name: 'Switch on in which existing hubs?' });
  await question.waitFor({ timeout: 5_000 });
  assert.deepEqual(await question.getByRole('checkbox').evaluateAll((boxes) => boxes.map((box) => box.checked)), [false, false], 'a hub is chosen before the person chose');
  assert.equal(await question.getByRole('checkbox', { name: 'House', exact: true }).count(), 1);
  assert.equal(await question.getByRole('checkbox', { name: 'Garden', exact: true }).count(), 1);
  assert.equal(switches().length, 0, 'the switch was sent before its question was answered');
  assert.equal(await row.getByRole('switch').isChecked(), false, 'the switch is drawn on while its question stands');

  await question.getByRole('checkbox', { name: 'House', exact: true }).check();
  await dialog.getByRole('button', { name: 'Switch on', exact: true }).click();
  await row.locator(`[data-reset="${MODULE}"]`).waitFor({ timeout: 5_000 });
  assert.deepEqual(switches().map((each) => each.body), [{ switch: 'ON', hub_ids: [HUB.id] }]);
  assert.equal(await row.getByRole('switch').isChecked(), true);
  // The count in its plural form (known-traps.md, "A count needs the plural form").
  await page.getByText('Plans is on, and on in 1 hub.').first().waitFor({ timeout: 5_000 });

  // The hub chosen has it on; the other has it off and may switch it on itself (check 3).
  row = await hubSwitch(page, HUB);
  assert.equal(await row.getByRole('switch').isChecked(), true, 'the chosen hub is off');
  row = await hubSwitch(page, GARDEN);
  assert.equal(await row.getByRole('switch').isChecked(), false, 'a hub nobody chose was switched on');
  assert.equal(await row.getByRole('switch').isEnabled(), true, 'a hub may not switch on what the workspace has on');

  // A hub made afterwards starts with it on.
  await page.goto(`${served.origin}/`);
  await page.getByRole('tree').first().waitFor({ timeout: 10_000 });
  await chooseNew(page, 'Create hub');
  const create = page.locator('dialog[open]');
  await create.getByRole('textbox', { name: 'Hub name' }).fill('Workshop');
  await create.getByRole('button', { name: 'Create hub' }).click();
  await create.waitFor({ state: 'hidden', timeout: 10_000 });
  const workshop = hubs.find((each) => each.name === 'Workshop');
  assert.ok(workshop, 'the hub was not made');
  row = await hubSwitch(page, workshop);
  assert.equal(await row.getByRole('switch').isChecked(), true, 'a hub made afterwards starts without the product');

  // Off at the workspace takes it from every hub: the switch there is locked and says why,
  // although the hub keeps a value of its own (check 3).
  row = await workspaceSwitch(page);
  await row.getByRole('switch').click();
  await page.getByRole('dialog').getByRole('button', { name: 'Switch off', exact: true }).click();
  await page.getByText('Plans is off. Its data is kept.').first().waitFor({ timeout: 5_000 });
  row = await hubSwitch(page, HUB);
  assert.equal(await row.getByRole('switch').isChecked(), false, 'the hub kept the product the workspace has off');
  assert.equal(await row.getByRole('switch').isEnabled(), false, 'a hub is offered a switch the server refuses');
  await row.getByRole('switch').hover({ force: true });
  await page.getByText('Off for the whole workspace, so it cannot be switched on for one hub.').first().waitFor({ timeout: 5_000 });

  // On again, nothing chosen: each hub has the state it had (check 13).
  row = await workspaceSwitch(page);
  await row.getByRole('switch').click();
  const again = page.getByRole('dialog').getByRole('group', { name: 'Switch on in which existing hubs?' });
  await again.waitFor({ timeout: 5_000 });
  assert.equal(await again.getByRole('checkbox').count(), 3, 'the hub made since is not offered');
  assert.deepEqual(await again.getByRole('checkbox').evaluateAll((boxes) => boxes.map((box) => box.checked)), [false, false, false]);
  await page.getByRole('dialog').getByRole('button', { name: 'Switch on', exact: true }).click();
  await page.getByText('Plans is on.', { exact: true }).first().waitFor({ timeout: 5_000 });
  assert.deepEqual(switches().at(-1).body, { switch: 'ON' }, 'hubs were sent that nobody chose');
  for (const [hub, expected] of [[HUB, true], [GARDEN, false], [workshop, true]]) {
    row = await hubSwitch(page, hub);
    assert.equal(await row.getByRole('switch').isChecked(), expected, `${hub.name} did not get the state it had`);
  }
  assert.deepEqual(walk.errors(), []);
});

test('chromium: leaving the question switches nothing on', async (t) => {
  const browser = await sharedChromium(t);
  const shared = workspace([HUB, GARDEN]);
  const walk = await productWalk(browser, { modules: shared.on, answer: shared.answer });
  t.after(walk.close);
  const { page } = walk;

  const row = await workspaceSwitch(page);
  await row.getByRole('switch').click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('button', { name: 'Keep it off', exact: true }).last().click();
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.equal(await row.getByRole('switch').isChecked(), false);
  assert.deepEqual(walk.written.filter((each) => each.method === 'PUT'), []);
  assert.deepEqual(walk.errors(), []);
});

test('chromium: without a hub the switch goes through without a question (UC-INS-21 check 2)', async (t) => {
  const browser = await sharedChromium(t);
  const shared = workspace([]);
  const walk = await productWalk(browser, { modules: shared.on, answer: shared.answer });
  t.after(walk.close);
  const { page } = walk;

  const row = await workspaceSwitch(page);
  await row.getByRole('switch').click();
  await page.locator(`[data-switched-on="${MODULE}"]`).waitFor({ timeout: 5_000 });
  assert.equal(await page.getByRole('dialog').count(), 0, 'a question without a hub to choose');
  assert.deepEqual(walk.written.filter((each) => each.method === 'PUT').map((each) => each.body), [{ switch: 'ON' }]);
  assert.deepEqual(walk.errors(), []);
});

// The switch pressed before the hub list has answered waits for it: decided on the hubs read so
// far - none - it would go through unasked (check 13).
test('chromium: a switch pressed while the hub list is still being read waits and then asks (UC-INS-21 check 13)', async (t) => {
  const browser = await sharedChromium(t);
  const shared = workspace([HUB, GARDEN]);
  const walk = await productWalk(browser, { modules: shared.on, answer: shared.answer });
  t.after(walk.close);
  const { page } = walk;
  let release;
  const held = new Promise((resolve) => { release = resolve; });
  await walk.context.route('**/api/v1/containers?*', async (route) => {
    if (new URL(route.request().url()).searchParams.get('type') === 'HUB') await held;
    return route.fallback();
  });

  const row = await workspaceSwitch(page);
  await row.getByRole('switch').click();
  await page.waitForTimeout(500);
  assert.equal(await page.getByRole('dialog').count(), 0, 'the question stands before the hubs are read');
  assert.deepEqual(walk.written.filter((each) => each.method === 'PUT'), [], 'the switch went through unasked while the hub list was out');
  assert.equal(await row.getByRole('switch').isChecked(), false, 'the switch is drawn on while it waits');

  release();
  const question = page.getByRole('dialog').getByRole('group', { name: 'Switch on in which existing hubs?' });
  await question.waitFor({ timeout: 5_000 });
  assert.equal(await question.getByRole('checkbox').count(), 2, 'the question does not offer both hubs');
  assert.deepEqual(walk.written.filter((each) => each.method === 'PUT'), []);
  assert.deepEqual(walk.errors(), []);
});

// On a phone the question takes the whole screen, its primary in the bar (page-system §5).
test('chromium: the question on a phone fills the screen and switches on in the hubs chosen (UC-INS-21 check 13)', async (t) => {
  const browser = await sharedChromium(t);
  const shared = workspace([HUB, GARDEN]);
  const walk = await productWalk(browser, { modules: shared.on, answer: shared.answer, width: 390, height: 844, isMobile: true });
  t.after(walk.close);
  const { page } = walk;

  const row = await workspaceSwitch(page);
  await row.getByRole('switch').click();
  const dialog = page.getByRole('dialog');
  const question = dialog.getByRole('group', { name: 'Switch on in which existing hubs?' });
  await question.waitFor({ timeout: 5_000 });
  const box = await dialog.boundingBox();
  assert.ok(box.width >= 390 * 0.9 && box.height >= 844 * 0.9, `the question is ${box.width}x${box.height} on a 390x844 screen`);
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), 'the question scrolls sideways');
  await question.getByRole('checkbox', { name: 'Garden', exact: true }).check();
  await question.getByRole('checkbox', { name: 'House', exact: true }).check();
  await dialog.getByRole('button', { name: 'Switch on', exact: true }).click();
  await page.getByText('Plans is on, and on in 2 hubs.').first().waitFor({ timeout: 5_000 });
  assert.deepEqual(walk.written.filter((each) => each.method === 'PUT').map((each) => each.body), [{ switch: 'ON', hub_ids: [GARDEN.id, HUB.id] }]);
  assert.deepEqual(walk.errors(), []);
});
