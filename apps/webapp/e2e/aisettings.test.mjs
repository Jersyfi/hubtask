// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The workspace's AI screen, walked as an administrator meets it.
//
// UC-AI-03 check 2: the consent names who receives the content and where, in words - not "the
// provider". Against a stubbed API: what is asserted is what the screen shows. Chromium only.

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

const ownProvider = {
  kind: 'OPENAI_COMPATIBLE', base_url: 'https://models.example.org/v1', completion_model: 'chat',
  has_api_key: true, jurisdiction: 'EEA', processing_allowed: false,
  created_at: '2026-09-01T00:00:00Z', version: 1,
};

async function open(browser, routes = {}) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const sent = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (routes[path]) return routes[path](route, request, sent);
    if (path === '/ai-provider' && request.method() === 'GET') return route.fulfill({ json: ownProvider });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  return { page, sent, close: () => context.close() };
}

test('chromium: the consent names who receives the content and where', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/administration/ai`);
  const consent = page.getByRole('switch', { name: /Send this workspace/ });
  await consent.waitFor({ timeout: 15_000 });
  const label = (await consent.getAttribute('aria-label')) ?? (await page.getByText(/Send this workspace/).first().textContent());
  assert.match(label ?? '', /models\.example\.org/, `the consent does not name the receiver: ${label}`);
  assert.match(label ?? '', /in the EEA/, `the consent does not say where: ${label}`);
});
