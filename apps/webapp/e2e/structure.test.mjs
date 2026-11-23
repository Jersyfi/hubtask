// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// How much structure a collection has (UC-WRK-23 checks 1, 3, 4, 6, 7, 9, 10) and switching it without
// loss (UC-WRK-24 checks 1, 2, 4, 6, 7), walked over the shared
// fixture's structure hub as three readers; the structure's choices are `MultiSelect`, its always-on
// and locked options saying why (UC-WRK-32 check 1) and a save without a view marking the views
// and moving focus there (UC-WRK-32 check 3):
//
// * the owner opens a *List* collection and meets no board, no timeline, no twist and on an entry
//   no date, person, label, notes or child; the Structure panel names where each setting comes
//   from, resets what is set here, and sends a preset as itself; creating a collection offers
//   the presets; the list's one level, named "item", is "item" on an entry's badge and on every
//   control that adds one, where an unnamed level keeps the catalogue's words;
// * the owner narrows a project and a hub: a dialog first names what would no longer be shown, and for
//   the hub the collections keeping their own value; *Hide* is the default, *Flatten* says it is final;
// * an administrator meets a project whose views and task dates the workspace locked: the lock
//   and who set it beside the setting, the controls readable and unavailable, and a preset chosen
//   under the lock keeping the locked value;
// * the owner's unsaved change meets settings changed elsewhere: the bar counts the owner's own,
//   offers no Save, and the notice applies the owner's change to the current settings or shows them;
// * a member sees the list as its settings make it, and no Structure panel.
//
// Chromium only; what the server decides is its own tests' (check 8).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { ACCOUNT, OTHER, OWNER_GRANTS, PAGE, SHAPE_HUB, SHAPE_ITEMS, SHAPES, forgetUnstubbed, showStructures, stub, openSettings, viewLink, openNewCollection, kindChoice } from './fixture.mjs';
import { serve } from './serve.mjs';
import { reveal } from './reveal.mjs';
import { openBeside } from './several.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());
showStructures();

/**
 * The list's settings as somebody else changes them (check 10): from `change` on, the list's read
 * answers the changed settings and the stream carries the container's change record. The stream
 * is closed with a short `retry`, so the client comes back for a record written since.
 */
function elsewhere() {
  const live = { container: undefined, log: [], delivered: 0 };
  // `inherited`: changed where the list inherits it from, so the list's own version stays.
  live.change = (settings, { inherited = false } = {}) => {
    const list = live.container ?? SHAPES.list;
    live.container = { ...list, version: inherited ? list.version : list.version + 1, structure: { ...list.structure, settings: { ...list.structure.settings, ...settings } } };
    live.log.push({ op: 'UPSERT', entity: 'container', entity_id: list.id, container_id: list.parent_id, payload: live.container });
  };
  return live;
}

/** A signed-in tab whose reader holds `role` on the workspace, with the structure writes recorded. */
async function as(browser, role, width = 1280, live = undefined) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  // Every reader starts from the fixture as written: an earlier walk's saves are forgotten.
  forgetUnstubbed();
  const written = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (request.method() === 'PATCH' && path.endsWith('/structure')) written.push(request.postDataJSON());
    if (request.method() === 'POST' && path === '/containers') written.push(request.postDataJSON());
    if (live && path === '/stream') {
      const news = live.log.slice(live.delivered);
      const first = live.delivered;
      live.delivered = live.log.length;
      const frames = news.map((each, i) => `id: s-${first + i + 1}\ndata: ${JSON.stringify(each)}\n\n`).join('');
      return route.fulfill({ status: 200, contentType: 'text/event-stream', body: `retry: 250\n\n${frames}` });
    }
    if (live?.container && path === `/containers/${SHAPES.list.id}` && request.method() === 'GET') return route.fulfill({ json: live.container });
    if (live?.container && path === '/containers' && url.searchParams.get('parent_id') === SHAPE_HUB.id) {
      return route.fulfill({ json: { ...PAGE, data: [live.container, SHAPES.locked] } });
    }
    if (live?.container && path === `/containers/${SHAPES.list.id}/structure` && request.method() === 'PATCH') {
      const { settings = {} } = request.postDataJSON();
      live.container = { ...live.container, version: live.container.version + 1, structure: { ...live.container.structure, settings: { ...live.container.structure.settings, ...settings } } };
      return route.fulfill({ json: { ...live.container, kept_by_lock: [], clipped_by_levels: [] } });
    }
    if (path === '/memberships') {
      const scope = url.searchParams.get('scope_type');
      const rows = scope === 'TENANT' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role }, { id: 'm9', scope_type: 'TENANT', account_id: OTHER.id, role: 'ADMIN' }] : [];
      return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: role === 'OWNER' ? OWNER_GRANTS : [] } });
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
  return { context, page, failures, written };
}

async function openCollection(page, collection) {
  await page.goto(`${served.origin}/collections/${collection.id}`);
  await page.getByRole('heading', { name: collection.name, level: 1 }).waitFor({ state: 'attached', timeout: 15_000 });
  // The role arrives with the memberships; give the screen the moment to draw it.
  await page.waitForTimeout(600);
}

async function menuRows(page, name) {
  // A reader with nothing to do to the place itself has no menu at all.
  await page.waitForTimeout(400);
  if ((await page.getByRole('button', { name: `Actions for ${name}` }).count()) === 0) return [];
  await reveal(page.getByRole('button', { name: `Actions for ${name}` }));
  const menu = page.getByRole('menu', { name: `Actions for ${name}` });
  await menu.waitFor({ timeout: 5_000 });
  return menu.getByRole('menuitem').evaluateAll((items) => items.map((item) => item.querySelector('.label')?.textContent.trim() ?? ''));
}

// The structure's settings are spread over the place's sections (user-experience.md §5.7).
const SECTION_OF = { levels: 'Types & forms', fields: 'Types & forms', views: 'Views', counts: 'Views', completion: 'Workflow', capture: 'Views', celebration: 'Views', automation: 'Rules' };

