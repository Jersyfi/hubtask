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

import { stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const TENANT = '01936f2a-7c1e-7000-8000-0000000034a1';

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

  const free = page.getByRole('row').filter({ hasText: 'Free' });
  assert.doesNotMatch(await free.textContent() ?? '', /Legal hold/);
  assert.equal(await free.getByRole('button', { name: 'Delete' }).count(), 1);
});
