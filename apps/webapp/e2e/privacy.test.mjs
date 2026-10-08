// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The register of people's requests, walked where a deadline is extended (UC-PRV-01 checks 9 and
// 10): the extension offered only on a case that answers a bound, the form starting on that bound
// and sending three facts, and an extended case showing both dates and its reason as a sentence.
// An installation-wide case is the operator's: the screen offers nothing on it (UC-PRV-06).
//
// Chromium only: what this asserts is the rendered row and the request that leaves, and neither
// has an engine-specific part.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { PRIVACY_REQUESTS, stub, forgetUnstubbed, unstubbedSoFar } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const [OPEN, EXTENDED, INSTALLATION] = PRIVACY_REQUESTS;

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser) {
  const written = [];
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    // The write is kept for the assertions and answered by the fixture, as every walk's is.
    if (path.endsWith(':extend') && request.method() === 'POST') {
      written.push({ path, body: request.postDataJSON(), idempotencyKey: request.headers()['idempotency-key'] });
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
  await page.goto(`${served.origin}/administration/privacy`);
  await page.locator('section.panel', { hasText: OPEN.subject_email }).waitFor();
  return { page, failures, written, close: () => context.close() };
}

const row = (page, request) => page.locator('section.panel', { hasText: request.subject_email });

test('chromium: an extended case shows both deadlines and why', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await open(browser);
    const text = await row(page, EXTENDED).innerText();
    assert.match(text, /Owed .+, extended from .+/);
    assert.match(text, /Extended once because of the number of requests\. They were told on .+2026\./);
    // Once extended, never offered again.
    assert.equal(await row(page, EXTENDED).getByRole('button', { name: 'Extend the deadline' }).count(), 0);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

test('chromium: the extension starts on the bound the case answers and sends three facts', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, written, close } = await open(browser);
    const open_ = row(page, OPEN);
    await open_.getByRole('button', { name: 'Extend the deadline' }).click();

    const until = open_.getByLabel('New deadline');
    assert.equal(await until.inputValue(), OPEN.extendable_until);
    assert.equal(await until.getAttribute('max'), OPEN.extendable_until);

    await until.fill('2026-12-01');
    await open_.getByLabel('Why').selectOption('NUMBER_OF_REQUESTS');
    await open_.getByLabel('The day you told them').fill('2026-09-12');
    await open_.getByRole('button', { name: 'Extend it' }).click();

    for (let waited = 0; written.length === 0 && waited < 5000; waited += 50) await page.waitForTimeout(50);
    assert.equal(written.length, 1);
    assert.deepEqual(written[0].body, { due_on: '2026-12-01', reason: 'NUMBER_OF_REQUESTS', informed_on: '2026-09-12' });
    assert.ok(written[0].idempotencyKey, 'the extension carries an Idempotency-Key');
    assert.deepEqual(failures, []);
    assert.deepEqual(unstubbedSoFar().filter((each) => each.includes('privacy')), []);
    await close();
  } finally {
    await browser.close();
  }
});

test('chromium: an installation-wide case offers no extension here', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await open(browser);
    assert.equal(await row(page, INSTALLATION).getByRole('button', { name: 'Extend the deadline' }).count(), 0);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});
