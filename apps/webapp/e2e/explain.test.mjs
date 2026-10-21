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
import { ACCOUNT, ARCHIVED, ARCHIVED_ENTRY, ARCHIVED_HUB, COLLECTION, IN_ARCHIVED, ITEMS, UNDER_ARCHIVED, UNDER_ARCHIVED_ENTRY, signedIn } from './fixture.mjs';

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

for (const width of WIDTHS) {
  test(`chromium: ${width} px — an archived entry says why once; its children stay writable`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures } = await as(browser, width, 'OWNER');
    await page.goto(`${served.origin}/items/${ARCHIVED_ENTRY.id}`);
    const note = page.locator('main [role="note"]');
    await note.waitFor({ timeout: 15_000 });
    assert.match(await note.innerText(), /This entry is archived, so nothing on it can be changed\. Unarchive it/);
    assert.equal(await note.getByRole('button', { name: 'Unarchive' }).count(), 1);

    // Nothing that writes it is offered, and nothing repeats why.
    assert.equal(await page.locator('button[data-detail]').count(), 0, 'a detail row still opens an editor');
    assert.equal(await page.getByLabel('Say something').count(), 0, 'a comment can be written on an archived entry');
    assert.equal(await page.locator('main').getByText('nothing on it can be changed').filter({ visible: true }).count(), 1);
    const title = page.getByLabel('Title', { exact: false }).first();
    if ((await title.count()) > 0) assert.equal(await title.getAttribute('title'), null, 'the title still carries the reason as a tooltip');

    // The completion box says done or not done and names the notice.
    const box = page.getByRole('checkbox', { name: `Mark ${ARCHIVED_ENTRY.title} as done` });
    assert.equal(await box.getAttribute('aria-disabled'), 'true');
    assert.match(await description(box), /This entry is archived/);

    // Its child is an entry of its own and is not archived with it (ArchiveWorkItem.go).
    await page.getByText(UNDER_ARCHIVED_ENTRY.title).first().waitFor({ timeout: 10_000 });
    const child = page.getByRole('checkbox', { name: `Mark ${UNDER_ARCHIVED_ENTRY.title} as done` });
    assert.equal(await child.getAttribute('aria-disabled'), null, 'the child of an archived entry is frozen');
    assert.equal(await page.getByRole('button', { name: `Move ${UNDER_ARCHIVED_ENTRY.title}` }).count(), 1, 'the child lost its menu');
    assert.deepEqual(failures, []);
  });

  test(`chromium: ${width} px — an entry in an archived collection says the collection's reason, and a viewer reads its role's`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures } = await as(browser, width, 'OWNER');
    await page.goto(`${served.origin}/items/${IN_ARCHIVED.id}`);
    await page.waitForFunction(() => document.querySelector('main [role="note"]')?.textContent?.includes('This collection is archived'), undefined, { timeout: 15_000 });
    assert.equal(await page.locator('main [role="note"]').getByRole('button').count(), 0, 'an Unarchive is offered that the collection would refuse');

    const viewer = await as(browser, width, 'VIEWER');
    await viewer.page.goto(`${served.origin}/items/${ITEMS[0].id}`);
    await viewer.page.waitForFunction(() => document.querySelector('main [role="note"]')?.textContent?.includes('Your role can read entries here'), undefined, { timeout: 15_000 });
    // The entry's menu offers to read who is on it, not to share it.
    const menu = viewer.page.locator('[data-opener="entry-menu"]').first();
    if ((await menu.count()) > 0) {
      await menu.click();
      assert.equal(await viewer.page.getByRole('menuitem', { name: 'Share this entry' }).count(), 0, 'a viewer is offered to share');
    }
    assert.deepEqual(failures, []);
    assert.deepEqual(viewer.failures, []);
  });
}

/** A collection's template and a workspace-wide one, and one private saved view. */
const TEMPLATES = [
  { id: '01a0e2e0-0000-7000-8000-0000000007a1', scope_type: 'COLLECTION', scope_id: COLLECTION.id, name: 'Room checklist', root_type: 'TASK', nodes: [], created_at: '2026-09-01T08:00:00Z', version: 1 },
  { id: '01a0e2e0-0000-7000-8000-0000000007a2', scope_type: 'TENANT', scope_id: null, name: 'Weekly review', root_type: 'TASK', nodes: [], created_at: '2026-09-01T08:00:00Z', version: 1 },
];
const VIEWS = [
  { id: '01a0e2e0-0000-7000-8000-0000000007b1', scope_type: 'ACCOUNT', scope_id: ACCOUNT.id, name: 'Mine, due soon', layout: 'LIST_COLLAPSED', query: {}, sharing: 'PRIVATE', version: 1 },
];

