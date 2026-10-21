// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Asking AI about several undecided jumble entries at once (UC-JUM-07 check 12): a mode the page's
// menu starts, a checkbox on each undecided card, one ask per entry, each followed on its card.
// Against a stubbed API. Chromium only. `EVIDENCE=<dir>` keeps screenshots.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { MANIFEST, PERMISSIONS, signedIn } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const entry = (n, status = 'NEW') => ({
  id: `01a0e2e0-0000-7000-8000-0000000008e${n}`, channel: 'EMAIL', sender: 'orders@example.org',
  raw_subject: `Order ${n}`, raw_body: 'Please ship.', attachments: [], status, received_at: '2026-10-21T09:00:00Z',
});
const ENTRIES = [entry(1), entry(2), entry(3)];

async function openJumble(browser, width, { ai = true, permissions, onAsk } = {}) {
  const { page, context, failures } = await signedIn(browser, width, 1000);
  const asked = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') {
      return route.fulfill({
        json: {
          ...MANIFEST, features: { ...MANIFEST.features, ai_suggestions: ai },
          ...(permissions ? { workspace_permissions: permissions } : {}),
        },
      });
    }
    if (path === '/jumble/entries' && request.method() === 'GET') {
      return route.fulfill({ json: { data: ENTRIES, page: { next_cursor: null, has_more: false } } });
    }
    const asking = /^\/jumble\/entries\/([^/:]+):suggest$/.exec(path);
    if (asking && request.method() === 'POST') {
      asked.push(asking[1]);
      return onAsk ? onAsk(route) : route.fulfill({ status: 202, json: { job_id: `01a0e2e0-0000-7000-8000-0000000008a${asked.length}` } });
    }
    if (path.startsWith('/jobs/')) return route.fulfill({ json: { job_id: path.slice(6), status: 'RUNNING' } });
    if (path === '/suggestions') return route.fulfill({ json: { items: [], has_more: false, next_cursor: null } });
    return route.fallback();
  });
  await page.goto(`${served.origin}/jumble`);
  await page.getByText('Order 1').first().waitFor({ timeout: 15_000 });
  return { page, failures, asked, close: () => context.close() };
}

async function startSelecting(page) {
  await page.getByRole('button', { name: 'Jumble actions' }).first().click();
  await page.getByRole('menuitem', { name: 'Select entries' }).click();
  await page.locator('[data-jumble-selection]').waitFor({ timeout: 5_000 });
}

async function evidence(page, name) {
  if (!process.env.EVIDENCE) return;
  await page.screenshot({ path: join(process.env.EVIDENCE, name), fullPage: true });
}

for (const width of [1280, 768, 375]) {
  test(`chromium: ${width} px — the selected undecided entries are asked about, each on its own card`, { timeout: 90_000 }, async () => {
    const browser = await chromium.launch();
    const { page, failures, asked, close } = await openJumble(browser, width);
    try {
      await startSelecting(page);
      await page.getByLabel('Select Order 1').check();
      await page.getByLabel('Select Order 3').check();
      await evidence(page, `selection-1-selected-${width}.png`);
      await page.getByRole('button', { name: 'Ask for suggestions' }).click();
      for (let tries = 0; tries < 100 && asked.length < 2; tries++) await new Promise((resolve) => setTimeout(resolve, 50));
      assert.deepEqual([...asked].sort(), [ENTRIES[0].id, ENTRIES[2].id].sort());
      await page.locator('[data-jumble-selection]').waitFor({ state: 'detached', timeout: 5_000 });
      const card = (subject) => page.locator('article.entry').filter({ hasText: subject });
      for (const [subject, pending] of [['Order 1', 1], ['Order 2', 0], ['Order 3', 1]]) {
        assert.equal(Math.min(await card(subject).getByText('Suggesting…', { exact: true }).count(), 1), pending, subject);
      }
      await evidence(page, `selection-2-asked-${width}.png`);
      assert.deepEqual(failures, []);
    } finally {
      await close();
      await browser.close();
    }
  });
}

test('chromium: a refusal that holds for all is said once and stops the rest', { timeout: 90_000 }, async () => {
  const browser = await chromium.launch();
  const { page, asked, close } = await openJumble(browser, 1280, {
    onAsk: (route) => route.fulfill({
      status: 503, contentType: 'application/problem+json',
      body: JSON.stringify({ status: 503, code: 'unavailable', detail_code: 'ai.unavailable', request_id: 'req_s' }),
    }),
  });
  try {
    await startSelecting(page);
    await page.getByRole('button', { name: 'Select every entry on screen' }).click();
    await page.getByRole('button', { name: 'Ask for suggestions' }).click();
    await page.getByRole('alert').filter({ hasText: /switched off or out of reach/ }).waitFor({ timeout: 5_000 });
    assert.equal(await page.getByText(/switched off or out of reach/).count(), 1);
    assert.ok(asked.length <= 3);
    assert.equal(await page.getByText('Suggesting…', { exact: true }).count(), 0);
  } finally {
    await close();
    await browser.close();
  }
});

test('chromium: who may only read, and a workspace without AI, are offered no selection', { timeout: 90_000 }, async () => {
  const browser = await chromium.launch();
  try {
    for (const options of [{ permissions: PERMISSIONS.VIEWER }, { ai: false }]) {
      const { page, close } = await openJumble(browser, 1280, options);
      try {
        await page.waitForTimeout(300);
        assert.equal(await page.getByRole('button', { name: 'Jumble actions' }).count(), 0, JSON.stringify(options));
        assert.equal(await page.getByRole('checkbox').count(), 0);
      } finally {
        await close();
      }
    }
  } finally {
    await browser.close();
  }
});
