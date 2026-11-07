// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// An entry carried from a list onto a collection in the sidebar (UC-WRK-06 check 8, the navigation
// concept §5.1): within the hub it moves at once, the toast offers Undo, and Undo carries it back;
// into another hub's collection, reached through *Starred*, only after a dialog that says what stays
// behind; a collection the reader may not write in is not outlined, takes nothing and says why.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, COLLECTION, GROCERIES, GROCERY_ITEMS, GROCERY_LABELS, HUB, ITEMS, signedIn } from './fixture.mjs';

const served = await serve(join(dirname(fileURLToPath(import.meta.url)), '..', 'dist'));
test.after(() => served.close());

test('UC-WRK-06 check 8: an entry dragged from the list onto a collection of the sidebar moves there, with Undo', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await signedIn(browser, 1440, 900);
  t.after(() => context.close());
  const entry = ITEMS[0];
  // The hub's two collections, as its listing answers them.
  await page.route(new RegExp(`/api/v1/containers\\?.*parent_id=${COLLECTION.parent_id}`), (route) => route.fulfill({
    json: { data: [COLLECTION, GROCERIES], page: { next_cursor: null, has_more: false } },
  }));
  const moves = [];
  await page.route(new RegExp(`/api/v1/items/${entry.id}:move$`), (route) => {
    const body = route.request().postDataJSON();
    moves.push(body);
    return route.fulfill({ json: { item: { ...entry, collection_id: body.target_collection_id, version: entry.version + moves.length }, dropped_references: [] } });
  });
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  const row = page.locator(`[data-row="${entry.id}"]`);
  await row.waitFor({ timeout: 15_000 });
  const target = page.locator('aside.sidenav [data-entry-target] [data-id]', { hasText: GROCERIES.name }).first();
  await target.waitFor({ timeout: 10_000 });

  await row.hover();
  const grip = await row.locator('[data-grip]').boundingBox();
  const onto = await target.boundingBox();
  await page.mouse.move(grip.x + grip.width / 2, grip.y + grip.height / 2);
  await page.mouse.down();
  await page.mouse.move(grip.x + grip.width / 2, grip.y + grip.height / 2 + 8, { steps: 3 });
  await page.mouse.move(onto.x + onto.width / 2, onto.y + onto.height / 2, { steps: 12 });
  // Before letting go the collection is outlined as the place the entry would go.
  await page.waitForFunction((name) => [...document.querySelectorAll('aside.sidenav [data-entry-target] [data-id]')].some((each) => each.textContent.includes(name) && each.matches('[data-drop="inside"]')), GROCERIES.name, { timeout: 5_000 })
    .catch(() => assert.fail('the collection is not outlined while the entry is over it'));
  await page.mouse.up();

  await page.getByRole('button', { name: 'Undo' }).waitFor({ timeout: 5_000 });
  assert.equal(moves.length, 1, JSON.stringify(moves));
  assert.equal(moves[0].target_collection_id, GROCERIES.id, JSON.stringify(moves[0]));
  assert.ok((await page.locator('[role="status"]').allInnerTexts()).some((each) => each.includes(`${entry.title} moved to ${GROCERIES.name}.`)));
  await page.getByRole('button', { name: 'Undo' }).click();
  await page.waitForFunction(() => document.body.innerText.includes('Undone.'), undefined, { timeout: 5_000 });
  assert.equal(moves.length, 2);
  assert.equal(moves[1].target_collection_id, COLLECTION.id);
  assert.deepEqual(failures, []);
});

const hub = (n, name) => ({ id: `01a0e2e0-0000-7000-8000-0000000009${n}0`, type: 'HUB', parent_id: null, name, order_key: `b${n}`, version: 1 });
const GARDEN = hub(1, 'Garden');
const LOFT = hub(2, 'Loft');
const BEDS = { id: '01a0e2e0-0000-7000-8000-000000000911', type: 'COLLECTION', parent_id: GARDEN.id, name: 'Beds', order_key: 'a0', version: 1 };
const SHELF = { id: '01a0e2e0-0000-7000-8000-000000000921', type: 'COLLECTION', parent_id: LOFT.id, name: 'Shelf', order_key: 'a0', version: 1 };

/** Carries the entry's grip onto a sidebar row and holds it there; `drop` lets go. */
async function carry(page, row, target) {
  await row.hover();
  const grip = await row.locator('[data-grip]').boundingBox();
  const onto = await target.boundingBox();
  await page.mouse.move(grip.x + grip.width / 2, grip.y + grip.height / 2);
  await page.mouse.down();
  await page.mouse.move(grip.x + grip.width / 2, grip.y + grip.height / 2 + 8, { steps: 3 });
  await page.mouse.move(onto.x + onto.width / 2, onto.y + onto.height / 2, { steps: 12 });
  await page.waitForTimeout(200);
}

