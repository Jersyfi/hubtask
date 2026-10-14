// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Adding a backup target from the web app (UC-BAK-01 check 10): an SFTP target carries the host
// key the server requires and names its user in the configuration, so what leaves is a target the
// server can accept rather than one it always refuses.
//
// Chromium only: what this asserts is the rendered form and the request that leaves.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { stub, forgetUnstubbed } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const HOST_KEY = 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleOnlyNotARealKey nas';

test('chromium: an SFTP target is sent with its host key and its user in the configuration', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const written = [];
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/backup-targets' && request.method() === 'POST') {
      written.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: { id: '01a0e2e0-0000-7000-8000-0000000007a1', ...request.postDataJSON(), credentials: undefined, encryption_mode: 'NONE' } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  forgetUnstubbed();
  await page.goto(`${served.origin}/administration/backup`);

  const form = page.locator('form', { hasText: 'Add a target' });
  await form.waitFor({ timeout: 15_000 });
  await form.getByLabel('Name').fill('The NAS');
  await form.getByLabel('Kind').selectOption('SFTP');
  await form.getByLabel('Host', { exact: true }).fill('nas.local');
  await form.getByLabel('Path').fill('/backups');
  await form.getByLabel('User').fill('hubtask');
  await form.getByLabel('Host key').fill(HOST_KEY);
  await form.getByLabel('Password').fill('e2e-only');
  await form.getByRole('button', { name: 'Add', exact: true }).click();
  for (let waited = 0; written.length === 0 && waited < 5000; waited += 50) await page.waitForTimeout(50);

  assert.deepEqual(written[0], {
    name: 'The NAS',
    kind: 'SFTP',
    config: { host: 'nas.local', path: '/backups', username: 'hubtask', host_key: HOST_KEY },
    credentials: { password: 'e2e-only' },
  });
  assert.deepEqual(failures, []);
});
