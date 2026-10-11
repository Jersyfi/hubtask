// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Private hubs in the web app (UC-ID-16): *Private* offered when a hub is made where the workspace
// has more than one person and not where it has one (check 8); a hub made private from its own
// menu, with a confirmation that names who loses it, and shared again; the lock on a private hub.
// Chromium only, as the shell walk.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, HUB, MANIFEST, OTHER, WORKSPACE, signedIn } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

/** Opens the create-hub dialog from the overview's head. */
async function openCreate(page) {
  await page.goto(`${served.origin}/`);
  await page.getByRole('button', { name: 'Create hub' }).first().click();
  const dialog = page.locator('dialog[open]');
  await dialog.waitFor({ timeout: 10_000 });
  return dialog;
}

test('chromium: *Private* is offered where the workspace has more than one person, and sent', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  let created;
  await context.route('**/api/v1/containers', (route) => {
    if (route.request().method() !== 'POST') return route.fallback();
    created = route.request().postDataJSON();
    return route.fulfill({ status: 201, json: { ...HUB, id: '01a0e2e0-0000-7000-8000-0000000005c1', name: created.name, private: created.private === true } });
  });

  const dialog = await openCreate(page);
  const toggle = dialog.getByRole('switch', { name: 'Private' });
  await toggle.waitFor({ timeout: 10_000 });
  await dialog.getByRole('textbox', { name: 'Hub name' }).fill('Diary');
  await toggle.check();
  await dialog.getByRole('button', { name: 'Create hub' }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 10_000 });
  assert.equal(created?.private, true, `the body was ${JSON.stringify(created)}`);
  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed().filter((each) => !each.includes('/containers/01a0e2e0-0000-7000-8000-0000000005c1')), []);
});

