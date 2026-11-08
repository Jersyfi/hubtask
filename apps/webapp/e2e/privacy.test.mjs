// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The register of people's requests, walked where a deadline is extended (UC-PRV-01 checks 9 and
// 10): the extension offered only on a case that answers a bound, the form starting on that bound
// and sending three facts, and an extended case showing both dates and its reason as a sentence.
// An installation-wide case is the operator's: the screen offers nothing on it (UC-PRV-06).
//
// A case is its record page (page-system §9.3): its acts are its head's, the extension a Dialog M.
//
// Chromium only: what this asserts is the rendered row and the request that leaves, and neither
// has an engine-specific part.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, MANIFEST, PERMISSIONS, PRIVACY_REQUESTS, stub, forgetUnstubbed, unstubbedSoFar } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const [OPEN, EXTENDED, INSTALLATION, ERASURE, KEPT, WAITS_FOR_OWNER] = PRIVACY_REQUESTS;
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

async function open(browser, role = 'OWNER', options = {}) {
  const written = [];
  // What the server answers a recording with, where a walk needs the case it made in the register.
  const { recordAnswer, refuseStart, ...contextOptions } = options;
  const recorded = [];
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, ...contextOptions });
  if (options.now) await context.clock.setFixedTime(options.now);
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    // The workspace counts in Berlin, which is where a deadline named as a day ends.
    if (path === '/tenant' && request.method() === 'GET') return route.fulfill({ json: WORKSPACE });
    // What the reader may do is the manifest's answer, a role held through a group included.
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, roles: ROLES, workspace_permissions: PERMISSIONS[role] } });
    if (path === '/memberships' && new URL(request.url()).searchParams.get('scope_type') === 'TENANT') {
      return route.fulfill({ json: { data: [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role }], page: { next_cursor: null, has_more: false } } });
    }
    if (path.endsWith('/erasure-preview') && request.method() === 'GET') {
      written.push({ path: `${path}?${new URL(request.url()).searchParams}`, preview: true });
      return route.fulfill({ json: { mode: 'FULL_DELETE', kept: KEPT.kept } });
    }
    if (/^\/privacy\/requests\/[^/:]+$/.test(path) && request.method() === 'PATCH') {
      written.push({ path, body: request.postDataJSON() });
      if (refuseStart) {
        return route.fulfill({
          status: 409, contentType: 'application/problem+json',
          body: JSON.stringify({ status: 409, code: 'conflict', detail_code: refuseStart, request_id: 'req_p' }),
        });
      }
      const before = PRIVACY_REQUESTS.find((each) => path.endsWith(each.id));
      return route.fulfill({ json: { ...before, ...request.postDataJSON() } });
    }
    // The writes are kept for the assertions and answered by the fixture, as every walk's is.
    if (path.endsWith(':extend') && request.method() === 'POST') {
      written.push({ path, body: request.postDataJSON(), idempotencyKey: request.headers()['idempotency-key'] });
    }
    if (path === '/privacy/requests' && request.method() === 'POST') {
      written.push({ path, body: request.postDataJSON() });
      if (recordAnswer) {
        const answer = recordAnswer(request.postDataJSON());
        recorded.push(answer);
        return route.fulfill({ status: 201, json: answer });
      }
      return route.fulfill({ status: 201, json: { ...OPEN, ...request.postDataJSON() } });
    }
    if (path === '/privacy/requests' && request.method() === 'GET' && recorded.length > 0) {
      return route.fulfill({ json: { data: [...PRIVACY_REQUESTS, ...recorded], page: { next_cursor: null, has_more: false } } });
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
  await page.locator('main tbody tr.row', { hasText: OPEN.subject_email }).waitFor();
  // The erasure's mode note reads its preview on load, so a read stands among the writes.
  const writes = () => written.filter((each) => !each.preview);
  return { page, failures, written, writes, close: () => context.close() };
}

/** A case's own record page (page-system §9.3), its main region. */
async function caseOf(page, request) {
  await page.goto(`${served.origin}/administration/privacy/${request.id}`);
  await page.locator('[data-page-kind="record"] h1', { hasText: request.subject_email }).waitFor({ timeout: 10_000 });
  return page.locator('main');
}

/** One act of a case's head menu: *Extend the deadline…*, *Refuse it…*. */
async function caseMenu(page, item) {
  await page.getByRole('button', { name: /^More for / }).first().click();
  await page.getByRole('menuitem', { name: item }).click();
  return page.locator('dialog[open]');
}

/** *Record a request*, the head's primary, its Dialog M; the day and the deadline under its fold. */
async function recordForm(page) {
  await page.getByRole('button', { name: 'Record a request' }).first().click();
  const dialog = page.locator('dialog[open]');
  await dialog.waitFor();
  return dialog;
}

const headMenuOffers = async (page, item) => {
  const more = page.getByRole('button', { name: /^More for / });
  if (await more.count() === 0) return false;
  await more.first().click();
  const offered = await page.getByRole('menuitem', { name: item }).count() > 0;
  await page.keyboard.press('Escape');
  return offered;
};

