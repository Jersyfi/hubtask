// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A module switched on is drawn on at once, without a reload (UC-INS-21), with a card that says
// where it is now and what it still needs (user-experience.md §2.8), and a module's page
// reached while it is off says so in place of its forms - no read refused, no form that fails -
// with the switch at hand, which asks in which existing hubs as *Modules* does (UC-INS-21 check
// 13). The server's half of the hubs named: config/ModuleSwitch_test.go. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { HUB, PAGE } from './fixture.mjs';
import { serve } from './serve.mjs';
import { productWalk } from './productwalk.mjs';
import { chipsOf, focused, valueOf } from './several.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const template = (key, modules) => ({
  key, version: 1, name: key, origin: 'HUBTASK', provenance: `${key}@1`, created_at: '2026-09-01T00:00:00Z',
  update_available: false, taken_version: null, hub_document: {},
  document: { modules: Object.fromEntries(modules.map((name) => [name, { switch: 'ON' }])) },
});

/** The workspace's switches, kept in memory as the server keeps them. */
function modulesBackend(off) {
  const states = ['PLANNING', 'TIME', 'SERVICE_DESK'].map((module) => ({ module, on: !off.includes(module), source: 'DEFAULT' }));
  return (method, path) => {
    if (path === '/tenant/modules' && method === 'GET') return { json: { modules: states } };
    if (path === '/config/templates' && method === 'GET') return { json: { data: [template('itil_service_desk', ['workflows', 'service_desk']), template('kanban_team', ['workflows'])] } };
    const one = /^\/tenant\/modules\/([A-Z_]+)$/.exec(path);
    if (one && method === 'PUT') {
      const state = states.find((each) => each.module === one[1]);
      Object.assign(state, { on: true, source: 'WORKSPACE' });
      return { json: { modules: states } };
    }
    return undefined;
  };
}

/**
 * Switches a module on at the workspace as a person does where hubs exist: the switch, then its
 * question answered with no hub chosen (UC-INS-21 check 13).
 */
async function switchOn(page, row) {
  await row.getByRole('switch').click();
  await answerNone(page);
}

/** The switch-on's question answered with no hub chosen: the product is on for hubs made from now on. */
async function answerNone(page) {
  await page.getByRole('dialog').getByRole('button', { name: 'Switch on', exact: true }).click();
}

test('chromium: a module switched on is drawn on without a reload', async (t) => {
  const browser = await sharedChromium(t);
  const walk = await productWalk(browser, { modules: [], answer: modulesBackend(['PLANNING', 'TIME', 'SERVICE_DESK']) });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/administration/modules`);
  for (const module of ['PLANNING', 'TIME']) {
    const row = page.locator(`[data-module="${module}"]`);
    const control = row.getByRole('switch');
    await control.waitFor({ timeout: 10_000 });
    assert.equal(await control.isChecked(), false, `${module} starts off`);
    await switchOn(page, row);
    await row.locator(`[data-reset="${module}"]`).waitFor({ timeout: 5_000 });
    assert.equal(await control.isChecked(), true, `${module} is drawn off after its write switched it on`);
    // The card under the switch says where the product is now (user-experience.md §2.8), one at a time.
    const card = page.locator(`[data-switched-on="${module}"]`);
    await card.waitFor({ timeout: 5_000 });
    assert.equal(await page.locator('[data-switched-on]').count(), 1, 'an earlier card stayed');
  }
  const time = page.locator('[data-switched-on="TIME"]');
  assert.equal(await time.getByRole('link', { name: 'Find it in Time' }).getAttribute('href'), '/time');
  assert.equal(await time.getByRole('link', { name: /^Set up time/u }).getAttribute('href'), '/time/settings');
  await time.getByRole('button', { name: 'Close' }).click();
  await time.waitFor({ state: 'detached', timeout: 5_000 });
  // Beside each switch that is on, its product's settings (user-experience.md §8.2); none while off.
  assert.equal(await page.locator('[data-module="TIME"]').getByRole('link', { name: 'Time settings' }).getAttribute('href'), '/time/settings');
  assert.equal(await page.locator('[data-module-settings="SERVICE_DESK"]').count(), 0, 'a module that is off links its settings');
  assert.deepEqual(walk.errors(), []);
});

// §2.8: the card's *Start with…* is the templates that need the module, one press each - at the
// workspace the press opens the page *Create hub* with that template chosen.
test('chromium: a module switched on starts with the templates that need it', async (t) => {
  const browser = await sharedChromium(t);
  const walk = await productWalk(browser, { modules: [], answer: modulesBackend(['SERVICE_DESK']) });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/administration/modules`);
  const row = page.locator('[data-module="SERVICE_DESK"]');
  await row.getByRole('switch').waitFor({ timeout: 10_000 });
  await switchOn(page, row);
  const card = page.locator('[data-switched-on="SERVICE_DESK"]');
  const start = card.locator('[data-start-with]');
  await start.getByRole('button', { name: 'ITIL service desk', exact: true }).waitFor({ timeout: 5_000 });
  assert.equal(await start.locator('[data-start-template]').count(), 1, 'a template that does not need the desk is offered');
  await start.getByRole('button', { name: 'ITIL service desk', exact: true }).click();
  // *Create hub* is a page of its own (ADR-0111), the template chosen on it.
  await page.waitForURL('**/hubs/new?template=itil_service_desk', { timeout: 5_000 });
  await page.locator('#main').getByRole('radio', { name: /^ITIL service desk/u, checked: true }).waitFor({ timeout: 5_000 });
  assert.deepEqual(walk.errors(), []);
});

