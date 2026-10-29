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
import { chooseInRow, pageOffers, rowMenu, rowOffers } from './rowmenu.mjs';

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

  await chooseInRow(row, 'Open the password');
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
  assert.equal(await rowOffers(row, 'Open the password'), 0);
  await chooseInRow(row, 'Close the password');
  await row.getByText('Password open', { exact: true }).waitFor({ state: 'detached' });
  assert.equal(sent[2].path, `/admin/tenants/${TENANT}:close-password`);
  assert.equal(await rowOffers(row, 'Open the password'), 1);
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
  assert.equal(await rowOffers(held, 'Delete'), 0);

  const leaving = page.getByRole('row').filter({ hasText: 'Leaving' });
  assert.match(await leaving.textContent() ?? '', /the deletion waits until every hold is released/);
  // Nobody can release a hold while the workspace is being deleted: the row names the way out.
  assert.match(await leaving.textContent() ?? '', /cancel the deletion first/);

  const free = page.getByRole('row').filter({ hasText: 'Free' });
  assert.doesNotMatch(await free.textContent() ?? '', /Legal hold/);
  assert.equal(await rowOffers(free, 'Delete'), 1);
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
  await chooseInRow(customer, 'Suspend');
  let dialog = page.getByRole('dialog', { name: 'Suspend Acme?' });
  await dialog.waitFor();
  assert.match(await dialog.textContent() ?? '', /Nothing in it is lost/);
  assert.doesNotMatch(await dialog.textContent() ?? '', /your own workspace/i);
  await dialog.getByRole('button', { name: 'Keep it running' }).click();
  await dialog.waitFor({ state: 'hidden' });
  assert.deepEqual(sent, []);

  await chooseInRow(customer, 'Suspend');
  await dialog.getByRole('button', { name: 'Suspend it' }).click();
  await dialog.waitFor({ state: 'hidden' });
  assert.deepEqual(sent, [`${TENANT}:suspend`]);

  // The own workspace: the same dialog, and the warning that the session ends.
  await chooseInRow(own, 'Suspend');
  dialog = page.getByRole('dialog', { name: `Suspend ${WORKSPACE.display_name}?` });
  await dialog.waitFor();
  assert.match(await dialog.textContent() ?? '', /This is your own workspace. Your session ends at once/);
  await dialog.getByRole('button', { name: 'Keep it running' }).click();
  await dialog.waitFor({ state: 'hidden' });

  await chooseInRow(own, 'Delete');
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
  assert.equal(await rowOffers(going, 'Cancel deletion'), 0);

  const leaving = page.getByRole('row').filter({ hasText: 'Leaving' });
  await chooseInRow(leaving, 'Cancel deletion');
  await leaving.getByText('Active').waitFor();
  assert.deepEqual(sent, [`${TENANT}:cancel-deletion`]);
  assert.equal(await rowOffers(leaving, 'Suspend'), 1);
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
  assert.equal(await pageOffers(single.page, 'Suspend'), 0);
  assert.equal(await pageOffers(single.page, 'Delete'), 0);
  assert.equal(await pageOffers(single.page, 'Resume'), 1);

  const multi = await operatorConsole(browser, { mode: 'multi', rows: [workspace(), suspended] });
  t.after(multi.close);
  await multi.page.getByRole('button', { name: 'New workspace' }).waitFor();
  assert.equal(await pageOffers(multi.page, 'Suspend'), 1);
  assert.equal(await pageOffers(multi.page, 'Delete'), 2);
});

