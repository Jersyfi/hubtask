// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *Create hub* is a page of its own (ADR-0111): UC-WRK-01 checks 9 and 10, UC-ID-16 check 9, and
// which hub a reader is offered (UC-WRK-01 check 6, UC-ID-16 check 1), walked per reader against
// the manifest the server answers:
//
// * at 1280 × 800 the name, *Private*, colour, symbol and description stand beside the starts, all
//   on the screen (check 9); *Empty hub* is chosen when the page opens and stays offered whatever is
//   searched, the chosen start is marked and the summary says what it sets up (check 10);
// * a template chosen with *Private* on is sent private in its one request (UC-ID-16 check 9);
// * whoever may shape the workspace gets *Private* only where the workspace allows private hubs;
//   anybody else gets a private hub of their own and no template, and nobody else the page at all.
//
// * below 600 px the page is the hub's details and *Create hub* at the thumb; the starts are a
//   second step that opens only when asked for and that back closes (UC-WRK-01 check 11).
//
// The whole path by keyboard, a missing and a taken name, the page in German, the details as two
// columns at 768, the phone in German, and leaving with something typed. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { ACCOUNT, DEFAULT_STRUCTURE, HUB, MANIFEST, PAGE, PERMISSIONS, WORKSPACE, newRows, signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const NEW_HUB = '01a0e2e0-0000-7000-8000-0000000018c1';
const template = (key, modules, hub = {}) => ({
  key, version: 1, name: key, origin: 'HUBTASK', provenance: `${key}@1`, created_at: '2026-09-01T00:00:00Z',
  update_available: false, taken_version: null, hub_document: hub,
  document: { modules: Object.fromEntries(['workflows', ...modules].map((name) => [name, { switch: 'ON' }])) },
});
const named = (count, word) => Object.fromEntries(Array.from({ length: count }, (_, at) => [`${word}${at}`, { name: `${word} ${at + 1}` }]));
const CATALOGUE = [
  template('household', [], { types: named(1, 'chore') }),
  template('kanban_team', [], { types: named(1, 'card'), workflows: named(1, 'flow'), samples: named(3, 'sample') }),
  template('scrum_team', ['planning', 'knowledge'], { types: named(2, 'story'), workflows: named(2, 'flow'), roles: named(1, 'role') }),
  template('bug_tracking', [], { types: named(1, 'bug') }),
  template('classic_project', ['planning'], { types: named(4, 'item') }),
  template('itil_service_desk', ['service_desk'], { types: named(4, 'ticket') }),
  template('internal_it', ['service_desk'], { types: named(1, 'request') }),
  template('approvals', ['service_desk'], { types: named(3, 'approval') }),
];
const planned = (key) => ({
  template: key, upgrade: [], document: {}, mode: 'MERGE', base_hash: 'b1', plan_hash: 'p1', refusals: [], operations: [],
  flags: { hides_data: false, changes_permissions: false }, counts: { added: 1, changed: 0, removed: 0 },
  changes: [{ path: `workflows.${key}`, section: 'workflows', key, op: 'ADD', fields: [] }],
});

/**
 * A signed-in tab whose reader holds `permissions` on the workspace and is offered a hub of their
 * own as `offered` says; `allowed` is the workspace's *Private hubs allowed* as `/tenant` answers
 * it. Records what is made and every plan asked; `taken` refuses a name as the server does.
 * `catalogue` and `planOf` replace the catalogue and the plan answered; `planHeld` is awaited
 * before a plan is answered, so a walk decides when the plan arrives. `refused` is a problem every
 * create is answered with.
 */
async function as(browser, { permissions = PERMISSIONS.OWNER, offered = true, allowed = true, width = 1280, height = 800, locale = 'en', taken, hasTouch, catalogue = CATALOGUE, planOf = planned, planHeld, refused } = {}) {
  const { page, failures, context } = await signedIn(browser, width, height, { hasTouch });
  const made = [];
  const plans = [];
  const hubs = [];
  /** The plans answered and the hubs asked for, in the order the stub saw them. */
  const events = [];
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    if (path === '/accounts/me' && method === 'GET') return route.fulfill({ json: { ...ACCOUNT, locale } });
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, workspace_permissions: permissions, private_hub_offered: offered } });
    }
    if (path === '/tenant' && method === 'GET') return route.fulfill({ json: { ...WORKSPACE, private_hubs_offered: allowed } });
    if (path === '/config/templates' && method === 'GET') return route.fulfill({ json: { data: catalogue } });
    const plan = /^\/config\/templates\/([a-z_]+):plan$/.exec(path);
    if (plan && method === 'POST') {
      plans.push(plan[1]);
      // A plan the walk answers when it says so: what a slow server does, without a timer.
      if (planHeld) await planHeld(plan[1]);
      events.push(`plan:${plan[1]}`);
      return route.fulfill({ json: planOf(plan[1]) });
    }
    if ((path === '/containers' || path === '/containers:from-template') && method === 'POST') {
      const body = request.postDataJSON();
      made.push({ path, body });
      events.push(`made:${path}`);
      if (refused) return route.fulfill({ status: refused.status, contentType: 'application/problem+json', body: JSON.stringify(refused) });
      if (taken && body.name === taken) {
        return route.fulfill({ status: 409, contentType: 'application/problem+json',
          body: JSON.stringify({ status: 409, code: 'errors.conflict', detail_code: 'containers.name_taken', params: { name: body.name } }) });
      }
      const hub = { id: NEW_HUB, type: body.type ?? 'HUB', parent_id: body.parent_id ?? null, order_key: 'a1', version: 1, structure: DEFAULT_STRUCTURE,
        name: body.name, description: body.description ?? null, icon: body.icon ?? null, color_token: body.color_token ?? null, private: body.private === true };
      hubs.push(hub);
      return route.fulfill({ status: 201, json: hub });
    }
    if (path === `/containers/${NEW_HUB}` && method === 'GET' && hubs[0]) return route.fulfill({ json: hubs[0] });
    if (path === '/memberships' && url.searchParams.get('scope_type') === 'HUB') {
      return route.fulfill({ json: { ...PAGE, data: hubs.map((hub) => ({ id: 'm-new', scope_type: 'HUB', scope_id: hub.id, account_id: ACCOUNT.id, role: 'OWNER' })), grantable_roles: [] } });
    }
    return stub(route);
  });
  return { page, failures, context, made, plans, events };
}

