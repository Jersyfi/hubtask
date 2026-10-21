// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Proposals prepared on arrival (UC-JUM-07 check 10) as an administrator switches them, against a
// stubbed API. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { AI_SOURCE, MANIFEST, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, { ai = true } = {}) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const sent = [];
  let current = AI_SOURCE;
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, features: { ...MANIFEST.features, ai_suggestions: ai } } });
    }
    if (path === '/ai-provider' && request.method() === 'GET') return route.fulfill({ status: 404, json: { code: 'ai.not_configured' } });
    if (path === '/ai-source' && request.method() === 'GET') return route.fulfill({ json: current });
    if (path === '/ai-arrival-proposals' && request.method() === 'PUT') {
      const body = request.postDataJSON();
      sent.push(body);
      current = { ...current, prepare_on_arrival: body.prepare };
      return route.fulfill({ json: current });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  return { page, sent, close: () => context.close() };
}

async function waitFor(condition, timeout = 10_000) {
  const until = Date.now() + timeout;
  while (Date.now() < until) {
    if (condition()) return;
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  throw new Error('timed out');
}

test('chromium: proposals on arrival are off until an administrator switches them on, and off again', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/administration/ai`);
  const toggle = page.getByRole('switch', { name: 'Prepare proposals for new jumble entries' });
  await toggle.waitFor({ timeout: 15_000 });
  assert.equal(await toggle.isChecked(), false, 'on before anybody switched it');

  await toggle.click();
  await waitFor(() => sent.length === 1);
  assert.deepEqual(sent[0], { prepare: true });
  await page.getByText('New jumble entries get a proposal.').waitFor({ state: 'attached', timeout: 10_000 });

  await toggle.click();
  await waitFor(() => sent.length === 2);
  assert.deepEqual(sent[1], { prepare: false });
});

test('chromium: with AI off the switch is not there at all', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, { ai: false });
  t.after(close);

  await page.goto(`${served.origin}/administration/ai`);
  await page.getByRole('heading', { level: 1 }).waitFor({ timeout: 15_000 });
  await page.waitForTimeout(300);
  assert.equal(await page.getByRole('switch', { name: 'Prepare proposals for new jumble entries' }).count(), 0);
});
