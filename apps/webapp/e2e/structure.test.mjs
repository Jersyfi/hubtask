// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// How much structure a collection has (UC-WRK-23 checks 1, 3, 4, 6, 7, 9) and switching it without
// loss (UC-WRK-24 checks 1, 2, 4, 6, 7), walked over the shared
// fixture's structure hub as three readers:
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
// * a member sees the list as its settings make it, and no Structure panel.
//
// Chromium only; what the server decides is its own tests' (check 8).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, OTHER, OWNER_GRANTS, PAGE, SHAPE_HUB, SHAPE_ITEMS, SHAPES, forgetUnstubbed, showStructures, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());
showStructures();

/** A signed-in tab whose reader holds `role` on the workspace, with the structure writes recorded. */
async function as(browser, role, width = 1280) {
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
  await page.getByRole('button', { name: `Actions for ${name}` }).click();
  const menu = page.getByRole('menu', { name: `Actions for ${name}` });
  await menu.waitFor({ timeout: 5_000 });
  return menu.getByRole('menuitem').evaluateAll((items) => items.map((item) => item.querySelector('.label')?.textContent.trim() ?? ''));
}

async function openPanel(page, name) {
  await menuRows(page, name);
  await page.getByRole('menuitem', { name: 'Structure' }).click();
  const panel = page.locator('[data-structure-panel]');
  await panel.waitFor({ timeout: 5_000 });
  return panel;
}

test('chromium: the owner - a list offers what a list offers, and its panel says where each value comes from (checks 1, 3, 4)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures, written } = await as(browser, 'OWNER');
  t.after(() => context.close());

  // Check 4: the list offers no board or timeline - no switch at all, one way to look - and no twist.
  await openCollection(page, SHAPES.list);
  await page.getByText(SHAPE_ITEMS[0].title).first().waitFor();
  assert.equal(await page.getByRole('radio', { name: 'Board' }).count(), 0, 'a board is offered');
  assert.equal(await page.getByRole('radio', { name: 'Timeline' }).count(), 0, 'a timeline is offered');
  assert.equal(await page.getByRole('button', { name: `Expand ${SHAPE_ITEMS[0].title}` }).count(), 0, 'a twist for children a list does not hold');
  assert.equal(await page.getByRole('button', { name: new RegExp(`inside ${SHAPE_ITEMS[0].title}`) }).count(), 0, 'a child is offered');

  // Check 4: on an entry, no date, person, label, note editor or child.
  await page.goto(`${served.origin}/items/${SHAPE_ITEMS[0].id}`);
  await page.getByRole('heading', { name: SHAPE_ITEMS[0].title }).first().waitFor({ timeout: 15_000 });
  await page.waitForTimeout(600);
  for (const detail of ['due', 'assignee', 'labels', 'reminders']) {
    assert.equal(await page.locator(`[data-detail="${detail}"]`).count(), 0, `the entry offers ${detail}`);
  }
  assert.equal(await page.getByLabel('Notes').count(), 0, 'a note editor');
  assert.equal(await page.getByRole('button', { name: /^Add (a task|a work package|an activity) inside/ }).count(), 0, 'a child entry');

  // Checks 1 and 3: the panel - every setting with its source, a reset where it is set here.
  await openCollection(page, SHAPES.list);
  const panel = await openPanel(page, SHAPES.list.name);
  for (const setting of ['levels', 'fields', 'views', 'counts', 'completion', 'capture', 'celebration', 'automation']) {
    const section = panel.locator(`[data-setting="${setting}"]`);
    assert.equal(await section.count(), 1, `${setting} missing`);
    const chip = (await section.locator('.head button').first().textContent()).trim();
    assert.equal(chip, setting === 'automation' ? 'Default' : 'Set here', `${setting}: ${chip}`);
    assert.equal(await section.getByRole('button', { name: /^Reset / }).count(), setting === 'automation' ? 0 : 1, `${setting}: reset`);
  }
  // The chip opens where the value comes from.
  await panel.locator('[data-setting="views"] .head button').first().click();
  const details = page.getByRole('dialog', { name: 'Where Views comes from' }).or(page.locator('[popover]:popover-open'));
  await details.first().waitFor({ timeout: 5_000 });
  assert.ok((await details.first().textContent()).includes('Set by this collection.'));
  await page.keyboard.press('Escape');

  // The fields offer UX-12's categories, in its own dialog, which writes nothing here unless saved.
  await panel.locator('[data-setting="fields"]').getByRole('button', { name: /^Categories of / }).first().click();
  const categories = page.getByRole('dialog', { name: 'Categories' });
  await categories.waitFor({ timeout: 5_000 });
  await categories.getByRole('button', { name: 'Cancel' }).last().click();
  await categories.waitFor({ state: 'hidden', timeout: 5_000 });

  // A reset, then a preset: one save each, the change said in the request.
  await panel.getByRole('button', { name: 'Reset Success' }).click();
  await panel.locator('[data-reset-pending]').waitFor();
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await page.waitForFunction(() => document.querySelector('[data-save-bar] .bar') === null, null, { timeout: 5_000 });
  await panel.getByRole('combobox', { name: 'Preset' }).selectOption('PROJECT');
  assert.ok((await panel.locator('[data-setting="preset"]').textContent()).includes('Project replaces the settings below'));
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await page.waitForFunction(() => document.querySelector('[data-save-bar] .bar') === null, null, { timeout: 5_000 });
  assert.deepEqual(written, [{ reset: ['celebration'] }, { preset: 'PROJECT' }]);
  assert.deepEqual(failures, []);
});

