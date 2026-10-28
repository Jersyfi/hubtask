// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The workspace's own *Address* screen, walked as its owner meets it (UC-INS-19 check 3): with the
// installation's switch off there is no control and the screen says why; with it on, *Change
// address* opens the operator's dialog - the costs, the lockout rule, the proof by password - and
// the browser goes on to the new address; inside the cooldown the button is absent and the day
// stands in its place.
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

const MULTI = { ...MANIFEST, tenancy_mode: 'multi', workspace_hosts: 'subdomain' };

const OFF = { host: 'meier.tasks.example.net', owners_may_change: false, may_change: false };
const ON = { host: 'meier.tasks.example.net', owners_may_change: true, may_change: true };
const COOLING = {
  host: 'familie-meier.tasks.example.net', owners_may_change: true, may_change: false,
  changed_at: '2026-10-14T09:30:00Z', changeable_from: '2026-11-13T09:30:00Z',
};

async function open(browser, address, width = 1280) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  const sent = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: MULTI });
    if (path === '/tenant' && request.method() === 'GET') {
      return route.fulfill({ json: { ...WORKSPACE, slug: 'meier', display_name: 'Meier family', address } });
    }
    if (path === '/auth/step-up') {
      return route.fulfill({ status: 201, json: { step_up_token: 'hbt_sup_tok', expires_at: '2099-01-01T00:00:00Z', method: 'PASSWORD' } });
    }
    if (path === '/tenant:change-address') {
      const body = request.postDataJSON() ?? {};
      const proof = request.headers()['x-hubtask-step-up'] ?? null;
      sent.push({ body, proof });
      if (body.slug && !body.dry_run && !proof) {
        return route.fulfill({ status: 403, json: { code: 'errors.forbidden', detail_code: 'auth.step_up_required', status: 403, params: { methods: 'PASSWORD', purpose: 'change_address' }, request_id: 'req_s' } });
      }
      const answer = { from: 'meier.tasks.example.net', members_mailed: 3, members_told_at_sign_in: 0, dry_run: body.dry_run === true };
      if (body.slug) answer.to = `${body.slug}.tasks.example.net`;
      return route.fulfill({ json: answer });
    }
    return stub(route);
  });
  // The new address's front door, where the browser goes once the change went through.
  const arrived = [];
  await context.route((url) => url.hostname === 'familie-meier.tasks.example.net', (route) => {
    arrived.push(route.request().url());
    return route.fulfill({ contentType: 'text/html', body: '<!doctype html><title>Sign in</title>' });
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/administration/address`);
  await page.getByText('Main address').waitFor();
  return { context, page, sent, arrived };
}

test('chromium: with owners not allowed to change it, the address stands without a control and says why', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, sent } = await open(browser, OFF);
  t.after(() => context.close());

  await page.getByRole('heading', { name: 'Address' }).waitFor();
  await page.getByText('meier.tasks.example.net').waitFor();
  await page.getByText("Changing it is up to this installation's operator.", { exact: false }).waitFor();
  assert.equal(await page.getByRole('button', { name: 'Change address' }).count(), 0, 'a control the owner may not use is drawn');
  assert.equal(sent.length, 0);
  // The row sits under "Workspace and people", beside *Workspace*, which no longer shows the address.
  const section = page.getByRole('navigation', { name: 'Administration', exact: true });
  assert.equal(await section.getByRole('treeitem', { name: 'Address', exact: true }).getAttribute('aria-current'), 'page');
  await page.goto(`${served.origin}/administration/workspace`);
  await page.getByLabel(/^Name/).waitFor();
  assert.equal(await page.getByText('meier.tasks.example.net').count(), 0, 'the workspace screen still shows the address');
  assert.equal(await page.getByText("The installation's operator changes it", { exact: false }).count(), 0);
});

test('chromium: the owner changes the address with the operator dialog and the password, and goes on to it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, sent, arrived } = await open(browser, ON);
  t.after(() => context.close());

  await page.getByText('People reach this workspace here, and every link Hubtask sends uses it.', { exact: false }).waitFor();
  await page.getByRole('button', { name: 'Change address' }).click();
  const dialog = page.getByRole('dialog', { name: 'Change the address of Meier family' });
  await dialog.waitFor();
  await dialog.getByLabel(/^New address/).fill('familie-meier');
  await dialog.getByText('familie-meier.tasks.example.net').waitFor();
  const text = (await dialog.textContent()) ?? '';
  assert.match(text, /What the change ends/);
  assert.match(text, /Passkeys made at the old address stop working\./);
  assert.ok(sent.every((each) => each.body.dry_run === true), 'something was changed before it was confirmed');

  await dialog.getByRole('button', { name: 'Change address' }).click();
  const prove = page.getByRole('dialog', { name: 'Prove it is you' });
  await prove.waitFor();
  assert.match((await prove.textContent()) ?? '', /A passkey belongs to the old address, so it cannot confirm this\./);
  await prove.getByLabel('Your password').fill('the owner');
  await prove.getByRole('button', { name: 'Prove it' }).click();
  await page.waitForURL((url) => url.hostname === 'familie-meier.tasks.example.net');
  assert.deepEqual(sent.at(-1), { body: { slug: 'familie-meier' }, proof: 'hbt_sup_tok' });
  assert.equal(new URL(arrived.at(-1)).pathname, '/');
});

test('chromium: 375 px — inside the cooldown the button is absent and the day is named in its place', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page } = await open(browser, COOLING, 375);
  t.after(() => context.close());

  await page.getByText('familie-meier.tasks.example.net').waitFor();
  await page.getByText('Changed on October 14, 2026. It can be changed again from November 13, 2026', { exact: false }).waitFor();
  assert.equal(await page.getByRole('button', { name: 'Change address' }).count(), 0);
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(overflow <= 0, `the page scrolls sideways by ${overflow} px`);
});
