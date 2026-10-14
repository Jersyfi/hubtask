// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The installation's journal says what it records in words the installation's reader knows
// (deployments.md D1): a single installation is not told of provisioning, suspension or deletion
// it cannot do, nor of an operator; an installation of many is.
//
// Against a stubbed API. Chromium only.

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

async function journalOf(browser, tenancy) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, tenancy_mode: tenancy } });
    if (path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/instance/journal`);
  await page.getByText(/never anybody.s content/).waitFor({ timeout: 15_000 });
  const text = (await page.locator('main').textContent()) ?? '';
  await context.close();
  return text;
}

test('chromium: a single installation reads its journal without suspension, deletion or an operator', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const single = await journalOf(browser, 'single');
  assert.doesNotMatch(single, /provisioned|suspended|deleted|operator/i, single);
  assert.match(single, /every workspace you create/, single);

  const multi = await journalOf(browser, 'multi');
  assert.match(multi, /provisioned, suspended, resumed and deleted/, multi);
});
