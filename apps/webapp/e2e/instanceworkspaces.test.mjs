// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// An operator opens the password for one workspace, walked as they meet it (ADR-0078 §3): the
// dialog says what it costs, sends the hours, the requester and the reason, the row then says until
// when and for whom, and one press closes it again. A refusal lands at the field it is about.
//
// Against a stubbed API: what is asserted is what the screen sends and what it shows of the answers.
// Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { MANIFEST, WORKSPACE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const TENANT = '01936f2a-7c1e-7000-8000-0000000034a1';

/** The manifest of an installation that runs several workspaces; the fixture's is single mode. */
const MULTI = { ...MANIFEST, tenancy_mode: 'multi' };

/** The workspace as the control plane lists it, with an opening where one is given. */
const workspace = (opening = null) => ({
  id: TENANT, slug: 'acme', display_name: 'Acme', status: 'ACTIVE', default_locale: 'en',
  default_time_zone: 'UTC', created_at: '2026-09-01T00:00:00Z', purge_after: null,
  ...(opening ? { password_opening: opening } : {}),
});

async function open(browser) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const sent = [];
  let row = workspace();
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    if (path === '/meta/capabilities') return route.fulfill({ json: MULTI });
    if (path === '/admin/tenants' && request.method() === 'GET') return route.fulfill({ json: [row] });
    if (path === `/admin/tenants/${TENANT}:open-password`) {
      const body = request.postDataJSON() ?? {};
      sent.push({ path, body });
      if (body.hours > 168) {
        return route.fulfill({
          status: 422,
          json: {
            code: 'errors.validation', detail_code: 'admin.password_opening_hours', status: 422,
            request_id: 'req_o', params: { maximum: '168' },
            field_errors: [{ path: '/hours', code: 'admin.password_opening_hours', params: { maximum: '168' } }],
          },
        });
      }
      row = workspace({
        until: new Date(Date.now() + body.hours * 3_600_000).toISOString(),
        requester: body.requester, reason: body.reason,
      });
      return route.fulfill({ json: row });
    }
    if (path === `/admin/tenants/${TENANT}:close-password`) {
      sent.push({ path, body: request.postDataJSON() });
      row = workspace();
      return route.fulfill({ json: row });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  return { page, sent, close: () => context.close() };
}

test('chromium: an operator opens the password for one workspace, sees it on the row, and closes it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/instance/workspaces`);
  const row = page.getByRole('row').filter({ hasText: 'Acme' });
  await row.waitFor();
  assert.doesNotMatch(await row.textContent() ?? '', /Password open/);

  await row.getByRole('button', { name: 'Open the password' }).click();
  const dialog = page.getByRole('dialog', { name: 'Open the password for Acme' });
  await dialog.waitFor();
  // What it does, before the button.
  assert.match(await dialog.textContent() ?? '', /administrators are told when it opens and when it closes/);
  // A day unless said otherwise.
  assert.equal(await dialog.getByLabel('For how many hours').inputValue(), '24');

  // A week and an hour is refused, at the field, and the dialog stays.
  await dialog.getByLabel('For how many hours').fill('169');
  await dialog.getByLabel(/^Who asked for it/).fill('TICKET-4711');
  await dialog.getByLabel(/^Why/).fill('the directory answers 500');
  await dialog.getByRole('button', { name: 'Open it' }).click();
  await dialog.getByText('An opening lasts from one hour to 168 hours.').first().waitFor();
  assert.ok(await dialog.isVisible(), 'a refused opening closed the dialog');

  await dialog.getByLabel('For how many hours').fill('48');
  await dialog.getByRole('button', { name: 'Open it' }).click();
  await dialog.waitFor({ state: 'hidden' });
  assert.deepEqual(sent[1], {
    path: `/admin/tenants/${TENANT}:open-password`,
    body: { hours: 48, requester: 'TICKET-4711', reason: 'the directory answers 500' },
  });

  // The row says it stands, until when and for whom - and offers to close it.
  await row.getByText('Password open', { exact: true }).waitFor();
  assert.match(await row.textContent() ?? '', /asked for by TICKET-4711/);
  assert.equal(await row.getByRole('button', { name: 'Open the password' }).count(), 0);
  await row.getByRole('button', { name: 'Close the password' }).click();
  await row.getByRole('button', { name: 'Open the password' }).waitFor();
  assert.equal(sent[2].path, `/admin/tenants/${TENANT}:close-password`);
  assert.doesNotMatch(await row.textContent() ?? '', /Password open/);
});

