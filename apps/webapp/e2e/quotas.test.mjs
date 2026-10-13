// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The workspace's *Limits* as an owner reads them (UC-INS-09 check 5): a ceiling the installation
// locked says it is set by the installation, one it only suggested says it is the installation's
// default, and one the workspace carries says so.
//
// Against a stubbed API. Chromium only.

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

test('chromium: a locked limit is set by the installation, an open one is its default', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/quotas') {
      return route.fulfill({ json: [
        { quota: 'items', limit: 5000, used: 12, configured: false, locked: true },
        { quota: 'export_jobs', limit: 3, used: 0, configured: false, locked: false },
        { quota: 'webhook_targets', limit: 10, used: 1, configured: true, locked: false },
      ] });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/administration/quotas`);
  const row = (name) => page.getByRole('row').filter({ hasText: name });
  await row('Entries').waitFor({ timeout: 15_000 });

  assert.match(await row('Entries').textContent() ?? '', /Set by the installation/);
  assert.doesNotMatch(await row('Entries').textContent() ?? '', /default/);
  assert.match(await row('Export jobs').textContent() ?? '', /The installation's default/);
  assert.match(await row('Webhook').textContent() ?? '', /This workspace/);
});
