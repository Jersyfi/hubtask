// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Proposals prepared on arrival (UC-JUM-07 checks 10 and 11): an administrator switches them, and
// a jumble card follows the proposal nobody asked for and starts its acceptance on the collection
// it names. Against a stubbed API. Chromium only. `EVIDENCE=<dir>` keeps screenshots.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { AI_SOURCE, DESTINATIONS, MANIFEST, signedIn, stub } from './fixture.mjs';
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

async function evidence(page, name) {
  if (!process.env.EVIDENCE) return;
  // `SCHEME=dark` takes the same pictures in the dark theme.
  if (process.env.SCHEME) await page.emulateMedia({ colorScheme: process.env.SCHEME });
  await page.screenshot({ path: join(process.env.EVIDENCE, name.replace('.png', `-${process.env.SCHEME ?? 'light'}.png`)), fullPage: true });
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
  await evidence(page, 'arrival-0-switch-1280.png');

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

// The card of an arrival the workspace prepares a proposal for.

const JOB = '01a0e2e0-0000-7000-8000-0000000009a1';
const ARRIVAL = {
  id: '01a0e2e0-0000-7000-8000-0000000009e1', channel: 'EMAIL', sender: 'orders@example.org',
  raw_subject: 'Order 42', raw_body: 'Please ship by Friday.', attachments: [], status: 'NEW',
  received_at: '2026-10-21T09:00:00Z', arrival_job_id: JOB,
};
const PROPOSAL = {
  id: '01a0e2e0-0000-7000-8000-0000000009b1', target_type: 'JUMBLE_ENTRY', target_id: ARRIVAL.id,
  kind: 'FIELDS', status: 'PROPOSED', payload: { title: 'Ship order 42', collection_id: DESTINATIONS[0].id },
  source: 'AI', model: 'local-model-7b', prompt_id: 'suggest-fields', prompt_version: 'v4',
  produced_at: '2030-01-01T00:00:01Z', created_at: '2030-01-01T00:00:01Z', version: 1,
};

async function openJumble(browser, width, { job, entries = [ARRIVAL] }) {
  const { page, context, failures } = await signedIn(browser, width, 1000);
  await page.clock.install();
  let proposed = false;
  const accepted = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, features: { ...MANIFEST.features, ai_suggestions: true } } });
    }
    if (path === '/jumble/entries' && request.method() === 'GET') {
      return route.fulfill({ json: { data: entries, page: { next_cursor: null, has_more: false } } });
    }
    if (path.startsWith('/jobs/')) {
      const answer = job();
      if (answer.outcome_code === 'ai.proposed') proposed = true;
      return route.fulfill({ json: { job_id: path.slice(6), progress: null, result_url: null, error_code: null, created_at: '2026-10-21T09:00:00Z', finished_at: null, ...answer } });
    }
    if (path === '/suggestions') {
      return route.fulfill({ json: { items: proposed ? [PROPOSAL] : [], has_more: false, next_cursor: null } });
    }
    if (path === `/suggestions/${PROPOSAL.id}:accept`) {
      accepted.push(request.postDataJSON());
      return route.fulfill({ json: { ...PROPOSAL, status: 'ACCEPTED' } });
    }
    return route.fallback();
  });
  await page.goto(`${served.origin}/jumble`);
  await page.getByText('Order 42').first().waitFor({ timeout: 15_000 });
  return { page, failures, accepted, close: () => context.close() };
}

/** Moves the page's clock on a step at a time: each read of the follow schedules the next wait. */
async function advance(page, ms, step = 1_000) {
  for (let gone = 0; gone < ms; gone += step) {
    await page.clock.runFor(step);
    await new Promise((resolve) => setTimeout(resolve, 20));
  }
}

for (const width of [1280, 768, 375]) {
  test(`chromium: ${width} px — a new entry shows the proposal nobody asked for, with its collection`, { timeout: 90_000 }, async () => {
    const browser = await chromium.launch();
    let reads = 0;
    const { page, failures, accepted, close } = await openJumble(browser, width, {
      job: () => (++reads < 2 ? { status: 'RUNNING' } : { status: 'SUCCEEDED', outcome_code: 'ai.proposed', finished_at: '2026-10-21T09:00:05Z' }),
    });
    try {
      await page.getByText('Suggesting…', { exact: true }).first().waitFor({ timeout: 5_000 });
      await evidence(page, `arrival-1-suggesting-${width}.png`);
      await advance(page, 8_000);
      await page.getByText('Ship order 42').first().waitFor({ timeout: 5_000 });
      assert.equal(
        (await page.locator('[data-proposed-collection]').textContent())?.trim(),
        `${DESTINATIONS[0].hub_name} · ${DESTINATIONS[0].name}`,
      );
      // Nothing counts it: no figure beside the jumble anywhere it is named.
      for (const link of await page.getByRole('link', { name: /Jumble/ }).all()) {
        assert.doesNotMatch((await link.textContent()) ?? '', /\d/, 'the jumble is counted');
      }
      await evidence(page, `arrival-2-proposed-${width}.png`);

      await page.getByRole('button', { name: 'Make this entry' }).first().click();
      assert.equal(await page.getByLabel('Where it goes').first().inputValue(), DESTINATIONS[0].id, 'the proposed collection is not preselected');
      await page.getByRole('button', { name: 'Make the entry' }).first().click();
      await waitFor(() => accepted.length === 1);
      assert.equal(accepted[0]?.overrides?.collection_id, DESTINATIONS[0].id);
      assert.deepEqual(failures, []);
    } finally {
      await close();
      await browser.close();
    }
  });
}

test('chromium: a spent budget is said once for the page, not on every card', { timeout: 90_000 }, async () => {
  const browser = await chromium.launch();
  const second = { ...ARRIVAL, id: '01a0e2e0-0000-7000-8000-0000000009e2', raw_subject: 'Order 43', arrival_job_id: '01a0e2e0-0000-7000-8000-0000000009a2' };
  const { page, failures, close } = await openJumble(browser, 1280, {
    entries: [ARRIVAL, second],
    job: () => ({ status: 'FAILED', error_code: 'ai.unavailable', error_params: { reason: 'budget_spent', renews_at: '2026-10-22T00:00:00Z' } }),
  });
  try {
    await advance(page, 4_000);
    await page.locator('[data-arrival-budget]').waitFor({ timeout: 5_000 });
    assert.equal(await page.locator('[data-arrival-budget]').count(), 1);
    assert.equal(await page.getByText(/Today's AI budget/).count(), 1, 'the budget was said on a card too');
    assert.equal(await page.getByText('Suggesting…', { exact: true }).count(), 0);
    await evidence(page, 'arrival-3-budget-1280.png');
    assert.deepEqual(failures, []);
  } finally {
    await close();
    await browser.close();
  }
});
