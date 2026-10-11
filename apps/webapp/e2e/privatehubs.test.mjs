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
import { HUB, OTHER, WORKSPACE, signedIn } from './fixture.mjs';

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
