// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The register of people's requests, walked where a deadline is extended (UC-PRV-01 checks 9 and
// 10): the extension offered only on a case that answers a bound, the form starting on that bound
// and sending three facts, and an extended case showing both dates and its reason as a sentence.
// An installation-wide case is the operator's: the screen offers nothing on it (UC-PRV-06).
//
// Chromium only: what this asserts is the rendered row and the request that leaves, and neither
// has an engine-specific part.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, MANIFEST, PRIVACY_REQUESTS, stub, forgetUnstubbed, unstubbedSoFar } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const [OPEN, EXTENDED, INSTALLATION, ERASURE, KEPT] = PRIVACY_REQUESTS;
const WORKSPACE = {
  id: '01a0e2e0-0000-7000-8000-0000000000b0', slug: 'house', display_name: 'House', status: 'ACTIVE',
  default_locale: 'en', default_time_zone: 'Europe/Berlin', require_admin_totp: false,
  created_at: '2026-09-01T00:00:00Z', version: 1,
};

const served = await serve(DIST);
test.after(() => served.close());

/**
 * The reader's role in the workspace. The owner holds DELETE_CONTAINER, which starting an erasure
 * asks for; an administrator does not (data-protection.md §4).
 */
const ROLES = [
  { role: 'OWNER', permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS', 'DELETE_CONTAINER'] },
  { role: 'ADMIN', permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS'] },
];

async function open(browser, role = 'OWNER') {
  const written = [];
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    // The workspace counts in Berlin, which is where a deadline named as a day ends.
    if (path === '/tenant' && request.method() === 'GET') return route.fulfill({ json: WORKSPACE });
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, roles: ROLES } });
    if (path === '/memberships' && new URL(request.url()).searchParams.get('scope_type') === 'TENANT') {
      return route.fulfill({ json: { data: [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role }], page: { next_cursor: null, has_more: false } } });
    }
    if (path.endsWith('/erasure-preview') && request.method() === 'GET') {
      written.push({ path: `${path}?${new URL(request.url()).searchParams}`, preview: true });
      return route.fulfill({ json: { mode: 'FULL_DELETE', kept: KEPT.kept } });
    }
    if (/^\/privacy\/requests\/[^/:]+$/.test(path) && request.method() === 'PATCH') {
      written.push({ path, body: request.postDataJSON() });
      const before = PRIVACY_REQUESTS.find((each) => path.endsWith(each.id));
      return route.fulfill({ json: { ...before, ...request.postDataJSON() } });
    }
    // The writes are kept for the assertions and answered by the fixture, as every walk's is.
    if (path.endsWith(':extend') && request.method() === 'POST') {
      written.push({ path, body: request.postDataJSON(), idempotencyKey: request.headers()['idempotency-key'] });
    }
    if (path === '/privacy/requests' && request.method() === 'POST') {
      written.push({ path, body: request.postDataJSON() });
      return route.fulfill({ status: 201, json: { ...OPEN, ...request.postDataJSON() } });
    }
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
  await page.goto(`${served.origin}/administration/privacy`);
  await page.locator('section.panel', { hasText: OPEN.subject_email }).waitFor();
  return { page, failures, written, close: () => context.close() };
}

const row = (page, request) => page.locator('section.panel', { hasText: request.subject_email });

test('chromium: an extended case shows both deadlines and why', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await open(browser);
    const text = await row(page, EXTENDED).innerText();
    assert.match(text, /Owed .+, extended from .+/);
    assert.match(text, /Extended once because of the number of requests\. They were told on .+2026\./);
    // Once extended, never offered again.
    assert.equal(await row(page, EXTENDED).getByRole('button', { name: 'Extend the deadline' }).count(), 0);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

test('chromium: the extension starts on the bound the case answers and sends three facts', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, written, close } = await open(browser);
    const open_ = row(page, OPEN);
    await open_.getByRole('button', { name: 'Extend the deadline' }).click();

    const until = open_.getByLabel('New deadline');
    assert.equal(await until.inputValue(), OPEN.extendable_until);
    assert.equal(await until.getAttribute('max'), OPEN.extendable_until);

    await until.fill('2026-12-01');
    await open_.getByLabel('Why').selectOption('NUMBER_OF_REQUESTS');
    await open_.getByLabel('The day you told them').fill('2026-09-12');
    await open_.getByRole('button', { name: 'Extend it' }).click();

    for (let waited = 0; written.length === 0 && waited < 5000; waited += 50) await page.waitForTimeout(50);
    assert.equal(written.length, 1);
    assert.deepEqual(written[0].body, { due_on: '2026-12-01', reason: 'NUMBER_OF_REQUESTS', informed_on: '2026-09-12' });
    assert.ok(written[0].idempotencyKey, 'the extension carries an Idempotency-Key');
    assert.deepEqual(failures, []);
    assert.deepEqual(unstubbedSoFar().filter((each) => each.includes('privacy')), []);
    await close();
  } finally {
    await browser.close();
  }
});

