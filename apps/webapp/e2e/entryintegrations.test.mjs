// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What other tools put on an entry: a hub's forges added under *Settings → Code* with the secret
// shown once (UC-INT-13 check 1), an entry's pull requests, branches and commits under *Code*
// (UC-INT-13 check 3), and its files in a store linked under *Files* (UC-INT-14 checks 1, 2).
// The server is stubbed at the network edge; what is walked is what the screens draw and send.
// Chromium: no engine-specific part.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, COLLECTION, HUB, ITEMS, signedIn } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const ENTRY = ITEMS[0];
const AT = '2026-10-30T09:00:00Z';
const HOOK = {
  id: '01a0e2e0-0000-7000-8000-000000002301', hub_id: HUB.id, kind: 'GITHUB', name: 'website', enabled: true,
  run_as: ACCOUNT.id, created_at: AT, updated_at: AT, last_delivery_at: null,
};
const PULL = {
  id: '01a0e2e0-0000-7000-8000-000000002311', item_id: ENTRY.id, hub_id: HUB.id, collection_id: COLLECTION.id,
  hook_id: HOOK.id, kind: 'PULL_REQUEST', repository: 'team/web', ref: '7', title: 'KITCHEN-1 the login form',
  url: 'https://forge.example.org/team/web/pull/7', author: 'ada', state: 'MERGED', happened_at: AT, updated_at: AT,
};
const BRANCH = { ...PULL, id: '01a0e2e0-0000-7000-8000-000000002312', kind: 'BRANCH', ref: 'feature/KITCHEN-1-login', title: 'feature/KITCHEN-1-login', state: 'OPEN', url: '' };

const json = (route, body, status = 200) => route.fulfill({ status, json: body });

test('chromium: a forge is added under Settings → Code and its secret is shown once', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 1280, 1000);
  t.after(() => context.close());
  let hooks = [];
  let sent;
  await context.route(`**/api/v1/hubs/${HUB.id}/code-hooks`, (route) => {
    if (route.request().method() === 'POST') {
      sent = route.request().postDataJSON();
      const made = { ...HOOK, kind: sent.kind, name: sent.name };
      hooks = [made];
      return json(route, { ...made, secret: 'e2e-forge-secret' }, 201);
    }
    return json(route, { data: hooks });
  });

  await page.goto(`${served.origin}/hubs/${HUB.id}/settings/code`);
  await page.getByText('No forge posts to this hub yet').waitFor({ timeout: 15_000 });
  await page.getByRole('button', { name: 'Add a forge' }).click();
  const dialog = page.getByRole('dialog', { name: 'Add a forge' });
  await dialog.getByRole('textbox', { name: 'Name' }).fill('website');
  await dialog.getByRole('button', { name: 'Add', exact: true }).click();
  // Masked until revealed, and only in this answer.
  await page.getByText('The secret of website').waitFor({ timeout: 5_000 });
  await page.getByRole('button', { name: 'Show the secret' }).click();
  await page.getByText('e2e-forge-secret').first().waitFor({ timeout: 5_000 });
  assert.deepEqual(sent, { kind: 'GITHUB', name: 'website' });
  // The address the forge posts to stands beside it.
  assert.ok(await page.getByText(`/api/v1/code-intake/`).first().isVisible(), 'the intake address is not shown');
  assert.deepEqual(failures, []);
});

test('chromium: an entry shows its code and links a file from a store', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 1280, 1400);
  t.after(() => context.close());
  let files = [];
  let sent;
  await context.route(`**/api/v1/items/${ENTRY.id}/code-links`, (route) => json(route, { data: [PULL, BRANCH] }));
  await context.route(`**/api/v1/items/${ENTRY.id}/file-links`, (route) => {
    if (route.request().method() === 'POST') {
      sent = route.request().postDataJSON();
      const made = {
        id: '01a0e2e0-0000-7000-8000-000000002321', item_id: ENTRY.id, collection_id: COLLECTION.id,
        provider: 'NEXTCLOUD', address: sent.address, name: 'Login flow.pdf', added_by: ACCOUNT.id, added_at: AT,
      };
      files = [made];
      return json(route, made, 201);
    }
    return json(route, { data: files });
  });

  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  const code = page.locator('[data-entry-code]');
  await code.waitFor({ timeout: 15_000 });
  const pull = code.getByRole('link', { name: PULL.title });
  assert.equal(await pull.getAttribute('href'), PULL.url);
  assert.equal(await pull.getAttribute('target'), '_blank');
  assert.ok(await code.getByText('Merged').isVisible(), 'the merged state is not said');
  assert.ok(await code.getByText('Open').isVisible(), 'the open branch is not said');
  // A branch the forge gave no address is a name, not a link.
  assert.equal(await code.getByRole('link', { name: BRANCH.title }).count(), 0);

  const section = page.locator('[data-entry-files]');
  await section.getByText('No file from a store is linked yet.').waitFor({ timeout: 5_000 });
  await section.getByRole('button', { name: 'Link a file' }).click();
  const dialog = page.getByRole('dialog', { name: 'Link a file from a store' });
  await dialog.getByRole('textbox', { name: 'Address' }).fill('https://cloud.example.org/s/aBcD1234');
  await dialog.getByRole('button', { name: 'Link', exact: true }).click();
  const linked = section.getByRole('link', { name: 'Login flow.pdf' });
  await linked.waitFor({ timeout: 5_000 });
  assert.deepEqual(sent, { address: 'https://cloud.example.org/s/aBcD1234' });
  assert.equal(await linked.getAttribute('href'), 'https://cloud.example.org/s/aBcD1234');
  assert.ok(await section.getByText('Nextcloud').isVisible(), 'the store is not said');
  assert.deepEqual(failures, []);
});
