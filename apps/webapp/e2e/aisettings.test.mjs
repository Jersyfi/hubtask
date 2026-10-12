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

const LARGE = { id: '01a0e2e0-0000-7000-8000-0000000006a1', display_name: 'Large (EU)', processor: 'Example AI SAS', jurisdiction: 'EEA' };

/** A workspace's AI source under a rule, as `GET /ai-source` answers it. */
const sourceUnder = (sources, extra = {}) => ({
  source: 'OWN', sources, installation_sources: sources, sources_origin: 'INSTANCE', min_jurisdiction: 'ANY',
  min_jurisdiction_origin: 'DEFAULT', offered: [LARGE], search_rebuilding: false, version: 1, ...extra,
});

function sourceRoutes(answer) {
  let current = answer;
  return {
    '/ai-source': (route, request, sent) => {
      if (request.method() === 'PUT') {
        const body = request.postDataJSON();
        sent.push(body);
        current = { ...current, source: body.source, offered_model_id: body.offered_model_id };
      }
      return route.fulfill({ json: current });
    },
    '/quotas': (route) => route.fulfill({ json: [
      { quota: 'ai_tokens_per_day', limit: 200000, used: 1234 },
      { quota: 'ai_own_tokens_per_day', limit: 0, used: 56 },
    ] }),
  };
}

// D1 (UC-AI-06 check 8): nothing offered, nothing decided - today's screen, and no word of an
// offered model, a lock or a minimum.
test('chromium: a private installation sees only its own model, as always', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/administration/ai`);
  await page.getByLabel(/^Provider/).waitFor({ timeout: 15_000 });
  const text = await page.locator('main').textContent();
  assert.doesNotMatch(text ?? '', /your provider|offered|This workspace may use|must process content/i);
});

// D5 (UC-AI-05 checks 1, 2; UC-AI-02 check 5): the offered model locked - one card, one switch,
// processor and where in words, and no own form. Switching it on is the consent, one request.
test('chromium: a consumer switches on the offered model with one control', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser, sourceRoutes(sourceUnder('OFFERED', { sources_lock: 'INSTANCE' })));
  t.after(close);

  await page.goto(`${served.origin}/administration/ai`);
  const use = page.getByRole('switch', { name: 'Use Large (EU)' });
  await use.waitFor({ timeout: 15_000 });
  assert.match(await page.locator('main').textContent() ?? '', /sent to Example AI SAS in the EEA when somebody asks/);
  assert.equal(await page.getByLabel(/^Provider/).count(), 0, 'an own model is offered where only the provider’s may be used');
  assert.equal(await page.getByLabel('This workspace may use').count(), 0, 'a locked rule is offered for narrowing');
  assert.match(await page.locator('main').textContent() ?? '', /The offered model: today 1,234 tokens of 200,000 tokens used/);
  // Only the offered model may be used: its budget, and no word of an own one (P-05).
  assert.doesNotMatch(await page.locator('main').textContent() ?? '', /Our own model:/);
  assert.equal(await page.getByLabel(/^Daily budget of our own model/).count(), 0);

  await use.check();
  for (let i = 0; i < 50 && sent.length === 0; i++) await page.waitForTimeout(100);
  assert.deepEqual(sent[0], { source: 'OFFERED', offered_model_id: LARGE.id });
  assert.ok(await use.isChecked(), 'the switch does not show the model in use');
});

// D6 (UC-AI-05 check 7, UC-AI-06 check 4): either source on one screen, the own form kept, and the
// minimum said with who set it.
test('chromium: a company chooses between the offered model and its own', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser, sourceRoutes(sourceUnder('EITHER', {
    min_jurisdiction: 'EEA', min_jurisdiction_origin: 'INSTANCE',
  })));
  t.after(close);

  await page.goto(`${served.origin}/administration/ai`);
  await page.getByRole('switch', { name: 'Use Large (EU)' }).waitFor({ timeout: 15_000 });
  const own = page.getByRole('switch', { name: 'Use our own model' });
  assert.ok(await own.isChecked(), 'the own model in use is not shown as chosen');
  await page.getByLabel(/^Provider/).waitFor();
  assert.match(await page.locator('main').textContent() ?? '', /must process content in the EEA: set for every workspace on this installation/);
  // A declaration the minimum refuses is not offered (P-05).
  const offeredWhere = await page.getByLabel(/^Where the provider processes/).locator('option').allTextContents();
  assert.ok(!offeredWhere.some((each) => /adequacy|third country/i.test(each)), offeredWhere.join(' | '));
  await page.getByLabel('This workspace may use').waitFor();

  await page.getByRole('switch', { name: 'Use Large (EU)' }).check();
  for (let i = 0; i < 50 && sent.length === 0; i++) await page.waitForTimeout(100);
  assert.deepEqual(sent[0], { source: 'OFFERED', offered_model_id: LARGE.id });
});

// UC-AI-07 checks 2 and 4, UC-AI-06 check 6: either source allowed - each source's use and cap side
// by side, and the workspace sets its own model's budget here, an empty field being no budget.
test('chromium: each source shows its own budget, and the workspace sets its own', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const budgets = [];
  const routes = sourceRoutes(sourceUnder('EITHER'));
  routes['/ai-budget'] = (route, request) => {
    budgets.push(request.postDataJSON());
    return route.fulfill({ json: [
      { quota: 'ai_tokens_per_day', limit: 200000, used: 1234 },
      { quota: 'ai_own_tokens_per_day', limit: 5000, used: 56, configured: true },
    ] });
  };
  const { page, close } = await open(browser, routes);
  t.after(close);

  await page.goto(`${served.origin}/administration/ai`);
  await page.getByRole('switch', { name: 'Use Large (EU)' }).waitFor({ timeout: 15_000 });
  const text = await page.locator('main').textContent() ?? '';
  assert.match(text, /The offered model: today 1,234 tokens of 200,000 tokens used/);
  assert.match(text, /Our own model: today 56 tokens used, with no budget/);
  assert.match(text, /in this workspace's time zone/);

  const field = page.getByLabel(/^Daily budget of our own model/);
  await field.fill('5000');
  await page.getByRole('button', { name: 'Save the budget' }).click();
  for (let i = 0; i < 50 && budgets.length === 0; i++) await page.waitForTimeout(100);
  assert.deepEqual(budgets[0], { own_tokens_per_day: 5000 });

  await field.fill('');
  await page.getByRole('button', { name: 'Save the budget' }).click();
  for (let i = 0; i < 50 && budgets.length < 2; i++) await page.waitForTimeout(100);
  assert.deepEqual(budgets[1], { own_tokens_per_day: null });
});