test('chromium: the service desk page with the module off says so, reads nothing and offers the switch', async (t) => {
  const browser = await sharedChromium(t);
  const walk = await productWalk(browser, { modules: [], answer: modulesBackend(['SERVICE_DESK']) });
  t.after(walk.close);
  const { page } = walk;
  const asked = [];
  page.on('request', (request) => asked.push(new URL(request.url()).pathname));

  await page.goto(`${served.origin}/administration/service-desk`);
  await page.getByText('Service desk is switched off in this workspace').waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('main form').count(), 0, 'a form is offered that the server refuses');
  assert.equal(await page.getByRole('navigation', { name: 'Where you are in the administration' }).count(), 1, 'no trail back to the administration');
  assert.deepEqual(asked.filter((path) => /business-calendars|sla-policies|reply-templates|requester-organisations/.test(path)), [], 'the desk was read while off');

  await page.getByRole('button', { name: 'Switch Service desk on' }).click();
  await answerNone(page);
  await page.waitForTimeout(300);
  assert.ok(walk.written.some((each) => each.method === 'PUT' && each.path === '/tenant/modules/SERVICE_DESK'), 'the switch was not sent');
  assert.deepEqual(walk.errors(), []);
});

// Scenes homeplans/homeassets/homeagents, UC-WRK-36 check 22: a product switched off is drawn as its first empty state -
// its tile in its colour, its sentence, the switch, and that nothing kept for it is lost.
test('chromium: a product\'s home with its module off is drawn as its empty state with the switch', async (t) => {
  const browser = await sharedChromium(t);
  const walk = await productWalk(browser, { modules: [], answer: modulesBackend(['PLANNING']) });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/plans`);
  // A product without templates of its own: its band, the sentence and the switch (scene empty).
  await page.locator('.band[data-product="plans"]').getByRole('heading', { level: 1, name: 'Plans' }).waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('.empty[data-kind="unused"] > .tile').count(), 1, 'the product\'s tile');
  await page.locator('[data-module-off-note]').getByText('Nothing kept for it is lost while it is off.').waitFor();
  await page.getByRole('button', { name: /^Switch .+ on$/u }).click();
  await answerNone(page);
  await page.waitForTimeout(300);
  assert.ok(walk.written.some((each) => each.method === 'PUT' && each.path === '/tenant/modules/PLANNING'), 'the switch was not sent');
  assert.deepEqual(walk.errors(), []);
});

// Scene empty (the desk): a product switched off whose templates make a hub starts with its band, a
// template as the main way, *Start empty* beside it, and the templates as cards; the main way makes
// the hub from the template, which switches the product on.
test('chromium: a switched-off desk starts from a template, with start empty beside it and the templates as cards', async (t) => {
  const browser = await sharedChromium(t);
  const walk = await productWalk(browser, { modules: [], answer: modulesBackend(['SERVICE_DESK']) });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/desk`);
  await page.locator('.band[data-product="desk"]').getByRole('heading', { level: 1, name: 'Service desk' }).waitFor({ timeout: 10_000 });
  const main = page.getByRole('button', { name: 'Start with ITIL service desk' });
  await main.waitFor({ timeout: 10_000 });
  await page.getByRole('button', { name: 'Start empty' }).waitFor();
  await page.getByRole('heading', { name: 'Or start with a template' }).waitFor();
  assert.equal(await page.getByRole('button', { name: 'ITIL service desk', exact: true }).count(), 1, 'the template is not a card');
  // The main way opens the page *Create hub* (ADR-0111) on the template; Cancel comes back.
  await main.click();
  await page.waitForURL('**/hubs/new?template=itil_service_desk', { timeout: 5_000 });
  await page.locator('#main').getByRole('radio', { name: /^ITIL service desk/u, checked: true }).waitFor({ timeout: 5_000 });
  await page.locator('#main').getByRole('button', { name: 'Cancel', exact: true }).click();
  await page.waitForURL('**/desk', { timeout: 5_000 });
  await page.getByRole('button', { name: 'Start empty' }).click();
  await answerNone(page);
  await page.waitForTimeout(300);
  assert.ok(walk.written.some((each) => each.method === 'PUT' && each.path === '/tenant/modules/SERVICE_DESK'), 'start empty did not switch the desk on');
  assert.deepEqual(walk.errors(), []);
});