/** What the sidebar's *Hubs* section offers to create, once the tree is drawn. */
async function offered(page) {
  await page.getByRole('tree').first().waitFor({ timeout: 10_000 });
  return newRows(page);
}

const main = (page) => page.locator('#main');
const nameField = (page, label = 'Hub name') => main(page).getByRole('textbox', { name: label });
const primary = (page, label = 'Create hub') => main(page).getByRole('button', { name: label, exact: true });

/** Opens the page from the navigation's *Create hub*, as a person does. */
async function openFromNavigation(page, row = 'Create hub') {
  await page.goto(`${served.origin}/`);
  await offered(page);
  await page.locator('aside.sidenav').getByRole('button', { name: row, exact: true }).first().click();
  await page.waitForURL('**/hubs/new', { timeout: 10_000 });
  await nameField(page).waitFor({ timeout: 10_000 });
}

// UC-WRK-01 check 9 at its width and height; the focus starts in the name.
test('chromium: 1280 × 800 - Create hub opens a page with the details beside the starts, all on the screen', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await as(browser);
  t.after(() => context.close());

  await openFromNavigation(page);
  assert.equal(await main(page).getByRole('heading', { level: 1, name: 'Create hub' }).count(), 1);
  await page.waitForFunction(() => document.activeElement?.getAttribute('type') === 'text' || document.activeElement?.tagName === 'INPUT', null, { timeout: 5_000 });
  assert.equal(await page.evaluate(() => document.activeElement?.closest('[data-create-hub]') !== null && document.activeElement?.id !== ''), true, 'the focus is not in the name');

  const sw = main(page).getByRole('switch', { name: 'Private' });
  await sw.waitFor({ timeout: 10_000 });
  const boxes = {
    name: await nameField(page).boundingBox(),
    private: await sw.boundingBox(),
    colour: await main(page).getByRole('radio', { name: 'No colour' }).boundingBox(),
    symbol: await main(page).getByRole('radio', { name: 'Default symbol' }).boundingBox(),
    description: await main(page).getByRole('textbox', { name: 'Description' }).boundingBox(),
    empty: await main(page).getByRole('radio', { name: /^Empty hub/u }).boundingBox(),
  };
  for (const [part, box] of Object.entries(boxes)) {
    assert.ok(box && box.y >= 0 && box.y + box.height <= 800, `${part} is not on the screen: ${JSON.stringify(box)}`);
  }
  // Beside, not under: the starts begin right of the details.
  assert.ok(boxes.empty.x > boxes.description.x + boxes.description.width, 'the starts are not beside the details');
  // Each column scrolls on its own; the page itself does not.
  assert.equal(await page.evaluate(() => document.scrollingElement.scrollHeight <= window.innerHeight), true, 'the page scrolls as a whole');
  assert.deepEqual(failures, []);
});

// UC-WRK-01 check 10.
test('chromium: Empty hub is chosen at opening and stays offered whatever is searched; a template is ticked and said', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, made } = await as(browser);
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/new`);
  const empty = main(page).getByRole('radio', { name: /^Empty hub/u });
  await main(page).getByRole('radio', { name: /^Kanban team/u }).waitFor({ timeout: 10_000 });
  assert.equal(await empty.isChecked(), true, 'Empty hub is not chosen when the page opens');
  const summary = main(page).locator('[data-hub-summary]');
  // The tile and the summary name the start one way: the workspace's default.
  assert.match(await summary.innerText(), /Empty hub[\s\S]*No template\. The hub starts with the workspace's default\./u);
  assert.match(await empty.evaluate((radio) => radio.closest('label')?.textContent ?? ''), /with the workspace's default\./u);

  // A search without a match: no template drawn, the sentence and the reset, Empty hub still there.
  const search = main(page).getByRole('searchbox', { name: 'Search templates' });
  await search.fill('zebra');
  await main(page).getByText('No template matches “zebra”.').waitFor({ timeout: 5_000 });
  assert.equal(await main(page).getByRole('radio', { name: /^Kanban team/u }).count(), 0, 'a template is drawn that the search does not find');
  assert.equal(await empty.isChecked(), true, 'the search took Empty hub away');
  assert.equal(await main(page).getByRole('status').filter({ hasText: 'No template found – “Empty hub” always stays available.' }).count(), 1);
  await main(page).getByRole('button', { name: 'Reset search' }).click();
  assert.equal(await search.inputValue(), '');

  // A family narrows the starts; a template chosen and then filtered away stays chosen.
  const kanban = main(page).getByRole('radio', { name: /^Kanban team/u });
  await kanban.check();
  const tile = kanban.locator('xpath=ancestor::label[1]');
  // Under the pointer, as a press leaves it; read once the state's transition has ended: the
  // tick filled in the edge's own colour.
  const handle = await tile.elementHandle();
  await page.waitForFunction((element) => getComputedStyle(element.querySelector('.tick')).backgroundColor === getComputedStyle(element).borderTopColor, handle, { timeout: 5_000 });
  const look = await tile.evaluate((element) => {
    const style = getComputedStyle(element);
    const tick = getComputedStyle(element.querySelector('.tick'));
    return { edge: style.borderTopColor, shadow: style.boxShadow, tick: tick.backgroundColor };
  });
  assert.notEqual(look.shadow, 'none', 'the chosen edge is not doubled');
  assert.equal(look.tick, look.edge, 'the tick is not filled in the edge\'s colour');
  assert.equal(await nameField(page).inputValue(), 'Kanban team', 'the template does not name the hub');
  assert.match(await summary.innerText(), /Template[\s\S]*Kanban team[\s\S]*This sets up[\s\S]*Collection\s*a first one, named like the hub[\s\S]*Entry types\s*1/u);
  await main(page).getByRole('radiogroup', { name: 'Families' }).getByRole('radio', { name: /^Service/u }).check();
  assert.equal(await kanban.count(), 0, 'the family did not narrow the starts');
  assert.equal(await main(page).getByRole('radio', { name: /^ITIL service desk/u }).count(), 1);
  assert.match(await summary.innerText(), /Kanban team/u, 'the choice went with the filter');
  assert.equal(made.length, 0, 'something was made before the press');
  assert.deepEqual(failures, []);
});

// The samples tick says how many sample entries the template brings, as the draft draws it.
test('chromium: the samples tick counts the sample entries the template brings', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await as(browser);
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/new`);
  await main(page).getByRole('radio', { name: /^Kanban team/u }).check();
  const tick = main(page).locator('[data-hub-summary]').getByRole('checkbox', { name: 'Add sample entries to try it (3)', exact: true });
  await tick.waitFor({ timeout: 5_000 });
  assert.equal(await tick.isChecked(), true);
  assert.deepEqual(failures, []);
});

