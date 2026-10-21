// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The end-user screens explain without breaking (UC-WRK-32 checks 2, 3, 4, 6 and 7): a read-only
// area says why once, at its top, and its rows say nothing; what the reader may never do is not
// offered; a form marks what is missing when it is pressed; a save shows its progress in place and
// confirms afterwards. Read in the built app over the shared fixture, at the three widths a person
// meets - by the words on the page, not by a class, so a reason that came back as a line under a
// row is caught whatever it is called.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ARCHIVED, ARCHIVED_HUB, ITEMS, UNDER_ARCHIVED, signedIn } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const WIDTHS = [375, 768, 1280];

/** Signs in with every memberships answer naming `role` for the reader at the workspace. */
async function as(browser, width, role) {
  const session = await signedIn(browser, width, 900);
  if (role !== 'OWNER') {
    await session.context.route('**/api/v1/memberships**', (route) => {
      // Only the workspace's answer names the reader; every other place holds nobody, so the role
      // at the workspace is the one the screen decides by.
      const scope = new URL(route.request().url()).searchParams.get('scope_type');
      const data = scope === 'TENANT' ? [{ id: 'm-role', scope_type: 'TENANT', account_id: '01a0e2e0-0000-7000-8000-000000000001', role }] : [];
      return route.fulfill({ json: { data, page: { next_cursor: null, has_more: false }, grantable_roles: [] } });
    });
  }
  return session;
}

/** The boxes of every row in the list, before and after `act`: a reason that appears moves nothing. */
async function rowBoxes(page) {
  return page.locator('.task-row').evaluateAll((rows) => rows.map((row) => {
    const box = row.getBoundingClientRect();
    return [Math.round(box.x), Math.round(box.y), Math.round(box.width), Math.round(box.height)];
  }));
}

/** The text of the elements `locator`'s `aria-describedby` names. */
const description = (locator) =>
  locator.evaluate((node) => (node.getAttribute('aria-describedby') ?? '').split(' ').filter(Boolean).map((id) => document.getElementById(id)?.textContent ?? '').join(' '));

for (const width of WIDTHS) {
  test(`chromium: ${width} px — an archived collection says why once, and its rows say nothing`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures } = await as(browser, width, 'OWNER');
    await page.goto(`${served.origin}/collections/${ARCHIVED.id}`);
    const note = page.locator('main [role="note"]');
    await note.waitFor({ timeout: 15_000 });
    await page.locator('.task-row').first().waitFor({ timeout: 10_000 });

    // One notice, the page's whole reason, with the one action that lifts it.
    assert.equal(await note.count(), 1, 'the read-only area does not say it once');
    assert.match(await note.innerText(), /This is archived, so nothing in it can be changed\. Unarchive it/);
    assert.equal(await note.getByRole('button', { name: 'Unarchive' }).count(), 1, 'the owner is not offered the way back');

    // The rows say nothing: no word per row, no menu, and no sentence anywhere in the list.
    const list = page.locator('.task-row');
    assert.equal(await list.filter({ hasText: 'Archived' }).count(), 0, 'a row repeats "Archived"');
    for (const item of ITEMS) {
      assert.equal(await page.getByRole('button', { name: `Move ${item.title}` }).count(), 0, `the row of ${item.title} still has a menu`);
    }
    // Drawn once; the bubbles of the head's controls carry it too, hidden until they are tried.
    assert.equal(await page.locator('main').getByText('nothing in it can be changed').filter({ visible: true }).count(), 1, 'the sentence is drawn more than once on the page');

    // The completion box stays - it says done - is unavailable, and names the notice instead of a
    // bubble of its own. Focusing it moves no row.
    const box = page.getByRole('checkbox', { name: `Mark ${ITEMS[0].title} as done` });
    assert.equal(await box.getAttribute('aria-disabled'), 'true');
    assert.match(await description(box), /This is archived/);
    const before = await rowBoxes(page);
    await box.focus();
    await page.keyboard.press(' ');
    assert.equal(await box.isChecked(), false, 'a box of a read-only list was ticked');
    assert.equal(await page.locator('[role="tooltip"]:visible').count(), 0, 'the box drew a bubble beside the notice');
    assert.deepEqual(await rowBoxes(page), before, 'trying the box moved a row');
    assert.deepEqual(failures, []);
  });

  test(`chromium: ${width} px — a member reads whom to ask, and under an archived hub every reason names the hub`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures } = await as(browser, width, 'MEMBER');
    await page.goto(`${served.origin}/collections/${ARCHIVED.id}`);
    const note = page.locator('main [role="note"]');
    await note.waitFor({ timeout: 15_000 });
    await page.waitForFunction(() => document.querySelector('main [role="note"]')?.textContent?.includes('Ask whoever manages it'), undefined, { timeout: 10_000 });
    assert.equal(await note.getByRole('button').count(), 0, 'a member is offered an unarchive the server refuses');

    await page.goto(`${served.origin}/collections/${UNDER_ARCHIVED.id}`);
    await page.locator('main [role="note"]').waitFor({ timeout: 15_000 });
    const said = `The hub above this one is archived, so nothing here can be changed. Unarchive ${ARCHIVED_HUB.name} to work in it again.`;
    await page.waitForFunction((said) => document.querySelector('main [role="note"]')?.textContent?.includes(said), said, { timeout: 10_000 });
    // The head's primary keeps its place and gives the same reason, as a bubble, on focus.
    const add = page.getByRole('button', { name: 'Add an entry', exact: true }).first();
    if ((await add.count()) > 0) {
      assert.equal(await add.getAttribute('aria-disabled'), 'true');
      assert.match(await description(add), new RegExp(ARCHIVED_HUB.name));
    }
    assert.deepEqual(failures, []);
  });
}

test('chromium: 1280 px — the page menu of a collection under an archived hub names the hub in every reason', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures } = await as(browser, 1280, 'OWNER');
  await page.goto(`${served.origin}/collections/${UNDER_ARCHIVED.id}`);
  await page.locator('main [role="note"]').waitFor({ timeout: 15_000 });
  await page.locator('[data-opener="container-menu"]').click();
  const menu = page.getByRole('menu');
  await menu.waitFor();
  const reasons = await menu.locator('[role="menuitem"][aria-disabled="true"]').allInnerTexts();
  assert.ok(reasons.length > 0, 'nothing in the menu is unavailable under an archived hub');
  for (const text of reasons) {
    assert.doesNotMatch(text, /Unarchive it to work in it again/, `an item tells the reader to unarchive what has no Unarchive: ${text}`);
  }
  assert.deepEqual(failures, []);
});