// UC-INS-07 check 2: a short name the installation reserves for itself is refused at the field of
// *New workspace*, with its own sentence, and the dialog stays for another.
test('chromium: a reserved short name is refused at the field of New workspace', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  const sent = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: MULTI });
    if (path === '/admin/tenants' && request.method() === 'GET') return route.fulfill({ json: [workspace()] });
    if (path === '/admin/tenants' && request.method() === 'POST') {
      const body = request.postDataJSON() ?? {};
      sent.push(body);
      return route.fulfill({
        status: 422,
        json: {
          code: 'errors.validation', detail_code: 'admin.slug_reserved', status: 422,
          request_id: 'req_r', params: { value: body.slug },
          field_errors: [{ path: '/slug', code: 'admin.slug_reserved' }],
        },
      });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();

  await page.goto(`${served.origin}/instance/workspaces`);
  await page.getByRole('row').filter({ hasText: 'Acme' }).waitFor();
  await page.getByRole('button', { name: 'New workspace' }).click();
  const dialog = page.getByRole('dialog', { name: 'New workspace' });
  await dialog.waitFor();

  await dialog.getByLabel(/^Address/).fill('www');
  await dialog.getByLabel(/^Name/).fill('Web');
  await dialog.getByLabel(/^The owner's email address/).fill('eva@acme.example');
  await dialog.getByRole('button', { name: 'Create it' }).click();

  await dialog.getByText('This short name is reserved for the installation itself. Choose another.').waitFor();
  assert.equal(await dialog.getByLabel(/^Address/).getAttribute('aria-invalid'), 'true');
  assert.ok(await dialog.isVisible(), 'a refused short name closed the dialog');
  assert.equal(sent.length, 1);
  assert.equal(sent[0].slug, 'www');
});

// UC-INS-07 check 2: a short name the list did not know as taken - another operator took it a
// moment ago - is the server's refusal, said at the field of *New workspace*, and the dialog stays.
test('chromium: a short name taken meanwhile is refused at the field of New workspace', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  const sent = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: MULTI });
    if (path === '/admin/tenants' && request.method() === 'GET') return route.fulfill({ json: [workspace()] });
    if (path === '/admin/tenants' && request.method() === 'POST') {
      const body = request.postDataJSON() ?? {};
      sent.push(body);
      return route.fulfill({
        status: 409,
        json: {
          code: 'errors.conflict', detail_code: 'admin.slug_taken', status: 409,
          request_id: 'req_t', params: { slug: body.slug },
          field_errors: [{ path: '/slug', code: 'admin.slug_taken', params: { slug: body.slug } }],
        },
      });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();

  await page.goto(`${served.origin}/instance/workspaces`);
  await page.getByRole('row').filter({ hasText: 'Acme' }).waitFor();
  await page.getByRole('button', { name: 'New workspace' }).click();
  const dialog = page.getByRole('dialog', { name: 'New workspace' });
  await dialog.waitFor();

  await dialog.getByLabel(/^Address/).fill('sales');
  await dialog.getByLabel(/^Name/).fill('Sales');
  await dialog.getByLabel(/^The owner's email address/).fill('eva@acme.example');
  await dialog.getByRole('button', { name: 'Create it' }).click();

  await dialog.getByText("That subdomain is already a workspace's. Choose another.").waitFor();
  assert.equal(await dialog.getByLabel(/^Address/).getAttribute('aria-invalid'), 'true');
  assert.ok(await dialog.isVisible(), 'a refused short name closed the dialog');
  assert.equal(sent.length, 1);
  assert.equal(sent[0].slug, 'sales');
});