// UC-INS-21: Posts is a switch of its own beside Pages; switching it on switches Pages on in the
// same write, as the server answers (setting/Modules_test.go proves the refusal of Pages off under
// it), and the card leads to the posts' home.
test('chromium: switching Posts on draws Pages on with it and leads to the posts\' home', async (t) => {
  const browser = await sharedChromium(t);
  const states = ['KNOWLEDGE', 'POSTS'].map((module) => ({ module, on: false, source: 'DEFAULT' }));
  const answer = (method, path) => {
    if (path === '/tenant/modules' && method === 'GET') return { json: { modules: states } };
    if (path === '/config/templates' && method === 'GET') return { json: { data: [] } };
    if (path === '/tenant/modules/POSTS' && method === 'PUT') {
      for (const state of states) Object.assign(state, { on: true, source: 'WORKSPACE' });
      return { json: { modules: states } };
    }
    return undefined;
  };
  const walk = await productWalk(browser, { modules: [], answer });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/administration/modules`);
  const posts = page.locator('[data-module="POSTS"]');
  await posts.getByRole('switch').waitFor({ timeout: 10_000 });
  await posts.getByText('Posts', { exact: true }).first().waitFor();
  await switchOn(page, posts);
  await page.locator('[data-switched-on="POSTS"]').getByRole('link', { name: 'Find it in Posts' }).waitFor({ timeout: 5_000 });
  assert.equal(await page.locator('[data-switched-on="POSTS"]').getByRole('link', { name: 'Find it in Posts' }).getAttribute('href'), '/posts');
  assert.equal(await page.locator('[data-module="KNOWLEDGE"]').getByRole('switch').isChecked(), true, 'Pages stayed off under Posts');
  assert.deepEqual(walk.errors(), []);
});

const GARDEN = { ...HUB, id: '01a0e2e0-0000-7000-8000-00000014b0b1', name: 'Garden', order_key: 'a1' };

/** Plans off at the workspace among these hubs; the switch is kept as the server keeps it. */
function plansOff(hubs) {
  const on = [];
  const answer = (method, path, query) => {
    if (path === '/containers' && method === 'GET') return { json: { ...PAGE, data: query.get('type') === 'HUB' ? hubs : [] } };
    if (path === '/config/templates' && method === 'GET') return { json: { data: [] } };
    if (path === '/tenant/modules/PLANNING' && method === 'PUT') {
      on.push('PLANNING');
      return { json: { modules: [{ module: 'PLANNING', on: true, source: 'WORKSPACE' }] } };
    }
    return undefined;
  };
  return { on, answer };
}

const sentSwitches = (walk) => walk.written.filter((each) => each.method === 'PUT' && each.path === '/tenant/modules/PLANNING').map((each) => each.body);
const optionNames = (list) => list.getByRole('option').evaluateAll((options) => options.map((option) => document.getElementById(option.getAttribute('aria-labelledby'))?.textContent?.trim()));

// UC-INS-21 check 13 on a product's own page: the switch asks the question of *Modules*, the hubs
// a list from data and so the field (design-system.md §4), none chosen; the hub chosen goes with
// the switch, and the page gives way to the product.
for (const [width, height] of [[1280, 900], [390, 844]]) {
  test(`chromium: ${width} px - a product's page while it is off asks in which existing hubs and switches on in the one chosen (UC-INS-21 check 13)`, async (t) => {
    const browser = await sharedChromium(t);
    const shared = plansOff([HUB, GARDEN]);
    const walk = await productWalk(browser, { modules: shared.on, answer: shared.answer, width, height, isMobile: width < 600 });
    t.after(walk.close);
    const { page } = walk;

    await page.goto(`${served.origin}/plans`);
    const control = page.getByRole('button', { name: 'Switch Plans on' });
    await control.waitFor({ timeout: 10_000 });
    await control.click();
    const dialog = page.getByRole('dialog', { name: 'Switch Plans on' });
    await dialog.waitFor({ timeout: 5_000 });
    const field = dialog.getByRole('button', { name: /^Switch on in which existing hubs\?/u });
    await field.waitFor({ timeout: 5_000 });
    assert.equal(await valueOf(field), 'Nothing chosen', 'a hub is chosen before the person chose');
    assert.deepEqual(sentSwitches(walk), [], 'the switch was sent before its question was answered');

    await field.click();
    const list = page.getByRole('listbox');
    await list.waitFor({ timeout: 5_000 });
    assert.deepEqual(await optionNames(list), ['House', 'Garden']);
    await list.getByRole('option', { name: 'Garden', exact: true }).click();
    if (width < 600) await page.getByRole('button', { name: 'Done', exact: true }).click();
    else await page.keyboard.press('Escape');
    await list.waitFor({ state: 'detached', timeout: 5_000 });
    assert.deepEqual(await chipsOf(field), ['Garden']);
    await dialog.getByRole('button', { name: 'Switch on', exact: true }).click();

    await page.getByText('Plans is on, and on in 1 hub.').first().waitFor({ timeout: 5_000 });
    assert.deepEqual(sentSwitches(walk), [{ switch: 'ON', hub_ids: [GARDEN.id] }]);
    await control.waitFor({ state: 'detached', timeout: 10_000 });
    assert.deepEqual(walk.errors(), []);
  });
}

