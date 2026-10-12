// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The workspace's position on people keeping their content out of AI, and the consent register
// (UC-PRV-05 checks 9, 11, 12), as an administrator meets them. Against a stubbed API. Chromium
// only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { AI_SOURCE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, source) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const sent = [];
  let current = source;
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/ai-provider' && request.method() === 'GET') return route.fulfill({ status: 404, json: { code: 'ai.not_configured' } });
    if (path === '/ai-source' && request.method() === 'GET') return route.fulfill({ json: current });
    if (path === '/ai-person-opt-out' && request.method() === 'PUT') {
      const body = request.postDataJSON();
      sent.push(body);
      current = { ...current, person_opt_out: body.person_opt_out, person_opt_out_basis: body.basis, person_opt_out_contact: body.contact, person_opt_out_origin: 'WORKSPACE' };
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

test('chromium: AI for everybody needs its basis, and says how many people it touches', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser, { ...AI_SOURCE, keeping_out_count: 2 });
  t.after(close);

  await page.goto(`${served.origin}/administration/ai`);
  const section = page.locator('[data-person-opt-out]');
  await section.waitFor({ timeout: 15_000 });
  await section.getByLabel("AI is part of everybody's work").check();
  assert.match((await section.textContent()) ?? '', /2 people keep their content out of AI now/);

  const save = section.getByRole('button', { name: 'Save' });
  assert.equal(await save.isDisabled() || (await save.getAttribute('aria-disabled')) === 'true', true, 'saved without a basis');
  await section.getByLabel('Legal basis').selectOption('WORKS_AGREEMENT');
  await section.getByLabel('Whom people may ask').fill('the works council');
  await save.click();
  await waitFor(() => sent.length === 1);
  assert.deepEqual(sent[0], { person_opt_out: 'NOT_OFFERED', basis: 'WORKS_AGREEMENT', contact: 'the works council' });
});

test('chromium: under the installation\'s lock the screen says who decided and offers no control', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, { ...AI_SOURCE, person_opt_out_lock: 'INSTANCE', person_opt_out_origin: 'INSTANCE' });
  t.after(close);

  await page.goto(`${served.origin}/administration/ai`);
  const section = page.locator('[data-person-opt-out]');
  await section.waitFor({ timeout: 15_000 });
  assert.match((await section.textContent()) ?? '', /set for every workspace on this installation/);
  assert.equal(await section.getByRole('radio').count(), 0, 'a control the server would refuse');
});

test('chromium: the consent register names who keeps their content out', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, AI_SOURCE);
  t.after(close);

  await page.goto(`${served.origin}/administration/privacy`);
  const register = page.locator('[data-consent-register]');
  await register.waitFor({ timeout: 15_000 });
  assert.match((await register.textContent()) ?? '', /Kim Keeper/);
});

async function waitFor(condition, timeout = 10_000) {
  const until = Date.now() + timeout;
  while (Date.now() < until) {
    if (condition()) return;
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  throw new Error('timed out');
}
