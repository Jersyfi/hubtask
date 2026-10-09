// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The legal holds, walked as the owner and as an administrator (UC-LIF-06 checks 1 and 8): the owner
// is offered the four scopes, picks a person or a hub rather than typing an identifier, and sees
// what each hold covers in words; an administrator reads the list and is offered neither placing
// nor lifting (P-05).
//
// Chromium only: what this asserts is the rendered form and the request that leaves.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, HUB, MANIFEST, OTHER, stub, forgetUnstubbed } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const ROLES = [
  { role: 'OWNER', permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS', 'DELETE_CONTAINER'] },
  { role: 'ADMIN', permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS', 'READ_CONFIGURATION'] },
];

const HOLD = {
  id: '01a0e2e0-0000-7000-8000-0000000003a1', reason: 'Pending litigation, ref. 4 O 128/26',
  placed_by: ACCOUNT.id, placed_at: '2026-09-20T08:00:00Z', scope: { kind: 'ACCOUNT', id: OTHER.id },
};

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, role) {
  const written = [];
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, roles: ROLES } });
    if (path === '/memberships' && url.searchParams.get('scope_type') === 'TENANT') {
      return route.fulfill({ json: { data: [
        { id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role },
        { id: 'm9', scope_type: 'TENANT', account_id: OTHER.id, role: 'MEMBER' },
      ], page: { next_cursor: null, has_more: false } } });
    }
    if (path === '/legal-holds' && request.method() === 'GET') return route.fulfill({ json: [HOLD] });
    if (path === '/legal-holds' && request.method() === 'POST') {
      written.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: { ...HOLD, id: '01a0e2e0-0000-7000-8000-0000000003a2', ...request.postDataJSON() } });
    }
    if (path === '/retention-policies') return route.fulfill({ json: [] });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  forgetUnstubbed();
  await page.goto(`${served.origin}/administration/retention`);
  await page.getByText("Mara Lind's data").waitFor();
  return { page, failures, written, close: () => context.close() };
}

test('chromium: the owner places a hold on a person, picked by name', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, written, close } = await open(browser, 'OWNER');
    const form = page.locator('form', { hasText: 'Place a hold' });
    const scopes = await form.getByLabel('What it covers').locator('option').allTextContents();
    assert.equal(scopes.length, 4, scopes.join(', '));

    await form.getByLabel('What it covers').selectOption('ACCOUNT');
    await form.getByLabel('Whose data').selectOption({ label: OTHER.display_name });
    await form.getByLabel('Why').fill('Pending litigation');
    await form.getByRole('button', { name: 'Place it' }).click();
    for (let waited = 0; written.length === 0 && waited < 5000; waited += 50) await page.waitForTimeout(50);
    assert.deepEqual(written[0], { scope: { kind: 'ACCOUNT', id: OTHER.id }, reason: 'Pending litigation' });

    // A hub is picked from the tree, not typed.
    await form.getByLabel('What it covers').selectOption('CONTAINER');
    assert.ok((await form.getByLabel('Which hub or collection').locator('option').allTextContents()).includes(HUB.name));
    assert.equal(await page.getByRole('button', { name: 'Lift it' }).count(), 1);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

test('chromium: an administrator reads the holds and is offered neither placing nor lifting', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await open(browser, 'ADMIN');
    await page.waitForTimeout(200);
    assert.equal(await page.locator('form', { hasText: 'Place a hold' }).count(), 0);
    assert.equal(await page.getByRole('button', { name: 'Lift it' }).count(), 0);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});
