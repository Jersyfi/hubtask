// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// One palette for everything (UC-WRK-27), walked against the shared fixture:
//
// * check 1 - `⌘K`, `Ctrl K` and the bar's field open it on every framed page; on a phone the
//   shortcut and a tap on the bottom bar's *Search* open it in a dialog;
// * check 2 - an action the role refuses is absent: a viewer is offered no *Add an entry*, no
//   *Select entries* and no *New task in Kitchen*, a member is offered all three;
// * check 3 - a listed action shows its key, and `?` opens the list of every shortcut;
// * check 5 - by keyboard alone: arrows and Enter perform the highlighted row, Escape closes and
//   returns focus where it was; an action that opens a dialog hands focus into it and back.
//
// Check 4, the quick narrowings, is walked by `search.test.mjs` through the same field. Chromium;
// the keyboard sequence of check 5 also runs in all three engines in `engines.test.mjs`.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, COLLECTION, ITEMS, MANIFEST, PAGE, PERMISSIONS, OWNER_GRANTS, signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';
import { SHORTCUTS } from '../src/lib/shortcuts.ts';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const FIELD = 'Search everything';

/** The palette's list. */
const listOf = (page) => page.locator('#search-menu');

/** What the list offers, as the rows' visible names. */
async function rowNames(page) {
  return listOf(page).getByRole('option').evaluateAll((options) => options.map((option) => option.querySelector('.title')?.textContent?.trim() ?? option.textContent?.trim() ?? ''));
}

/** Presses ↓ until the highlighted row's name is `name`; fails after a full turn. */
async function arrowTo(page, name) {
  const field = page.getByRole('combobox', { name: FIELD });
  for (let presses = 0; presses < 40; presses += 1) {
    await page.keyboard.press('ArrowDown');
    const active = await field.getAttribute('aria-activedescendant');
    if (active && (await page.locator(`[id="${active}"] .title`).textContent())?.trim() === name) return;
  }
  assert.fail(`no row named ${name}`);
}

/** A signed-in tab whose reader holds `role` on the workspace and nothing below it. */
async function as(browser, role) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: PERMISSIONS[role] } });
    if (path === '/memberships') {
      const scope = url.searchParams.get('scope_type');
      const rows = scope === 'TENANT' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role }] : [];
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
  return { context, page, failures };
}

async function onKitchen(page) {
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.getByText(ITEMS[2].title).first().waitFor();
  // The role arrives with the memberships; give the head the moment to draw it.
  await page.waitForTimeout(600);
}

test('chromium: ⌘K, Ctrl K and the field open the palette on every framed page (check 1)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280);
  t.after(() => context.close());

  const pages = [`/collections/${COLLECTION.id}`, `/items/${ITEMS[0].id}`, '/search', '/profile', '/administration/workspace'];
  for (const path of pages) {
    for (const chord of ['Meta+k', 'Control+k']) {
      await page.goto(`${served.origin}${path}`);
      await page.getByRole('main').waitFor();
      await page.waitForTimeout(300);
      await page.locator('body').click({ position: { x: 1, y: 300 } });
      await page.keyboard.press(chord);
      await listOf(page).waitFor({ timeout: 5_000 });
      const focused = await page.evaluate(() => document.activeElement?.getAttribute('role'));
      assert.equal(focused, 'combobox', `${chord} on ${path}: focus is not in the field`);
      // ⌘K closes it again from its own field; Ctrl K there is the field's key, so Escape.
      await page.keyboard.press(chord === 'Meta+k' ? chord : 'Escape');
      await listOf(page).waitFor({ state: 'detached', timeout: 5_000 });
    }
  }

  await page.getByRole('combobox', { name: FIELD }).click();
  await listOf(page).waitFor();
  assert.deepEqual(failures, []);
});

test('chromium: 375 px — the shortcut and a tap on Search open the palette in a dialog (check 1)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 375, 800, { hasTouch: true });
  t.after(() => context.close());

  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.getByText(ITEMS[2].title).first().waitFor();

  await page.keyboard.press('Control+k');
  const dialog = page.getByRole('dialog', { name: 'Search or run a command' });
  await dialog.waitFor();
  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'hidden' });

  await page.getByRole('navigation').getByRole('link', { name: 'Search' }).tap();
  await dialog.waitFor();
  assert.equal(await page.evaluate(() => document.activeElement?.getAttribute('role')), 'combobox');
  await dialog.getByRole('option', { name: /Open search/ }).tap();
  await page.waitForURL(/\/search$/);
  assert.deepEqual(failures, []);
});

test('chromium: an action the role refuses is absent (check 2)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  const offered = {};
  for (const role of ['MEMBER', 'VIEWER']) {
    const { context, page, failures } = await as(browser, role);
    t.after(() => context.close());
    await onKitchen(page);
    await page.getByRole('combobox', { name: FIELD }).click();
    await listOf(page).waitFor();
    const here = await rowNames(page);
    await page.getByRole('combobox', { name: FIELD }).fill('kitch');
    await page.getByRole('option', { name: /Search in Kitchen/ }).waitFor();
    await page.waitForTimeout(400);
    offered[role] = { here, typed: await rowNames(page) };
    assert.deepEqual(failures, []);
  }

  assert.ok(offered.MEMBER.here.includes('Add an entry'), 'a member is offered the add');
  assert.ok(offered.MEMBER.here.includes('Select entries'), 'a member is offered the selection');
  assert.ok(offered.MEMBER.typed.includes('New task in Kitchen'), 'a member is offered a new task by name');
  assert.ok(!offered.VIEWER.here.includes('Add an entry'), 'a viewer was offered the add');
  assert.ok(!offered.VIEWER.here.includes('Select entries'), 'a viewer was offered the selection');
  assert.ok(!offered.VIEWER.typed.includes('New task in Kitchen'), 'a viewer was offered a new task');
  assert.ok(offered.VIEWER.typed.includes('Kitchen'), 'a viewer still finds the collection');
  assert.ok(!offered.VIEWER.typed.some((name) => /administration/i.test(name)), 'a viewer was offered the administration');
});

