// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Adding a backup target from the web app (UC-BAK-01 check 10): every kind this build speaks with
// every field it needs. An SFTP target carries the host key the server requires, names its user in
// the configuration and may log in with a key; a WebDAV target names its address; and a target that
// leaves the workspace unprotected - unencrypted, or reached in the clear - goes only once its owner
// has accepted the risk the server names (check 6), so what leaves is a target the server accepts.
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
    encryption_mode: 'AES256_GCM',
  });
  assert.deepEqual(failures, []);
});

/** The target form, signed in as the owner, with every target the page writes recorded. */
async function targetForm(browser) {
  const written = [];
  const context = await browser.newContext({ viewport: { width: 1280, height: 1100 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/backup-targets' && request.method() === 'POST') {
      written.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: { id: '01a0e2e0-0000-7000-8000-0000000007a2', ...request.postDataJSON(), credentials: undefined } });
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
  const sent = async () => {
    for (let waited = 0; written.length === 0 && waited < 5000; waited += 50) await page.waitForTimeout(50);
    return written[0];
  };
  return { page, form, sent, written, failures, close: () => context.close() };
}

test('chromium: a WebDAV target is sent with its address and user, the password as a credential', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { form, sent, failures, close } = await targetForm(browser);
  t.after(close);

  await form.getByLabel('Name').fill('The cloud folder');
  await form.getByLabel('Kind').selectOption('WEBDAV');
  await form.getByLabel('Address').fill('https://dav.example.invalid/backups/');
  await form.getByLabel('User').fill('hubtask');
  await form.getByLabel('Password').fill('e2e-only');
  await form.getByRole('button', { name: 'Add', exact: true }).click();

  assert.deepEqual(await sent(), {
    name: 'The cloud folder',
    kind: 'WEBDAV',
    config: { url: 'https://dav.example.invalid/backups/', username: 'hubtask' },
    credentials: { password: 'e2e-only' },
    encryption_mode: 'AES256_GCM',
  });
  assert.deepEqual(failures, []);
});

test('chromium: an SFTP target can log in with a private key and its passphrase', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { form, sent, failures, close } = await targetForm(browser);
  t.after(close);

  const key = 'key-line-one-e2e-only\nkey-line-two-e2e-only';
  await form.getByLabel('Name').fill('The NAS by key');
  await form.getByLabel('Kind').selectOption('SFTP');
  await form.getByLabel('Host', { exact: true }).fill('nas.local');
  await form.getByLabel('Path').fill('/backups');
  await form.getByLabel('User').fill('hubtask');
  await form.getByLabel('Host key').fill(HOST_KEY);
  await form.getByLabel('Private key').fill(key);
  await form.getByLabel('Key passphrase').fill('e2e-only');
  await form.getByRole('button', { name: 'Add', exact: true }).click();

  const body = await sent();
  assert.deepEqual(body.credentials, { private_key: `${key}\n`, private_key_passphrase: 'e2e-only' });
  assert.equal(body.kind, 'SFTP');
  assert.deepEqual(failures, []);
});

test('chromium: an unprotected target is added only once its owner has accepted the risk the server names', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, form, sent, written, failures, close } = await targetForm(browser);
  t.after(close);

  await form.getByLabel('Name').fill('A plain folder');
  await form.getByLabel('Kind').selectOption('WEBDAV');
  await form.getByLabel('Address').fill('http://dav.example.invalid/backups/');
  await form.getByLabel('Encryption').selectOption('NONE');

  // Both reasons, in the server's own words, before anything is sent.
  await form.getByText('Archives at this target are stored unencrypted.').waitFor({ timeout: 5_000 });
  await form.getByText('This target is reached over a connection anybody on the network can read.').waitFor();
  // Pressed anyway - forced past the reason it carries - nothing leaves.
  await form.getByRole('button', { name: 'Add', exact: true }).click({ force: true });
  await page.waitForTimeout(300);
  assert.equal(written.length, 0, 'the target was sent before the risk was accepted');

  await form.getByLabel('I accept this and want the target anyway.').check();
  await form.getByRole('button', { name: 'Add', exact: true }).click();
  const body = await sent();
  assert.equal(body.encryption_mode, 'NONE');
  assert.equal(body.insecure_acknowledged, true);
  assert.deepEqual(body.config, { url: 'http://dav.example.invalid/backups/' });
  assert.deepEqual(failures, []);
});