// The keyboard's path through the same question: focus starts on the list, Escape cancels,
// switches nothing and gives the focus back; Enter on the primary switches.
test('chromium: the product page\'s question by the keyboard - Escape switches nothing, Enter on Switch on sends the hub ticked', async (t) => {
  const browser = await sharedChromium(t);
  const shared = plansOff([HUB, GARDEN]);
  const walk = await productWalk(browser, { modules: shared.on, answer: shared.answer });
  t.after(walk.close);
  const { page } = walk;
  const onTheField = () => page.waitForFunction(() => document.activeElement?.getAttribute('aria-haspopup') === 'listbox', null, { timeout: 5_000 });

  await page.goto(`${served.origin}/plans`);
  const control = page.getByRole('button', { name: 'Switch Plans on' });
  await control.waitFor({ timeout: 10_000 });
  await control.focus();
  await page.keyboard.press('Enter');
  const dialog = page.getByRole('dialog', { name: 'Switch Plans on' });
  await dialog.waitFor({ timeout: 5_000 });
  await onTheField();
  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.deepEqual(sentSwitches(walk), [], 'Escape switched the product on');
  assert.equal(await focused(page), 'button "Switch Plans on"', 'the focus did not return to the switch');

  await page.keyboard.press('Enter');
  await dialog.waitFor({ timeout: 5_000 });
  await onTheField();
  await page.keyboard.press('Enter');
  await page.getByRole('listbox').waitFor({ timeout: 5_000 });
  await page.waitForFunction(() => document.activeElement?.getAttribute('role') === 'option', null, { timeout: 5_000 });
  await page.keyboard.press('Space');
  await page.keyboard.press('Escape');
  await page.getByRole('listbox').waitFor({ state: 'detached', timeout: 5_000 });
  await onTheField();
  await page.keyboard.press('Tab');
  await page.keyboard.press('Tab');
  assert.equal(await focused(page), 'button "Switch on"');
  await page.keyboard.press('Enter');
  await page.getByText('Plans is on, and on in 1 hub.').first().waitFor({ timeout: 5_000 });
  assert.deepEqual(sentSwitches(walk), [{ switch: 'ON', hub_ids: [HUB.id] }]);
  assert.deepEqual(walk.errors(), []);
});

