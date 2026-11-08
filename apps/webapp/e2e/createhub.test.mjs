// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Which hub a reader is offered to create (UC-WRK-01 check 6, UC-WRK-22 check 4, UC-ID-16 check 1),
// walked per reader against the manifest the server answers:
//
// * whoever may shape the workspace (`STRUCTURE`) is offered *Create hub*, and *Private* in its
//   dialog only where the workspace allows private hubs;
// * anybody else is offered *Create private hub* where the manifest says creating one would
//   succeed (`private_hub_offered`) - a dialog that makes a private hub and nothing else - and no
//   hub at all where it would not.
//
// In the overview's head and in the navigation. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { HUB, MANIFEST, PERMISSIONS, WORKSPACE, chooseNew, newRows, signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

/**
 * A signed-in tab whose reader holds `permissions` on the workspace and is offered a hub of their
 * own as `offered` says; `allowed` is the workspace's *Private hubs allowed* as `/tenant` answers it.
 * Records what a create sends.
 */
async function as(browser, { permissions, offered, allowed = true }) {
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  const sent = [];
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, workspace_permissions: permissions, private_hub_offered: offered } });
    }
    if (path === '/tenant' && request.method() === 'GET') {
      return route.fulfill({ json: { ...WORKSPACE, private_hubs_offered: allowed } });
    }
    if (path === '/containers' && request.method() === 'POST') {
      const body = request.postDataJSON();
      sent.push(body);
      return route.fulfill({ status: 201, json: { ...HUB, id: '01a0e2e0-0000-7000-8000-0000000018c1', name: body.name, private: body.private === true } });
    }
    return stub(route);
  });
  return { page, failures, context, sent };
}

/** What the sidebar's *Hubs* section offers to create, once the tree is drawn. */
async function offered(page) {
  await page.getByRole('tree').first().waitFor({ timeout: 10_000 });
  await page.waitForTimeout(600);
  const rows = await newRows(page);
  if (rows.length > 0) await page.keyboard.press('Escape');
  return rows;
}

test('chromium: a member is offered a private hub of their own and no shared hub, and the dialog makes it private', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, sent } = await as(browser, { permissions: PERMISSIONS.MEMBER, offered: true });
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  // The ＋ on the sidebar's *Hubs*, alone: *Today* carries no head action of its own (§E9), and a
  // member is offered no shared hub the server refuses.
  assert.deepEqual(await offered(page), ['Create private hub']);

  await chooseNew(page, 'Create private hub');
  const dialog = page.locator('dialog[open]');
  await dialog.waitFor({ timeout: 10_000 });
  assert.equal(await dialog.getByRole('heading', { name: 'Create private hub' }).count(), 1, 'the dialog does not say what it makes');
  assert.equal(await dialog.getByRole('switch', { name: 'Private' }).count(), 0, 'a switch offers a shared hub');
  assert.equal(await dialog.getByText('Only the people you give a role in it can open it.', { exact: false }).count(), 1, 'the dialog does not say the hub is private');
  await dialog.getByRole('textbox', { name: 'Hub name' }).fill('Diary');
  await dialog.getByRole('button', { name: 'Create private hub' }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 10_000 });
  assert.equal(sent.length, 1);
  assert.equal(sent[0].private, true, `the body was ${JSON.stringify(sent[0])}`);
  assert.deepEqual(failures, []);
});

test('chromium: where private hubs are not offered, a viewer is offered no hub at all', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await as(browser, { permissions: PERMISSIONS.VIEWER, offered: false, allowed: false });
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  assert.deepEqual(await offered(page), [], 'a hub offered to somebody the server refuses one');
  assert.deepEqual(failures, []);
});

test('chromium: whoever shapes the workspace where private hubs are switched off is offered a shared hub only', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, sent } = await as(browser, { permissions: PERMISSIONS.ADMIN, offered: false, allowed: false });
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  assert.deepEqual(await offered(page), ['Create hub'], 'not a shared hub alone for whoever may shape the workspace');

  await chooseNew(page, 'Create hub');
  const dialog = page.locator('dialog[open]');
  await dialog.waitFor({ timeout: 10_000 });
  await dialog.getByRole('textbox', { name: 'Hub name' }).fill('Garden');
  // The workspace's answer arrives with the dialog; give it the moment to draw a switch it should not.
  await page.waitForTimeout(400);
  assert.equal(await dialog.getByRole('switch', { name: 'Private' }).count(), 0, '*Private* offered where private hubs are off');
  await dialog.getByRole('button', { name: 'Create hub' }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 10_000 });
  assert.equal(sent.length, 1);
  assert.equal(sent[0].private, undefined, `the body was ${JSON.stringify(sent[0])}`);
  assert.deepEqual(failures, []);
});

