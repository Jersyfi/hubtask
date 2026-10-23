// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// One confirmation pattern for a final act (ux-inventory §E10): removing a backup schedule and
// withdrawing a retention rule ask in a dialog first, which says what stays; *Cancel* sends
// nothing, the confirmation sends the one removal. Removing a target asks the same way.
//
// Against a stubbed API. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { MANIFEST, PERMISSIONS, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const TARGET = { id: '01a0e2e0-0000-7000-8000-00000000c601', name: 'Confirm shelf', kind: 'S3', encryption_mode: 'NONE', enabled: true };
const SCHEDULE = { id: '01a0e2e0-0000-7000-8000-00000000c602', target_id: TARGET.id, scope: { kind: 'TENANT' }, rrule: 'FREQ=DAILY;BYHOUR=3', timezone: 'UTC', mode: 'FULL', enabled: true };
const POLICY = { id: '01a0e2e0-0000-7000-8000-00000000c603', scope: { kind: 'TENANT' }, data_kind: 'TRASH', retain_days: 30, action: 'HARD_DELETE', enabled: true };

async function open(browser, address) {
  const removed = [];
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (request.method() === 'DELETE') {
      removed.push(path);
      return route.fulfill({ status: 204 });
    }
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: PERMISSIONS.OWNER } });
    if (path === '/backup-targets') return route.fulfill({ json: [TARGET] });
    if (path === '/backup-schedules') return route.fulfill({ json: [SCHEDULE] });
    if (path === '/retention-policies') return route.fulfill({ json: [POLICY] });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}${address}`);
  return { page, removed, failures, close: () => context.close() };
}

async function settle(page, removed, count) {
  for (let waited = 0; removed.length < count && waited < 5000; waited += 50) await page.waitForTimeout(50);
}

test('chromium: removing a backup schedule asks first, and cancelling sends nothing', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, removed, failures, close } = await open(browser, '/administration/backup');
  t.after(close);

  const press = page.getByRole('button', { name: 'Remove the schedule' });
  await press.waitFor();
  await press.click();
  const dialog = page.getByRole('dialog', { name: 'Remove this schedule?' });
  await dialog.getByText('The archives it wrote stay where they are.', { exact: false }).waitFor();
  await dialog.getByRole('button', { name: 'Cancel' }).first().click();
  await dialog.waitFor({ state: 'detached' });
  assert.deepEqual(removed, [], 'cancelling removed the schedule');

  await press.click();
  await page.getByRole('dialog', { name: 'Remove this schedule?' }).getByRole('button', { name: 'Remove it' }).click();
  await settle(page, removed, 1);
  assert.deepEqual(removed, [`/backup-schedules/${SCHEDULE.id}`]);
  assert.deepEqual(failures, []);
});

test('chromium: removing a backup target asks in the same dialog', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, removed, close } = await open(browser, '/administration/backup');
  t.after(close);

  await page.getByRole('button', { name: 'Remove the target' }).click();
  await page.getByRole('dialog', { name: 'Remove this target?' }).getByRole('button', { name: 'Remove it' }).click();
  await settle(page, removed, 1);
  assert.deepEqual(removed, [`/backup-targets/${TARGET.id}`]);
});

test('chromium: withdrawing a retention rule asks first, and cancelling sends nothing', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, removed, failures, close } = await open(browser, '/administration/retention');
  t.after(close);

  const press = page.getByRole('button', { name: 'Withdraw it' });
  await press.waitFor();
  await press.click();
  const dialog = page.getByRole('dialog', { name: 'Withdraw this rule?' });
  await dialog.getByText('What it already removed stays removed', { exact: false }).waitFor();
  await dialog.getByRole('button', { name: 'Cancel' }).first().click();
  await dialog.waitFor({ state: 'detached' });
  assert.deepEqual(removed, [], 'cancelling withdrew the rule');

  await press.click();
  await page.getByRole('dialog', { name: 'Withdraw this rule?' }).getByRole('button', { name: 'Withdraw it' }).click();
  await settle(page, removed, 1);
  assert.deepEqual(removed, [`/retention-policies/${POLICY.id}`]);
  assert.deepEqual(failures, []);
});
