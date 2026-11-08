// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A schedule is set by choices, not by RFC 5545 (user-experience.md §5.4): the backup schedule's
// FrequencyPicker opens on the default nightly rule as *Daily at 03:00*, and *Weekly, Sunday,
// 22:30* leaves as the rule those choices stand for. A time zone is picked, not typed.
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

const TARGET = { id: '01a0e2e0-0000-7000-8000-0000000007b1', name: 'The NAS', kind: 'SFTP', scope: 'TENANT', config: {}, encryption_mode: 'AES256_GCM' };

test('chromium: a backup schedule is chosen as weekly on Sunday at 22:30 and sent as that rule', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const written = [];
  const context = await browser.newContext({ viewport: { width: 1280, height: 1100 }, timezoneId: 'Europe/Berlin' });
  t.after(() => context.close());
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/backup-targets' && request.method() === 'GET') return route.fulfill({ json: [TARGET] });
    if (path === '/backup-schedules' && request.method() === 'POST') {
      written.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: { id: '01a0e2e0-0000-7000-8000-0000000007b2', enabled: true, ...request.postDataJSON() } });
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

  // *Add a schedule* is the Schedules section's own add, a Dialog M (page-system §9.3).
  await page.getByRole('button', { name: 'Add a schedule', exact: true }).first().click({ timeout: 15_000 });
  const form = page.getByRole('dialog', { name: 'Add a schedule' });
  await form.waitFor();
  const often = form.getByLabel('How often');
  assert.equal(await often.inputValue(), 'DAILY', 'the default nightly rule reads as daily');
  assert.equal(await form.getByLabel('At', { exact: true }).inputValue(), '03:00');
  assert.equal(await form.getByLabel('Time zone').inputValue(), 'Europe/Berlin', 'the zone is the device\'s');
  assert.equal(await form.getByLabel('The rule itself').isVisible(), false, 'the raw rule is behind Advanced');

  await form.getByLabel('Write to').selectOption(TARGET.id);
  await often.selectOption('WEEKLY');
  await form.getByLabel('On which day').selectOption('SU');
  await form.getByLabel('At', { exact: true }).fill('22:30');
  await form.getByRole('button', { name: 'Add a schedule', exact: true }).click();
  for (let waited = 0; written.length === 0 && waited < 5000; waited += 50) await page.waitForTimeout(50);

  assert.equal(written[0]?.rrule, 'FREQ=WEEKLY;BYDAY=SU;BYHOUR=22;BYMINUTE=30');
  assert.equal(written[0]?.timezone, 'Europe/Berlin');
  assert.deepEqual(failures, []);
});