// UC-ID-16 check 9: the request that left carries the template and `private`, and the screen
// lands in the hub, private.
test('chromium: a template with Private on is made private in one request, and the page lands in the hub', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, made } = await as(browser);
  t.after(() => context.close());

  await openFromNavigation(page);
  await main(page).getByRole('radio', { name: /^Kanban team/u }).check();
  await main(page).getByRole('switch', { name: 'Private' }).check();
  assert.equal(await main(page).getByRole('heading', { level: 1, name: 'Create private hub' }).count(), 1, 'the title does not say private');
  await nameField(page).fill('Diary');
  await primary(page).click();
  await page.waitForURL(`**/hubs/${NEW_HUB}`, { timeout: 10_000 });
  assert.deepEqual(made.map((each) => each.path), ['/containers:from-template']);
  assert.equal(made[0].body.private, true, `the body was ${JSON.stringify(made[0].body)}`);
  assert.equal(made[0].body.template, 'kanban_team');
  assert.equal(made[0].body.name, 'Diary');
  assert.equal(made[0].body.samples, true);
  // Back goes where the person was before the page, not to the page again.
  await page.goBack();
  await page.waitForURL(`${served.origin}/`, { timeout: 10_000 });
  assert.deepEqual(failures, []);
});

test('chromium: the whole path by keyboard - arrows choose, five presses ask one plan, Enter in the name creates', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, made, plans } = await as(browser);
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/new`);
  await main(page).getByRole('radio', { name: /^Kanban team/u }).waitFor({ timeout: 10_000 });
  await page.waitForFunction(() => document.activeElement?.tagName === 'INPUT', null, { timeout: 5_000 });
  const focused = () => page.evaluate(() => {
    const element = document.activeElement;
    return `${element?.getAttribute('role') ?? element?.getAttribute('type') ?? element?.tagName}:${element?.getAttribute('aria-label') ?? element?.closest('label')?.innerText?.split('\n')[0] ?? ''}`;
  });
  const order = [];
  for (let step = 0; step < 6; step++) {
    await page.keyboard.press('Tab');
    order.push(await focused());
  }
  // Name, then *Private*, colour, symbol, description, then the search: the draft's markup order.
  assert.deepEqual(order.slice(0, 4), ['switch:', 'radio:No colour', 'radio:Default symbol', 'TEXTAREA:'], `the order was ${order.join(' → ')}`);
  assert.equal(order[4], 'search:', `the order was ${order.join(' → ')}`);
  assert.match(order[5], /^radio:All/u, `the families do not follow the search: ${order.join(' → ')}`);
  await page.keyboard.press('Tab');
  assert.match(await focused(), /^radio:Empty hub/u, 'the tiles are not entered at the chosen one');

  // Five arrow presses: each chooses, one plan is asked once the choice has rested.
  for (let step = 0; step < 5; step++) await page.keyboard.press('ArrowDown');
  const fifth = await page.evaluate(() => document.activeElement?.closest('label')?.querySelector('.title')?.textContent);
  await main(page).locator('[data-hub-summary]').getByText(fifth, { exact: true }).waitFor({ timeout: 5_000 });
  await main(page).locator('[data-plan-changes]').waitFor({ timeout: 5_000 });
  assert.ok(plans.length <= 1, `five presses asked ${plans.length} plans: ${plans.join(', ')}`);
  assert.equal(await page.evaluate(() => document.activeElement?.getAttribute('type')), 'radio', 'the arrows left the tiles');

  await nameField(page).focus();
  await page.keyboard.press('ControlOrMeta+A');
  await page.keyboard.type('Ops');
  await page.keyboard.press('Enter');
  await page.waitForURL(`**/hubs/${NEW_HUB}**`, { timeout: 10_000 });
  assert.equal(made.length, 1);
  assert.equal(made[0].body.name, 'Ops');
  assert.deepEqual(failures, []);
});

// A template the workspace took asks no plan: the press sends at once, however soon it comes after
// the choice - by Enter in the name and by the primary, chosen on the page and from the address.
test('chromium: a template the workspace already took, pressed at once after it is chosen, makes the hub', async (t) => {
  const browser = await sharedChromium(t);
  const catalogue = CATALOGUE.map((each) => (each.key === 'bug_tracking' ? { ...each, taken_version: 1 } : each));
  for (const way of ['enter', 'primary', 'address']) {
    const { page, failures, context, made, plans } = await as(browser, { catalogue });
    try {
      await page.goto(`${served.origin}/hubs/new${way === 'address' ? '?template=bug_tracking' : ''}`);
      const tile = main(page).getByRole('radio', { name: /^Bug tracking/u });
      await tile.waitFor({ timeout: 10_000 });
      if (way === 'address') await page.waitForFunction(() => document.querySelector('#main input[type="text"]')?.value === 'Bug tracking', null, { timeout: 5_000 });
      else await tile.check();
      if (way === 'primary') await primary(page).click();
      else {
        await nameField(page).focus();
        await page.keyboard.press('Enter');
      }
      await page.waitForURL((url) => !url.pathname.endsWith('/hubs/new'), { timeout: 10_000 });
      assert.deepEqual(made.map((each) => each.path), ['/containers:from-template'], `${way}: ${JSON.stringify(made)}`);
      assert.equal(made[0].body.template, 'bug_tracking');
      assert.deepEqual(plans, [], `${way}: a plan was asked for a template the workspace took`);
      assert.deepEqual(failures, []);
    } finally {
      await context.close();
    }
  }
});

// Enter in the name is the primary's path: it waits for the plan being read, and where the plan
// asks for the acknowledgement it asks for it - focus on the box - and sends nothing until it is
// given. The plan answers when the walk releases it, after Enter was pressed.
test('chromium: Enter in the name while the plan is read waits for it, then asks the acknowledgement first', async (t) => {
  const browser = await sharedChromium(t);
  let release;
  const held = new Promise((resolve) => (release = resolve));
  const { page, failures, context, made, events } = await as(browser, {
    planOf: (key) => ({ ...planned(key), flags: { hides_data: true, changes_permissions: false } }),
    planHeld: () => held,
  });
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/new`);
  const kanban = main(page).getByRole('radio', { name: /^Kanban team/u });
  await kanban.waitFor({ timeout: 10_000 });
  // Waited on before the choice, and bounded: a choice that sends no plan fails here, not hangs.
  const sent = page.waitForRequest((request) => request.method() === 'POST' && new URL(request.url()).pathname.endsWith('/config/templates/kanban_team:plan'), { timeout: 5_000 });
  await kanban.check();
  await sent;
  // The choice has rested and its plan is out: the primary waits, saying why.
  await page.waitForFunction((element) => element.getAttribute('aria-disabled') === 'true', await primary(page).elementHandle(), { timeout: 5_000 });
  await nameField(page).focus();
  await page.keyboard.press('Enter');
  // While the plan is out the primary says why it waits, and nothing was made.
  assert.equal(await primary(page).getAttribute('aria-disabled'), 'true');
  release();
  const box = main(page).getByRole('checkbox', { name: /^I have seen that this hides data/u });
  await box.waitFor({ timeout: 5_000 });
  await page.waitForFunction(() => document.activeElement?.closest('[data-starter-plan]') !== null && document.activeElement?.getAttribute('type') === 'checkbox', null, { timeout: 5_000 });
  assert.deepEqual(made, [], 'Enter made the hub before the acknowledgement');
  assert.deepEqual(events, ['plan:kanban_team']);

  await page.keyboard.press('Space');
  await nameField(page).focus();
  await page.keyboard.press('Enter');
  await page.waitForURL((url) => !url.pathname.endsWith('/hubs/new'), { timeout: 10_000 });
  assert.deepEqual(events, ['plan:kanban_team', 'made:/containers:from-template'], 'the hub was asked for before its plan was read');
  assert.equal(made[0].body.acknowledge, true, `the body was ${JSON.stringify(made[0].body)}`);
  assert.deepEqual(failures, []);
});