// UC-INS-20 checks 1 and 3: where the installation assigns hosts, *New workspace* asks for the
// workspace's host and offers the short name as optional, made from the host where it is left
// empty. A host already in the table is said while it is typed and nothing is sent; one the server
// refuses is said at the field with its own reason; no DNS is asked. With subdomains the dialog
// asks for the short name as before, and no longer says it cannot be changed.
test('chromium: New workspace asks for the host where the installation assigns hosts', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  const sent = [];
  let mode = 'assigned';
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MULTI, workspace_hosts: mode } });
    if (path === '/admin/tenants' && request.method() === 'GET') {
      return route.fulfill({ json: [{ ...workspace(), slug: 'sales', display_name: 'Sales', host: 'tasks-sales.corp.example' }] });
    }
    if (path === '/admin/tenants' && request.method() === 'POST') {
      const body = request.postDataJSON() ?? {};
      sent.push(body);
      if (body.host === 'tasks.corp.example') {
        return route.fulfill({
          status: 422,
          json: {
            code: 'errors.validation', detail_code: 'admin.host_installation', status: 422,
            request_id: 'req_h', params: { host: body.host },
            field_errors: [{ path: '/host', code: 'admin.host_installation', params: { host: body.host } }],
          },
        });
      }
      return route.fulfill({
        status: 201,
        json: {
          ...workspace(), id: '01936f2a-7c1e-7000-8000-0000000034b1', slug: 'tasks-hr', host: body.host,
          display_name: body.display_name, owner_account_id: '01936f2a-7c1e-7000-8000-0000000034b2',
          owner_redemption_token: 'hbt_rdm_e2e', default_hub_id: '01936f2a-7c1e-7000-8000-0000000034b3',
          example_collection_id: '01936f2a-7c1e-7000-8000-0000000034b4',
        },
      });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();

  await page.goto(`${served.origin}/instance/workspaces`);
  await page.getByRole('row').filter({ hasText: 'Sales' }).waitFor();
  await page.getByRole('button', { name: 'New workspace' }).click();
  const dialog = page.getByRole('dialog', { name: 'New workspace' });
  await dialog.waitFor();

  // The host, not a subdomain; nothing about DNS is asked, only said.
  assert.equal(await dialog.getByLabel(/^Address/).count(), 0);
  const host = dialog.getByLabel(/^Host/);
  const shortName = dialog.getByLabel(/^Short name/);
  assert.equal(await host.getAttribute('required'), '');
  assert.equal(await shortName.getAttribute('required'), null);
  assert.match(await dialog.textContent() ?? '', /Hubtask checks no DNS for it/);
  assert.match(await dialog.textContent() ?? '', /Left empty, it is made from the host/);

  // A host in the table is said while it is typed, and pressing sends nothing.
  await host.fill('Tasks-Sales.corp.example');
  await dialog.getByText('A workspace already answers at tasks-sales.corp.example.').waitFor();
  await dialog.getByLabel(/^Name/).fill('Human Resources');
  await dialog.getByLabel(/^The owner's email address/).fill('jana.koch@corp.example');
  await dialog.getByRole('button', { name: 'Create it' }).click();
  assert.equal(sent.length, 0);

  // The installation's own host is the server's refusal, at the field.
  await host.fill('tasks.corp.example');
  await dialog.getByRole('button', { name: 'Create it' }).click();
  await dialog.getByText('tasks.corp.example is this installation\'s own address. A workspace needs a host of its own.').waitFor();
  assert.equal(await host.getAttribute('aria-invalid'), 'true');
  assert.ok(await dialog.isVisible(), 'a refused host closed the dialog');

  // The short name the host makes is shown before it is made, and left to the server.
  await host.fill('tasks-hr.corp.example');
  assert.equal(await shortName.getAttribute('placeholder'), 'tasks-hr');
  await dialog.getByRole('button', { name: 'Create it' }).click();
  await page.getByText("The owner's token").first().waitFor();
  assert.deepEqual(sent[1], {
    host: 'tasks-hr.corp.example', display_name: 'Human Resources', owner_email: 'jana.koch@corp.example',
  });

  // With subdomains: the short name, and the hint without "cannot be changed".
  mode = 'subdomain';
  const subdomains = await context.newPage();
  await subdomains.goto(`${served.origin}/instance/workspaces`);
  await subdomains.getByRole('row').filter({ hasText: 'Sales' }).waitFor();
  await subdomains.getByRole('button', { name: 'New workspace' }).click();
  const plain = subdomains.getByRole('dialog', { name: 'New workspace' });
  await plain.waitFor();
  assert.equal(await plain.getByLabel(/^Host/).count(), 0);
  await plain.getByLabel(/^Address/).waitFor();
  assert.match(await plain.textContent() ?? '', /It is the start of the workspace's address, such as meier.tasks.example.net/);
  assert.doesNotMatch(await plain.textContent() ?? '', /cannot be changed/);
});

// UC-INS-19 checks 4, 5 and 6, UC-INS-20 check 4: the operator changes a workspace's address. The
// list shows each address; the dialog names the new address, what the change ends and who is told
// before anything is changed; a lockout is refused in the dialog before a proof is asked; the proof
// is the password, with the note why a passkey is not offered.
test('chromium: Change address names the new address and its costs, and is confirmed by the password', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  const sent = [];
  let lockout = true;
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MULTI, workspace_hosts: 'subdomain' } });
    if (path === '/admin/tenants' && request.method() === 'GET') {
      return route.fulfill({
        json: [
          { ...workspace(), slug: 'meier', display_name: 'Meier family', host: 'meier.tasks.example.net' },
          { ...workspace(), id: '01936f2a-7c1e-7000-8000-0000000034c1', slug: 'taken', display_name: 'Taken', host: 'taken.tasks.example.net' },
        ],
      });
    }
    if (path === '/auth/step-up') {
      return route.fulfill({ status: 201, json: { step_up_token: 'hbt_sup_tok', expires_at: '2099-01-01T00:00:00Z', method: 'PASSWORD' } });
    }
    if (path === `/admin/tenants/${TENANT}:change-address`) {
      const body = request.postDataJSON() ?? {};
      const proof = request.headers()['x-hubtask-step-up'] ?? null;
      sent.push({ body, proof });
      const tenant = { ...workspace(), slug: body.slug ?? 'meier', display_name: 'Meier family', host: body.slug ? `${body.slug}.tasks.example.net` : 'meier.tasks.example.net' };
      const answer = { tenant, from: 'meier.tasks.example.net', members_mailed: 6, members_told_at_sign_in: 1, dry_run: body.dry_run === true };
      if (body.slug) answer.to = `${body.slug}.tasks.example.net`;
      if (body.slug && lockout) {
        return route.fulfill({ status: 409, json: { code: 'errors.conflict', detail_code: 'admin.address_lockout_owner', status: 409, params: { reason: 'way_in' }, request_id: 'req_l' } });
      }
      if (body.slug && !body.dry_run && !proof) {
        return route.fulfill({ status: 403, json: { code: 'errors.forbidden', detail_code: 'auth.step_up_required', status: 403, params: { methods: 'PASSWORD', purpose: 'change_address' }, request_id: 'req_s' } });
      }
      return route.fulfill({ json: answer });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();

  await page.goto(`${served.origin}/instance/workspaces`);
  const row = page.getByRole('row').filter({ hasText: 'Meier family' });
  await row.waitFor();
  // The address line under the name.
  await row.getByText('meier.tasks.example.net').waitFor();
  await chooseInRow(row, 'Change address');
  const dialog = page.getByRole('dialog', { name: 'Change the address of Meier family' });
  await dialog.waitFor();

  // Before anything is changed: the new address, what ends, who is told.
  const field = dialog.getByLabel(/^New address/);
  await field.fill('familie-meier');
  const text = async () => (await dialog.textContent()) ?? '';
  await dialog.getByText('familie-meier.tasks.example.net').waitFor();
  await dialog.getByText('The 6 members with an email address are mailed the old and the new address.', { exact: false }).waitFor();
  assert.match(await text(), /\.tasks\.example\.net/);
  assert.match(await text(), /What the change ends/);
  assert.match(await text(), /meier\.tasks\.example\.net stops answering at once, without sending anybody on/);
  assert.match(await text(), /Everybody signs in again, at the new address\./);
  assert.match(await text(), /Passkeys made at the old address stop working\./);
  assert.match(await text(), /Links, calendar subscriptions, MCP and hubctl set up on the old address stop working/);
  assert.match(await text(), /The 1 without one is told at their next sign-in\./);
  assert.ok(sent.every((each) => each.body.dry_run === true), 'something was changed before it was confirmed');

  // A name another workspace answers at is said at the field.
  await field.fill('taken');
  await dialog.getByText('taken', { exact: false }).first().waitFor();
  const before = sent.length;
  await dialog.getByRole('button', { name: 'Change address' }).click();
  assert.equal(sent.length, before, 'a taken name was sent');

  // The last owner's lockout is refused in the dialog, before any proof.
  await field.fill('familie-meier');
  await dialog.getByRole('button', { name: 'Change address' }).click();
  await dialog.getByText("This would lock out the workspace's last owner").waitFor();
  assert.match(await text(), /The only owner signs in with a passkey alone/);
  assert.equal(await page.getByRole('dialog', { name: 'Prove it is you' }).count(), 0);

  // Confirmed by the password, with the note why a passkey is not offered.
  lockout = false;
  await dialog.getByRole('button', { name: 'Change address' }).click();
  const prove = page.getByRole('dialog', { name: 'Prove it is you' });
  await prove.waitFor();
  assert.match((await prove.textContent()) ?? '', /A passkey belongs to the old address, so it cannot confirm this\./);
  assert.equal(await prove.getByRole('button', { name: 'Use a passkey or security key' }).count(), 0);
  await prove.getByLabel('Your password').fill('the operator');
  await prove.getByRole('button', { name: 'Prove it' }).click();
  await dialog.waitFor({ state: 'hidden' });
  const last = sent.at(-1);
  assert.deepEqual(last, { body: { slug: 'familie-meier' }, proof: 'hbt_sup_tok' });
});
