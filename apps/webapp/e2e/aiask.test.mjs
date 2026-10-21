// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Asking AI about an entry ends on every end (UC-AI-04 check 4): a refused ask says so at once and
// leaves no pending card; a job that proposed something shows the proposal; a job that proposed
// nothing says why in a calm note; a job that takes longer than a minute says so honestly while the
// strip keeps following. Against a stubbed API: what is asserted is what the screen shows. The
// clock is Playwright's, so the minute passes in a moment. Chromium only, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ITEMS, MANIFEST, signedIn } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const ENTRY = ITEMS[0];
const JOB = '01a0e2e0-0000-7000-8000-0000000007a1';

const served = await serve(DIST);
test.after(() => served.close());

/** A proposal about the entry, made after any ask a walk makes. */
const PROPOSAL = {
  id: '01a0e2e0-0000-7000-8000-0000000007b1',
  target_type: 'WORK_ITEM',
  target_id: ENTRY.id,
  kind: 'FIELDS',
  status: 'PROPOSED',
  payload: { title: 'Book the tiler for Thursday' },
  source: 'AI',
  model: 'local-model-7b',
  prompt_id: 'suggest-item-fields',
  prompt_version: 'v3',
  produced_at: '2030-01-01T00:00:00Z',
  created_at: '2030-01-01T00:00:01Z',
  version: 1,
};

/**
 * Opens the entry with AI on, the ask answered by `onAsk` and the job by `job`. The listing holds
 * the proposal once the job says it proposed one.
 */
async function openEntry(browser, width, { onAsk, job }) {
  const { page, context, failures } = await signedIn(browser, width, 1000);
  await page.clock.install();
  let proposed = false;
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith('/api/v1/meta/capabilities')) {
      return route.fulfill({ json: { ...MANIFEST, features: { ...MANIFEST.features, ai_suggestions: true } } });
    }
    if (request.method() === 'POST' && path.includes(`/items/${ENTRY.id}:`)) return onAsk(route);
    if (path.endsWith(`/jobs/${JOB}`)) {
      const answer = job();
      if (answer.outcome_code === 'ai.proposed') proposed = true;
      return route.fulfill({ json: { job_id: JOB, progress: null, result_url: null, error_code: null, created_at: '2026-10-17T09:00:00Z', finished_at: null, ...answer } });
    }
    if (path.endsWith('/api/v1/suggestions')) {
      return route.fulfill({ json: { items: proposed ? [PROPOSAL] : [], has_more: false, data: proposed ? [PROPOSAL] : [], page: { next_cursor: null, has_more: false } } });
    }
    return route.fallback();
  });
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByRole('button', { name: 'Ask AI about this entry' }).waitFor({ timeout: 15_000 });
  return { page, failures, close: () => context.close() };
}

const accepted = (route) => route.fulfill({ status: 202, json: { job_id: JOB, status: 'QUEUED', progress: null, result_url: null } });

/** Asks one question from the ✦ of one subject: the title's unless another menu is named. */
async function ask(page, operation, menu = 'Ask AI about this entry') {
  await page.getByRole('button', { name: menu }).click();
  await page.getByRole('menuitem', { name: operation }).click();
}

async function evidence(page, name) {
  if (!process.env.EVIDENCE) return;
  await page.locator('main').first().screenshot({ path: join(process.env.EVIDENCE, name) });
}

const pending = (page) => page.getByText('Suggesting…', { exact: true });

/**
 * Moves the page's clock on by `ms`, a step at a time: each read of the follow schedules the next
 * wait only once its answer is in, so one long jump would pass a single wait.
 */
async function advance(page, ms, step = 1_000) {
  for (let gone = 0; gone < ms; gone += step) {
    await page.clock.runFor(step);
    await new Promise((resolve) => setTimeout(resolve, 20));
  }
}

