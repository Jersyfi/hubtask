// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The own domain as the main address, and back (UC-INS-15 checks 6, 7, 8 and 9), walked as the
// owner and the operator meet it: *Make it the main address* names what the change ends and what
// stays and who is told, asks for the password, and sends the browser on to the domain; the
// Address screen then names the subdomain beside it and the way back, which asks the same; after a
// break the workspace's administration and the installation's overview warn, and the operator's
// block of a main domain says that the subdomain becomes main in the same step.
//
// Against a stubbed API: what is asserted is what the screens send and what they show of the
// answers. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { MANIFEST, WORKSPACE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';
import { chooseInRow } from './rowmenu.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const MULTI = { ...MANIFEST, tenancy_mode: 'multi', workspace_hosts: 'subdomain' };
const TENANT = '01936f2a-7c1e-7000-8000-0000000212b1';
const SUB = 'acme.tasks.example.net';
const OWN = 'tasks.acme.com';
const FELL = '2026-10-27T09:10:00Z';

const own = (extra = {}) => ({
  domain: OWN, state: 'ACTIVE', since: '2026-10-20T08:00:00Z', is_main: false, certificate_asked: true, records: [], ...extra,
});

const address = (extra = {}) => ({
  host: SUB, owners_may_change: false, may_change: false, own_domains_allowed: true, may_manage_own_domain: true,
  own_domain: own(), ...extra,
});

const elevated = (route) => route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });

async function signedIn(browser, routes, width = 1280) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: MULTI });
    if (path === '/auth/sessions:elevate') return elevated(route);
    if (path === '/auth/step-up') {
      return route.fulfill({ status: 201, json: { step_up_token: 'hbt_sup_tok', expires_at: '2099-01-01T00:00:00Z', method: 'PASSWORD' } });
    }
    // A route this walk answers hands back the fulfilment's promise; any other is the fixture's.
    const answered = routes(route, request, path);
    if (answered !== undefined) return answered;
    return stub(route);
  });
  const arrived = [];
  await context.route((url) => url.hostname === OWN || url.hostname === SUB, (route) => {
    arrived.push(route.request().url());
    return route.fulfill({ contentType: 'text/html', body: '<!doctype html><title>Sign in</title>' });
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  return { context, page: await context.newPage(), arrived };
}

/** The Address screen with `current`, answering the two moves; what each move was sent. */
async function addressScreen(browser, current, width = 1280) {
  const sent = [];
  const opened = await signedIn(browser, (route, request, path) => {
    if (path === '/tenant' && request.method() === 'GET') {
      return route.fulfill({ json: { ...WORKSPACE, slug: 'acme', display_name: 'Acme Ltd', address: current } });
    }
    if (path === '/tenant/own-domain:make-main' || path === '/tenant/own-domain:switch-back') {
      const body = request.postDataJSON() ?? {};
      const proof = request.headers()['x-hubtask-step-up'] ?? null;
      sent.push({ path, body, proof });
      if (!body.dry_run && !proof) {
        return route.fulfill({ status: 403, json: { code: 'errors.forbidden', detail_code: 'auth.step_up_required', status: 403, params: { methods: 'PASSWORD', purpose: 'change_address' }, request_id: 'req_m' } });
      }
      const toOwn = path.endsWith('make-main');
      return route.fulfill({ json: { from: toOwn ? SUB : OWN, to: toOwn ? OWN : SUB, members_mailed: 23, members_told_at_sign_in: 2, dry_run: body.dry_run === true } });
    }
    return undefined;
  }, width);
  await opened.page.goto(`${served.origin}/administration/address`);
  await opened.page.getByText('Main address', { exact: true }).waitFor();
  return { ...opened, sent };
}

test('chromium: the owner makes the active domain the main address, told what it costs, with the password', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, sent, arrived } = await addressScreen(browser, address());
  t.after(() => context.close());

  const panel = page.locator('[data-own-domain="ACTIVE"]');
  await panel.getByRole('button', { name: 'Make it the main address' }).click();
  const dialog = page.getByRole('dialog', { name: `Make ${OWN} the main address` });
  await dialog.waitFor();
  await dialog.getByText('The 23 members with an email address are mailed the new address.', { exact: false }).waitFor();
  const text = (await dialog.textContent()) ?? '';
  for (const said of [
    'What the change ends', `Everybody signs in again, at ${OWN}.`,
    `Passkeys made at ${SUB} stop working. They stay listed in each person's settings, where they can be removed.`,
    'What stays', `${SUB} sends browsers on to ${OWN}, so links already sent keep working, and apps, MCP and hubctl set up there keep working.`,
    'It stays your way back: you can make it the main address again at any time, at the same cost.',
    'The 2 without one are told at their next sign-in.', `Every link Hubtask sends from now on uses ${OWN}.`,
  ]) assert.ok(text.includes(said), `"${said}" is not said in ${text}`);
  assert.ok(sent.every((each) => each.body.dry_run === true), 'the main address moved before it was confirmed');

  await dialog.getByRole('button', { name: 'Make it the main address' }).click();
  const prove = page.getByRole('dialog', { name: 'Prove it is you' });
  await prove.waitFor();
  await prove.getByLabel('Your password').fill('the owner');
  await prove.getByRole('button', { name: 'Prove it' }).click();
  await page.waitForURL((url) => url.hostname === OWN);
  assert.deepEqual(sent.at(-1), { path: '/tenant/own-domain:make-main', body: {}, proof: 'hbt_sup_tok' });
  assert.equal(new URL(arrived.at(-1)).pathname, '/');
});

