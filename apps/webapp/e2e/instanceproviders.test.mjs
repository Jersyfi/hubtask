// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Withdrawing an offered provider, walked as an operator meets it (ADR-0076, UC-INS-11 check 5,
// SC-20): the screen shows how many workspaces use the provider, announces the withdrawal for a
// day two weeks ahead unless another is chosen, keeps offering it on request, and withdraws it now
// only with the number typed back.
//
// Against a stubbed API: what is asserted is what the screen sends and what it shows of the
// answers. Chromium only.

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

/** The installation's provider as the operator's projection answers it. */
const offered = (withdrawAt = null) => ({
  id: 'platform', scope: 'installation', issuer: 'https://login.platform.example', client_id: 'hubtask',
  display_name: 'The platform', kind: 'GENERIC', provisioning: 'INVITED_ONLY', position: 0,
  enabled: true, offered_here: false, allowed_email_domains: [], allowed_directories: [],
  created_at: '2026-09-01T00:00:00Z', version: 1, withdraw_at: withdrawAt, offered_workspaces: 12,
});

async function open(browser) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const sent = [];
  let row = offered();
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    if (path === '/admin/identity-providers' && request.method() === 'GET') return route.fulfill({ json: [row] });
    if (path === '/admin/identity-providers/platform:withdraw') {
      const body = request.postDataJSON() ?? {};
      sent.push({ path, body });
      if (body.confirm_count !== undefined && body.confirm_count !== 12) {
        return route.fulfill({
          status: 422,
          json: {
            code: 'errors.validation', detail_code: 'identity_provider.withdraw_count_mismatch', status: 422,
            request_id: 'req_w', params: { count: '12' },
            field_errors: [{ path: '/confirm_count', code: 'identity_provider.withdraw_count_mismatch' }],
          },
        });
      }
      row = offered(body.confirm_count === 12 ? new Date().toISOString() : body.withdraw_at);
      return route.fulfill({ json: row });
    }
    if (path === '/admin/identity-providers/platform:cancel-withdrawal') {
      sent.push({ path, body: request.postDataJSON() });
      row = offered();
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

test('chromium: a withdrawal shows the count, is announced for a day, and can be called off', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/instance/providers`);
  const row = page.getByRole('row').filter({ hasText: 'The platform' });
  await row.waitFor();
  // A number, never which workspaces.
  assert.match(await row.textContent() ?? '', /Workspaces using it: 12/);

  await row.getByRole('button', { name: 'Withdraw', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Withdraw this provider' });
  await dialog.waitFor();
  assert.match(await dialog.textContent() ?? '', /Workspaces using it: 12/);
  // Two weeks ahead, already filled in, and nothing earlier than tomorrow on offer.
  const day = dialog.getByLabel(/^Withdrawn on/);
  const chosen = await day.inputValue();
  const ahead = Math.round((Date.parse(`${chosen}T12:00:00Z`) - Date.now()) / 86_400_000);
  assert.ok(ahead >= 13 && ahead <= 15, `the default day is ${chosen}`);
  assert.ok((await day.getAttribute('min')) > new Date(Date.now() - 86_400_000).toISOString().slice(0, 10));

  await dialog.getByRole('button', { name: 'Withdraw on this day' }).click();
  await dialog.waitFor({ state: 'hidden' });
  assert.equal(sent.length, 1);
  assert.equal(sent[0].path, '/admin/identity-providers/platform:withdraw');
  assert.ok(Date.parse(sent[0].body.withdraw_at) > Date.now() + 12 * 86_400_000, JSON.stringify(sent[0].body));
  assert.equal(sent[0].body.confirm_count, undefined, 'an announcement asked for the count');

  // Until the day: said on the row, and undone with one press.
  await row.getByText(/^Withdrawn on /).waitFor();
  await row.getByRole('button', { name: 'Keep offering it' }).click();
  await row.getByText('Offered', { exact: true }).waitFor();
  assert.equal(sent[1].path, '/admin/identity-providers/platform:cancel-withdrawal');
});

test('chromium: withdraw now needs the number typed back, and a wrong one is refused with the right one', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/instance/providers`);
  const row = page.getByRole('row').filter({ hasText: 'The platform' });
  await row.waitFor();
  await row.getByRole('button', { name: 'Withdraw', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Withdraw this provider' });
  await dialog.waitFor();

  await dialog.getByLabel('Workspaces using it', { exact: true }).fill('11');
  await dialog.getByRole('button', { name: 'Withdraw now' }).click();
  // The server's sentence, with the number as it stands; the dialog stays for a second try.
  await page.getByText('12 workspaces use this provider now').waitFor();
  assert.ok(await dialog.isVisible(), 'a refused withdrawal closed the dialog');
  assert.ok(Date.parse(sent[0].body.withdraw_at) <= Date.now(), 'withdraw now named a day ahead');

  await dialog.getByLabel('Workspaces using it', { exact: true }).fill('12');
  await dialog.getByRole('button', { name: 'Withdraw now' }).click();
  await dialog.waitFor({ state: 'hidden' });
  assert.equal(sent[1].body.confirm_count, 12);
  await row.getByText('Withdrawn', { exact: true }).waitFor();
  assert.ok(await row.getByRole('button', { name: 'Keep offering it' }).isVisible());
});