async function toSection(page, section) {
  await page.getByRole('navigation', { name: 'Parts of the structure' }).getByRole('link', { name: new RegExp(`^${section}( |$)`, 'u') }).click();
  await page.locator('[data-structure-panel]').waitFor({ timeout: 5_000 });
  // *Types & forms* folds the levels' names and what each carries under *Advanced* (page-system §9.6).
  if (section === 'Types & forms') {
    const advanced = page.locator('[data-structure-panel]').getByRole('button', { name: /^Names of the levels and what each carries/u });
    if ((await advanced.getAttribute('aria-expanded')) === 'false') await advanced.click();
  }
}

/** What a control's `aria-describedby` says, as a reader hears it after the name. */
function describedBy(control) {
  return control.evaluate((node) =>
    (node.getAttribute('aria-describedby') ?? '').split(' ').filter(Boolean).map((id) => document.getElementById(id)?.textContent?.trim() ?? '').join(' '),
  );
}

/** The element that has focus, as `role "name"` - never the page's body. */
function focused(page) {
  return page.evaluate(() => {
    const node = document.activeElement;
    if (!node || node === document.body) return 'BODY';
    const label = node.getAttribute('aria-labelledby')?.split(' ').map((id) => document.getElementById(id)?.textContent?.trim() ?? '').join(' ');
    return `${node.getAttribute('role') ?? node.tagName.toLowerCase()} "${(label ?? node.getAttribute('aria-label') ?? node.textContent ?? '').replace(/\s+/gu, ' ').trim()}"`;
  });
}

async function openPanel(page, name, section = 'Templates and presets') {
  await openSettings(page, name);
  await toSection(page, section);
  return page.locator('[data-structure-panel]');
}

// The settings page's sections at a phone's width and a desk's: none out of reach, none widening the
// page. And on *Labels* each colour is named by its colour, not by the name field's label.
for (const width of [390, 1280]) {
  test(`chromium: ${width} px - every section of the settings is in sight, and a label's colours are named`, async (t) => {
    const browser = await sharedChromium(t);
    const { context, page, failures } = await as(browser, 'OWNER', width);
    t.after(() => context.close());

    await openCollection(page, SHAPES.list);
    // The settings' own address: on a phone the section list is the page (PS-9).
    await openSettings(page, SHAPES.list.name);
    const panel = page.locator('[data-structure-tabs]');
    const tabs = panel.getByRole('navigation', { name: 'Parts of the structure' }).getByRole('link');
    await tabs.filter({ hasText: 'People' }).waitFor({ timeout: 10_000 });
    assert.equal(await tabs.count(), 11);
    // On a phone the section list is the page; on a desk a column beside the section. The page
    // never widens for them, and each one is reachable - it comes into view when focused.
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'the sections widened the page');
    for (let index = 0; index < 10; index += 1) {
      const box = await tabs.nth(index).evaluate((tab) => {
        tab.scrollIntoView({ block: 'nearest', inline: 'nearest' });
        return tab.getBoundingClientRect().toJSON();
      });
      assert.ok(box.right <= width + 1 && box.left >= -1, `a section stands at ${box.left}-${box.right}, past the screen's ${width}`);
    }
    await tabs.filter({ hasText: 'Labels' }).click();
    // The section's table is loaded when it is opened (LazyView); a label is made in its Dialog S.
    await page.getByRole('button', { name: 'New label' }).click();
    const made = page.getByRole('dialog', { name: 'New label' });
    await made.getByRole('radio').first().waitFor({ timeout: 10_000 });
    const colours = await made.getByRole('radio').evaluateAll((each) => each.map((radio) => radio.getAttribute('aria-label')));
    assert.ok(colours.length >= 10);
    assert.ok(colours.includes('Slate') && colours.includes('Violet'), `the colours are named ${colours.join(', ')}`);
    assert.deepEqual(failures, []);
  });
}