test('chromium: the own domain is the main address; the subdomain beside it is the way back, at the same cost', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const main = address({ host: OWN, address_host: SUB, own_domain: own({ is_main: true }) });
  const { context, page, sent, arrived } = await addressScreen(browser, main);
  t.after(() => context.close());

  const panel = page.locator('[data-main="own"]');
  await panel.waitFor();
  const text = (await panel.textContent()) ?? '';
  for (const said of [
    'Own domain · Active', OWN, 'People reach this workspace here, and every link Hubtask sends uses it.', 'Also', SUB,
    `Sends browsers on to ${OWN} and still answers apps. It is your way back if the domain fails.`,
  ]) assert.ok(text.includes(said), `"${said}" is not said in ${text}`);
  assert.equal(await page.getByRole('heading', { name: 'Use your own domain' }).count(), 0, 'the own domain is drawn twice');

  await panel.getByRole('button', { name: `Make ${SUB} the main address again` }).click();
  const dialog = page.getByRole('dialog', { name: `Make ${SUB} the main address again` });
  await dialog.waitFor();
  const said = (await dialog.textContent()) ?? '';
  assert.match(said, new RegExp(`Passkeys made at ${OWN.replaceAll('.', '\\.')} stop working\\.`));
  assert.ok(said.includes(`${OWN} stays set up and sends browsers on to ${SUB}, so you can make it the main address again later.`), said);
  await dialog.getByRole('button', { name: 'Make it the main address' }).click();
  const prove = page.getByRole('dialog', { name: 'Prove it is you' });
  await prove.waitFor();
  await prove.getByLabel('Your password').fill('the owner');
  await prove.getByRole('button', { name: 'Prove it' }).click();
  await page.waitForURL((url) => url.hostname === SUB);
  assert.deepEqual(sent.at(-1), { path: '/tenant/own-domain:switch-back', body: {}, proof: 'hbt_sup_tok' });
  assert.equal(new URL(arrived.at(-1)).pathname, '/');
});

test('chromium: 375 px — the main own domain and its way back fit without the page scrolling sideways', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const main = address({ host: OWN, address_host: SUB, own_domain: own({ is_main: true }) });
  const { context, page } = await addressScreen(browser, main, 375);
  t.after(() => context.close());
  await page.locator('[data-main="own"]').waitFor();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(overflow <= 0, `the page scrolls sideways by ${overflow} px`);
});

