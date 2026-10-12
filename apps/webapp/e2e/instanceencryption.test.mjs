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

test('chromium: a key the ring lost names the kinds it would leave unreadable', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  // The fixture's keyring: k0 is out of the ring while three values still name it.
  await context.route('**/api/v1/**', (route) => stub(route));
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();

  await page.goto(`${served.origin}/instance/encryption`);
  const banner = page.getByText('A key is missing and values still name it');
  await banner.waitFor({ timeout: 15_000 });
  const body = await page.locator('main').textContent() ?? '';
  assert.match(body, /Still sealed under k0: 1 sign-in provider secret\./);
  assert.match(body, /Still sealed under k0: 2 webhook signing secrets\./);
  // A key the ring holds is not a stranded one, and the contract's store names never show.
  assert.doesNotMatch(body, /Still sealed under k2/);
  assert.doesNotMatch(body, /identity_provider|webhook_subscription/);
});