// A workspace under a legal hold cannot be deleted (data-protection.md §5): the row says a hold is
// in force and offers no *Delete*; a workspace pending deletion under a hold says its deletion
// waits. Neither says which hold - that stays the workspace's (P-01).
test('chromium: a workspace under a legal hold is marked and offers no deletion', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  const rows = [
    { ...workspace(), legal_hold: true },
    { ...workspace(), id: '01936f2a-7c1e-7000-8000-0000000034a2', slug: 'leaving', display_name: 'Leaving',
      status: 'PENDING_DELETION', purge_after: '2026-09-20T00:00:00Z', legal_hold: true },
    { ...workspace(), id: '01936f2a-7c1e-7000-8000-0000000034a3', slug: 'free', display_name: 'Free', legal_hold: false },
  ];
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    if (path === '/meta/capabilities') return route.fulfill({ json: MULTI });
    if (path === '/admin/tenants' && route.request().method() === 'GET') return route.fulfill({ json: rows });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/instance/workspaces`);

  const held = page.getByRole('row').filter({ hasText: 'acme' });
  await held.waitFor();
  assert.match(await held.textContent() ?? '', /Legal hold/);
  assert.match(await held.textContent() ?? '', /cannot be deleted until its owner releases every hold/);
  assert.equal(await held.getByRole('button', { name: 'Delete' }).count(), 0);

  const leaving = page.getByRole('row').filter({ hasText: 'Leaving' });
  assert.match(await leaving.textContent() ?? '', /the deletion waits until every hold is released/);
  // Nobody can release a hold while the workspace is being deleted: the row names the way out.
  assert.match(await leaving.textContent() ?? '', /cancel the deletion first/);

  const free = page.getByRole('row').filter({ hasText: 'Free' });
  assert.doesNotMatch(await free.textContent() ?? '', /Legal hold/);
  assert.equal(await free.getByRole('button', { name: 'Delete' }).count(), 1);
});

/**
 * The installation's console against a stubbed control plane: the manifest in the mode given, the
 * rows given, and every lifecycle write recorded and answered as the server does.
 */
async function operatorConsole(browser, { mode, rows, width = 1280 }) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  const sent = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, tenancy_mode: mode } });
    if (path === '/admin/tenants' && request.method() === 'GET') return route.fulfill({ json: rows });
    const shift = /^\/admin\/tenants\/([^/:]+):(suspend|resume|cancel-deletion)$/.exec(path);
    if (shift) {
      sent.push(`${shift[1]}:${shift[2]}`);
      const row = rows.find((each) => each.id === shift[1]);
      if (row && shift[2] === 'suspend') row.status = 'SUSPENDED';
      if (row && shift[2] === 'cancel-deletion') Object.assign(row, { status: 'ACTIVE', purge_after: null });
      return route.fulfill({ status: 204 });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/instance/workspaces`);
  return { page, sent, close: () => context.close() };
}

/** The operator's own workspace (the fixture's `GET /tenant`) and a customer's, both active. */
const lifecycleRows = () => [
  { ...workspace(), id: WORKSPACE.id, slug: WORKSPACE.slug, display_name: WORKSPACE.display_name },
  { ...workspace(), display_name: 'Acme' },
];