// The status region exists empty from the start and is filled by a filter, so a screen reader
// hears the count as a change of a region it knows.
test('chromium: the family count is a status region that exists empty and is filled on a filter', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await as(browser);
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/new`);
  await main(page).getByRole('radio', { name: /^Kanban team/u }).waitFor({ timeout: 10_000 });
  const region = main(page).getByRole('status');
  assert.equal(await region.count(), 1, 'there is no status region before a filter');
  const before = await region.evaluate((element) => ({ text: element.textContent, display: getComputedStyle(element).display, hidden: element.hidden }));
  assert.equal(before.text, '', `the region is ${JSON.stringify(before)}`);
  assert.notEqual(before.display, 'none', 'the region is not drawn, so it is not in the tree');
  assert.equal(before.hidden, false);
  const handle = await region.elementHandle();
  await main(page).getByRole('searchbox', { name: 'Search templates' }).fill('kanban');
  await page.waitForFunction((element) => element.isConnected && element.textContent.startsWith('1 template found'), handle, { timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('chromium: a missing name and a taken name land on the name field, which takes the focus', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, made } = await as(browser, { taken: 'House' });
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/new`);
  await main(page).getByRole('radio', { name: /^Kanban team/u }).waitFor({ timeout: 10_000 });
  await main(page).getByRole('textbox', { name: 'Description' }).focus();
  await primary(page).click();
  const field = nameField(page);
  await main(page).getByText('Give it a name first.').waitFor({ timeout: 5_000 });
  assert.equal(await field.getAttribute('aria-invalid'), 'true');
  assert.equal(await field.evaluate((element) => element === document.activeElement), true, 'the focus did not move to the name');
  const described = await field.evaluate((element) => (element.getAttribute('aria-describedby') ?? '').split(' ').map((id) => document.getElementById(id)?.textContent ?? '').join(' '));
  assert.match(described, /Give it a name first\./u, 'the error does not describe the field');
  assert.equal(made.length, 0);

  await field.fill('House');
  await main(page).getByRole('textbox', { name: 'Description' }).focus();
  await primary(page).click();
  await main(page).locator('[data-create-hub] .name-error').waitFor({ timeout: 5_000 });
  assert.equal(await field.getAttribute('aria-invalid'), 'true');
  assert.equal(await field.evaluate((element) => element === document.activeElement), true, 'a taken name did not take the focus');
  assert.equal(made.length, 1);
  assert.match(page.url(), /\/hubs\/new$/u, 'a refused name left the page');
  assert.deepEqual(failures, []);
});

