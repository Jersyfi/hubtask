// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The installation's own backup targets, walked as an operator meets them (UC-BAK-07 check 1):
// a target is added behind a fresh proof with its credential going in and never coming back, and
// the export dialog offers it by name only - never where it points.
//
// Against a stubbed API: what is asserted is what the screen sends and what it shows. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { MANIFEST, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const TARGET = {
  id: '01a0e2e0-0000-7000-8000-000000001901', name: 'Provider exports', kind: 'S3', scope: 'INSTANCE',
  config: { bucket: 'installation-exports', endpoint: 'https://s3.example.org' },
  encryption_mode: 'AES256_GCM', enabled: true, warnings: [],
};
const WORKSPACE = {
  id: '01a0e2e0-0000-7000-8000-000000001911', slug: 'acme', display_name: 'Acme GmbH', status: 'ACTIVE',
  created_at: '2026-09-01T08:00:00Z', member_count: 3, version: 1,
};

async function open(browser) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 1000 } });
  const sent = [];
  let targets = [TARGET];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, tenancy_mode: 'multi', instance: { reachable: true } } });
    }
    if (path === '/auth/step-up') {
      return route.fulfill({ status: 201, json: { step_up_token: 'hbt_sup_tok', expires_at: '2099-01-01T00:00:00Z', method: 'PASSWORD' } });
    }
    if (path === '/admin/backup-targets' && request.method() === 'GET') return route.fulfill({ json: targets });
    if (path === '/admin/backup-targets' && request.method() === 'POST') {
      const proof = request.headers()['x-hubtask-step-up'] ?? null;
      sent.push({ method: 'POST', path, body: request.postDataJSON(), proof });
      if (proof !== 'hbt_sup_tok') {
        return route.fulfill({ status: 403, json: { code: 'errors.forbidden', detail_code: 'auth.step_up_required', status: 403, params: { methods: 'PASSWORD' }, request_id: 'req_t' } });
      }
      const body = request.postDataJSON();
      const added = { id: '01a0e2e0-0000-7000-8000-000000001902', name: body.name, kind: body.kind, scope: 'INSTANCE', config: body.config, encryption_mode: 'AES256_GCM', enabled: true, warnings: [] };
      targets = [...targets, added];
      return route.fulfill({ status: 201, json: added });
    }
    if (path === '/admin/tenants' && request.method() === 'GET') return route.fulfill({ json: [WORKSPACE] });
    if (path.endsWith(':export') && request.method() === 'POST') {
      sent.push({ method: 'POST', path, body: request.postDataJSON() });
      return route.fulfill({ status: 202, json: { job_id: '01a0e2e0-0000-7000-8000-000000001921', status: 'QUEUED' } });
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

test('chromium: an operator adds a target of the installation behind a fresh proof', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/instance/backup-targets`);
  const row = page.getByRole('row').filter({ hasText: 'Provider exports' });
  await row.waitFor({ timeout: 15_000 });
  // Where it points is the operator's to read here.
  assert.match(await row.textContent() ?? '', /installation-exports/);

  await page.getByLabel('Name').fill('Second exports');
  await page.getByLabel('Kind').selectOption('S3');
  await page.getByLabel('Endpoint').fill('https://s3.example.org');
  await page.getByLabel('Bucket').fill('second-exports');
  await page.getByLabel('Access key').fill('AKIA');
  await page.getByLabel('Secret key').fill('the-secret-key');
  await page.getByRole('button', { name: 'Add', exact: true }).click();

  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Your password').fill('the operator');
  await dialog.getByRole('button', { name: 'Prove it' }).click();
  await page.getByRole('row').filter({ hasText: 'Second exports' }).waitFor({ timeout: 10_000 });

  const posts = sent.filter((each) => each.path === '/admin/backup-targets');
  assert.equal(posts.length, 2, 'the add was not retried with the proof');
  assert.equal(posts[0].proof, null);
  assert.equal(posts[1].proof, 'hbt_sup_tok');
  assert.deepEqual(posts[1].body.credentials, { access_key: 'AKIA', secret_key: 'the-secret-key' });
  // The credential went in and is nowhere on the screen.
  assert.doesNotMatch(await page.locator('main').textContent() ?? '', /the-secret-key/);
});

test('chromium: the export offers the installation\'s target by name, never where it points', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/instance/workspaces`);
  const row = page.getByRole('row').filter({ hasText: 'Acme GmbH' });
  await row.waitFor({ timeout: 15_000 });
  await row.getByRole('button', { name: /Export/ }).click();
  const dialog = page.getByRole('dialog');
  const picker = dialog.getByLabel('Target', { exact: true });
  await picker.waitFor();
  const offered = await picker.locator('option').allTextContents();
  assert.ok(offered.includes('Provider exports'), `offered ${offered}`);
  assert.ok(!offered.join(' ').includes('installation-exports') && !offered.join(' ').includes('s3.example.org'));

  const accepted = page.waitForResponse((response) => response.url().endsWith(':export'), { timeout: 10_000 });
  await dialog.getByRole('button', { name: 'Start the export' }).click();
  await accepted;
  const exported = sent.find((each) => each.path.endsWith(':export'));
  assert.deepEqual(exported?.body, { target_id: TARGET.id });
});