for (const width of [1280, 375]) {
  const size = width === 1280 ? 'desktop' : '375';

  test(`chromium: ${width} px — a refused ask says so at once and leaves no pending card`, { timeout: 90_000 }, async () => {
    const browser = await chromium.launch();
    const { page, failures, close } = await openEntry(browser, width, {
      onAsk: (route) =>
        route.fulfill({
          status: 503,
          contentType: 'application/problem+json',
          body: JSON.stringify({ status: 503, code: 'unavailable', detail_code: 'ai.unavailable', request_id: 'req_a' }),
        }),
      job: () => ({ status: 'QUEUED' }),
    });
    try {
      await ask(page, 'Suggest a title');
      await page.getByRole('alert').filter({ hasText: /switched off or out of reach/ }).waitFor({ timeout: 5_000 });
      assert.equal(await pending(page).count(), 0, 'no pending card after a refusal');
      assert.ok(await page.getByRole('button', { name: 'Ask AI about this entry' }).isEnabled(), 'asking again is possible');
      await evidence(page, `1-refused-${size}.png`);
      assert.deepEqual(failures, []);
    } finally {
      await close();
      await browser.close();
    }
  });

  test(`chromium: ${width} px — a job that proposed something shows the proposal`, { timeout: 90_000 }, async () => {
    const browser = await chromium.launch();
    let reads = 0;
    const { page, failures, close } = await openEntry(browser, width, {
      onAsk: accepted,
      job: () => (++reads < 2 ? { status: 'RUNNING' } : { status: 'SUCCEEDED', outcome_code: 'ai.proposed', finished_at: '2026-10-17T09:00:03Z' }),
    });
    try {
      await ask(page, 'Suggest a title');
      await pending(page).first().waitFor({ timeout: 5_000 });
      await advance(page, 5_000);
      await page.getByText('Book the tiler for Thursday').first().waitFor({ timeout: 5_000 });
      await pending(page).waitFor({ state: 'detached', timeout: 5_000 });
      await evidence(page, `2-proposed-${size}.png`);
      assert.deepEqual(failures, []);
    } finally {
      await close();
      await browser.close();
    }
  });

  test(`chromium: ${width} px — a job that proposed nothing says why, at once`, { timeout: 90_000 }, async () => {
    const browser = await chromium.launch();
    const { page, failures, close } = await openEntry(browser, width, {
      onAsk: accepted,
      job: () => ({ status: 'SUCCEEDED', outcome_code: 'ai.nothing_to_change', finished_at: '2026-10-17T09:00:02Z' }),
    });
    try {
      await ask(page, 'Summarise the entry', 'Ask AI about the notes');
      await advance(page, 2_000);
      const note = page.locator('[data-note="ai.nothing_to_change"]');
      await note.waitFor({ timeout: 5_000 });
      assert.equal(await note.textContent(), 'AI found nothing to propose here.');
      assert.equal(await pending(page).count(), 0, 'pending ends with the note');
      await evidence(page, `3-nothing-to-change-${size}.png`);
      assert.deepEqual(failures, []);
    } finally {
      await close();
      await browser.close();
    }
  });

  test(`chromium: ${width} px — a job past a minute is said to be slow, and still followed`, { timeout: 90_000 }, async () => {
    const browser = await chromium.launch();
    let done = false;
    let reads = 0;
    const { page, failures, close } = await openEntry(browser, width, {
      onAsk: accepted,
      job: () => {
        reads++;
        return done ? { status: 'SUCCEEDED', outcome_code: 'ai.proposed', finished_at: '2026-10-17T09:02:00Z' } : { status: 'RUNNING' };
      },
    });
    try {
      await ask(page, 'Suggest a title');
      await advance(page, 70_000);
      const slow = page.locator('[data-note="app.suggestions.slow"]');
      await slow.waitFor({ timeout: 5_000 });
      assert.equal(await slow.textContent(), 'The AI is taking longer than usual. The suggestion appears here when it arrives.');
      assert.equal(await pending(page).count(), 1, 'the pending card stays while the job runs');
      await evidence(page, `4-slow-${size}.png`);

      const before = reads;
      await advance(page, 16_000);
      assert.ok(reads > before, 'still reading the job after the minute');

      done = true;
      await advance(page, 16_000);
      await page.getByText('Book the tiler for Thursday').first().waitFor({ timeout: 5_000 });
      assert.equal(await slow.count(), 0, 'the slow note goes with the proposal');
      assert.deepEqual(failures, []);
    } finally {
      await close();
      await browser.close();
    }
  });
}
