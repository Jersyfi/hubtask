// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Withdrawing an offered provider, walked as an operator meets it (ADR-0076, UC-INS-11 check 5):
// the screen shows how many workspaces use the provider, announces the withdrawal for a
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

async function open(browser, answer = () => undefined) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const sent = [];
  let row = offered();
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    const answered = answer(route, request, path);
    if (answered) return answered;
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
            field_errors: [{
              path: '/confirm_count', code: 'identity_provider.withdraw_count_mismatch', params: { count: '12' },
            }],
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
  // Removal comes after the withdrawal (ADR-0077 §2): offered and used, it says so instead of acting.
  const remove = row.getByRole('button', { name: 'Remove', exact: true });
  assert.ok(await remove.isDisabled(), 'Remove acts on a provider still offered and used');
  assert.match(await row.textContent() ?? '', /Withdraw it first/);

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
  // Announced, Remove still waits - for the day, not for a withdrawal that has already been made.
  assert.ok(await remove.isDisabled(), 'Remove acts while the offer still runs');
  assert.match(await row.textContent() ?? '', /Its withdrawal is announced: it can be removed from /);
  assert.doesNotMatch(await row.textContent() ?? '', /Withdraw it first/);
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
  // Beside the field, where the dialog shows it: the count chosen by its plural, never a placeholder.
  await dialog.getByText('12 workspaces use this provider now').waitFor();
  assert.doesNotMatch(await dialog.textContent() ?? '', /\{count/);
  assert.ok(await dialog.isVisible(), 'a refused withdrawal closed the dialog');
  assert.ok(Date.parse(sent[0].body.withdraw_at) <= Date.now(), 'withdraw now named a day ahead');

  await dialog.getByLabel('Workspaces using it', { exact: true }).fill('12');
  await dialog.getByRole('button', { name: 'Withdraw now' }).click();
  await dialog.waitFor({ state: 'hidden' });
  assert.equal(sent[1].body.confirm_count, 12);
  await row.getByText('Withdrawn', { exact: true }).waitFor();
  assert.ok(await row.getByRole('button', { name: 'Keep offering it' }).isVisible());
  // Withdrawn, it may be removed - and the dialog says what that costs, which a withdrawal did not.
  assert.doesNotMatch(await row.textContent() ?? '', /Withdraw it first|can be removed from/);
  const remove = row.getByRole('button', { name: 'Remove', exact: true });
  assert.ok(await remove.isEnabled(), 'Remove waits for a provider whose offer has ended');
  await remove.click();
  const removal = page.getByRole('dialog', { name: 'Remove this provider' });
  await removal.waitFor();
  assert.match(
    await removal.textContent() ?? '',
    /deletes the provider and every connection between a person and it; offering it again does not restore them/,
  );
});

// UC-INS-11 check 4: the installation's screen says what its lock on the ways to sign in does to each
// row - on in every workspace for one admitted as "Only people invited here", each workspace's own
// switch for one admitted by domain - and, without the lock, what a lock would do.
test('chromium: each offered provider says what the lock on the ways to sign in does to it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  let rows = [
    { ...offered(), installation_lock: 'TURNS_ON' },
    {
      ...offered(), id: 'entra', display_name: 'Entra', kind: 'MICROSOFT', provisioning: 'DOMAINS',
      allowed_directories: ['contoso'], installation_lock: 'LEAVES_TO_WORKSPACE', offered_workspaces: 3,
    },
  ];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/admin/identity-providers' && request.method() === 'GET') return route.fulfill({ json: rows });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();

  await page.goto(`${served.origin}/instance/providers`);
  const mandated = page.getByRole('row').filter({ hasText: 'The platform' });
  await mandated.waitFor();
  assert.match(await mandated.textContent() ?? '', /On in every workspace: the ways to sign in are locked/);
  const byDomain = page.getByRole('row').filter({ hasText: 'Entra' });
  assert.match(await byDomain.textContent() ?? '', /Each workspace's own choice/);

  // Without the lock: the invite-only offer says what a lock would do, the other says nothing.
  rows = rows.map((row) => ({ ...row, installation_lock: 'NONE' }));
  await page.reload();
  await mandated.waitFor();
  assert.match(await mandated.textContent() ?? '', /Locking the ways to sign in with providers among them turns it on/);
  assert.equal(await byDomain.locator('[data-lock]').count(), 0);
});