test('chromium: the owner - a list offers what a list offers, and its panel says where each value comes from (checks 1, 3, 4)', async (t) => {
  const browser = await sharedChromium(t);
  const { context, page, failures, written } = await as(browser, 'OWNER');
  t.after(() => context.close());

  // Check 4: the list offers no board or timeline - no switch at all, one way to look - and no twist.
  await openCollection(page, SHAPES.list);
  await page.getByText(SHAPE_ITEMS[0].title).first().waitFor();
  assert.equal(await viewLink(page, 'Board').count(), 0, 'a board is offered');
  assert.equal(await viewLink(page, 'Timeline').count(), 0, 'a timeline is offered');
  assert.equal(await page.getByRole('button', { name: `Expand ${SHAPE_ITEMS[0].title}` }).count(), 0, 'a twist for children a list does not hold');
  assert.equal(await page.getByRole('button', { name: new RegExp(`inside ${SHAPE_ITEMS[0].title}`) }).count(), 0, 'a child is offered');
  // A date and a person the entry holds from before stay stored and are not drawn on its row.
  const kept = await page.locator(`[data-row="${SHAPE_ITEMS[0].id}"]`).first().textContent();
  assert.ok(!kept.includes('2026') && !kept.includes('JW'), `the row draws what a list does not carry: ${kept}`);
  // ...and its filter asks about nothing a list does not carry: no due date to order by.
  // The order is chosen in the View panel (E-21), the one place a list's question is asked.
  await page.locator('[data-opener="view"]').click();
  const orderBy = page.locator('[data-view-panel]').getByLabel('Sort by');
  await orderBy.waitFor({ state: 'visible', timeout: 5_000 });
  const orders = await orderBy.locator('option').evaluateAll((all) => all.map((each) => each.value));
  assert.ok(orders.includes('title') && !orders.includes('due_at'), `the list is ordered by ${orders}`);
  await page.keyboard.press('Escape');

  // Check 4: on an entry, no date, person, label, note editor or child.
  await page.goto(`${served.origin}/items/${SHAPE_ITEMS[0].id}`);
  await page.getByRole('heading', { name: SHAPE_ITEMS[0].title }).first().waitFor({ timeout: 15_000 });
  await page.waitForTimeout(600);
  for (const detail of ['due', 'assignee', 'labels', 'reminders']) {
    assert.equal(await page.locator(`[data-detail="${detail}"]`).count(), 0, `the entry offers ${detail}`);
  }
  assert.equal(await page.getByLabel('Notes').count(), 0, 'a note editor');
  // What the entry still holds from before - a date, a person, a cover - stays stored and is not drawn.
  // The head's action bar draws no person and no date chip for it (§6.9).
  assert.equal(await page.locator('[data-tour="entry"] .action-bar').locator('.person, .chip').count(), 0, 'a person or a date in the head');
  assert.equal(await page.locator('[data-tour="entry"] .cover-stripe').count(), 0, 'a cover');
  assert.equal(await page.getByRole('button', { name: /^Add (a task|a work package|an activity) inside/ }).count(), 0, 'a child entry');

  // Checks 1 and 3: the panel - every setting with its source, a reset where it is set here.
  await openCollection(page, SHAPES.list);
  const panel = await openPanel(page, SHAPES.list.name);
  for (const setting of ['levels', 'fields', 'views', 'counts', 'completion', 'capture', 'celebration', 'automation']) {
    await toSection(page, SECTION_OF[setting]);
    const section = panel.locator(`[data-setting="${setting}"]`);
    assert.equal(await section.count(), 1, `${setting} missing`);
    const chip = (await section.locator('.head button').first().textContent()).trim();
    assert.equal(chip, setting === 'automation' ? 'Default' : 'Set here', `${setting}: ${chip}`);
    assert.equal(await section.getByRole('button', { name: /^Reset / }).count(), setting === 'automation' ? 0 : 1, `${setting}: reset`);
  }
  // The chip opens where the value comes from.
  await toSection(page, 'Views');
  await panel.locator('[data-setting="views"] .head button').first().click();
  const details = page.getByRole('dialog', { name: 'Where Views comes from' }).or(page.locator('[popover]:popover-open'));
  await details.first().waitFor({ timeout: 5_000 });
  assert.ok((await details.first().textContent()).includes('Set by this collection.'));
  await page.keyboard.press('Escape');

  // The fields offer UX-12's categories, in its own dialog, which writes nothing here unless saved.
  await toSection(page, 'Types & forms');
  await panel.locator('[data-setting="fields"]').getByRole('button', { name: /^Categories of / }).first().click();
  const categories = page.getByRole('dialog', { name: 'Categories' });
  await categories.waitFor({ timeout: 5_000 });
  await categories.getByRole('button', { name: 'Cancel' }).last().click();
  await categories.waitFor({ state: 'hidden', timeout: 5_000 });

  // A reset, then a preset: one save each, the change said in the request.
  await toSection(page, 'Views');
  await panel.getByRole('button', { name: 'Reset Success' }).click();
  await panel.locator('[data-reset-pending]').waitFor();
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await page.waitForFunction(() => document.querySelector('[data-save-bar] .bar') === null, null, { timeout: 5_000 });
  await toSection(page, 'Templates and presets');
  await panel.getByRole('combobox', { name: 'Preset' }).selectOption('PROJECT');
  assert.ok((await panel.locator('[data-setting="preset"]').textContent()).includes('Project replaces the settings below'));
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await page.waitForFunction(() => document.querySelector('[data-save-bar] .bar') === null, null, { timeout: 5_000 });
  assert.deepEqual(written, [{ reset: ['celebration'] }, { preset: 'PROJECT' }]);
  assert.deepEqual(failures, []);
});