test('chromium: the owner - a narrowing first shows what it would stop showing, Hide by default, Flatten on choice (UC-WRK-24 checks 2, 4, 6)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
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
  const browser = await chromium.launch();
  t.after(() => browser.close());
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
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await as(browser, 'OWNER');
  t.after(() => context.close());

  // The list named its one level "item": the head's add says so (a list adds at its end by typing).
  await openCollection(page, SHAPES.list);
  await page.getByText(SHAPE_ITEMS[0].title).first().waitFor();
  const said = await page.locator('[data-opener="add-entry"]').evaluateAll((controls) => controls.map((control) => control.textContent.trim()));
  assert.ok(said.length >= 1, `add controls: ${said}`);
  assert.ok(said.every((text) => text === 'Add item'), `add controls read ${JSON.stringify(said)}`);
  assert.equal(await page.getByRole('button', { name: 'Add an entry', exact: true }).count(), 0, 'an add left unnamed');

  // An entry's badge names its level by that name.
  await page.goto(`${served.origin}/items/${SHAPE_ITEMS[0].id}`);
  await page.getByRole('heading', { name: SHAPE_ITEMS[0].title }).first().waitFor({ timeout: 15_000 });
  await page.waitForTimeout(600);
  assert.equal((await page.locator('.marks .badge').first().textContent()).trim(), 'item');

  // Without a name, the catalogue's words: the project adds "an entry", and no name leaks into it.
  await openCollection(page, SHAPES.locked);
  assert.ok((await page.getByRole('button', { name: 'Add an entry', exact: true }).count()) > 0, 'the unnamed project lost its add');
  assert.equal(await page.getByRole('button', { name: 'Add item', exact: true }).count(), 0, 'a name leaked into another collection');
  assert.deepEqual(failures, []);
});