// UC-INS-08 check 1: *Suspend* asks first and names the workspace; nothing is sent until the
// second press. The operator's own workspace is marked, and its dialogs say that the operator's
// own session ends - the deletion's too.
test('chromium: suspending asks first, and the own workspace is marked and warned about', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await operatorConsole(browser, { mode: 'multi', rows: lifecycleRows() });
  t.after(close);

  const own = page.getByRole('row').filter({ hasText: WORKSPACE.slug });
  const customer = page.getByRole('row').filter({ hasText: 'Acme' });
  await own.getByText('Your workspace', { exact: true }).waitFor();
  assert.equal(await customer.getByText('Your workspace', { exact: true }).count(), 0);

  // A customer's workspace: the dialog names it, says what it does, and keeping it sends nothing.
  await customer.getByRole('button', { name: 'Suspend' }).click();
  let dialog = page.getByRole('dialog', { name: 'Suspend Acme?' });
  await dialog.waitFor();
  assert.match(await dialog.textContent() ?? '', /Nothing in it is lost/);
  assert.doesNotMatch(await dialog.textContent() ?? '', /your own workspace/i);
  await dialog.getByRole('button', { name: 'Keep it running' }).click();
  await dialog.waitFor({ state: 'hidden' });
  assert.deepEqual(sent, []);

  await customer.getByRole('button', { name: 'Suspend' }).click();
  await dialog.getByRole('button', { name: 'Suspend it' }).click();
  await dialog.waitFor({ state: 'hidden' });
  assert.deepEqual(sent, [`${TENANT}:suspend`]);

  // The own workspace: the same dialog, and the warning that the session ends.
  await own.getByRole('button', { name: 'Suspend' }).click();
  dialog = page.getByRole('dialog', { name: `Suspend ${WORKSPACE.display_name}?` });
  await dialog.waitFor();
  assert.match(await dialog.textContent() ?? '', /This is your own workspace. Your session ends at once/);
  await dialog.getByRole('button', { name: 'Keep it running' }).click();
  await dialog.waitFor({ state: 'hidden' });

  await own.getByRole('button', { name: 'Delete' }).click();
  dialog = page.getByRole('dialog', { name: 'Delete this workspace' });
  await dialog.waitFor();
  assert.match(await dialog.textContent() ?? '', /only an operator from another workspace can cancel the deletion/);
  assert.equal(sent.length, 1, 'opening the dialogs sent something');
});

// UC-INS-08 check 5: a workspace pending deletion offers *Cancel deletion*, also under a hold past
// its deadline; one press calls it off. Once the removal has begun the row says so and offers
// nothing, which the server would refuse.
test('chromium: a pending deletion is called off with one press, until the removal begins', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const rows = [
    { ...workspace(), display_name: 'Leaving', status: 'PENDING_DELETION', purge_after: '2026-09-20T00:00:00Z', legal_hold: true },
    { ...workspace(), id: '01936f2a-7c1e-7000-8000-0000000034a4', slug: 'going', display_name: 'Going',
      status: 'PENDING_DELETION', purge_after: '2026-09-20T00:00:00Z', purge_started_at: '2026-09-20T00:05:00Z' },
  ];
  const { page, sent, close } = await operatorConsole(browser, { mode: 'multi', rows });
  t.after(close);

  const going = page.getByRole('row').filter({ hasText: 'Going' });
  await going.waitFor();
  assert.match(await going.textContent() ?? '', /the deletion can no longer be called off/);
  assert.equal(await going.getByRole('button', { name: 'Cancel deletion' }).count(), 0);

  const leaving = page.getByRole('row').filter({ hasText: 'Leaving' });
  await leaving.getByRole('button', { name: 'Cancel deletion' }).click();
  await leaving.getByRole('button', { name: 'Suspend' }).waitFor();
  assert.deepEqual(sent, [`${TENANT}:cancel-deletion`]);
  assert.match(await leaving.textContent() ?? '', /Active/);
});

// UC-INS-07 check 4: in single mode the one workspace is the installation - no *New workspace*,
// and no *Suspend* or *Delete* either, which the server refuses there. In multi mode all three are
// there; *Resume* and *Cancel deletion* are the way back in both.
test('chromium: single mode offers no new workspace, no suspension and no deletion', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const suspended = { ...workspace(), id: '01936f2a-7c1e-7000-8000-0000000034a5', slug: 'paused', display_name: 'Paused', status: 'SUSPENDED' };

  const single = await operatorConsole(browser, { mode: 'single', rows: [workspace(), suspended] });
  t.after(single.close);
  await single.page.getByRole('row').filter({ hasText: 'acme' }).waitFor();
  assert.equal(await single.page.getByRole('button', { name: 'New workspace' }).count(), 0);
  assert.equal(await single.page.getByRole('button', { name: 'Suspend' }).count(), 0);
  assert.equal(await single.page.getByRole('button', { name: 'Delete' }).count(), 0);
  assert.equal(await single.page.getByRole('button', { name: 'Resume' }).count(), 1);

  const multi = await operatorConsole(browser, { mode: 'multi', rows: [workspace(), suspended] });
  t.after(multi.close);
  await multi.page.getByRole('button', { name: 'New workspace' }).waitFor();
  assert.equal(await multi.page.getByRole('button', { name: 'Suspend' }).count(), 1);
  assert.equal(await multi.page.getByRole('button', { name: 'Delete' }).count(), 2);
});