// The navigation concept's grammar (§5): ＋ on a section head creates at its top level - on the hub
// level's *Collections*, a collection in that hub.
test('chromium: inside a hub, ＋ on Collections makes a collection in that hub, with its preset (ux-inventory.md §E1)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, sent } = await as(browser, { permissions: PERMISSIONS.ADMIN, offered: false, allowed: false });
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/${HUB.id}`);
  const level = page.locator('aside.sidenav').getByRole('region', { name: HUB.name });
  const make = level.getByRole('button', { name: 'New collection', exact: true });
  await make.waitFor({ timeout: 10_000 });
  await make.click();
  const dialog = page.locator('dialog[open]');
  await dialog.waitFor({ timeout: 10_000 });
  assert.equal(await dialog.getByRole('heading', { name: `New collection in ${HUB.name}` }).count(), 1, 'the dialog does not make a collection');
  // The kinds as cards (the navigation concept's *New collection*).
  await dialog.locator('[data-collection-kind]').first().waitFor({ timeout: 10_000 });
  assert.ok((await dialog.locator('[data-collection-kind]').count()) > 0, 'the dialog offers no preset');
  await dialog.getByRole('textbox').first().fill('Groceries');
  await dialog.getByRole('button', { name: 'Create', exact: true }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 10_000 });
  assert.equal(sent.length, 1);
  assert.equal(sent[0].type, 'COLLECTION', `the body was ${JSON.stringify(sent[0])}`);
  assert.equal(sent[0].parent_id, HUB.id, `the body was ${JSON.stringify(sent[0])}`);
  assert.deepEqual(failures, []);
});

// §5.2 and UX-4: *Create hub* asks the name, then shows the template gallery - a family filter above
// cards side by side - and folds the description, colour and symbol below it, so the templates are
// in view when the dialog opens.
test('chromium: the hub dialog shows the gallery under the name, filtered by family, the look folded', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await as(browser, { permissions: PERMISSIONS.ADMIN, offered: false, allowed: false });
  t.after(() => context.close());
  const template = (key) => ({
    key, version: 1, name: key, origin: 'HUBTASK', provenance: `${key}@1`, created_at: '2026-09-01T00:00:00Z',
    update_available: false, taken_version: null, hub_document: { collections: {} }, document: { modules: { workflows: { switch: 'ON' } } },
  });
  const keys = ['kanban_team', 'scrum_team', 'bug_tracking', 'classic_project', 'hybrid_project', 'itil_service_desk', 'internal_it', 'approvals'];
  await context.route('**/api/v1/config/templates*', (route) => route.fulfill({ json: { data: keys.map(template) } }));

  await page.goto(`${served.origin}/`);
  await offered(page);
  await chooseNew(page, 'Create hub');
  const dialog = page.locator('dialog[open]');
  const gallery = dialog.locator('[data-hub-gallery]');
  await gallery.locator('[data-hub-template="kanban_team"]').waitFor({ timeout: 10_000 });
  // The gallery comes right after the name; the look waits folded behind one press.
  const order = await dialog.evaluate((element) => {
    const name = element.querySelector('input');
    const cards = element.querySelector('[data-hub-gallery]');
    const symbol = element.querySelector('[data-look-fields]');
    return {
      galleryAfterName: Boolean(name && cards && name.compareDocumentPosition(cards) & Node.DOCUMENT_POSITION_FOLLOWING),
      lookShown: Boolean(symbol && symbol.getClientRects().length > 0),
    };
  });
  assert.ok(order.galleryAfterName, 'the gallery is not under the name');
  assert.equal(order.lookShown, false, 'the colours and symbols are not folded');
  const first = await gallery.locator('[data-hub-template="kanban_team"]').boundingBox();
  assert.ok(first && first.y < 900, 'the first template is below the fold');
  const second = await gallery.locator('[data-hub-template="scrum_team"]').boundingBox();
  assert.ok(second && Math.abs(second.y - first.y) < 2, 'the templates stand in one column');
  // The family filter narrows the gallery to one family.
  const filter = gallery.getByRole('group', { name: 'Families' });
  await filter.getByRole('button', { name: 'Service', exact: true }).click();
  assert.equal(await gallery.locator('[data-hub-template="kanban_team"]').count(), 0, 'the filter left another family');
  assert.equal(await gallery.locator('[data-hub-template="itil_service_desk"]').count(), 1);
  await filter.getByRole('button', { name: 'All', exact: true }).click();
  assert.equal(await gallery.locator('[data-hub-template="kanban_team"]').count(), 1);
  await dialog.getByRole('button', { name: /Description, colour and symbol/u }).click();
  await dialog.locator('[data-look-fields]').waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
});
