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

import { ACCOUNT, HUB, ITEMS, MANIFEST, OTHER, stub, forgetUnstubbed } from './fixture.mjs';
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

/** A hold on one entry: listed by the entry's title, never by its identifier (P-08). */
const ENTRY_HOLD = { ...HOLD, id: '01a0e2e0-0000-7000-8000-0000000003a3', scope: { kind: 'ITEM', id: ITEMS[1].id } };

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, role, { policies = [], kinds = [] } = {}) {
  const written = [];
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, roles: ROLES, retention_data_kinds: kinds } });
    }
    if (path === '/memberships' && url.searchParams.get('scope_type') === 'TENANT') {
      return route.fulfill({ json: { data: [
        { id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role },
        { id: 'm9', scope_type: 'TENANT', account_id: OTHER.id, role: 'MEMBER' },
      ], page: { next_cursor: null, has_more: false } } });
    }
    if (path === '/legal-holds' && request.method() === 'GET') return route.fulfill({ json: [HOLD, ENTRY_HOLD] });
    if (path === '/search') return route.fulfill({ json: { data: [ITEMS[0]], items: [ITEMS[0]], page: { next_cursor: null, has_more: false } } });
    if (path === '/legal-holds' && request.method() === 'POST') {
      written.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: { ...HOLD, id: '01a0e2e0-0000-7000-8000-0000000003a2', ...request.postDataJSON() } });
    }
    if (path === '/retention-policies') return route.fulfill({ json: policies });
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
    // Exactly the four scopes a hold can be placed on (UC-LIF-06 check 8), nothing else.
    const scopes = await form.getByLabel('What it covers').locator('option')
      .evaluateAll((options) => options.map((option) => option.value));
    assert.deepEqual([...scopes].sort(), ['ACCOUNT', 'CONTAINER', 'ITEM', 'TENANT'], scopes.join(', '));

    await form.getByLabel('What it covers').selectOption('ACCOUNT');
    await form.getByLabel('Whose data').selectOption({ label: OTHER.display_name });
    await form.getByLabel('Why').fill('Pending litigation');
    await form.getByRole('button', { name: 'Place it' }).click();
    for (let waited = 0; written.length === 0 && waited < 5000; waited += 50) await page.waitForTimeout(50);
    assert.deepEqual(written[0], { scope: { kind: 'ACCOUNT', id: OTHER.id }, reason: 'Pending litigation' });

    // A hub is picked from the tree, not typed.
    await form.getByLabel('What it covers').selectOption('CONTAINER');
    assert.ok((await form.getByLabel('Which hub or collection').locator('option').allTextContents()).includes(HUB.name));
    assert.equal(await page.getByRole('button', { name: 'Lift it' }).count(), 2);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

test('chromium: the owner holds one entry found by its title, and the list names it', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, written, close } = await open(browser, 'OWNER');
    // The hold already placed on an entry reads as the entry's title.
    await page.getByText(`The entry “${ITEMS[1].title}”, and everything under it`).waitFor();
    assert.equal(await page.getByText(ITEMS[1].id).count(), 0, 'no identifier on the screen');

    const form = page.locator('form', { hasText: 'Place a hold' });
    await form.getByLabel('What it covers').selectOption('ITEM');
    assert.equal(await form.locator('input:not([type="search"])').count(), 0, 'no box to type an identifier into');
    await form.getByLabel('Which entry').fill('tiles');
    await form.getByRole('button', { name: ITEMS[0].title }).click();
    await form.getByText(`Chosen: ${ITEMS[0].title}`).waitFor();
    await form.getByLabel('Why').fill('Pending litigation');
    await form.getByRole('button', { name: 'Place it' }).click();
    for (let waited = 0; written.length === 0 && waited < 5000; waited += 50) await page.waitForTimeout(50);
    assert.deepEqual(written[0], { scope: { kind: 'ITEM', id: ITEMS[0].id }, reason: 'Pending litigation' });
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

// P-12, UC-LIF-04: a rule and the classes a rule can name are drawn in words, never as the constant
// the server keys them by.
test('chromium: the rules and the form name each class of data in words', async () => {
  const browser = await chromium.launch();
  try {
    const kinds = [
      { data_kind: 'COMPLETED_ITEM', anchor: 'completed_at', actions: ['ARCHIVE', 'TRASH', 'NOTIFY_ONLY'] },
      { data_kind: 'AI_SUGGESTION', anchor: 'created_at', actions: ['HARD_DELETE'] },
    ];
    const policies = [{
      id: '01a0e2e0-0000-7000-8000-0000000003b1', scope: { kind: 'TENANT' }, data_kind: 'AI_SUGGESTION',
      retain_days: 30, action: 'HARD_DELETE', enabled: true,
    }];
    const { page, failures, close } = await open(browser, 'OWNER', { policies, kinds });
    await page.locator('.name', { hasText: 'AI suggestions' }).waitFor();
    const options = await page.getByLabel('What it applies to').locator('option').allTextContents();
    assert.ok(options.includes('Completed tasks, work packages and activities'), options.join(' | '));
    assert.ok(options.includes('AI suggestions'), options.join(' | '));
    const text = await page.locator('main').innerText();
    for (const key of ['AI_SUGGESTION', 'COMPLETED_ITEM']) assert.ok(!text.includes(key), `${key} is on the screen`);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});
