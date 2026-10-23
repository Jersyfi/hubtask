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

import { MANIFEST, WORKSPACE, stub } from './fixture.mjs';
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
  const standings = [
    { quota: 'items', limit: 5000, used: 12, configured: false, locked: false },
    // The workspace's own ceiling, as a server that names the level answers it: no operator either.
    { quota: 'export_jobs', limit: 3, used: 0, configured: true, locked: false, source: 'EXCEPTION', lock: null },
  ];

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

// UC-INS-10 check 1, UC-INS-13 check 3: on a plan, *Limits* names it and every row says who set it -
// the plan, its lock, the installation, or the operator's exception.
test('chromium: a workspace on a plan names it, and each limit says who set it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, tenancy_mode: 'multi' } });
    if (path === '/tenant') return route.fulfill({ json: { ...WORKSPACE, sign_in_policy: { plan: 'Family' } } });
    if (path === '/quotas') {
      return route.fulfill({ json: [
        { quota: 'items', limit: 5000, used: 4210, configured: false, locked: false, source: 'PLAN', lock: 'PLAN' },
        { quota: 'export_jobs', limit: 3, used: 0, configured: false, locked: false, source: 'PLAN', lock: null },
        { quota: 'media_bytes', limit: 20000000000, used: 0, configured: true, locked: false, source: 'EXCEPTION', lock: null },
        { quota: 'webhook_targets', limit: 10, used: 1, configured: false, locked: true, source: 'INSTANCE', lock: 'INSTANCE' },
        { quota: 'automation_runs_per_hour', limit: 1000, used: 0, configured: false, locked: false, source: 'DEFAULT', lock: null },
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

  assert.equal((await page.getByTestId('limits-plan').textContent())?.trim(), 'Plan: Family');
  assert.match(await row('Entries').textContent() ?? '', /Set by your plan/);
  assert.match(await row('Export jobs').textContent() ?? '', /Set by your plan/);
  assert.match(await row('Webhook').textContent() ?? '', /Set by the installation/);
  // One chip per limit, with its lock where a level locked it (UC-WRK-32 check 5).
  assert.equal(await row('Entries').locator('[data-source]').getAttribute('data-source'), 'PLAN-locked');
  assert.equal(await row('Export jobs').locator('[data-source]').getAttribute('data-source'), 'PLAN');
  const storage = await page.getByRole('row').filter({ hasText: /storage/i }).textContent() ?? '';
  assert.match(storage, /Set by the operator/, storage);
  // The page is read here and changed by the operator: said once, at the top.
  assert.equal(await page.getByRole('note').filter({ hasText: /operator/ }).count(), 1);
  const runs = await page.getByRole('row').filter({ hasText: /Automation/i }).textContent() ?? '';
  assert.match(runs, /Hubtask's default/, runs);
});

// UC-INS-10 check 5: the people limit is a row like any other; reached, the screen says nobody new
// comes in and nobody here goes, and what is counted.
test('chromium: a reached people limit says nobody new comes in and nobody here goes', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const reached = await openLimits(browser, [
    { quota: 'items', limit: 5000, used: 12, configured: false, locked: false, source: 'PLAN', lock: null },
    { quota: 'accounts', limit: 6, used: 6, ratio: 1, configured: false, locked: false, source: 'PLAN', lock: 'PLAN' },
  ], { tenancy: 'multi' });
  const said = (await reached.page.locator('main').textContent()) ?? '';
  const row = (await reached.page.getByRole('row').filter({ hasText: 'People' }).textContent()) ?? '';
  await reached.close();
  assert.match(row, /6/, row);
  assert.match(row, /Set by your plan/, row);
  assert.match(said, /The limit for people is reached\./, said);
  assert.match(said, /Everybody here stays as they are\./, said);
  assert.match(said, /service accounts and invitations whose link ran out are not counted/, said);

  const room = await openLimits(browser, [
    { quota: 'accounts', limit: 6, used: 4, configured: false, locked: false, source: 'PLAN', lock: null },
  ], { tenancy: 'multi' });
  const roomy = (await room.page.locator('main').textContent()) ?? '';
  await room.close();
  assert.doesNotMatch(roomy, /limit for people is reached/, roomy);
});

// ux-inventory.md §E8: a limit is also shown beside the feature it bounds, with the same chip as
// on *Limits* - the hour's runs under the runs' strip, set by the plan and locked there.
test('chromium: a limit beside its feature says who set it, as Limits does', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await openLimits(browser, [
    { quota: 'automation_runs_per_hour', limit: 500, used: 34, ratio: 0.068, configured: false, locked: false, source: 'PLAN', lock: 'PLAN' },
  ], { tenancy: 'multi' });
  t.after(close);
  const onLimits = await page.getByRole('row').nth(1).locator('[data-source]').getAttribute('data-source');

  await page.goto(`${served.origin}/administration/runs`);
  const hourly = page.locator('[data-hourly]');
  await hourly.waitFor({ timeout: 15_000 });
  assert.match(await hourly.textContent() ?? '', /This hour: 34 of 500 runs/);
  assert.equal(await hourly.locator('[data-source]').getAttribute('data-source'), 'PLAN-locked');
  assert.equal(onLimits, 'PLAN-locked', 'Limits and the feature disagree');
});