/** Opens one of the collection's set-up dialogs from its page menu. */
async function openFromMenu(page, name) {
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.locator('[data-opener="container-menu"]').click();
  await page.getByRole('menuitem', { name, exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.waitFor();
  return dialog;
}

for (const [role, may] of [['OWNER', { use: true, shape: true }], ['MEMBER', { use: true, shape: false }], ['VIEWER', { use: false, shape: false }]]) {
  test(`chromium: 1280 px — templates and fields offer a ${role.toLowerCase()} only what the role allows, with no reason for the rest`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, context, failures } = await as(browser, 1280, role);
    await context.route('**/api/v1/templates**', (route) => route.fulfill({ json: { data: TEMPLATES, page: { next_cursor: null, has_more: false } } }));

    const dialog = await openFromMenu(page, 'Templates');
    await dialog.getByText('Room checklist').waitFor({ timeout: 10_000 });
    // Let the role arrive: nothing that writes is drawn before it, so wait for the answer first.
    await page.waitForTimeout(400);
    const rows = dialog.locator('li');
    assert.equal(await rows.nth(0).getByRole('button', { name: 'Use this template' }).count(), may.use ? 1 : 0);
    assert.equal(await rows.nth(0).getByRole('button', { name: 'Edit' }).count(), may.shape ? 1 : 0);
    // A workspace-wide template is changed where it was defined, which no screen here is.
    assert.equal(await rows.nth(1).getByRole('button', { name: 'Edit' }).count(), 0);
    assert.equal(await dialog.getByRole('button', { name: 'Define a template' }).count(), may.shape ? 1 : 0);
    assert.equal(await dialog.locator('[aria-disabled="true"]').count(), 0, 'a control is drawn off with a reason');

    if (may.shape) {
      // Generate is never switched off for an empty description: pressed, the field says so.
      const generate = dialog.getByRole('button', { name: /Generate from a description/ });
      if ((await generate.count()) > 0) {
        await generate.click();
        await dialog.getByRole('button', { name: 'Generate', exact: true }).click();
        await page.waitForFunction(() => document.activeElement?.getAttribute('aria-invalid') === 'true');
      }
    }
    await page.keyboard.press('Escape');

    const fields = await openFromMenu(page, 'Custom fields').catch(() => undefined);
    if (fields) {
      assert.equal(await fields.getByLabel('Key', { exact: true }).count(), may.shape ? 1 : 0, 'the define form follows the role');
      if (may.shape) {
        // Pressed with no type ticked: the group says it, and nothing was said before.
        for (const box of await fields.getByRole('checkbox').all()) if (await box.isChecked()) await box.uncheck();
        assert.equal(await fields.getByText('Choose at least one type').count(), 0, 'the missing type is announced in advance');
        await fields.getByLabel('Key', { exact: true }).fill('room');
        await fields.getByRole('button', { name: 'Define a field' }).click();
        await fields.getByText('Choose at least one type').waitFor();
        await page.waitForFunction(() => document.activeElement?.getAttribute('aria-invalid') === 'true');
      }
    }
    assert.deepEqual(failures, []);
  });
}

test('chromium: 1280 px — a saved view offers sharing only to who may share, and a way it cannot take says why in the list', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  for (const [role, mayShare] of [['OWNER', true], ['MEMBER', false]]) {
    const { page, context, failures } = await as(browser, 1280, role);
    await context.route('**/api/v1/**/views**', (route) => route.fulfill({ json: VIEWS }));
    await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
    await page.locator('[data-opener="views"]').click();
    const dialog = page.getByRole('dialog');
    await dialog.getByText('Mine, due soon').waitFor({ timeout: 10_000 });
    await page.waitForTimeout(400);
    await dialog.getByRole('button', { name: 'Edit' }).click();
    assert.equal(await dialog.getByRole('button', { name: 'Save who sees it' }).count(), mayShare ? 1 : 0, `${role}: sharing offered against the role`);
    assert.equal(await dialog.getByText('A public link is not offered').filter({ visible: true }).count(), 0, 'the refusal is a line under the field');
    if (mayShare) {
      const option = dialog.getByLabel('Who sees it').locator('option', { hasText: 'Anyone with a link' });
      assert.match(await option.textContent(), /— A public link is not offered/);
      assert.equal(await option.getAttribute('disabled'), '');
    }
    assert.deepEqual(failures, []);
    await context.close();
  }
});
