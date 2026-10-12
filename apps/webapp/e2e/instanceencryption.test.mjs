// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The keyring as an operator reads it (UC-INS-16 checks 1, 3, 4): a count per key across every
// kind and both levels, and for a key the ring no longer holds, which kinds would stay unreadable -
// in words, never naming a workspace and never showing a value.
//
// Against a stubbed API: what is asserted is what the screen shows. Chromium only.

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

const STATUS = {
  active_key_id: 'k2',
  keys: [
    { key_id: 'k2', active: true, in_ring: true, sealed_values: 12, stores: [
      { store: 'webhook_subscription', sealed_values: 10 }, { store: 'ai_provider', sealed_values: 2 },
    ] },
    { key_id: 'k1', active: false, in_ring: true, sealed_values: 0, stores: [] },
    { key_id: 'gone', active: false, in_ring: false, sealed_values: 3, stores: [
      { store: 'identity_provider', sealed_values: 1 }, { store: 'webhook_subscription', sealed_values: 2 },
    ] },
  ],
};

test('chromium: a key the ring lost names the kinds it would leave unreadable', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    if (path === '/admin/encryption') return route.fulfill({ json: STATUS });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();

  await page.goto(`${served.origin}/instance/encryption`);
  const banner = page.getByText('A key is missing and values still name it');
  await banner.waitFor({ timeout: 15_000 });
  const body = await page.locator('main').textContent() ?? '';
  assert.match(body, /Still sealed under gone: 1 sign-in provider secret\./);
  assert.match(body, /Still sealed under gone: 2 webhook signing secrets\./);
  // A key the ring holds is not a stranded one, and the contract's store names never show.
  assert.doesNotMatch(body, /Still sealed under k2/);
  assert.doesNotMatch(body, /identity_provider|webhook_subscription/);
});
