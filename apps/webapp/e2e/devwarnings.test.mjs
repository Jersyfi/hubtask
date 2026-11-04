// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The development build says what the production build keeps quiet: Svelte's dev warnings. One of
// them, state_proxy_equality_mismatch, marks a comparison with a `$state` proxy that answers false
// for the very object it holds - a wrong selection or check that no production test sees. Walked
// on the dev server, signed out and signed in; every `[svelte]` warning fails the walk.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

import { chromium } from 'playwright';
import { createServer } from 'vite';

import { signedIn } from './fixture.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const server = await createServer({ root, configFile: join(root, 'vite.config.ts'), server: { port: 0 }, logLevel: 'error' });
await server.listen();
const origin = server.resolvedUrls.local[0].replace(/\/$/, '');
test.after(() => server.close());

async function warningsOn(page, path) {
  const warnings = [];
  page.on('console', (message) => {
    if (message.type() === 'warning' && message.text().includes('[svelte]')) warnings.push(message.text().split('\n')[0]);
  });
  await page.goto(`${origin}${path}`);
  await page.getByRole('main').or(page.getByRole('button', { name: /sign in/i })).first().waitFor({ timeout: 60_000 });
  await page.waitForTimeout(1_500);
  return warnings;
}

test('chromium: the dev build signs in and opens Today without a Svelte warning', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  const out = await browser.newContext();
  t.after(() => out.close());
  await out.route('**/api/v1/**', (route) => route.fulfill({ status: 401, json: { code: 'auth.unauthenticated' } }));
  assert.deepEqual(await warningsOn(await out.newPage(), '/'), [], 'signed out');

  const { context, page } = await signedIn(browser, 1280);
  t.after(() => context.close());
  assert.deepEqual(await warningsOn(page, '/'), [], 'signed in');
});