// A refusal that names no field is the form's, as in *Create collection*: the focus moves to its
// sentence, which reads it out and scrolls it into view - on a phone the details are the scrolled
// page and *Create hub* stands in the foot bar, far from the sentence (design-system.md §13).
for (const [width, height] of [[1280, 800], [390, 844]]) {
  test(`chromium: ${width} × ${height} - a refusal for another reason than the name is in view and takes the focus`, async (t) => {
    const browser = await sharedChromium(t);
    const refused = { status: 403, code: 'errors.forbidden', detail_code: 'access.not_permitted' };
    const { page, failures, context, made } = await as(browser, { width, height, refused, hasTouch: width < 768 });
    t.after(() => context.close());

    await page.goto(`${served.origin}/hubs/new`);
    await nameField(page).waitFor({ timeout: 10_000 });
    await nameField(page).fill('House');
    const description = main(page).getByRole('textbox', { name: 'Description' });
    await description.scrollIntoViewIfNeeded();
    await description.focus();
    await primary(page).click();
    const alert = main(page).getByRole('alert');
    await alert.waitFor({ timeout: 5_000 });
    await page.waitForFunction(() => document.activeElement?.getAttribute('role') === 'alert', null, { timeout: 5_000 });
    const seen = await alert.evaluate((element) => {
      const box = element.getBoundingClientRect();
      return { top: box.top, bottom: box.bottom, height: window.innerHeight };
    });
    assert.ok(seen.top >= 0 && seen.bottom <= seen.height, `the refusal is out of view: ${JSON.stringify(seen)}`);
    assert.equal(made.length, 1);
    assert.equal(await nameField(page).getAttribute('aria-invalid'), null, 'the name is marked for a refusal that is not about it');
    assert.deepEqual(failures, []);
  });
}

// A hub is made by the server and never queued: offline the page says so where its refusals go, in
// the one shape this branch's screens say it, and *Create hub* sends nothing.
test('chromium: 1280 × 800 - offline the page says that creating a hub needs a connection, and Create hub sends nothing', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, made, plans } = await as(browser);
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/new`);
  await main(page).getByRole('radio', { name: /^Kanban team/u }).waitFor({ timeout: 10_000 });
  await context.setOffline(true);
  const line = main(page).locator('[data-offline-line]');
  await line.waitFor({ timeout: 10_000 });
  assert.equal((await line.textContent()).trim(), 'Creating a hub needs a connection.');
  await main(page).getByRole('radio', { name: /^Kanban team/u }).check();
  await nameField(page).fill('House');
  assert.equal(await primary(page).getAttribute('aria-disabled'), 'true');
  const reason = await primary(page).evaluate((button) => document.getElementById(button.getAttribute('aria-describedby') ?? '')?.textContent?.trim());
  assert.equal(reason, 'Creating a hub needs a connection.');
  await nameField(page).press('Enter');
  assert.deepEqual(made, []);
  assert.deepEqual(plans, [], 'a plan is asked offline');
  await context.setOffline(false);
  await line.waitFor({ state: 'detached', timeout: 10_000 });
  assert.deepEqual(failures, []);
});

// UC-WRK-01 check 6, UC-ID-16 check 1, owner's answer Q4: a private hub of one's own, no switch, no template.
test('chromium: a member is offered a private hub of their own, without the switch and without templates', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, made } = await as(browser, { permissions: PERMISSIONS.MEMBER, offered: true });
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  assert.deepEqual(await offered(page), ['Create private hub']);
  await page.locator('aside.sidenav').getByRole('button', { name: 'Create private hub', exact: true }).first().click();
  await page.waitForURL('**/hubs/new', { timeout: 10_000 });
  await main(page).getByRole('heading', { level: 1, name: 'Create private hub' }).waitFor({ timeout: 10_000 });
  await main(page).getByRole('radio', { name: /^Empty hub/u }).waitFor();
  assert.equal(await main(page).getByRole('switch').count(), 0, 'a switch offers a shared hub');
  assert.equal(await main(page).getByRole('searchbox').count(), 0, 'the catalogue is searched');
  assert.equal(await main(page).getByRole('radio', { name: /^Kanban team/u }).count(), 0, 'a template is offered the server refuses');
  assert.equal(await main(page).getByText('Templates set up the workspace as well, so they need the right to shape it.', { exact: false }).count(), 1, 'the page does not say why there is no template');
  await nameField(page).fill('Diary');
  await primary(page).click();
  await page.waitForURL(`**/hubs/${NEW_HUB}`, { timeout: 10_000 });
  assert.deepEqual(made.map((each) => each.path), ['/containers']);
  assert.equal(made[0].body.private, true, `the body was ${JSON.stringify(made[0].body)}`);
  assert.deepEqual(failures, []);
});

test('chromium: where private hubs are not offered, a viewer is offered no hub, and the address shows nothing', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await as(browser, { permissions: PERMISSIONS.VIEWER, offered: false, allowed: false });
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  assert.deepEqual(await offered(page), [], 'a hub offered to somebody the server refuses one');
  // Typed, the address answers as any address the reader may not use: no form.
  await page.goto(`${served.origin}/hubs/new`);
  await main(page).getByRole('heading', { level: 1, name: 'There is nothing at this address.' }).waitFor({ timeout: 10_000 });
  assert.equal(await main(page).getByRole('textbox', { name: 'Hub name' }).count(), 0, 'the form is drawn for somebody who may make no hub');
  assert.equal(await main(page).getByRole('link', { name: 'Back to the start' }).count(), 1);
  assert.deepEqual(failures, []);
});

test('chromium: whoever shapes the workspace where private hubs are switched off makes a shared hub, no switch', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, made } = await as(browser, { permissions: PERMISSIONS.ADMIN, offered: false, allowed: false });
  t.after(() => context.close());

  await openFromNavigation(page);
  await main(page).getByRole('radio', { name: /^Kanban team/u }).waitFor({ timeout: 10_000 });
  assert.equal(await main(page).getByRole('switch', { name: 'Private' }).count(), 0, '*Private* offered where private hubs are off');
  await nameField(page).fill('Garden');
  await main(page).getByRole('radio', { name: /^Project Every level/u }).check();
  await primary(page).click();
  await page.waitForURL(`**/hubs/${NEW_HUB}`, { timeout: 10_000 });
  assert.equal(made[0].path, '/containers');
  assert.equal(made[0].body.private, undefined, `the body was ${JSON.stringify(made[0].body)}`);
  assert.equal(made[0].body.preset, 'PROJECT');
  assert.deepEqual(failures, []);
});

test('chromium: a template the address names but the reader cannot read leaves Empty hub chosen', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await as(browser);
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/new?template=no_such_template`);
  await main(page).getByRole('radio', { name: /^Kanban team/u }).waitFor({ timeout: 10_000 });
  assert.equal(await main(page).getByRole('radio', { name: /^Empty hub/u }).isChecked(), true);
  assert.equal(await nameField(page).inputValue(), '', 'a name is filled in for nothing');
  await page.goto(`${served.origin}/hubs/new?template=scrum_team`);
  await main(page).getByRole('radio', { name: /^Scrum team/u, checked: true }).waitFor({ timeout: 10_000 });
  assert.equal(await nameField(page).inputValue(), 'Scrum team');
  assert.deepEqual(failures, []);
});