test('chromium: a product\'s page while it is off switches on at once where no hub is offered (UC-INS-21 checks 2, 13)', async (t) => {
  const browser = await sharedChromium(t);
  const shared = plansOff([]);
  const walk = await productWalk(browser, { modules: shared.on, answer: shared.answer });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/plans`);
  const control = page.getByRole('button', { name: 'Switch Plans on' });
  await control.waitFor({ timeout: 10_000 });
  await control.click();
  await page.getByText('Plans is on.', { exact: true }).first().waitFor({ timeout: 5_000 });
  assert.equal(await page.getByRole('dialog').count(), 0, 'a question without a hub to choose');
  assert.deepEqual(sentSwitches(walk), [{ switch: 'ON' }]);
  await control.waitFor({ state: 'detached', timeout: 10_000 });
  assert.deepEqual(walk.errors(), []);
});

// A private hub counts only by the role held on it, read from its memberships. Where that read
// fails, the hub is not offered and the question still comes with the hubs that are decided -
// never a switch that waits for an answer that does not come (UC-INS-21 check 13).
test('chromium: a private hub whose memberships fail to read is left out, and the question still comes (UC-INS-21 check 13)', async (t) => {
  const browser = await sharedChromium(t);
  const secret = { ...GARDEN, name: 'Secret', private: true };
  const shared = plansOff([HUB, secret]);
  const walk = await productWalk(browser, {
    modules: shared.on,
    answer: (method, path, query, body) => {
      if (path === '/memberships' && method === 'GET' && query.get('scope_id') === secret.id) {
        return { status: 429, contentType: 'application/problem+json', json: { title: 'Too many requests', status: 429, code: 'rate_limited' } };
      }
      return shared.answer(method, path, query, body);
    },
  });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/plans`);
  const control = page.getByRole('button', { name: 'Switch Plans on' });
  await control.waitFor({ timeout: 10_000 });
  await control.click();
  const dialog = page.getByRole('dialog', { name: 'Switch Plans on' });
  await dialog.waitFor({ timeout: 5_000 });
  await dialog.getByRole('button', { name: /^Switch on in which existing hubs\?/u }).click();
  const list = page.getByRole('listbox');
  await list.waitFor({ timeout: 5_000 });
  assert.deepEqual(await optionNames(list), ['House'], 'a private hub whose role is unknown was offered');
  assert.deepEqual(sentSwitches(walk), []);
});