// UC-INS-11 check 1: choosing Google fills the one issuer it has; choosing another provider
// takes it out again, and what somebody typed stays.
test('chromium: choosing Google fills its issuer, another provider takes it out', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/instance/providers`);
  await page.getByRole('button', { name: 'Offer a provider' }).click();
  const kind = page.getByLabel('Which provider');
  const issuer = page.getByLabel(/^Issuer/);
  await kind.selectOption('GOOGLE');
  assert.equal(await issuer.inputValue(), 'https://accounts.google.com');
  await kind.selectOption('MICROSOFT');
  assert.equal(await issuer.inputValue(), '');
  await issuer.fill('https://login.microsoftonline.com/1234/v2.0');
  await kind.selectOption('GENERIC');
  assert.equal(await issuer.inputValue(), 'https://login.microsoftonline.com/1234/v2.0');
});

// UC-INS-11 check 3, P-12: the installation's refusal of a generic provider for anyone names the
// provider in words, never by the contract's `GENERIC`.
test('chromium: a refusal about a provider names it in words', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, (route, request, path) =>
    path === '/admin/identity-providers' && request.method() === 'POST'
      ? route.fulfill({
        status: 422,
        json: {
          code: 'errors.validation', detail_code: 'identity_provider.installation_invited_only', status: 422,
          request_id: 'req_k', params: { kind: 'GENERIC' },
        },
      })
      : undefined);
  t.after(close);

  await page.goto(`${served.origin}/instance/providers`);
  await page.getByRole('button', { name: 'Offer a provider' }).click();
  await page.getByLabel('Which provider').selectOption('GENERIC');
  await page.getByLabel(/^Issuer/).fill('https://login.platform.example');
  await page.getByLabel(/^Client id/).fill('hubtask');
  await page.getByLabel(/^Client secret/).fill('s3cr3t');
  await page.getByRole('button', { name: 'Save the provider' }).click();
  const said = page.getByText(/A provider without a directory/).first();
  await said.waitFor({ timeout: 10_000 });
  assert.doesNotMatch((await page.locator('body').innerText()), /GENERIC/);
});

// #1480: offered to every workspace, a provider that admits by organisation is refused the personal
// accounts' directory as a workspace's is (identity_provider.directory_personal): said on the field
// as it is typed, and *Save* waits for it; under *Only people invited here* it may be named.
test('chromium: the personal directory is refused in the installation provider list before the save', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  let posted = 0;
  const { page, close } = await open(browser, (route, request, path) => {
    if (path !== '/admin/identity-providers' || request.method() !== 'POST') return undefined;
    posted += 1;
    return route.fulfill({ status: 201, json: { ...offered(), id: 'entra', kind: 'MICROSOFT' } });
  });
  t.after(close);

  await page.goto(`${served.origin}/instance/providers`);
  await page.getByRole('button', { name: 'Offer a provider' }).click();
  const dialog = page.locator('dialog[open]');
  await dialog.getByLabel('Which provider').selectOption('MICROSOFT');
  await dialog.getByLabel(/^Issuer/).fill('https://login.microsoftonline.com/72f988bf-86f1-41af-91ab-2d7cd011db47/v2.0');
  await dialog.getByLabel(/^Client id/).fill('hubtask');
  await dialog.getByLabel(/^Client secret/).fill('s3cr3t');
  const admission = dialog.getByLabel(/^Who this provider may admit/);
  const save = dialog.getByRole('button', { name: 'Save the provider' });
  const refusal = /is where this provider's personal accounts live/;

  await admission.selectOption('DOMAINS');
  const directories = dialog.getByLabel(/^Microsoft directories that may sign in/);
  await directories.fill(' 9188040D-6C67-4C5B-B112-36A304B66DAD , 72f988bf-86f1-41af-91ab-2d7cd011db47');
  await dialog.getByText(refusal).first().waitFor({ timeout: 5_000 });
  assert.equal(await save.isDisabled(), true, 'Save is offered for a list the server refuses');

  await admission.selectOption('INVITED_ONLY');
  assert.equal(await dialog.getByText(refusal).count(), 0, 'the sentence stands where the list only narrows');
  assert.equal(await save.isDisabled(), false);
  await save.click();
  await page.waitForFunction(() => document.querySelector('dialog[open]') === null, null, { timeout: 5_000 }).catch(() => {});
  assert.equal(posted, 1, 'the permitted list was not sent');
});
