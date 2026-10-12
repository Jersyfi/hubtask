// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Keeping one's own content out of AI, as a person meets it on their profile (UC-PRV-05 checks 4,
// 7, 8, 9). Against a stubbed API: what is asserted is what the screen shows and what it sends.
// Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { AI_OBJECTION, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const offered = { ...AI_OBJECTION, shown: true };
const partOfWork = {
  ...AI_OBJECTION, shown: true, person_opt_out: 'NOT_OFFERED', basis: 'WORKS_AGREEMENT',
  contact: 'the works council', since: '2026-10-12T09:00:00Z', keeping_out: true, notice_pending: true,
};

async function open(browser, position) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const sent = [];
  let current = position;
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/privacy/ai-objection' && request.method() === 'GET') return route.fulfill({ json: current });
    if (request.method() === 'POST' && path.startsWith('/privacy/')) {
      sent.push(path);
      if (path === '/privacy/consents:withdraw') current = { ...current, keeping_out: true, in_effect: true };
      if (path === '/privacy/consents:give') current = { ...current, keeping_out: false, in_effect: false };
      if (path === '/privacy/ai-objection:noticed') {
        current = { ...current, notice_pending: false };
        return route.fulfill({ json: current });
      }
      return route.fulfill({ json: { id: '01a0e2e0-0000-7000-8000-0000000000f1', purpose: 'ai_processing', granted: false } });
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

test('chromium: the profile offers Keep my content out of AI, and it records both ways', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser, offered);
  t.after(close);

  await page.goto(`${served.origin}/profile`);
  const keepOut = page.getByRole('switch', { name: 'Keep my content out of AI' });
  await keepOut.waitFor({ timeout: 15_000 });
  assert.equal(await keepOut.isChecked(), false);

  await keepOut.click();
  await assert.doesNotReject(() => waitFor(() => sent.includes('/privacy/consents:withdraw')));
  await keepOut.click();
  await assert.doesNotReject(() => waitFor(() => sent.includes('/privacy/consents:give')));
});

test('chromium: where there is nothing to choose, the profile shows nothing', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, AI_OBJECTION);
  t.after(close);

  await page.goto(`${served.origin}/profile`);
  await page.getByRole('button', { name: 'Save' }).waitFor({ timeout: 15_000 });
  assert.equal(await page.locator('[data-ai-objection]').count(), 0, 'a section was drawn with nothing to choose');
});

test('chromium: where AI is part of everybody\'s work, the basis instead of a switch, and the notice once', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser, partOfWork);
  t.after(close);

  await page.goto(`${served.origin}/profile`);
  const section = page.locator('[data-ai-objection]');
  await section.waitFor({ timeout: 15_000 });
  const text = await section.textContent();
  assert.match(text ?? '', /part of everybody's work, on the basis of a works agreement/);
  assert.match(text ?? '', /ask the works council/);
  assert.equal(await page.getByRole('switch', { name: 'Keep my content out of AI' }).count(), 0, 'a switch the server would not honour');

  const notice = page.locator('[data-ai-notice]');
  await notice.waitFor({ timeout: 15_000 });
  assert.match((await notice.textContent()) ?? '', /no longer takes effect/);
  await notice.getByRole('button', { name: 'Understood' }).click();
  await notice.waitFor({ state: 'detached', timeout: 15_000 });
  assert.ok(sent.includes('/privacy/ai-objection:noticed'), 'the acknowledgement was not sent');
});

async function waitFor(condition, timeout = 10_000) {
  const until = Date.now() + timeout;
  while (Date.now() < until) {
    if (condition()) return;
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  throw new Error('timed out');
}
