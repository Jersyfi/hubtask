// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// An administrative token is created in the web app behind a fresh proof (UC-ID-17 check 3), for
// oneself and for a service account (UC-INS-06 check 2): the server asks, the dialog takes the proof
// once, the mint is sent again with it, and the token is shown once. The proof is the person's
// creating it, never the service account's.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, MANIFEST, OWNER_GRANTS, PAGE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const SERVICE = { id: '01a0e2e0-0000-7000-8000-0000000000f1', kind: 'SERVICE', display_name: 'Shop platform', status: 'ACTIVE' };
const OWNER = { role: 'OWNER', permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS', 'DELETE_CONTAINER'] };

async function open(browser, { timezoneId, operator = true, serviceRole = undefined } = {}) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, ...(timezoneId ? { timezoneId } : {}) });
  const mints = [];
  const proofs = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') {
      // The reader is in the operator register unless a walk says otherwise: minting
      // `admin:tenants` asks the register as well as the proof.
      return route.fulfill({ json: { ...MANIFEST, roles: [OWNER], token_scopes: ['account:read', 'admin:provisioning', 'admin:tenants'], instance: { reachable: operator } } });
    }
    if (path === '/accounts/me') return route.fulfill({ json: { ...ACCOUNT, has_password: true, has_second_factor: false } });
    if (path === '/auth/service-accounts' && request.method() === 'GET') return route.fulfill({ json: [SERVICE] });
    // A walk that names the service account's role reads as an administrator: every role but OWNER.
    if (serviceRole && path === '/memberships' && new URL(request.url()).searchParams.get('scope_type') === 'TENANT') {
      return route.fulfill({ json: { ...PAGE, data: [
        { id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'ADMIN' },
        { id: 'm-svc', scope_type: 'TENANT', account_id: SERVICE.id, role: serviceRole },
      ], grantable_roles: OWNER_GRANTS.filter((role) => role !== 'OWNER') } });
    }
    if (path === '/auth/tokens' && request.method() === 'POST') {
      const proof = request.headers()['x-hubtask-step-up'] ?? null;
      const body = request.postDataJSON();
      mints.push({ proof, body });
      if (body.scopes.some((scope) => scope.startsWith('admin:')) && proof !== 'hbt_sup_tok') {
        return route.fulfill({ status: 403, json: { code: 'errors.forbidden', detail_code: 'auth.step_up_required', status: 403, params: { methods: 'PASSWORD' }, request_id: 'req_t' } });
      }
      return route.fulfill({ status: 201, json: {
        id: 't-new', name: body.name, prefix: 'hbt_pat_e2e', scopes: body.scopes, created_at: '2026-10-13T00:00:00Z',
        expires_at: body.expires_at, last_used_at: null, revoked_at: null, token: 'hbt_pat_e2e-the-whole-token',
      } });
    }
    if (path === '/auth/step-up') {
      proofs.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: { step_up_token: 'hbt_sup_tok', expires_at: '2099-01-01T00:00:00Z', method: 'PASSWORD' } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  return { page, mints, proofs, context, close: () => context.close() };
}

const INSTALLATION = 'What may this token do on the installation?';

async function mintAdministrative(page, within, choice = 'Full control') {
  await within.getByLabel(/^What it is for/).fill('Provisioning');
  await within.getByLabel(/^Expires/).fill(new Date(Date.now() + 30 * 86_400_000).toISOString().slice(0, 10));
  if (choice) await within.getByRole('group', { name: INSTALLATION }).getByLabel(choice, { exact: true }).check();
  await within.getByRole('button', { name: 'Create the token' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.waitFor();
  await dialog.getByLabel('Your password').fill('the person creating it');
  await dialog.getByRole('button', { name: 'Prove it' }).click();
  await page.getByText('Your new token').first().waitFor({ timeout: 15_000 });
}

test('chromium: an administrative token of one\'s own is created behind a fresh proof (UC-ID-17 check 3)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, mints, proofs, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/profile/tokens`);
  await page.getByRole('button', { name: 'Create the token' }).waitFor();
  await mintAdministrative(page, page.locator('main'));

  assert.deepEqual(proofs, [{ password: 'the person creating it' }], 'the proof was asked more than once, or not at all');
  assert.deepEqual(mints.map((each) => each.proof), [null, 'hbt_sup_tok']);
  assert.deepEqual(mints[1].body.scopes, ['admin:tenants']);
  assert.equal(mints[1].body.account_id, undefined, 'a token of one\'s own named an account');
});

test('chromium: a service account\'s administrative token is created behind the person\'s fresh proof (UC-INS-06 check 2)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, mints, proofs, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/administration/service-accounts`);
  const section = page.locator('section', { hasText: SERVICE.display_name });
  await section.getByRole('button', { name: 'Its tokens' }).click();
  await section.getByRole('button', { name: 'Create the token' }).waitFor();
  await mintAdministrative(page, section);

  assert.deepEqual(proofs, [{ password: 'the person creating it' }]);
  assert.deepEqual(mints.map((each) => each.proof), [null, 'hbt_sup_tok']);
  assert.equal(mints[1].body.account_id, SERVICE.id, 'the token was not minted for the service account');
  assert.deepEqual(mints[1].body.scopes, ['admin:tenants']);
});

