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

import { MANIFEST, stub } from './fixture.mjs';
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

/** *Limits* with the given standings, on an installation of the given tenancy. */
async function openLimits(browser, standings, { tenancy = 'single' } = {}) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, tenancy_mode: tenancy } });
    if (path === '/quotas') return route.fulfill({ json: standings });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/administration/quotas`);
  await page.getByRole('row').nth(1).waitFor({ timeout: 15_000 });
  return { page, close: () => context.close() };
}

// P-12: the attachment storage is a size, not a count of bytes; every other limit stays a count.
test('chromium: attachment storage reads as a size', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await openLimits(browser, [
    { quota: 'media_bytes', limit: 10737418240, used: 2500000, ratio: 0.0002, configured: false, locked: false },
    { quota: 'items', limit: 5000, used: 12, configured: false, locked: false },
  ]);
  t.after(close);

  const storage = await page.getByRole('row').filter({ hasText: 'Attachment storage' }).textContent() ?? '';
  assert.match(storage, /10\.7\s?GB/, storage);
  assert.match(storage, /2\.5\s?MB/, storage);
  assert.doesNotMatch(storage, /10737418240/, storage);
  assert.match(await page.getByRole('row').filter({ hasText: 'Entries' }).textContent() ?? '', /5000/);
});

// deployments.md D1: on a single installation *Limits* names neither the installation nor an
// operator, and says where a limit is changed - or whom to ask, for a reader who cannot go there.
// An installation of many keeps its own sentences.
test('chromium: a single installation is told where a limit changes without the operator', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const standings = [{ quota: 'items', limit: 5000, used: 12, configured: false, locked: false }];

  const single = await openLimits(browser, standings, { tenancy: 'single' });
  const said = (await single.page.locator('main').textContent()) ?? '';
  await single.close();
  assert.doesNotMatch(said, /operator|installation's to set/i, said);
  assert.match(said, /Applies to every workspace you create/, said);
  assert.match(said, /ask whoever set Hubtask up/, said);

  const multi = await openLimits(browser, standings, { tenancy: 'multi' });
  const many = (await multi.page.locator('main').textContent()) ?? '';
  await multi.close();
  assert.match(many, /the operator of this installation/, many);
});