test('chromium: an installation-wide case is listed and offers nothing here', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await open(browser);
    const installation = row(page, INSTALLATION);
    // Listed, with its deadline: it is this workspace's to see.
    assert.match(await installation.innerText(), /Owed /);
    // And no control that moves it - the operator's, through the API or hubctl (UC-PRV-06/6).
    assert.equal(await installation.getByRole('button').count(), 0);
    // The same controls stand on this workspace's own case, so the absence is the scope's.
    for (const name of ['Start answering it', 'Refuse it', 'Extend the deadline']) {
      assert.equal(await row(page, OPEN).getByRole('button', { name }).count(), 1, name);
    }
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

test('chromium: a case is recorded with its own deadline, owed to the end of that day', async () => {
  // UC-PRV-01 check 6: the record form names a day, and the case is owed until its last second in
  // the workspace's zone.
  const browser = await chromium.launch();
  try {
    const { page, failures, written, close } = await open(browser);
    const form = page.locator('form', { hasText: 'Record a request' });
    await form.getByLabel('Their address').fill('new@example.invalid');
    await form.getByLabel('Deadline').fill('2027-03-28');
    await form.getByRole('button', { name: 'Record it' }).click();
    for (let waited = 0; written.length === 0 && waited < 5000; waited += 50) await page.waitForTimeout(50);

    assert.equal(written.length, 1);
    assert.equal(written[0].body.subject_email, 'new@example.invalid');
    assert.equal(written[0].body.due_at, '2027-03-28T21:59:59.000Z');
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

test('chromium: starting an erasure asks first, naming the person and what the mode removes', async () => {
  // UC-PRV-03 check 8, P-04: nothing is sent before the confirmation.
  const browser = await chromium.launch();
  try {
    const { page, failures, written, close } = await open(browser);
    await row(page, ERASURE).getByRole('button', { name: 'Start answering it' }).click();

    const dialog = page.getByRole('dialog');
    await dialog.waitFor();
    assert.match(await dialog.innerText(), new RegExp(`Erase ${ERASURE.subject_email}\\?`));
    assert.match(await dialog.innerText(), /account goes, and every comment they wrote goes with it/);
    // What the hold keeps, said before the start (UC-PRV-03 check 11), for the case's own mode.
    await dialog.getByText('A hold on a hub or collection keeps 2 comments, 1 assignment').waitFor();
    assert.match(await dialog.innerText(), /Art\. 17\(3\)\(e\)/);
    assert.ok(written[0].preview && written[0].path.endsWith('mode=FULL_DELETE'), JSON.stringify(written));
    written.length = 0;

    await dialog.locator('footer').getByRole('button', { name: 'Keep the case open' }).click();
    assert.equal(written.filter((each) => !each.preview).length, 0);

    await row(page, ERASURE).getByRole('button', { name: 'Start answering it' }).click();
    await page.getByRole('dialog').locator('footer').getByRole('button', { name: 'Erase' }).click();
    const writes = () => written.filter((each) => !each.preview);
    for (let waited = 0; writes().length === 0 && waited < 5000; waited += 50) await page.waitForTimeout(50);
    assert.equal(writes().length, 1);
    assert.ok(writes()[0].path.endsWith(ERASURE.id));
    assert.deepEqual(writes()[0].body, { status: 'IN_PROGRESS' });
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

test('chromium: an administrator is not offered the start of an erasure', async () => {
  // UC-PRV-03 check 1, P-05: the server asks the owner's right; the screen does not offer what it
  // would refuse. Other cases still start.
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await open(browser, 'ADMIN');
    await page.waitForTimeout(200);
    assert.equal(await row(page, ERASURE).getByRole('button', { name: 'Start answering it' }).count(), 0);
    assert.equal(await row(page, OPEN).getByRole('button', { name: 'Start answering it' }).count(), 1);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

test('chromium: a case a hold kept part of says so, with the legal basis', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await open(browser);
    const text = await row(page, KEPT).innerText();
    assert.match(text, /Partly completed: a legal hold keeps part of it, for legal claims \(Art\. 17\(3\)\(e\) GDPR\)/);
    assert.match(text, /A hold on a hub or collection keeps 2 comments, 1 assignment and their name on 0 entries\./);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});