test('chromium: a workspace of one is not offered *Private*', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  await context.route('**/api/v1/tenant', (route) => route.fulfill({ json: { ...WORKSPACE, private_hubs_offered: false } }));

  const dialog = await openCreate(page);
  await dialog.getByRole('textbox', { name: 'Hub name' }).waitFor({ timeout: 10_000 });
  // The read has answered when the dialog has: give it the moment the workspace takes to arrive.
  await page.waitForTimeout(500);
  assert.equal(await dialog.getByRole('switch', { name: 'Private' }).count(), 0);
  await page.keyboard.press('Escape');
  await page.goto(`${served.origin}/hubs/${HUB.id}`);
  await page.getByRole('heading', { name: HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
  await page.getByRole('button', { name: `Actions for ${HUB.name}` }).click();
  assert.equal(await page.getByRole('menuitem', { name: /Make private/ }).count(), 0, 'nothing to keep it from');
  assert.deepEqual(failures, []);
});

test('chromium: the owner of the hub makes it private, told whom it shuts out, and shares it again', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  let isPrivate = false;
  const sent = [];
  // The hub's level and the hub itself answer what was last written, as the server would.
  await context.route((url) => url.pathname.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB', (route) =>
    route.fulfill({ json: { data: [{ ...HUB, private: isPrivate, version: HUB.version + sent.length }], items: [], page: { next_cursor: null, has_more: false } } }));
  await context.route(`**/api/v1/containers/${HUB.id}`, (route) => route.fulfill({ json: { ...HUB, private: isPrivate, version: HUB.version + sent.length } }));
  await context.route(`**/api/v1/containers/${HUB.id}/privacy`, (route) => {
    const body = route.request().postDataJSON();
    sent.push(body.private);
    isPrivate = body.private;
    return route.fulfill({ json: { ...HUB, private: isPrivate, version: HUB.version + sent.length } });
  });

  await page.goto(`${served.origin}/hubs/${HUB.id}`);
  await page.getByRole('heading', { name: HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
  await page.getByRole('button', { name: `Actions for ${HUB.name}` }).click();
  await page.getByRole('menuitem', { name: 'Make private' }).click();
  const dialog = page.locator('dialog[open]');
  await dialog.waitFor({ timeout: 5_000 });
  assert.ok((await dialog.getByRole('heading').first().textContent()).includes(`Make ${HUB.name} private?`));
  // Mara holds a role on the workspace and nothing on the hub: she is the one who loses it.
  await dialog.getByText(`Who loses it: ${OTHER.display_name}.`).waitFor({ timeout: 5_000 });
  await dialog.getByRole('button', { name: 'Make private' }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
  await page.getByText('Private — only the people who hold a role in it can open it.').waitFor({ timeout: 10_000 });

  await page.getByRole('button', { name: `Actions for ${HUB.name}` }).click();
  await page.getByRole('menuitem', { name: 'Share with the workspace' }).click();
  await page.locator('dialog[open]').getByRole('button', { name: 'Share with the workspace' }).click();
  await page.locator('dialog[open]').waitFor({ state: 'hidden', timeout: 5_000 });
  assert.deepEqual(sent, [true, false]);
  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

test('chromium: an owner of the hub without the right to create hubs is offered sharing only (P-05)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  let isPrivate = false;
  // A member of the workspace, the hub's owner: MEMBER carries no STRUCTURE in the matrix.
  await context.route('**/api/v1/meta/capabilities', (route) => route.fulfill({ json: {
    ...MANIFEST, roles: [...MANIFEST.roles, { role: 'MEMBER', permissions: ['READ', 'WRITE_ITEMS'] }],
  } }));
  await context.route((url) => url.pathname.endsWith('/api/v1/memberships') && url.searchParams.get('scope_type') === 'TENANT', (route) =>
    route.fulfill({ json: { data: [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'MEMBER' }], items: [], page: { next_cursor: null, has_more: false } } }));
  await context.route((url) => url.pathname.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB', (route) =>
    route.fulfill({ json: { data: [{ ...HUB, private: isPrivate }], items: [], page: { next_cursor: null, has_more: false } } }));
  await context.route(`**/api/v1/containers/${HUB.id}`, (route) => route.fulfill({ json: { ...HUB, private: isPrivate } }));

  const menuOf = async () => {
    await page.goto(`${served.origin}/hubs/${HUB.id}`);
    await page.getByRole('heading', { name: HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
    await page.waitForTimeout(500);
    await page.getByRole('button', { name: `Actions for ${HUB.name}` }).click();
    await page.getByRole('menu').waitFor({ timeout: 5_000 });
    return page.getByRole('menuitem').allTextContents();
  };
  const shared = await menuOf();
  assert.ok(!shared.some((each) => each.includes('Make private')), `offered ${shared}`);
  isPrivate = true;
  const owned = await menuOf();
  assert.ok(owned.some((each) => each.includes('Share with the workspace')), `offered ${owned}`);
  assert.deepEqual(failures, []);
});

test('chromium: the administration lists the private hubs - whose and how big, never a name', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());

  await page.goto(`${served.origin}/administration/workspace`);
  const nav = page.getByRole('navigation', { name: 'Administration', exact: true });
  await nav.getByRole('treeitem', { name: 'Private hubs', exact: true }).click();
  const table = page.getByRole('table', { name: 'Private hubs' });
  await table.waitFor({ timeout: 10_000 });
  const rows = table.getByRole('row');
  // The head and the two hubs of the fixture: one of Mara's, one nobody is left in.
  assert.equal(await rows.count(), 3);
  await table.getByRole('rowheader', { name: OTHER.display_name }).waitFor({ timeout: 5_000 });
  await table.getByRole('rowheader', { name: 'Nobody — its last member left' }).waitFor();
  assert.ok((await table.textContent()).includes('In the trash, removed for good on'));
  assert.equal(await page.getByRole('link', { name: /House|Diary/ }).count(), 0, 'no hub is named or linked');
  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

test('chromium: no private hub, or no right to the list, and the row is absent', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  for (const answer of [{ json: [] }, { status: 403, json: { code: 'access.not_permitted' } }]) {
    const { page, failures, context } = await signedIn(browser, 1280, 900);
    await context.route('**/api/v1/private-hubs', (route) => route.fulfill(answer));
    await page.goto(`${served.origin}/administration/workspace`);
    const nav = page.getByRole('navigation', { name: 'Administration', exact: true });
    await nav.getByRole('treeitem', { name: 'Limits', exact: true }).waitFor({ timeout: 10_000 });
    await page.waitForTimeout(300);
    assert.equal(await nav.getByRole('treeitem', { name: 'Private hubs', exact: true }).count(), 0, `answered ${answer.status ?? 200}`);
    assert.deepEqual(failures, []);
    await context.close();
  }
});