test('chromium: an expiry typed as a day is that day where the person is (UC-ID-17)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, mints, close } = await open(browser, { timezoneId: 'Europe/Berlin' });
  t.after(close);

  await page.goto(`${served.origin}/profile/tokens`);
  await page.getByRole('button', { name: 'Create the token' }).waitFor();
  // A day well inside the year the server allows, the same distance ahead whenever this runs.
  const day = new Date(Date.now() + 40 * 86_400_000).toISOString().slice(0, 10);
  await page.getByLabel(/^What it is for/).fill('Nightly export');
  await page.getByLabel(/^Expires/).fill(day);
  await page.getByLabel('account:read', { exact: true }).check();
  await page.getByRole('button', { name: 'Create the token' }).click();
  await page.getByText('Your new token').first().waitFor({ timeout: 15_000 });

  // "Nothing" is preselected on one's own panel: an ordinary token stays one.
  assert.deepEqual(mints.at(-1).body.scopes, ['account:read']);
  const sent = mints.at(-1).body.expires_at;
  // The last second of that day in Berlin, which is still that day on a Berlin clock - not
  // 23:59:59 UTC, which is the next day there.
  const onBerlinClock = new Intl.DateTimeFormat('en-CA', { timeZone: 'Europe/Berlin', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(new Date(sent));
  assert.equal(onBerlinClock, `${day}, 23:59:59`, `the expiry ${sent} is not the end of ${day} in Berlin`);
  // And the date control offers no day the server would refuse: the last is 364 days ahead.
  const max = await page.getByLabel(/^Expires/).getAttribute('max');
  assert.ok(Date.parse(`${max}T23:59:59+01:00`) <= Date.now() + 365 * 86_400_000 + 3_600_000, `the control offers ${max}, past the year`);
});

test('chromium: a service account\'s token panel speaks about the service account', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/administration/service-accounts`);
  const section = page.locator('section', { hasText: SERVICE.display_name });
  await section.getByRole('button', { name: 'Its tokens' }).click();
  await section.getByRole('button', { name: 'Create the token' }).waitFor();
  assert.equal(await section.getByText('This service account has no tokens.').count(), 1);
  assert.equal(await section.getByText('You have no tokens.').count(), 0, 'the panel told the administrator they have no tokens');
});

// #1331: minting `admin:tenants` or `admin:provisioning` asks the operator register as well as a fresh proof, so somebody the
// register does not name - a workspace's owner on an installation run by somebody else - is not
// offered the scope (P-05): on their own panel nor on a service account's. Every other scope stays.
test('chromium: the operator scope is offered only to somebody the operator register names', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, { operator: false });
  t.after(close);

  await page.goto(`${served.origin}/profile/tokens`);
  const main = page.locator('main');
  await main.getByLabel('account:read', { exact: true }).waitFor({ timeout: 10_000 });
  assert.equal(await main.getByLabel('admin:tenants', { exact: true }).count(), 0, 'the operator scope is offered outside the register');
  assert.equal(await main.getByLabel('admin:provisioning', { exact: true }).count(), 0, 'the platform\'s scope is offered outside the register');
  assert.equal(await main.getByRole('group', { name: INSTALLATION }).count(), 0, 'the installation choice is drawn outside the register');

  await page.goto(`${served.origin}/administration/service-accounts`);
  const section = page.locator('section', { hasText: SERVICE.display_name });
  await section.getByRole('button', { name: 'Its tokens' }).click();
  await section.getByLabel('account:read', { exact: true }).waitFor({ timeout: 10_000 });
  assert.equal(await section.getByLabel('admin:tenants', { exact: true }).count(), 0, 'the operator scope is offered on a service account\'s panel outside the register');
  assert.equal(await section.getByLabel('admin:provisioning', { exact: true }).count(), 0, 'the platform\'s scope is offered on a service account\'s panel outside the register');
});

// UC-INS-06 check 6 on the screen (the preview the owner approved, with its "Nothing" of 2026-10-18):
// the installation's two scopes are one choice. A person's own panel preselects nothing; a service
// account's preselects "Provisioning only", and that token is minted with the narrow scope alone,
// behind the person's proof.
test('chromium: the installation choice offers provisioning or full control, preselected by the panel', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, mints, proofs, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/profile/tokens`);
  const own = page.locator('main').getByRole('group', { name: INSTALLATION });
  await own.waitFor({ timeout: 10_000 });
  assert.deepEqual(await own.getByRole('radio').evaluateAll((radios) => radios.map((radio) => radio.labels[0].textContent.trim())),
    ['Nothing', 'Provisioning only', 'Full control']);
  assert.equal(await own.getByLabel('Nothing', { exact: true }).isChecked(), true, 'one\'s own panel preselects an installation scope');
  assert.equal(await page.locator('main').getByLabel('admin:provisioning', { exact: true }).count(), 0, 'the narrow scope is a checkbox as well');
  // EVIDENCE=<dir> keeps what the person sees, for the owner's look (docs/evidence/).
  if (process.env.EVIDENCE) await page.locator('main form').screenshot({ path: join(process.env.EVIDENCE, 'own.png') });

  await page.goto(`${served.origin}/administration/service-accounts`);
  const section = page.locator('section', { hasText: SERVICE.display_name });
  await section.getByRole('button', { name: 'Its tokens' }).click();
  const choice = section.getByRole('group', { name: INSTALLATION });
  await choice.waitFor();
  assert.equal(await choice.getByLabel('Provisioning only', { exact: true }).isChecked(), true,
    'a service account\'s panel does not preselect provisioning');
  if (process.env.EVIDENCE) await section.locator('form').screenshot({ path: join(process.env.EVIDENCE, 'service-account.png') });
  await mintAdministrative(page, section, null);

  assert.deepEqual(proofs, [{ password: 'the person creating it' }]);
  assert.deepEqual(mints.map((each) => each.proof), [null, 'hbt_sup_tok']);
  assert.deepEqual(mints[1].body.scopes, ['admin:provisioning']);
  assert.equal(mints[1].body.account_id, SERVICE.id);
});

// A service account's token is held to the ceiling (identity.md §13): an administrator is offered no
// mint for one that holds OWNER, and is told why; for one that holds MEMBER the form is there.
test('chromium: an administrator is offered no token for a service account that holds OWNER', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  for (const [serviceRole, offered] of [['OWNER', false], ['MEMBER', true]]) {
    const { page, close } = await open(browser, { serviceRole });
    t.after(close);
    await page.goto(`${served.origin}/administration/service-accounts`);
    const section = page.locator('section', { hasText: SERVICE.display_name });
    await section.getByText(serviceRole === 'OWNER' ? 'Owner' : 'Member', { exact: true }).waitFor({ timeout: 15_000 });
    await section.getByRole('button', { name: 'Its tokens' }).click();
    await section.getByText('This service account has no tokens.').waitFor();
    assert.equal(await section.getByRole('button', { name: 'Create the token' }).count(), offered ? 1 : 0, serviceRole);
    assert.equal(await section.getByText('This service account holds a role above yours').count(), offered ? 0 : 1, serviceRole);
    if (process.env.EVIDENCE) {
      await section.screenshot({ path: join(process.env.EVIDENCE, `service-account-${serviceRole.toLowerCase()}.png`) });
    }
  }
});