test('chromium: Cancel returns where the person came from, and leaving with something typed asks first', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, made } = await as(browser);
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/${HUB.id}`);
  await page.locator('aside.sidenav').getByRole('tree').first().waitFor({ timeout: 10_000 });
  await page.goto(`${served.origin}/`);
  await openFromNavigation(page);
  await main(page).getByRole('button', { name: 'Cancel', exact: true }).click();
  await page.waitForURL(`${served.origin}/`, { timeout: 10_000 });

  await page.locator('aside.sidenav').getByRole('button', { name: 'Create hub', exact: true }).first().click();
  await page.waitForURL('**/hubs/new', { timeout: 10_000 });
  await nameField(page).fill('Half typed');
  await main(page).getByRole('button', { name: 'Cancel', exact: true }).click();
  const leave = page.getByRole('dialog');
  await leave.waitFor({ timeout: 5_000 });
  assert.match(page.url(), /\/hubs\/new$/u, 'the page left without asking');
  assert.equal(await leave.getByRole('heading', { name: 'Leave without saving?' }).count(), 1);
  await leave.getByRole('button', { name: 'Stay', exact: true }).click();
  assert.equal(await nameField(page).inputValue(), 'Half typed');
  assert.equal(made.length, 0);
  assert.deepEqual(failures, []);
});

test('chromium: the page in German', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await as(browser, { locale: 'de' });
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/new`);
  await page.waitForFunction(() => document.documentElement.lang === 'de', null, { timeout: 15_000 });
  await main(page).getByRole('heading', { level: 1, name: 'Hub anlegen' }).waitFor({ timeout: 10_000 });
  await main(page).getByRole('heading', { level: 2, name: 'Womit der Hub beginnt' }).waitFor();
  await main(page).getByRole('radio', { name: /^Leerer Hub/u, checked: true }).waitFor();
  assert.equal(await main(page).getByRole('textbox', { name: 'Name des Hubs' }).count(), 1);
  assert.equal(await main(page).getByRole('switch', { name: 'Privat' }).count(), 1);
  assert.equal(await main(page).getByRole('searchbox', { name: 'Vorlagen durchsuchen' }).count(), 1);
  assert.match(await main(page).locator('[data-hub-summary]').innerText(), /Sammlungen legst du danach an\./u);
  assert.equal(await main(page).getByText(/^\d+ Vorlagen in \d+ Familien$/u).count(), 1, 'the count is not German');
  assert.deepEqual(failures, []);
});

test('chromium: 768 px - the details are a block of two columns above the starts', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await as(browser, { width: 768, height: 1024 });
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/new`);
  await main(page).getByRole('radio', { name: /^Kanban team/u }).waitFor({ timeout: 10_000 });
  const name = await nameField(page).boundingBox();
  const colour = await main(page).getByRole('radio', { name: 'No colour' }).boundingBox();
  const description = await main(page).getByRole('textbox', { name: 'Description' }).boundingBox();
  const empty = await main(page).getByRole('radio', { name: /^Empty hub/u }).boundingBox();
  assert.ok(colour.x > name.x + name.width, 'the look does not stand beside the name');
  assert.ok(Math.abs(colour.y - name.y) < 24, 'the look does not start on the name\'s line');
  assert.ok(empty.y > description.y + description.height, 'the starts are not under the details');
  assert.deepEqual(failures, []);
});

// From 600 to 904 px the head with *Create hub* stays while the starts scroll, as it does wider:
// the details and the starts are one scroll under it, the start's tools sticking at its top.
test('chromium: 768 × 1024 - scrolled to the last start, the head with Create hub and the search stay in view', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await as(browser, { width: 768, height: 1024 });
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/new`);
  await main(page).getByRole('radio', { name: /^Kanban team/u }).check();
  const last = main(page).getByRole('radio', { name: /^Approvals/u });
  const tile = await main(page).getByRole('radio', { name: /^Household/u }).boundingBox();
  await page.mouse.move(tile.x + tile.width / 2, tile.y + tile.height / 2);
  await page.mouse.wheel(0, 4000);
  const handle = await last.elementHandle();
  await page.waitForFunction((element) => element.closest('label').getBoundingClientRect().bottom <= window.innerHeight, handle, { timeout: 5_000 });
  for (const [what, part] of [['Create hub', primary(page)], ['the title', main(page).getByRole('heading', { level: 1, name: 'Create hub' })], ['the search', main(page).getByRole('searchbox', { name: 'Search templates' })]]) {
    const box = await part.boundingBox();
    assert.ok(box && box.y >= 0 && box.y + box.height <= 1024, `${what} scrolled away: ${JSON.stringify(box)}`);
  }
  const search = await main(page).getByRole('searchbox', { name: 'Search templates' }).boundingBox();
  const head = await primary(page).boundingBox();
  assert.ok(search.y > head.y + head.height, 'the search stands over the head');
  assert.equal(await page.evaluate(() => window.scrollY), 0, 'the page itself scrolled');
  assert.deepEqual(failures, []);
});

/** Which element has the focus, as a walk reads it: its type, its name, whether it is in the step. */
const focused = (page) => page.evaluate(() => {
  const element = document.activeElement;
  const label = element && 'labels' in element ? element.labels?.[0]?.textContent : undefined;
  return {
    tag: element?.tagName ?? 'NONE',
    type: element?.getAttribute('type') ?? '',
    name: (element?.getAttribute('aria-label') ?? label ?? element?.textContent ?? '').trim(),
    expanded: element?.getAttribute('aria-expanded') ?? null,
    inStep: Boolean(element?.closest('dialog[open]')),
  };
});

