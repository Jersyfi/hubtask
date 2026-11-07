// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The installation's *Configuration file*, walked as an operator meets it (user-experience.md
// §8.4): the level as the configuration document holds it - a decided key with its value and lock,
// an undecided one without a value, no AI area on an installation of one workspace - and, while a
// file enforces the level, which file.
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

const tighten = { workspace_may: 'TIGHTEN' };

const LEVEL = {
  sign_in: {
    min_length: { set: true, value: 14, locked: true, kind: 'COUNT', minimum: 8, maximum: 128, ...tighten },
    breach_check: { set: false, locked: false, kind: 'FLAG', ...tighten },
  },
  legal: {},
  localisation: {
    locale: { set: true, value: 'de', locked: false, kind: 'LOCALE' },
  },
  quotas: {},
  ai: {
    sources: { set: true, value: 'OFFERED', locked: true, kind: 'CHOICE', options: ['EITHER', 'OFFERED', 'OWN', 'NONE'], ...tighten },
  },
  source: 'database',
  is_enforced_from_file: false,
};

async function open(browser, level = LEVEL) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    if (path === '/admin/settings' && request.method() === 'GET') {
      return route.fulfill({ json: level, headers: { ETag: '"1"' } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/instance/configuration-file`);
  await page.locator('[data-instance-file]').waitFor({ timeout: 15_000 });
  return { page, close: () => context.close() };
}

test('chromium: the file holds every decided value with its lock, and an undecided key without one', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser);
  t.after(close);

  await page.getByRole('heading', { name: 'Configuration file' }).first().waitFor();
  const file = JSON.parse((await page.locator('[data-instance-file] .line .text').allTextContents()).join('\n'));
  assert.deepEqual(file.sign_in.min_length, { set: true, value: 14, locked: true });
  assert.deepEqual(file.sign_in.breach_check, { set: false, locked: false });
  assert.deepEqual(file.localisation.locale, { set: true, value: 'de', locked: false });
  // One workspace: its settings screen draws no AI area, and neither does its file.
  assert.equal(file.ai, undefined);
  assert.equal(await page.getByText("These values come from this installation's file").count(), 0);
});

test('chromium: while a file enforces the level, the screen names that file', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, { ...LEVEL, is_enforced_from_file: true, source: '/etc/hubtask/instance.json' });
  t.after(close);

  await page.getByText('/etc/hubtask/instance.json').waitFor({ timeout: 10_000 });
  assert.match((await page.locator('main').textContent()) ?? '', /These values come from this installation's file/);
});