test('chromium: the owner - creating a collection offers the presets, and the one chosen is sent (check 1)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures, written } = await as(browser, 'OWNER');
  t.after(() => context.close());
  await page.goto(`${served.origin}/hubs/${SHAPE_HUB.id}`);
  await page.getByRole('heading', { name: SHAPE_HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
  await page.getByRole('button', { name: 'Create collection' }).first().click();
  const dialog = page.locator('dialog[open]');
  await dialog.getByRole('radio', { name: 'List' }).waitFor({ timeout: 5_000 });
  for (const name of ['Like its hub', 'List', 'Tasks', 'Project', 'Flow']) {
    assert.equal(await dialog.getByRole('radio', { name, exact: true }).count(), 1, `${name} is not offered`);
  }
  assert.ok(await dialog.getByRole('radio', { name: 'Like its hub' }).isChecked(), 'following the hub is the default');
  await dialog.getByRole('textbox', { name: 'Collection name' }).fill('Pantry');
  await dialog.getByRole('radio', { name: 'List' }).check();
  await dialog.getByRole('button', { name: 'Create collection' }).click();
  await page.waitForFunction(() => document.querySelector('dialog[open]') === null, null, { timeout: 5_000 }).catch(() => {});
  assert.equal(written[0]?.preset, 'LIST');
  assert.equal(written[0]?.name, 'Pantry');
  assert.deepEqual(failures, []);
});

test('chromium: an administrator - a lock shows its value and who set it, and cannot be changed (check 7)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures, written } = await as(browser, 'ADMIN');
  t.after(() => context.close());

  await openCollection(page, SHAPES.locked);
  // A project opens with what is inside shown, its default view.
  assert.ok(await page.getByRole('checkbox', { name: 'Show what is inside' }).isChecked(), 'the project did not open as its outline');
  const panel = await openPanel(page, SHAPES.locked.name);
  const views = panel.locator('[data-setting="views"]');
  assert.equal((await views.locator('.head button').first().textContent()).trim(), 'Workspace, locked');
  assert.equal(await views.getByRole('button', { name: /^Reset / }).count(), 0, 'a locked setting offers a reset');
  // The controls stay, readable and unavailable, with the lock's reason.
  const board = views.getByRole('checkbox', { name: 'Board' });
  assert.equal(await board.getAttribute('aria-disabled'), 'true');
  assert.ok(await board.isChecked());
  await board.focus();
  assert.ok((await page.locator('[role="tooltip"]:visible, [popover]:popover-open').allTextContents()).join(' ').includes(`Locked by ${OTHER.display_name}`));
  await board.press('Space');
  assert.ok(await board.isChecked(), 'the locked box changed');
  // One field of one level: the task's dates.
  const dates = panel.locator('[data-setting="fields"]').getByRole('checkbox', { name: 'Dates' }).first();
  assert.equal(await dates.getAttribute('aria-disabled'), 'true');
  // The chip names who set the lock.
  await views.locator('.head button').first().click();
  await page.locator('[popover]:popover-open').first().waitFor({ timeout: 5_000 });
  assert.ok((await page.locator('[popover]:popover-open').first().textContent()).includes(`Locked by ${OTHER.display_name}. Ask them to change it.`));
  await page.keyboard.press('Escape');

  // A preset under the lock: sent as itself, the locked value kept and said.
  // List offers fewer levels, so it is previewed first and confirmed with the default, Hide.
  await panel.getByRole('combobox', { name: 'Preset' }).selectOption('LIST');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await page.getByRole('dialog', { name: 'Show less?' }).getByRole('button', { name: 'Hide and save' }).click();
  await page.getByText('1 locked setting kept its value.').first().waitFor({ timeout: 5_000 });
  assert.deepEqual(written, [{ preset: 'LIST', narrowing: 'HIDE' }]);
  assert.deepEqual(failures, []);
});

test('chromium: a member - the list as its settings make it, and no Structure panel (check 9)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await as(browser, 'MEMBER');
  t.after(() => context.close());

  await openCollection(page, SHAPES.list);
  await page.getByText(SHAPE_ITEMS[0].title).first().waitFor();
  const rows = await menuRows(page, SHAPES.list.name);
  assert.ok(!rows.some((row) => row.startsWith('Structure')), `offered ${rows}`);
  assert.equal(await page.getByRole('radio', { name: 'Board' }).count(), 0, 'a board is offered');
  await page.keyboard.press('Escape');

  await page.goto(`${served.origin}/hubs/${SHAPE_HUB.id}`);
  await page.getByRole('heading', { name: SHAPE_HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
  await page.waitForTimeout(600);
  // A member has nothing to do to a hub itself: no menu at all, and the people in the head's faces.
  const hubMenu = page.getByRole('button', { name: `Actions for ${SHAPE_HUB.name}` });
  const hubRows = (await hubMenu.count()) === 0 ? [] : await menuRows(page, SHAPE_HUB.name);
  assert.ok(!hubRows.some((row) => row.startsWith('Structure')), `offered ${hubRows}`);
  assert.equal(await page.locator('[data-opener="members"]').count(), 1, 'no way to the people');
  assert.deepEqual(failures, []);
});