test('chromium: an extended case shows both deadlines and why', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await open(browser);
    const text = await (await caseOf(page, EXTENDED)).innerText();
    assert.match(text, /Owed .+, extended from .+/);
    assert.match(text, /Extended once because of the number of requests\. They were told on .+2026\./);
    // Once extended, never offered again.
    assert.equal(await headMenuOffers(page, 'Extend the deadline…'), false);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

test('chromium: the day the person was told starts on today in the workspace, not on the device', async () => {
  // The server bounds the day by today in the workspace's zone (UC-PRV-01 check 9). At 11:00 UTC
  // Berlin still has the 12th while a device on Kiritimati (UTC+14) already has the 13th: a form
  // that started on the device's day would be refused for a day the person typed nothing into.
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await open(browser, 'OWNER', {
      timezoneId: 'Pacific/Kiritimati', now: new Date('2026-10-12T11:00:00Z'),
    });
    await caseOf(page, OPEN);
    const dialog = await caseMenu(page, 'Extend the deadline…');
    const informed = dialog.getByLabel('The day you told them');
    assert.equal(await informed.inputValue(), '2026-10-12');
    assert.equal(await informed.getAttribute('max'), '2026-10-12');
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

test('chromium: the extension starts on the bound the case answers and sends three facts', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, writes, close } = await open(browser);
    await caseOf(page, OPEN);
    const dialog = await caseMenu(page, 'Extend the deadline…');

    const until = dialog.getByLabel('New deadline');
    assert.equal(await until.inputValue(), OPEN.extendable_until);
    assert.equal(await until.getAttribute('max'), OPEN.extendable_until);

    await until.fill('2026-12-01');
    await dialog.getByLabel('Why').selectOption('NUMBER_OF_REQUESTS');
    await dialog.getByLabel('The day you told them').fill('2026-09-12');
    await dialog.getByRole('button', { name: 'Extend it' }).last().click();

    for (let waited = 0; writes().length === 0 && waited < 5000; waited += 50) await page.waitForTimeout(50);
    assert.equal(writes().length, 1);
    assert.deepEqual(writes()[0].body, { due_on: '2026-12-01', reason: 'NUMBER_OF_REQUESTS', informed_on: '2026-09-12' });
    assert.ok(writes()[0].idempotencyKey, 'the extension carries an Idempotency-Key');
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
    // Listed, with its deadline: it is this workspace's to see.
    assert.match(await page.locator('main tbody tr.row', { hasText: INSTALLATION.subject_email }).innerText(), /Owed /);
    // And no control that moves it - the operator's, through the API or hubctl (UC-PRV-06/6).
    await caseOf(page, INSTALLATION);
    assert.equal(await page.getByRole('button', { name: 'Start answering it' }).count(), 0);
    assert.equal(await page.getByRole('button', { name: /^More for / }).count(), 0);
    // The same controls stand on this workspace's own case, so the absence is the scope's.
    await caseOf(page, OPEN);
    assert.equal(await page.getByRole('button', { name: 'Start answering it' }).count(), 1);
    for (const name of ['Refuse it…', 'Extend the deadline…']) assert.equal(await headMenuOffers(page, name), true, name);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

test('chromium: the register offers no installation-wide case and says whom to ask', async () => {
  // UC-PRV-06 check 6: the form records this workspace's case only, and the page names the
  // operator as the one who answers a request across the installation.
  const browser = await chromium.launch();
  try {
    const { page, failures, writes, close } = await open(browser);
    const note = page.getByText('Raise it with whoever runs this installation', { exact: false });
    assert.equal(await note.count(), 1);
    const form = await recordForm(page);
    for (const role of ['option', 'radio', 'checkbox', 'button']) {
      assert.equal(await form.getByRole(role, { name: /installation|everywhere/i }).count(), 0, role);
    }
    await form.getByLabel('Their address').fill('new@example.invalid');
    await form.getByRole('button', { name: 'Record it' }).last().click();
    for (let waited = 0; writes().length === 0 && waited < 5000; waited += 50) await page.waitForTimeout(50);
    assert.equal(writes().length, 1);
    assert.equal(writes()[0].body.scope ?? 'TENANT', 'TENANT');
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

test('chromium: a case is recorded with its own deadline, owed to the end of that day', async () => {
  // UC-PRV-01 checks 1 and 6: the web app records a case, and the record form names a day, and the case is owed until its last second in
  // the workspace's zone.
  const browser = await chromium.launch();
  try {
    const { page, failures, writes, close } = await open(browser);
    const form = await recordForm(page);
    await form.getByLabel('Their address').fill('new@example.invalid');
    await form.getByRole('button', { name: /^When it arrived/ }).click();
    await form.getByLabel('Deadline', { exact: true }).fill('2027-03-28');
    await form.getByRole('button', { name: 'Record it' }).last().click();
    for (let waited = 0; writes().length === 0 && waited < 5000; waited += 50) await page.waitForTimeout(50);

    assert.equal(writes().length, 1);
    assert.equal(writes()[0].body.subject_email, 'new@example.invalid');
    assert.equal(writes()[0].body.due_at, '2027-03-28T21:59:59.000Z');
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

test('chromium: a request received two months ago is recorded with that day and shown overdue', async () => {
  // UC-PRV-01 check 2: the form names the day of receipt, never later than today in the
  // workspace's zone, and a case whose month has run out is recorded and marked overdue. The
  // answer is the server's, written out here: received 14 August in Berlin, owed to the end of
  // 14 September there.
  const browser = await chromium.launch();
  try {
    const { page, failures, writes, close } = await open(browser, 'OWNER', {
      now: new Date('2026-10-15T09:00:00Z'),
      recordAnswer: (sent) => ({
        ...OPEN, id: '01a0e2e0-0000-7000-8000-0000000000c9', ...sent,
        received_at: '2026-08-13T22:00:00Z', received_on: '2026-08-14', due_at: '2026-09-14T21:59:59Z',
        extendable_until: null,
      }),
    });
    const form = await recordForm(page);
    await form.getByRole('button', { name: /^When it arrived/ }).click();
    assert.equal(await form.getByLabel('Received on').getAttribute('max'), '2026-10-15');
    await form.getByLabel('Their address').fill('late@example.invalid');
    await form.getByLabel('Received on').fill('2026-08-14');
    await form.getByRole('button', { name: 'Record it' }).last().click();
    for (let waited = 0; writes().length === 0 && waited < 5000; waited += 50) await page.waitForTimeout(50);

    assert.equal(writes().length, 1);
    assert.equal(writes()[0].body.received_on, '2026-08-14');
    assert.equal(writes()[0].body.due_at, undefined);
    const late = page.locator('main tbody tr.row', { hasText: 'late@example.invalid' });
    await late.waitFor();
    assert.match(await late.innerText(), /Overdue/);
    const text = await (await caseOf(page, { id: '01a0e2e0-0000-7000-8000-0000000000c9', subject_email: 'late@example.invalid' })).innerText();
    assert.match(text, /Received .*14/);
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
    const { page, failures, written, writes, close } = await open(browser);
    await caseOf(page, ERASURE);
    written.length = 0;
    await page.getByRole('button', { name: 'Start answering it' }).first().click();

    const dialog = page.locator('dialog[open]');
    await dialog.waitFor();
    assert.match(await dialog.innerText(), new RegExp(`Erase ${ERASURE.subject_email}\\?`));
    assert.match(await dialog.innerText(), /account goes, and every comment they wrote goes with it/);
    // What the hold keeps, said before the start (UC-PRV-03 check 11), for the case's own mode.
    await dialog.getByText('A hold on a hub or collection keeps 2 comments, 1 assignment').waitFor();
    assert.match(await dialog.innerText(), /Art\. 17\(3\)\(e\)/);
    assert.ok(written.some((each) => each.preview && each.path.endsWith('mode=FULL_DELETE')), JSON.stringify(written));
    written.length = 0;

    await dialog.getByRole('button', { name: 'Keep the case open' }).last().click();
    assert.equal(writes().length, 0);

    await page.getByRole('button', { name: 'Start answering it' }).first().click();
    await page.locator('dialog[open]').getByRole('button', { name: 'Erase' }).last().click();
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

test('chromium: the erasure of the only owner is refused at the start, saying what to do first', async () => {
  // UC-PRV-03 check 14: the server refuses with memberships.last_owner; the screen says the
  // sentence, and the case stays where it was.
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await open(browser, 'OWNER', { refuseStart: 'memberships.last_owner' });
    await caseOf(page, ERASURE);
    await page.getByRole('button', { name: 'Start answering it' }).first().click();
    await page.locator('dialog[open]').getByRole('button', { name: 'Erase' }).last().click();
    // Said in the dialog, which stays open over the page.
    await page.locator('dialog[open]').getByText('The workspace needs at least one owner. Give somebody else Owner first.').waitFor();
    await page.keyboard.press('Escape');
    // A rest that waits for another owner says so, not a count of rules.
    assert.match(await (await caseOf(page, WAITS_FOR_OWNER)).innerText(), /The rest waits: the person is this workspace's only owner\. Give somebody else Owner first\./);
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
    await caseOf(page, ERASURE);
    await page.waitForTimeout(200);
    assert.equal(await page.getByRole('button', { name: 'Start answering it' }).count(), 0);
    await caseOf(page, OPEN);
    await page.getByRole('button', { name: 'Start answering it' }).first().waitFor({ timeout: 5_000 });
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
    const text = await (await caseOf(page, KEPT)).innerText();
    assert.match(text, /Partly completed: a legal hold keeps part of it, for legal claims \(Art\. 17\(3\)\(e\) GDPR\)/);
    assert.match(text, /A hold on a hub or collection keeps 2 comments, 1 assignment and their name on no entry\./);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});