// UC-WRK-01 check 11 at a phone's width and height.
test('chromium: 390 × 844 - the details are the page, Create hub stands at the thumb, the starts are a second step', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, made } = await as(browser, { width: 390, height: 844, hasTouch: true });
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/new`);
  await nameField(page).waitFor({ timeout: 10_000 });
  // The page is the screen: its own bar with ✕ and the title, neither bar of the frame.
  assert.equal(await page.locator('[data-phone-bar]').count(), 0, 'the frame draws its bar over the page');
  assert.equal(await page.getByRole('navigation', { name: 'Sections' }).count(), 0, 'the frame draws its bottom bar under the page');
  assert.equal(await main(page).getByRole('button', { name: 'Cancel', exact: true }).count(), 1, 'there is no ✕');

  // At opening: the details and the primary on the screen, the starts not on it at all.
  const create = primary(page);
  await create.waitFor({ timeout: 10_000 });
  const thumb = await create.boundingBox();
  assert.ok(thumb.y + thumb.height <= 844 && thumb.y >= 844 - 100, `Create hub is not at the thumb: ${JSON.stringify(thumb)}`);
  assert.ok(thumb.width >= 390 - 2 * 24, `Create hub is not across the width: ${JSON.stringify(thumb)}`);
  for (const part of [nameField(page), main(page).getByRole('switch', { name: 'Private' }), main(page).getByRole('textbox', { name: 'Description' })]) {
    const box = await part.boundingBox();
    assert.ok(box && box.y >= 0 && box.y + box.height <= thumb.y, `a detail is not above Create hub: ${JSON.stringify(box)}`);
  }
  assert.equal(await page.getByRole('radio', { name: /^Kanban team/u }).count(), 0, 'the starts are on the page');
  assert.equal(await page.getByRole('searchbox', { name: 'Search templates' }).count(), 0, 'the search is on the page');
  assert.equal(await page.evaluate(() => document.scrollingElement.scrollWidth <= window.innerWidth), true, 'the page scrolls sideways');
  // The look is one closed row; the mark beside the name keeps it in view.
  const look = main(page).getByRole('button', { name: 'Colour and symbol' });
  assert.equal(await look.getAttribute('aria-expanded'), 'false');
  assert.equal(await main(page).getByRole('radio', { name: 'No colour' }).count(), 0, 'the colours are open');
  await look.click();
  assert.equal(await look.getAttribute('aria-expanded'), 'true');
  await main(page).getByRole('radio', { name: 'Heart' }).check();
  await look.click();

  // The second step: a layer over the page, the address unchanged, the focus in its search.
  await main(page).getByRole('button', { name: 'Choose a template', exact: true }).click();
  const step = page.getByRole('dialog', { name: 'Choose a template' });
  await step.waitFor({ timeout: 5_000 });
  assert.match(page.url(), /\/hubs\/new$/u, 'the step changed the address');
  await page.waitForFunction(() => document.activeElement?.getAttribute('type') === 'search', null, { timeout: 5_000 });
  assert.equal((await focused(page)).inStep, true, 'the search is not in the step');
  assert.equal(await step.getByRole('radio', { name: /^Empty hub/u, checked: true }).count(), 1, 'Empty hub is not chosen');
  // Its families are one row that scrolls sideways; its tiles one per row.
  const families = step.getByRole('radiogroup', { name: 'Families' });
  assert.equal(await families.evaluate((row) => row.scrollWidth > row.clientWidth && getComputedStyle(row).flexWrap === 'nowrap'), true, 'the families are not one row');
  const columns = await step.locator('label.card').evaluateAll((cards) => cards.slice(0, 3).map((card) => Math.round(card.getBoundingClientRect().x)));
  assert.equal(new Set(columns).size, 1, `the tiles are not one per row: ${columns}`);

  // A tile chosen keeps the step open and the focus on the tile.
  await step.getByRole('radio', { name: /^Kanban team/u }).check();
  assert.equal(await step.isVisible(), true, 'choosing a tile closed the step');
  const onTile = await focused(page);
  assert.equal(onTile.type, 'radio', `the focus left the tile: ${JSON.stringify(onTile)}`);
  assert.equal(onTile.inStep, true);
  // While it is open the page under it is out of reach: Tab goes round the step, and past its last
  // control to the browser's own (the document's body here), never to a control of the page.
  for (let press = 0; press < 6; press += 1) {
    await page.keyboard.press('Tab');
    const at = await focused(page);
    assert.ok(at.inStep || at.tag === 'BODY', `the focus reached the page under the step: ${JSON.stringify({ ...at, name: at.name.slice(0, 40) })}`);
  }
  await step.getByRole('radio', { name: /^Kanban team/u }).focus();

  // Done returns, the summary names the template, the focus is back on the button that opened it.
  await step.getByRole('button', { name: 'Done', exact: true }).click();
  await step.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.match(await main(page).locator('[data-hub-summary]').innerText(), /Kanban team/u);
  const returned = await focused(page);
  assert.equal(returned.name, 'Change', `the focus did not come back to the button: ${JSON.stringify(returned)}`);
  assert.equal(await nameField(page).inputValue(), 'Kanban team', 'the template did not name the hub');

  // Back - the system's gesture - closes the step and keeps the page; Escape does too.
  await main(page).getByRole('button', { name: 'Change', exact: true }).click();
  await step.waitFor({ timeout: 5_000 });
  await page.goBack();
  await step.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.match(page.url(), /\/hubs\/new$/u, 'back left the page');
  assert.equal(await nameField(page).inputValue(), 'Kanban team', 'back lost what was typed');
  assert.equal((await focused(page)).name, 'Change', 'back did not give the focus back');
  await main(page).getByRole('button', { name: 'Change', exact: true }).click();
  await step.waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');
  await step.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.equal((await focused(page)).name, 'Change', 'Escape did not give the focus back');

  // Create without a name lands on the name.
  await nameField(page).fill('');
  await create.click();
  await main(page).getByText('Give it a name first.').waitFor({ timeout: 5_000 });
  assert.equal(await nameField(page).getAttribute('aria-invalid'), 'true');
  assert.equal(await nameField(page).evaluate((element) => element === document.activeElement), true, 'the focus did not move to the name');
  assert.equal(made.length, 0);

  // Named, it is made from the template in one request, and the page lands in the hub.
  await nameField(page).fill('Release train');
  await create.click();
  await page.waitForURL((url) => !url.pathname.endsWith('/hubs/new'), { timeout: 10_000 });
  assert.equal(made.length, 1);
  assert.equal(made[0].path, '/containers:from-template');
  assert.equal(made[0].body.template, 'kanban_team', `the body was ${JSON.stringify(made[0].body)}`);
  assert.equal(made[0].body.name, 'Release train');
  assert.equal(made[0].body.icon, 'heart', `the body was ${JSON.stringify(made[0].body)}`);
  assert.deepEqual(failures, []);
});

// UC-WRK-01 check 11 by keyboard: the draft's order, ✕ before the name, Create hub last.
test('chromium: 390 × 844 - Tab walks the details in the draft\'s order, from ✕ to Create hub', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await as(browser, { width: 390, height: 844 });
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/new`);
  await nameField(page).waitFor({ timeout: 10_000 });
  await page.waitForFunction(() => document.activeElement?.getAttribute('type') === 'text', null, { timeout: 5_000 });
  await page.keyboard.press('Shift+Tab');
  assert.equal((await focused(page)).name, 'Cancel', 'before the name is not ✕');
  await page.keyboard.press('Tab');
  const order = [];
  for (let press = 0; press < 5; press += 1) {
    await page.keyboard.press('Tab');
    const at = await focused(page);
    order.push(at.expanded === null ? at.name : `${at.name} (${at.expanded})`);
  }
  assert.deepEqual(order, ['Private', 'Colour and symbol (false)', 'Description', 'Choose a template', 'Create hub']);
  assert.deepEqual(failures, []);
});

