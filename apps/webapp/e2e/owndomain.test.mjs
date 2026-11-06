// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A workspace's own domain, walked as its owner and the operator meet it (UC-INS-15 checks 1, 2,
// 3, 4 and 10): the panel only where the installation allows own domains; a refusal at the field;
// the two records with copy buttons while waiting for DNS, and that mail is not affected; the
// states the check moves it through, each as drawn; the operator's table of own domains and the
// block with a reason, which the owner then reads word for word.
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
const TENANT = '01936f2a-7c1e-7000-8000-0000000211b1';
const MARK = 'hubtask-domain=q7n4kx2p8zram3vd';
const REASON = "Reported as a look-alike of another company's sign-in page.";

const records = (cname, txt) => [
  { type: 'CNAME', name: 'tasks.acme.com', value: 'domains.tasks.example.net', found: cname },
  { type: 'TXT', name: '_hubtask.tasks.acme.com', value: MARK, found: txt },
];

const own = (state, extra = {}) => ({
  domain: 'tasks.acme.com', state, since: '2026-10-27T09:10:00Z', is_main: false,
  certificate_asked: state === 'ACTIVE', records: state === 'BLOCKED' ? [] : records(false, false), ...extra,
});

const address = (extra = {}) => ({
  host: 'acme.tasks.example.net', owners_may_change: false, may_change: false,
  own_domains_allowed: true, may_manage_own_domain: true, ...extra,
});