test('chromium: the owner - a narrowing first shows what it would stop showing, Hide by default, Flatten on choice (UC-WRK-24 checks 2, 4, 6)', async (t) => {
  const browser = await sharedChromium(t);
  const { context, page, failures, written } = await as(browser, 'OWNER');
  t.after(() => context.close());

  // Check 4: an entry says what its collection hides under it.
  await openCollection(page, SHAPES.list);
  await page.getByText('3 hidden').first().waitFor({ timeout: 5_000 });

  await openCollection(page, SHAPES.locked);
  const panel = await openPanel(page, SHAPES.locked.name);
  await panel.getByRole('combobox', { name: 'Preset' }).selectOption('LIST');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Show less?' });
  await dialog.waitFor({ timeout: 5_000 });
  assert.deepEqual(written, [], 'a narrowing was saved before it was confirmed');
  const said = await dialog.textContent();
  for (const line of ['Work package: 4 entries', 'Activity: 11 entries', 'Task: Dates on 9 entries', 'The view Board']) {
    assert.ok(said.includes(line), `the preview does not say "${line}": ${said}`);
  }
  assert.ok(said.includes('15 entries move to the top level'), 'Flatten does not say what it moves');
  assert.ok(said.includes('does not undo this'), 'Flatten does not say it cannot be undone');
  assert.equal(await dialog.getByRole('radio', { name: 'Hide' }).isChecked(), true, 'Hide is not the default');

  // Cancelled, nothing is written; confirmed with Flatten, the choice travels.
  await dialog.getByRole('button', { name: 'Cancel' }).last().click();
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.deepEqual(written, []);
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await dialog.waitFor({ timeout: 5_000 });
  await dialog.getByRole('radio', { name: 'Flatten' }).check();
  await dialog.getByRole('button', { name: 'Flatten and save' }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.deepEqual(written, [{ preset: 'LIST', narrowing: 'FLATTEN' }]);
  assert.deepEqual(failures, []);
});

test('chromium: the owner - a hub that narrows names the collections keeping their own value (UC-WRK-24 check 7)', async (t) => {
  const browser = await sharedChromium(t);
  const { context, page, failures, written } = await as(browser, 'OWNER');
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/${SHAPE_HUB.id}`);
  await page.getByRole('heading', { name: SHAPE_HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
  await page.waitForTimeout(600);
  const panel = await openPanel(page, SHAPE_HUB.name);
  await panel.getByRole('combobox', { name: 'Preset' }).selectOption('LIST');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Show less?' });
  await dialog.waitFor({ timeout: 5_000 });
  const said = await dialog.textContent();
  assert.ok(said.includes('These collections keep their own value and do not change:'), `the preview does not name who keeps its own: ${said}`);
  assert.ok(said.includes(`${SHAPES.list.name}: Levels, Views`), `the preview does not name ${SHAPES.list.name}: ${said}`);
  await dialog.getByRole('button', { name: 'Hide and save' }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.deepEqual(written, [{ preset: 'LIST', narrowing: 'HIDE' }]);
  assert.deepEqual(failures, []);
});

test('chromium: the owner - a level is named by the name given to it, and by its type without one (check 6)', async (t) => {
  const browser = await sharedChromium(t);
  const { context, page, failures } = await as(browser, 'OWNER');
  t.after(() => context.close());

  // The list named its one level "item": the head's add says so (a list adds at its end by typing).
  await openCollection(page, SHAPES.list);
  await page.getByText(SHAPE_ITEMS[0].title).first().waitFor();
  const said = await page.locator('[data-opener="add-entry"]').evaluateAll((controls) => controls.map((control) => control.textContent.trim()));
  assert.ok(said.length >= 1, `add controls: ${said}`);
  // The foot's add says the level and the place: "item in “Shopping”" (work-surface concept §8, L-6).
  assert.ok(said.every((text) => text === `item in “${SHAPES.list.name}”`), `add controls read ${JSON.stringify(said)}`);
  assert.ok(!said.some((text) => /^(Add an entry|Entry in)/u.test(text)), 'an add left unnamed');

  // An entry's badge names its level by that name.
  await page.goto(`${served.origin}/items/${SHAPE_ITEMS[0].id}`);
  await page.getByRole('heading', { name: SHAPE_ITEMS[0].title }).first().waitFor({ timeout: 15_000 });
  await page.waitForTimeout(600);
  // The head's type beside the key names the level (§6.9).
  assert.match((await page.locator('[data-tour="entry"] .key-row').innerText()).trim(), /\bitem\b/);

  // Without a name, the catalogue's words: the project adds "an entry", and no name leaks into it.
  await openCollection(page, SHAPES.locked);
  await page.locator('[data-opener="add-entry"]').first().waitFor({ timeout: 5_000 }).catch(() => assert.fail('the unnamed project lost its add'));
  const unnamed = await page.locator('[data-opener="add-entry"]').evaluateAll((controls) => controls.map((control) => control.textContent.trim()));
  assert.ok(unnamed.every((text) => text === 'Add an entry' || text === `Entry in “${SHAPES.locked.name}”`), `a name leaked into another collection: ${JSON.stringify(unnamed)}`);
  assert.deepEqual(failures, []);
});

test('chromium: the owner - creating a collection offers the presets, and the one chosen is sent (check 1)', async (t) => {
  const browser = await sharedChromium(t);
  const { context, page, failures, written } = await as(browser, 'OWNER');
  t.after(() => context.close());
  await page.goto(`${served.origin}/hubs/${SHAPE_HUB.id}`);
  await page.getByRole('heading', { name: SHAPE_HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
  await openNewCollection(page);
  const dialog = page.locator('dialog[open]');
  // The kinds as tiles of one choice (UC-WRK-01 check 12).
  await kindChoice(dialog, 'List').waitFor({ timeout: 5_000 });
  for (const name of ['List', 'Tasks', 'Project', 'Flow', 'Scrum']) {
    assert.equal(await kindChoice(dialog, name).count(), 1, `${name} is not offered`);
  }
  // The hub's own structure is the first tile, chosen at opening.
  assert.equal(await kindChoice(dialog, 'Like its hub').isChecked(), true, 'the hub\'s own structure is not chosen at opening');
  await dialog.getByRole('textbox', { name: 'Name' }).fill('Pantry');
  await kindChoice(dialog, 'List').check();
  assert.equal(await kindChoice(dialog, 'List').isChecked(), true);
  await dialog.getByRole('button', { name: 'Create', exact: true }).click();
  await page.waitForFunction(() => document.querySelector('dialog[open]') === null, null, { timeout: 5_000 }).catch(() => {});
  assert.equal(written[0]?.preset, 'LIST');
  assert.equal(written[0]?.name, 'Pantry');
  assert.deepEqual(failures, []);
});

test('chromium: the owner - the hub\'s Edit changes its name, description and look, and its default preset (check 1)', async (t) => {
  const browser = await sharedChromium(t);
  const { context, page, failures, written } = await as(browser, 'OWNER');
  t.after(() => context.close());
  const renamed = [];
  page.on('request', (request) => {
    if (request.method() === 'PATCH' && request.url().endsWith(`/api/v1/containers/${SHAPE_HUB.id}`)) renamed.push(request.postDataJSON());
  });
  await page.goto(`${served.origin}/hubs/${SHAPE_HUB.id}`);
  await page.getByRole('heading', { name: SHAPE_HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
  await page.waitForTimeout(600);
  // One *Settings* instead of a rename and a separate look; a hub has nowhere to move to (§E9).
  const rows = await menuRows(page, SHAPE_HUB.name);
  assert.ok(rows.includes('Settings…'), rows.join());
  for (const gone of ['Edit…', 'Rename', 'Colour and symbol…', 'Move to another hub…']) assert.ok(!rows.includes(gone), `${gone} in ${rows}`);
  await page.getByRole('menuitem', { name: 'Settings…' }).click();
  const dialog = page.locator('[data-place-general]');
  await dialog.getByRole('textbox', { name: 'Hub name' }).waitFor({ timeout: 5_000 });
  assert.equal(await dialog.getByRole('textbox', { name: 'Hub name' }).inputValue(), SHAPE_HUB.name);
  // Nothing changed, nothing to save: the bar is not there until there is.
  assert.equal(await page.getByRole('region', { name: 'Unsaved changes' }).count(), 0, 'a save offered with nothing changed');
  // The hub has no preset of its own yet: it starts from the workspace's, and may choose one.
  await dialog.getByRole('radio', { name: 'List' }).waitFor({ timeout: 5_000 });
  assert.ok(await dialog.getByRole('radio', { name: "The workspace's default" }).isChecked());
  await dialog.getByRole('textbox', { name: 'Hub name' }).fill('Chores');
  await dialog.getByRole('textbox', { name: 'Description' }).fill('What the week asks for.');
  await dialog.getByRole('radio', { name: 'List' }).check();
  const bar = page.getByRole('region', { name: 'Unsaved changes' });
  assert.match(await bar.innerText(), /3 changes/u);
  await bar.getByRole('button', { name: 'Save' }).click();
  await bar.waitFor({ state: 'detached', timeout: 5_000 });
  assert.equal(renamed.length, 1);
  assert.equal(renamed[0].name, 'Chores');
  assert.equal(renamed[0].description, 'What the week asks for.');
  assert.deepEqual(written, [{ preset: 'LIST' }], 'the preset is its own write, sent as itself');
  // The hub's collections as cards, each a way in.
  await page.goto(`${served.origin}/hubs/${SHAPE_HUB.id}`);
  const cards = page.getByRole('list', { name: 'Collections' });
  for (const collection of Object.values(SHAPES)) {
    assert.equal(await cards.getByRole('link', { name: collection.name }).getAttribute('href'), `/collections/${collection.id}`);
  }
  assert.deepEqual(failures, []);
});

test('chromium: an administrator - a lock shows its value and who set it, and cannot be changed (check 7)', async (t) => {
  const browser = await sharedChromium(t);
  const { context, page, failures, written } = await as(browser, 'ADMIN');
  t.after(() => context.close());

  await openCollection(page, SHAPES.locked);
  // A project opens with what is inside shown, its default view.
  assert.equal(await page.getByRole('button', { name: 'Show what is inside', exact: true }).getAttribute('aria-pressed'), 'true', 'the project did not open as its outline');
  const panel = await openPanel(page, SHAPES.locked.name, 'Views');
  const views = panel.locator('[data-setting="views"]');
  assert.equal((await views.locator('.head button').first().textContent()).trim(), 'Locked by Mara Lind');
  assert.equal(await views.getByRole('button', { name: /^Reset / }).count(), 0, 'a locked setting offers a reset');
  // The controls stay, readable and unavailable, with the lock's reason.
  const board = views.getByRole('checkbox', { name: 'Board' });
  assert.equal(await board.getAttribute('aria-disabled'), 'true');
  assert.ok(await board.isChecked());
  await board.focus();
  assert.ok((await page.locator('[role="tooltip"]:visible, [popover]:popover-open').allTextContents()).join(' ').includes(`Locked by ${OTHER.display_name}`));
  await board.press('Space');
  assert.ok(await board.isChecked(), 'the locked box changed');
  // One field of one level: the task's dates, locked inside the field's list with its reason.
  await toSection(page, 'Types & forms');
  const field = panel.locator('[data-setting="fields"]').getByRole('button', { name: /^What Task carries/u });
  assert.equal((await field.locator('.value').textContent()).trim(), 'All 15 fields', 'the closed field does not say all are on');
  await field.click();
  const list = page.getByRole('listbox', { name: 'What Task carries' });
  await list.waitFor({ timeout: 5_000 });
  const dates = list.getByRole('option', { name: 'Dates', exact: true });
  assert.equal(await dates.getAttribute('aria-disabled'), 'true');
  assert.equal(await dates.getAttribute('aria-selected'), 'true');
  assert.match(await describedBy(dates), new RegExp(`Locked by ${OTHER.display_name}`, 'u'));
  await page.keyboard.press('Escape');
  await list.waitFor({ state: 'detached', timeout: 5_000 });
  // The chip names who set the lock.
  await toSection(page, 'Views');
  await views.locator('.head button').first().click();
  await page.locator('[popover]:popover-open').first().waitFor({ timeout: 5_000 });
  assert.ok((await page.locator('[popover]:popover-open').first().textContent()).includes(`Locked by ${OTHER.display_name}. Ask them to change it.`));
  await page.keyboard.press('Escape');

  // A preset under the lock: sent as itself, the locked value kept and said.
  // List offers fewer levels, so it is previewed first and confirmed with the default, Hide.
  await toSection(page, 'Templates and presets');
  await panel.getByRole('combobox', { name: 'Preset' }).selectOption('LIST');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await page.getByRole('dialog', { name: 'Show less?' }).getByRole('button', { name: 'Hide and save' }).click();
  await page.getByText('1 locked setting kept its value.').first().waitFor({ timeout: 5_000 });
  assert.deepEqual(written, [{ preset: 'LIST', narrowing: 'HIDE' }]);
  assert.deepEqual(failures, []);
});

// Check 10: the owner ticks "A date" under Capture; meanwhile somebody else takes "A leading
// amount" out of Capture and turns Success to every celebration. The bar counts the owner's one
// change and offers no Save; the notice names Capture, changed on both sides.
async function changedMeanwhile(page, panel, live) {
  const capture = panel.locator('[data-setting="capture"]');
  await capture.getByRole('checkbox', { name: 'A date' }).click();
  const bar = page.getByRole('region', { name: 'Unsaved changes' });
  await bar.getByText('1 change').waitFor({ timeout: 5_000 });
  live.change({ capture: ['LINES'], celebration: 'ALL' });
  const notice = page.getByRole('alert').filter({ hasText: 'The settings were changed meanwhile' });
  await notice.waitFor({ timeout: 15_000 });
  assert.ok((await notice.textContent()).includes('You also changed Capture: applying keeps your value there.'), await notice.textContent());
  assert.equal(await bar.getByText('1 change').count(), 1, 'the bar counts the change made elsewhere as the owner\'s');
  assert.equal(await bar.getByRole('button', { name: 'Save', exact: true }).count(), 0, 'Save is offered while the settings changed meanwhile');
  assert.equal(await bar.getByRole('button', { name: 'Apply my changes', exact: true }).count(), 1);
  return { notice, bar, capture };
}

test('chromium: the owner - settings changed meanwhile: applying my changes sends only mine (check 10)', async (t) => {
  const browser = await sharedChromium(t);
  const live = elsewhere();
  const { context, page, failures, written } = await as(browser, 'OWNER', 1280, live);
  t.after(() => context.close());

  await openCollection(page, SHAPES.list);
  const panel = await openPanel(page, SHAPES.list.name, 'Views');
  const { notice, bar, capture } = await changedMeanwhile(page, panel, live);

  await notice.getByRole('button', { name: 'Apply my changes to the current settings' }).click();
  await notice.waitFor({ state: 'detached', timeout: 5_000 });
  // The change made elsewhere is shown; the owner's own stays on top of it.
  assert.equal(await panel.locator('[data-setting="celebration"]').getByRole('combobox').inputValue(), 'ALL');
  assert.ok(await capture.getByRole('checkbox', { name: 'A date' }).isChecked());
  await bar.getByText('1 change').waitFor();
  await bar.getByRole('button', { name: 'Save', exact: true }).click();
  await page.waitForFunction(() => document.querySelector('[data-save-bar] .bar') === null, null, { timeout: 5_000 });
  assert.deepEqual(written, [{ settings: { capture: ['LINES', 'AMOUNT', 'DATE'] } }], 'a setting the owner did not change was sent');
  assert.deepEqual(failures, []);
});

test('chromium: the owner - settings changed meanwhile while the save was previewed: nothing is written over them (check 10)', async (t) => {
  const browser = await sharedChromium(t);
  const live = elsewhere();
  const { context, page, failures, written } = await as(browser, 'OWNER', 1280, live);
  t.after(() => context.close());

  await openCollection(page, SHAPES.list);
  const panel = await openPanel(page, SHAPES.list.name, 'Views');
  await panel.locator('[data-setting="capture"]').getByRole('checkbox', { name: 'A date' }).click();
  const bar = page.getByRole('region', { name: 'Unsaved changes' });
  await bar.getByText('1 change').waitFor({ timeout: 5_000 });
  // The save's preview is answered only once the other change is in force here.
  let release;
  const held = new Promise((resolve) => (release = resolve));
  await page.route('**/structure:preview', async (route) => {
    await held;
    await route.fallback();
  });
  const previewed = page.waitForRequest((request) => request.url().endsWith('/structure:preview') && request.postDataJSON()?.settings?.capture !== undefined, { timeout: 10_000 });
  await bar.getByRole('button', { name: 'Save', exact: true }).click();
  await previewed;
  // An inherited change: the list's own version, the one the save writes against, stays.
  live.change({ capture: ['LINES'], celebration: 'ALL' }, { inherited: true });
  const notice = page.getByRole('alert').filter({ hasText: 'The settings were changed meanwhile' });
  await notice.waitFor({ timeout: 15_000 });
  release();

  await notice.getByRole('button', { name: 'Apply my changes to the current settings' }).click()
    .catch(() => assert.fail(`the previewed save was written over the other change: ${JSON.stringify(written)}`));
  await notice.waitFor({ state: 'detached', timeout: 5_000 });
  await bar.getByRole('button', { name: 'Save', exact: true }).click();
  await page.waitForFunction(() => document.querySelector('[data-save-bar] .bar') === null, null, { timeout: 5_000 });
  assert.deepEqual(written, [{ settings: { capture: ['LINES', 'AMOUNT', 'DATE'] } }], 'the save previewed before the other change was written');
  assert.deepEqual(failures, []);
});

test('chromium: the owner - settings changed meanwhile: showing the current settings sends nothing (check 10)', async (t) => {
  const browser = await sharedChromium(t);
  const live = elsewhere();
  const { context, page, failures, written } = await as(browser, 'OWNER', 1280, live);
  t.after(() => context.close());

  await openCollection(page, SHAPES.list);
  const panel = await openPanel(page, SHAPES.list.name, 'Views');
  const { notice, capture } = await changedMeanwhile(page, panel, live);

  await notice.getByRole('button', { name: 'Show the current settings' }).click();
  await notice.waitFor({ state: 'detached', timeout: 5_000 });
  await page.waitForFunction(() => document.querySelector('[data-save-bar] .bar') === null, null, { timeout: 5_000 });
  assert.equal(await panel.locator('[data-setting="celebration"]').getByRole('combobox').inputValue(), 'ALL');
  assert.equal(await capture.getByRole('checkbox', { name: 'A date' }).isChecked(), false);
  assert.equal(await capture.getByRole('checkbox', { name: 'A leading amount' }).isChecked(), false);
  assert.deepEqual(written, []);
  assert.deepEqual(failures, []);
});

test('chromium: a member - the list as its settings make it, and no settings page (check 9)', async (t) => {
  const browser = await sharedChromium(t);
  const { context, page, failures } = await as(browser, 'MEMBER');
  t.after(() => context.close());

  await openCollection(page, SHAPES.list);
  await page.getByText(SHAPE_ITEMS[0].title).first().waitFor();
  const rows = await menuRows(page, SHAPES.list.name);
  assert.ok(!rows.some((row) => row.startsWith('Structure')), `offered ${rows}`);
  assert.equal(await viewLink(page, 'Board').count(), 0, 'a board is offered');
  await page.keyboard.press('Escape');

  await page.goto(`${served.origin}/hubs/${SHAPE_HUB.id}`);
  await page.getByRole('heading', { name: SHAPE_HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
  await page.waitForTimeout(600);
  // A member has nothing to do to a hub itself: no menu at all, and the people in the head's faces.
  const hubMenu = page.getByRole('button', { name: `Actions for ${SHAPE_HUB.name}` });
  const hubRows = (await hubMenu.count()) === 0 ? [] : await menuRows(page, SHAPE_HUB.name);
  assert.ok(!hubRows.some((row) => row.startsWith('Structure')), `offered ${hubRows}`);
  assert.equal(await page.locator('[data-opener="members"]').count(), 1, 'no way to the people');
  // Each face's initials stay whole: the face drawn over it covers only its edge.
  const covered = await page.locator('[data-opener="members"]').evaluate((button) => {
    const faces = [...button.querySelectorAll('.avatar, .more')].map((face) => {
      const words = document.createRange();
      words.selectNodeContents(face);
      return { box: face.getBoundingClientRect(), text: words.getBoundingClientRect(), name: face.textContent };
    }).sort((a, b) => a.box.left - b.box.left);
    return faces.slice(1).flatMap((face, i) => (faces[i].box.right > face.text.left ? [`${face.name} by ${Math.round(faces[i].box.right - face.text.left)} px`] : []));
  });
  assert.ok(await page.locator('[data-opener="members"] .avatar').count() >= 2, 'the head draws fewer than two faces');
  assert.deepEqual(covered, [], 'a face covers the initials of the next');
  assert.equal(await page.locator('[data-opener="structure"]').count(), 0, 'a ⚙ to settings the member may not change');
  // The address alone does not open them either: the page says whose they are. The completion
  // policy is one of them (UC-WRK-04 check 7), offered to the owner in Workflow above.
  await page.goto(`${served.origin}/collections/${SHAPES.list.id}/settings/structure`);
  await page.getByText('Only someone who may change how this place is set up sees its settings.').waitFor({ timeout: 15_000 });
  assert.equal(await page.locator('[data-structure-panel]').count(), 0);
  assert.deepEqual(failures, []);
});

// A place's settings are drawn by the one settings shell (user-experience.md §5.4, §5.7): the
// sections listed at the start, the save bar once something changed, and a move away asking
// before it drops the change - for a hub and for a collection alike.
for (const place of [
  { kind: 'hub', path: () => `/hubs/${SHAPE_HUB.id}`, name: () => SHAPE_HUB.name },
  { kind: 'collection', path: () => `/collections/${SHAPES.list.id}`, name: () => SHAPES.list.name },
]) {
  test(`chromium: a ${place.kind}'s settings have the shell's sections, save bar and leave guard`, async (t) => {
    const browser = await sharedChromium(t);
    const { context, page, failures } = await as(browser, 'OWNER');
    t.after(() => context.close());

    await page.goto(`${served.origin}${place.path()}`);
    await page.getByRole('heading', { name: place.name(), level: 1 }).waitFor({ timeout: 15_000 });
    await page.waitForTimeout(600);
    await openSettings(page, place.name());
    const shell = page.locator('[data-structure-tabs] [data-page-kind="settings"]');
    await shell.waitFor({ timeout: 10_000 });
    const sections = shell.getByRole('navigation', { name: 'Parts of the structure' }).getByRole('link');
    assert.ok((await sections.count()) >= 2, 'the sections are listed');

    const general = page.locator('[data-place-general]');
    const name = general.getByRole('textbox').first();
    await name.waitFor({ timeout: 5_000 });
    await name.fill(`${place.name()} renamed`);
    const bar = page.getByRole('region', { name: 'Unsaved changes' });
    await bar.getByText('1 change').waitFor({ timeout: 5_000 });

    // A move away asks; staying keeps the screen and the change.
    const settingsPath = new URL(page.url()).pathname;
    await page.evaluate(() => {
      const link = document.createElement('a');
      link.href = '/';
      document.body.append(link);
      link.click();
      link.remove();
    });
    const question = page.getByRole('dialog', { name: 'Leave without saving?' });
    await question.waitFor({ timeout: 5_000 });
    await question.getByRole('button', { name: 'Stay' }).last().click();
    await question.waitFor({ state: 'detached', timeout: 5_000 });
    assert.equal(new URL(page.url()).pathname, settingsPath);
    await bar.getByText('1 change').waitFor();
    assert.deepEqual(failures, []);
  });
}

/** The settings section at its address, with *Types & forms*' fold open. */
async function openSectionAt(page, collection, section, fold = /^Names of the levels and what each carries/u) {
  await page.goto(`${served.origin}/collections/${collection.id}/settings/${section}`);
  await page.locator('[data-structure-panel]').waitFor({ timeout: 15_000 });
  if (section !== 'types') return;
  const advanced = page.locator('[data-structure-panel]').getByRole('button', { name: fold });
  if ((await advanced.getAttribute('aria-expanded')) === 'false') await advanced.click();
}

/** The chips the closed field draws, in their order. */
const chipsOf = (field) => field.locator('.chips:not(.measure) .chip').allTextContents();

// What a level carries is one field (design-system.md §4; UC-WRK-32 checks 1 and 3): one stop named
// by its row, Enter opens the grouped list on the first chosen option, Space toggles and the save
// bar counts it, the always-on *Done or open* stays and says why, Escape closes onto the field. At
// a phone's width the list is the sheet and *Done* closes it. *None* leaves the locked one alone.
for (const width of [1280, 390]) {
  test(`chromium: ${width} px - what a level carries is one field: named, grouped, by the keyboard`, async (t) => {
    const browser = await sharedChromium(t);
    const { context, page, failures } = await as(browser, 'OWNER', width);
    t.after(() => context.close());

    await openSectionAt(page, SHAPES.list, 'types');
    // The list's one level is named "item"; the field is named by its row and says what is chosen.
    const field = page.locator('[data-setting="fields"]').getByRole('button', { name: /^What item carries/u });
    await field.waitFor({ timeout: 5_000 });
    assert.equal(await field.getAttribute('aria-haspopup'), 'listbox');
    assert.match(await field.evaluate((node) => node.getAttribute('aria-labelledby').split(' ').map((id) => document.getElementById(id)?.textContent?.trim()).join(' ')), /^What item carries Done or open, History,? and Custom fields$/u);
    assert.deepEqual(await chipsOf(field), ['Done or open', 'History', 'Custom fields']);
    assert.equal(await page.locator('[data-setting="fields"] input[type="checkbox"]').count(), 0, 'the ticks are still drawn');

    await field.focus();
    await page.keyboard.press('Enter');
    const list = page.getByRole('listbox', { name: 'What item carries' });
    await list.waitFor({ timeout: 5_000 });
    if (width === 390) await page.getByRole('dialog', { name: 'What item carries' }).waitFor({ timeout: 5_000 });
    assert.deepEqual(
      await list.getByRole('group').evaluateAll((groups) => groups.map((group) => document.getElementById(group.getAttribute('aria-labelledby'))?.textContent)),
      ['Status and time', 'People', 'Content', 'Extensions'],
    );
    assert.equal(await focused(page), 'option "Done or open"', 'Enter did not land on the first chosen option');
    // The always-on option: ticked, locked, and its reason said.
    const always = list.getByRole('option', { name: 'Done or open' });
    assert.equal(await always.getAttribute('aria-disabled'), 'true');
    assert.equal(await describedBy(always), 'Every entry can be done.');
    await page.keyboard.press('Space');
    assert.equal(await always.getAttribute('aria-selected'), 'true', 'Done or open was cleared');
    assert.equal(await focused(page), 'option "Done or open"');
    await page.keyboard.press('ArrowDown');
    assert.equal(await focused(page), 'option "Dates"');
    await page.keyboard.press('Space');
    assert.equal(await list.getByRole('option', { name: 'Dates' }).getAttribute('aria-selected'), 'true');
    const bar = page.getByRole('region', { name: 'Unsaved changes' });
    await bar.getByText('1 change').waitFor({ timeout: 5_000 });
    assert.equal(await focused(page), 'option "Dates"', 'the toggle moved focus');

    if (width === 390) await page.getByRole('dialog', { name: 'What item carries' }).getByRole('button', { name: 'Done', exact: true }).click();
    else await page.keyboard.press('Escape');
    await list.waitFor({ state: 'detached', timeout: 5_000 });
    assert.match(await focused(page), /^button "What item carries /u, 'closing left focus elsewhere');
    assert.deepEqual(await chipsOf(field), ['Done or open', 'Dates', 'History', 'Custom fields']);

    // *None* clears what can change; the always-on option stays, and the save sends it alone.
    await field.click();
    await list.waitFor({ timeout: 5_000 });
    await page.getByRole('button', { name: 'None', exact: true }).click();
    assert.deepEqual(await list.locator('[aria-selected="true"] .name > span:first-child').allTextContents(), ['Done or open']);
    assert.notEqual(await focused(page), 'BODY');
    if (width === 390) await page.getByRole('button', { name: 'Done', exact: true }).click();
    else await page.keyboard.press('Escape');
    await list.waitFor({ state: 'detached', timeout: 5_000 });
    assert.match(await focused(page), /^button "What item carries /u);
    assert.deepEqual(await chipsOf(field), ['Done or open']);
    const sent = page.waitForRequest((request) => request.method() === 'PATCH' && request.url().endsWith('/structure'));
    await bar.getByRole('button', { name: 'Save', exact: true }).click();
    assert.deepEqual((await sent).postDataJSON().settings.fields, { TASK: { capabilities: ['COMPLETION'] } });
    assert.deepEqual(failures, []);
  });
}

// A view must be offered (UC-WRK-32 check 3): the views are ticks named "(required)"; clearing the
// last one and saving marks the group, says the error as its description and moves focus to it.
for (const width of [1280, 390]) {
  test(`chromium: ${width} px - no view offered: Save marks the views, says why and moves focus there`, async (t) => {
    const browser = await sharedChromium(t);
    const { context, page, failures, written } = await as(browser, 'OWNER', width);
    t.after(() => context.close());

    await openSectionAt(page, SHAPES.list, 'views');
    const views = page.locator('[data-setting="views"]').getByRole('group', { name: 'Views offered (required)' });
    await views.waitFor({ timeout: 5_000 });
    const boxes = views.getByRole('checkbox');
    const only = boxes.and(page.locator(':checked'));
    assert.equal(await only.count(), 1, 'the list offers more than one view');
    await only.click();
    await page.getByRole('region', { name: 'Unsaved changes' }).getByRole('button', { name: 'Save', exact: true }).click();
    await page.locator('[data-setting="views"] [role="group"][data-invalid]').waitFor({ timeout: 5_000 });
    assert.equal(await describedBy(views), 'Offer at least one view.');
    await page.waitForFunction(() => document.activeElement?.getAttribute('aria-invalid') === 'true', null, { timeout: 5_000 }).catch(() => assert.fail('focus did not move to the views'));
    assert.equal(await page.evaluate(() => document.activeElement?.closest('[data-setting]')?.getAttribute('data-setting')), 'views');
    assert.deepEqual(written, [], 'a structure without a view was sent');
    assert.deepEqual(failures, []);
  });
}

// The same field in German at a phone's width: its name, the sentence the closed field says with
// every field on, its groups, and nothing running past the screen.
test('chromium: 390 px - in German the field says "Alle 15 Felder" and its groups, within the screen', async (t) => {
  const browser = await sharedChromium(t);
  const { context, page, failures } = await as(browser, 'ADMIN', 390);
  t.after(() => context.close());
  await context.route('**/api/v1/accounts/me', (route) => (route.request().method() === 'GET' ? route.fulfill({ json: { ...ACCOUNT, locale: 'de' } }) : route.fallback()));

  await openSectionAt(page, SHAPES.locked, 'types', /^Namen der Ebenen und was jede trägt/u);
  await page.waitForFunction(() => document.documentElement.lang === 'de', null, { timeout: 15_000 });
  const field = page.locator('[data-setting="fields"]').getByRole('button', { name: /^Was Aufgabe trägt/u });
  await field.waitFor({ timeout: 5_000 });
  assert.equal((await field.locator('.value').textContent()).trim(), 'Alle 15 Felder');
  const box = await field.boundingBox();
  assert.ok(box && box.x >= 0 && box.x + box.width <= 390, `the field stands at ${box?.x}-${(box?.x ?? 0) + (box?.width ?? 0)}`);
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'the page widened');
  await field.click();
  const sheet = page.getByRole('dialog', { name: 'Was Aufgabe trägt' });
  await sheet.waitFor({ timeout: 5_000 });
  assert.deepEqual(
    await sheet.getByRole('listbox').getByRole('group').evaluateAll((groups) => groups.map((group) => document.getElementById(group.getAttribute('aria-labelledby'))?.textContent)),
    ['Status und Zeit', 'Personen', 'Inhalt', 'Erweiterungen'],
  );
  await sheet.getByRole('button', { name: 'Fertig', exact: true }).click();
  await sheet.waitFor({ state: 'detached', timeout: 5_000 });
  assert.match(await focused(page), /^button "Was Aufgabe trägt /u);
  assert.deepEqual(failures, []);
});

// The list of what a level carries, opened where the field stands near the middle of the screen
// (anchor.ts, the shrunk fallback): at the reviewer's height of 990 it covers neither the field nor
// runs past the viewport, End focuses an option the reader sees, and the page does not scroll.
test('chromium: 1280×990 - what a level carries opens beside its field, inside the screen', async (t) => {
  const browser = await sharedChromium(t);
  const { context, page, failures } = await as(browser, 'ADMIN', 1280);
  t.after(() => context.close());
  await page.setViewportSize({ width: 1280, height: 990 });
  await openSectionAt(page, SHAPES.list, 'types');
  const field = page.locator('[data-setting="fields"]').getByRole('button', { name: /^What .* carries/u }).first();
  await field.focus();
  await page.keyboard.press('ArrowDown');
  await page.getByRole('listbox').waitFor({ timeout: 5_000 });
  assert.deepEqual(await openBeside(page), { coversField: false, insideViewport: true, endFocusesVisibleOption: true, behindScrolled: 0 });
  assert.deepEqual(failures, []);
});