test('chromium: after a break the workspace administration warns until the domain is main again', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const cases = [
    [own({ state: 'BROKEN', fell_back_at: FELL }), 'broken',
      `Its proof record could no longer be found on October 27, 2026 at`, `so ${SUB} became the main address again by itself. Everybody signs in there for now. Put the record back, then make the domain the main address again.`],
    [own({ state: 'BLOCKED', is_main: true, since: FELL, blocked_reason: 'A look-alike.' }), 'blocked',
      "This installation's operator blocked it on October 27, 2026", `so ${SUB} became the main address again by itself.`],
  ];
  for (const [domain, cause, ...says] of cases) {
    const { context, page } = await signedIn(browser, (route, request, path) => {
      if (path === '/tenant' && request.method() === 'GET') {
        return route.fulfill({ json: { ...WORKSPACE, slug: 'acme', display_name: 'Acme Ltd', address: address({ own_domain: domain }) } });
      }
      return undefined;
    });
    await page.goto(`${served.origin}/administration`);
    const warning = page.locator(`[data-fell-back="${cause}"]`);
    await warning.waitFor();
    const text = (await warning.textContent()) ?? '';
    assert.ok(text.includes(`${OWN} is no longer the main address`), text);
    for (const said of says) assert.ok(text.includes(said), `${cause}: "${said}" is not said in ${text}`);
    await warning.getByRole('button', { name: 'Address' }).click();
    await page.waitForURL((url) => url.pathname === '/administration/address');
    await context.close();
  }

  // Made main again, nothing warns.
  const { context, page } = await signedIn(browser, (route, request, path) => {
    if (path === '/tenant' && request.method() === 'GET') {
      return route.fulfill({ json: { ...WORKSPACE, slug: 'acme', display_name: 'Acme Ltd', address: address({ host: OWN, address_host: SUB, own_domain: own({ is_main: true }) }) } });
    }
    return undefined;
  });
  t.after(() => context.close());
  await page.goto(`${served.origin}/administration`);
  await page.getByTestId('admin-overview').waitFor();
  assert.equal(await page.locator('[data-fell-back]').count(), 0);
});

test('chromium: the installation overview warns of a fallen-back domain, and the block of a main one says it falls back', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const domains = [
    { domain: OWN, tenant_id: TENANT, tenant_name: 'Acme Ltd', state: 'ACTIVE', since: '2026-10-20T08:00:00Z', is_main: true, address_host: SUB },
    { domain: 'tasks.meier-familie.de', tenant_id: TENANT, tenant_name: 'Meier family', state: 'BROKEN', since: FELL, is_main: false, fell_back_at: FELL, address_host: 'meier.tasks.example.net' },
  ];
  const { context, page } = await signedIn(browser, (route, request, path) => {
    if (path === '/admin/overview') {
      return route.fulfill({ json: { workspaces_active: 2, workspaces_suspended: 0, workspaces_pending_deletion: 0, accounts_active: 4, accounts_total: 4, workspaces_service_only: 0 } });
    }
    if (path === '/admin/own-domains') return route.fulfill({ json: domains });
    if (path === '/admin/tenants' && request.method() === 'GET') {
      return route.fulfill({ json: [{ id: TENANT, slug: 'acme', display_name: 'Acme Ltd', status: 'ACTIVE', default_locale: 'en', default_time_zone: 'UTC', created_at: '2026-09-01T00:00:00Z', purge_after: null }] });
    }
    return undefined;
  });
  t.after(() => context.close());

  await page.goto(`${served.origin}/instance`);
  const warning = page.locator('[data-fell-back="tasks.meier-familie.de"]');
  await warning.waitFor();
  const text = (await warning.textContent()) ?? '';
  assert.ok(text.includes("An own domain fell back to its workspace's subdomain"), text);
  assert.ok(text.includes('tasks.meier-familie.de of Meier family, broken since October 27, 2026 at'), text);
  assert.ok(text.includes('The workspace answers at meier.tasks.example.net.'), text);
  assert.equal(await page.locator(`[data-fell-back="${OWN}"]`).count(), 0, 'a serving main domain warns');

  await warning.getByRole('button', { name: 'Own domains' }).click();
  await page.getByRole('heading', { name: 'Own domains' }).waitFor();
  await chooseInRow(page.getByRole('row').filter({ hasText: OWN }), 'Block domain');
  const dialog = page.getByRole('dialog', { name: `Block ${OWN}` });
  await dialog.waitFor();
  assert.ok(((await dialog.textContent()) ?? '').includes(
    `It stops answering at once. It is Acme Ltd's main address, so ${SUB} becomes the main address again in the same step, and everybody there signs in again. The workspace's owner sees that you blocked it, and why.`,
  ));
});