test('UC-WRK-06 check 8: onto another hub\'s starred collection after a dialog; onto one the reader may not write in, nothing', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await signedIn(browser, 1440, 900);
  t.after(() => context.close());
  const entry = ITEMS[0];
  const page0 = { next_cursor: null, has_more: false };
  // Two more hubs: the reader owns Garden, and holds nothing in the Loft beyond the workspace's
  // viewer - so the Loft's Shelf takes no entry. Both collections are starred.
  await page.route(/\/api\/v1\/containers\?/, (route) => {
    const url = new URL(route.request().url());
    if (url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { data: [HUB, GARDEN, LOFT], page: page0 } });
    const parent = url.searchParams.get('parent_id');
    const data = parent === HUB.id ? [COLLECTION, GROCERIES] : parent === GARDEN.id ? [BEDS] : parent === LOFT.id ? [SHELF] : undefined;
    return data ? route.fulfill({ json: { data, page: page0 } }) : route.fallback();
  });
  // The local copy holds every collection the reader sees, the other hubs' too.
  await page.route(/\/api\/v1\/sync:snapshot$/, (route) => {
    const records = [
      ...[HUB, COLLECTION, GROCERIES, GARDEN, BEDS, LOFT, SHELF].map((c) => ({ op: 'UPSERT', entity: 'container', entity_id: c.id, container_id: c.parent_id, payload: c })),
      ...GROCERY_LABELS.map((label) => ({ op: 'UPSERT', entity: 'label', entity_id: label.id, container_id: GROCERIES.id, payload: { ...label, collection_id: GROCERIES.id } })),
      ...GROCERY_ITEMS.map((each) => ({ op: 'UPSERT', entity: 'item', entity_id: each.id, container_id: GROCERIES.id, payload: each })),
    ].map((record) => JSON.stringify(record));
    return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${records.join('\n')}\n{"cursor":"c-e2e"}\n` });
  });
  await page.route(/\/api\/v1\/accounts\/[^/]+\/stars$/, (route) => route.fulfill({ json: { stars: [{ kind: 'COLLECTION', id: SHELF.id }, { kind: 'COLLECTION', id: BEDS.id }], version: 1 } }));
  await page.route(/\/api\/v1\/memberships\?/, (route) => {
    const url = new URL(route.request().url());
    const scope = url.searchParams.get('scope_type');
    const at = url.searchParams.get('scope_id');
    const own = (role) => [{ id: `m-${scope}-${at}`, scope_type: scope, scope_id: at, account_id: ACCOUNT.id, role }];
    const data = scope === 'TENANT' ? own('VIEWER') : scope === 'HUB' && (at === HUB.id || at === GARDEN.id) ? own('OWNER') : [];
    return route.fulfill({ json: { data, page: page0 } });
  });
  const moves = [];
  await page.route(new RegExp(`/api/v1/items/${entry.id}:move$`), (route) => {
    const body = route.request().postDataJSON();
    moves.push(body);
    return route.fulfill({ json: { item: { ...entry, collection_id: body.target_collection_id, version: entry.version + moves.length }, dropped_references: [] } });
  });
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  const row = page.locator(`[data-row="${entry.id}"]`);
  await row.waitFor({ timeout: 15_000 });
  // *‹ All hubs* shows the workspace level beside the list, with *Starred*.
  await page.locator('aside.sidenav').getByRole('button', { name: 'All hubs' }).click();
  const starred = (name) => page.locator('aside.sidenav [data-entry-target] [data-id]', { hasText: name }).first();
  await starred(SHELF.name).waitFor({ timeout: 10_000 });

  // The Shelf: not outlined while the entry is over it, and the drop says why and sends nothing.
  await carry(page, row, starred(SHELF.name));
  assert.equal(await starred(SHELF.name).getAttribute('data-drop'), null, 'a collection the reader may not write in is outlined');
  await page.mouse.up();
  await page.waitForFunction((words) => [...document.querySelectorAll('[aria-live]')].some((each) => each.textContent.includes(words)), `You may not write in ${SHELF.name}.`, { timeout: 5_000 });
  assert.equal(moves.length, 0);
  assert.equal(await page.getByRole('dialog').count(), 0);

  // The Beds, in another hub: outlined, and the drop asks first.
  await carry(page, row, starred(BEDS.name));
  await page.waitForFunction((name) => [...document.querySelectorAll('aside.sidenav [data-entry-target] [data-id]')].some((each) => each.textContent.includes(name) && each.matches('[data-drop="inside"]')), BEDS.name, { timeout: 5_000 });
  await page.mouse.up();
  const dialog = page.getByRole('dialog', { name: `Move “${entry.title}” to ${BEDS.name}?` });
  await dialog.waitFor({ timeout: 5_000 });
  assert.ok((await dialog.innerText()).includes(`${BEDS.name} is in ${GARDEN.name}.`), await dialog.innerText());
  assert.equal(moves.length, 0, 'it moved before the dialog was answered');
  await dialog.getByRole('button', { name: 'Move', exact: true }).click();
  await page.getByRole('button', { name: 'Undo' }).waitFor({ timeout: 5_000 });
  assert.equal(moves.length, 1);
  assert.equal(moves[0].target_collection_id, BEDS.id);
  assert.deepEqual(failures, []);
});