// The step is no route: a reload drops it, and with it the entry that held it, so the first back
// after the reload leaves the page as it would have without the step.
test('chromium: 390 × 844 - a reload with the step open drops its entry; the first back leaves the page', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await as(browser, { width: 390, height: 844, hasTouch: true });
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  await page.goto(`${served.origin}/hubs/new`);
  await main(page).getByRole('button', { name: 'Choose a template', exact: true }).click();
  await page.getByRole('dialog', { name: 'Choose a template' }).waitFor({ timeout: 5_000 });
  assert.equal(await page.evaluate(() => history.state?.isHeld), true, 'the step holds no entry');
  await page.reload();
  await nameField(page).waitFor({ timeout: 10_000 });
  await page.waitForFunction(() => history.state?.isHeld !== true, null, { timeout: 5_000 });
  assert.equal(await page.getByRole('dialog', { name: 'Choose a template' }).count(), 0, 'the step came back after the reload');
  await page.goBack();
  await page.waitForURL(`${served.origin}/`, { timeout: 10_000 });
  assert.deepEqual(failures, []);
});

test('chromium: the phone in German', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await as(browser, { width: 390, height: 844, locale: 'de' });
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/new`);
  await page.waitForFunction(() => document.documentElement.lang === 'de', null, { timeout: 15_000 });
  await main(page).getByRole('heading', { level: 1, name: 'Hub anlegen' }).waitFor({ timeout: 10_000 });
  assert.equal(await main(page).getByRole('button', { name: 'Abbrechen', exact: true }).count(), 1);
  assert.equal(await main(page).getByRole('button', { name: 'Farbe und Symbol' }).getAttribute('aria-expanded'), 'false');
  await main(page).getByRole('button', { name: 'Vorlage wählen', exact: true }).click();
  const step = page.getByRole('dialog', { name: 'Vorlage wählen' });
  await step.waitFor({ timeout: 5_000 });
  assert.equal(await step.getByRole('button', { name: 'Zurück zu den Angaben', exact: true }).count(), 1);
  assert.equal(await step.getByRole('searchbox', { name: 'Vorlagen durchsuchen' }).count(), 1);
  await step.getByRole('radio', { name: /^Kanban-Team/u }).check();
  await step.getByRole('button', { name: 'Fertig', exact: true }).click();
  await step.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.equal(await main(page).getByRole('button', { name: 'Ändern', exact: true }).count(), 1);
  assert.equal(await main(page).getByRole('button', { name: 'Hub anlegen', exact: true }).count(), 1);
  assert.deepEqual(failures, []);
});

// The navigation concept's grammar (§5): ＋ on a section head creates at its top level - on the hub
// level's *Collections*, a collection in that hub. A collection keeps its dialog.
test('chromium: inside a hub, ＋ on Collections makes a collection in that hub, with its preset (ux-inventory.md §E1)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, made } = await as(browser, { permissions: PERMISSIONS.ADMIN, offered: false, allowed: false });
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/${HUB.id}`);
  const level = page.locator('aside.sidenav').getByRole('region', { name: HUB.name });
  const make = level.getByRole('button', { name: 'New collection', exact: true });
  await make.waitFor({ timeout: 10_000 });
  await make.click();
  const dialog = page.locator('dialog[open]');
  await dialog.waitFor({ timeout: 10_000 });
  assert.equal(await dialog.getByRole('heading', { name: `New collection in ${HUB.name}` }).count(), 1, 'the dialog does not make a collection');
  await dialog.getByRole('radio', { name: /^Like its hub/u }).waitFor({ timeout: 10_000 });
  await dialog.getByRole('textbox').first().fill('Groceries');
  await dialog.getByRole('button', { name: 'Create', exact: true }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 10_000 });
  assert.equal(made.length, 1);
  assert.equal(made[0].body.type, 'COLLECTION', `the body was ${JSON.stringify(made[0].body)}`);
  assert.equal(made[0].body.parent_id, HUB.id, `the body was ${JSON.stringify(made[0].body)}`);
  assert.deepEqual(failures, []);
});
