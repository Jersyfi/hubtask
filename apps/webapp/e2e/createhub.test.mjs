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

import { HUB, MANIFEST, PERMISSIONS, WORKSPACE, signedIn, stub } from './fixture.mjs';
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

const shared = (page) => page.getByRole('button', { name: 'Create hub', exact: true });
const own = (page) => page.getByRole('button', { name: 'Create private hub', exact: true });

test('chromium: a member is offered a private hub of their own and no shared hub, and the dialog makes it private', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, sent } = await as(browser, { permissions: PERMISSIONS.MEMBER, offered: true });
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  await own(page).first().waitFor({ timeout: 10_000 });
  // The head and the navigation, each once.
  assert.equal(await own(page).count(), 2, 'not offered in both the head and the navigation');
  assert.equal(await shared(page).count(), 0, 'a member is offered a shared hub the server refuses');

  await own(page).first().click();
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
  await page.getByRole('navigation').first().waitFor({ timeout: 10_000 });
  await page.waitForTimeout(600);
  assert.equal(await own(page).count(), 0, 'a private hub offered where the server refuses one');
  assert.equal(await shared(page).count(), 0, 'a shared hub offered to somebody without STRUCTURE');
  assert.deepEqual(failures, []);
});

test('chromium: whoever shapes the workspace where private hubs are switched off is offered a shared hub only', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, sent } = await as(browser, { permissions: PERMISSIONS.ADMIN, offered: false, allowed: false });
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  await shared(page).first().waitFor({ timeout: 10_000 });
  assert.equal(await own(page).count(), 0, 'a private-only hub offered to whoever may shape the workspace');

  await shared(page).first().click();
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