async function open(browser, initial, width = 1280) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  const sent = [];
  let current = initial;
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: MULTI });
    if (path === '/tenant' && request.method() === 'GET') {
      return route.fulfill({ json: { ...WORKSPACE, slug: 'acme', display_name: 'Acme Ltd', address: current } });
    }
    if (path === '/tenant/own-domain' && request.method() === 'PUT') {
      const body = request.postDataJSON() ?? {};
      sent.push({ method: 'PUT', body });
      if (body.domain === 'acme.tasks.example.net') {
        return route.fulfill({
          status: 422,
          json: {
            code: 'errors.validation', detail_code: 'admin.own_domain_installation', status: 422, request_id: 'req_d',
            params: { domain: body.domain, installation_host: 'tasks.example.net' },
            field_errors: [{ path: '/domain', code: 'admin.own_domain_installation', params: { domain: body.domain, installation_host: 'tasks.example.net' } }],
          },
        });
      }
      current = { ...current, own_domain: own('PENDING') };
      return route.fulfill({ json: current.own_domain });
    }
    if (path === '/tenant/own-domain' && request.method() === 'DELETE') {
      sent.push({ method: 'DELETE' });
      const { own_domain: _, ...rest } = current;
      current = rest;
      return route.fulfill({ status: 204 });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/administration/address`);
  await page.getByText('Main address', { exact: true }).waitFor();
  return { context, page, sent };
}

test('chromium: where the installation does not allow own domains, the panel is absent', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page } = await open(browser, address({ own_domains_allowed: false }));
  t.after(() => context.close());
  await page.getByText('acme.tasks.example.net').waitFor();
  assert.equal(await page.getByRole('heading', { name: 'Use your own domain' }).count(), 0);
});

test('chromium: the owner types a domain, is refused at the field, then gets the two records to copy', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, sent } = await open(browser, address());
  t.after(() => context.close());

  const panel = page.locator('[data-own-domain]');
  await panel.getByRole('heading', { name: 'Use your own domain' }).waitFor();
  assert.match((await panel.textContent()) ?? '', /Mail is not affected\./);

  await panel.getByLabel('Domain').fill('acme.tasks.example.net');
  await panel.getByRole('button', { name: 'Show the records' }).click();
  await panel.getByText("tasks.example.net is this installation's own address. A domain at or beneath it cannot be your own.").waitFor();

  await panel.getByLabel('Domain').fill('tasks.acme.com');
  await panel.getByRole('button', { name: 'Show the records' }).click();
  await panel.getByText('Waiting for DNS').waitFor();
  assert.deepEqual(sent.at(-1), { method: 'PUT', body: { domain: 'tasks.acme.com' } });

  const text = (await panel.textContent()) ?? '';
  assert.match(text, /Add these two records where acme\.com is managed\./);
  assert.match(text, /neither record touches where mail for acme\.com goes/);
  for (const value of ['domains.tasks.example.net', '_hubtask.tasks.acme.com', MARK]) {
    await panel.getByText(value, { exact: true }).waitFor();
  }
  assert.equal(await panel.getByText('Not found yet').count(), 2);
  await panel.getByRole('button', { name: `Copy ${MARK}` }).click();
  assert.equal(await page.evaluate(() => navigator.clipboard.readText()), MARK);
  await panel.getByText('Checked by itself every few minutes.', { exact: false }).waitFor();

  await panel.getByRole('button', { name: 'Remove this domain' }).click();
  await panel.getByRole('button', { name: 'Show the records' }).waitFor();
  assert.deepEqual(sent.at(-1), { method: 'DELETE' });
});

test('chromium: each state the check reaches is drawn as approved', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const cases = [
    [own('VERIFIED', { records: records(true, true) }), ['Verified', 'Certificate on its way', 'Both records are found.'], false],
    [own('ACTIVE', { records: records(true, true) }), ['Active', 'The main address is still acme.tasks.example.net'], true],
    [own('BROKEN', { records: records(true, false) }), ['Broken', 'The proof record could no longer be found on October 27, 2026', 'Missing since October 27, 2026'], false],
    [own('BLOCKED', { blocked_reason: REASON }), ['Blocked', "This installation's operator blocked this domain", `The reason given: "${REASON}"`], true],
  ];
  for (const [domain, says, removable] of cases) {
    const { context, page } = await open(browser, address({ own_domain: domain }));
    const panel = page.locator(`[data-own-domain="${domain.state}"]`);
    await panel.waitFor();
    const text = (await panel.textContent()) ?? '';
    for (const said of says) assert.ok(text.includes(said), `${domain.state}: "${said}" is not said in ${text}`);
    assert.equal(await panel.getByRole('button', { name: 'Remove this domain' }).count(), removable ? 1 : 0, domain.state);
    if (domain.state === 'BROKEN') {
      assert.equal(await panel.locator('[data-record]').count(), 1, 'a broken domain shows the proof record alone');
    }
    await context.close();
  }
});

test('chromium: 375 px — the records stack without the page scrolling sideways', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page } = await open(browser, address({ own_domain: own('PENDING') }), 375);
  t.after(() => context.close());
  await page.getByText('Waiting for DNS').waitFor();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(overflow <= 0, `the page scrolls sideways by ${overflow} px`);
});

test('chromium: the operator sees every own domain and blocks one with a reason', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  const sent = [];
  let domains = [
    { domain: 'tasks.acme.com', tenant_id: TENANT, tenant_name: 'Acme Ltd', state: 'ACTIVE', since: '2026-10-20T08:00:00Z', is_main: false },
    { domain: 'intranet.northwind.example', tenant_id: TENANT, tenant_name: 'Northwind Studio', state: 'PENDING', since: '2026-10-27T08:00:00Z', is_main: false },
  ];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    if (path === '/meta/capabilities') return route.fulfill({ json: MULTI });
    if (path === '/admin/tenants' && request.method() === 'GET') {
      return route.fulfill({ json: [{ id: TENANT, slug: 'acme', display_name: 'Acme Ltd', status: 'ACTIVE', default_locale: 'en', default_time_zone: 'UTC', created_at: '2026-09-01T00:00:00Z', purge_after: null }] });
    }
    if (path === '/admin/own-domains') return route.fulfill({ json: domains });
    if (path === '/admin/own-domains/tasks.acme.com:block') {
      const body = request.postDataJSON() ?? {};
      sent.push(body);
      if (!body.reason) {
        return route.fulfill({
          status: 422,
          json: {
            code: 'errors.validation', detail_code: 'admin.own_domain_reason_required', status: 422, request_id: 'req_b',
            field_errors: [{ path: '/reason', code: 'admin.own_domain_reason_required' }],
          },
        });
      }
      const blocked = { ...domains[0], state: 'BLOCKED', since: '2026-10-28T10:00:00Z', blocked_reason: body.reason };
      domains = [blocked, domains[1]];
      return route.fulfill({ json: blocked });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/instance/workspaces`);
  await page.getByRole('heading', { name: 'Own domains' }).waitFor();
  const row = page.getByRole('row').filter({ hasText: 'tasks.acme.com' });
  assert.match((await row.textContent()) ?? '', /Acme Ltd.*Active/);
  assert.match((await page.getByRole('row').filter({ hasText: 'intranet.northwind.example' }).textContent()) ?? '', /Waiting for DNS/);

  await chooseInRow(row, 'Block domain');
  const dialog = page.getByRole('dialog', { name: 'Block tasks.acme.com' });
  await dialog.waitFor();
  assert.match((await dialog.textContent()) ?? '', /It stops answering at once\./);
  await dialog.getByRole('button', { name: 'Block domain' }).click();
  await dialog.getByText("Say why it is blocked; the workspace's owner reads it.").first().waitFor();

  await dialog.getByLabel(/^Reason/).fill(REASON);
  await dialog.getByRole('button', { name: 'Block domain' }).click();
  await dialog.waitFor({ state: 'hidden' });
  assert.deepEqual(sent.at(-1), { reason: REASON });
  const blocked = page.getByRole('row').filter({ hasText: 'tasks.acme.com' });
  await blocked.getByText('Blocked').waitFor();
  assert.match((await blocked.textContent()) ?? '', /look-alike/);
  assert.equal(await blocked.getByRole('button').count(), 0, 'a blocked row offers a menu');
});