test('chromium: a listed action shows its key, and ? lists every shortcut (check 3)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280);
  t.after(() => context.close());

  await onKitchen(page);
  await page.getByRole('combobox', { name: FIELD }).fill('help');
  const row = page.getByRole('option', { name: /Help and shortcuts/ });
  await row.waitFor();
  assert.equal(await row.getAttribute('aria-keyshortcuts'), 'Shift+?');
  assert.equal((await row.locator('kbd').textContent())?.trim(), '?');
  await page.keyboard.press('Escape');

  await page.locator('body').click({ position: { x: 1, y: 300 } });
  await page.keyboard.press('Shift+?');
  const help = page.getByRole('dialog', { name: 'Help and shortcuts' });
  await help.waitFor();
  const text = await help.innerText();
  for (const keys of ['⌘K', 'Ctrl K', '/', '?', 'Alt ↑', 'Alt Shift ↑', 'Ctrl click', 'Shift click', 'Esc']) {
    assert.ok(text.includes(keys), `the list does not show ${keys}`);
  }
  assert.ok(text.includes('Take the tour again'));
  assert.deepEqual(failures, []);
});

test('chromium: the rank keys the list shows are the keys that rank (check 3)', async (t) => {
  // The list is read from `lib/shortcuts.ts`; the keys are handled by the design system's
  // `rankIntent` on the list. Pressing every listed chord on a row in the middle and counting the
  // reorders it sends ties the two together - a key that drifted would be a key that sends nothing.
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280);
  t.after(() => context.close());
  const reorders = [];
  page.on('request', (request) => {
    if (request.method() === 'POST' && request.url().endsWith(':reorder')) reorders.push(request.url());
  });

  await onKitchen(page);
  const chords = SHORTCUTS.filter((each) => each.id.startsWith('rank-')).flatMap((each) => each.aria);
  assert.equal(chords.length, 4);
  for (const chord of chords) {
    await page.getByRole('checkbox', { name: `Mark ${ITEMS[2].title} as done` }).focus();
    const before = reorders.length;
    await page.keyboard.press(chord);
    await page.waitForTimeout(400);
    assert.equal(reorders.length, before + 1, `${chord} ranked nothing`);
  }
  assert.deepEqual(failures, []);
});

test('chromium: by keyboard alone, Enter performs the highlighted row and Escape returns focus (check 5)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280);
  t.after(() => context.close());

  await onKitchen(page);
  const checkbox = page.getByRole('checkbox', { name: `Mark ${ITEMS[2].title} as done` });
  await checkbox.focus();

  await page.keyboard.press('Control+k');
  await page.keyboard.type('kitch');
  await page.getByRole('option', { name: /Open Kitchen as a board/ }).waitFor();
  await arrowTo(page, 'Open Kitchen as a board');
  await page.keyboard.press('Enter');
  await page.getByRole('radio', { name: 'Board', checked: true }).waitFor();

  // Escape closes and returns focus to where it was.
  await page.getByRole('radio', { name: 'Board' }).focus();
  await page.keyboard.press('Control+k');
  await listOf(page).waitFor();
  await page.keyboard.press('Escape');
  await listOf(page).waitFor({ state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement?.getAttribute('aria-label') ?? document.activeElement?.textContent?.trim()), await page.getByRole('radio', { name: 'Board' }).evaluate((node) => node.getAttribute('aria-label') ?? node.textContent?.trim()));

  // Opened in the field, Escape closes and keeps focus there; a second empties the field.
  const field = page.getByRole('combobox', { name: FIELD });
  await field.click();
  await page.keyboard.type('milk');
  await listOf(page).waitFor();
  await page.keyboard.press('Escape');
  await listOf(page).waitFor({ state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement?.getAttribute('role')), 'combobox');
  assert.equal(await field.inputValue(), 'milk');
  await page.keyboard.press('Escape');
  assert.equal(await field.inputValue(), '');
  assert.deepEqual(failures, []);
});

test('chromium: 375 px — an action that opens a dialog takes focus into it and gives it back (check 5)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 375);
  t.after(() => context.close());

  await onKitchen(page);
  const checkbox = page.getByRole('checkbox', { name: `Mark ${ITEMS[2].title} as done` });
  await checkbox.focus();
  await page.keyboard.press('Control+k');
  await page.getByRole('dialog', { name: 'Search or run a command' }).waitFor();
  await page.keyboard.type('people');
  await arrowTo(page, 'People');
  await page.keyboard.press('Enter');

  const people = page.getByRole('dialog').filter({ hasNotText: 'Search or run a command' }).last();
  await people.waitFor();
  await page.waitForTimeout(200);
  assert.ok(await people.evaluate((node) => node.contains(document.activeElement)), 'focus is not inside the dialog the action opened');
  await page.keyboard.press('Escape');
  await page.waitForTimeout(200);
  assert.ok(await checkbox.evaluate((node) => node === document.activeElement), 'focus did not come back to where it was');
  assert.deepEqual(failures, []);
});
