// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// AI about a collection, where its subject is (ux-inventory.md §E4): the ✦ at the collection's
// figure asks how it stands, the answer stands under the head and is kept as an entry whose notes
// it is; a template drafted for the collection is drawn in *Structure → Templates*, not on the
// page. Without AI, neither is drawn. Chromium, over the shared fixture's collection.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { AI_MANIFEST, COLLECTION, ITEMS, MANIFEST, forgetUnstubbed, stub, unstubbedSoFar } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const JOB = '01a0e2e0-0000-7000-8000-0000000009c1';
const SUMMARY_ID = '01a0e2e0-0000-7000-8000-0000000009c2';
const DRAFT_ID = '01a0e2e0-0000-7000-8000-0000000009c3';
const SUMMARY = 'Three entries are open; the tiles are overdue since Monday.';

const suggestion = (id, kind, payload, prompt, status = 'PROPOSED', at = '2030-01-01T00:00:00Z') => ({
  id, target_type: 'CONTAINER', target_id: COLLECTION.id, kind, status, payload, source: 'AI', model: 'local-model-7b',
  prompt_id: prompt, prompt_version: 'v1', produced_at: at, created_at: at, version: 1, dropped_nodes: 0,
});

/** A tab whose workspace has AI or not, with the collection's suggestions and writes answered here. */
async function at(browser, hasAi, width = 1280) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  forgetUnstubbed();
  const state = { asked: false, summary: undefined, dismissed: [], created: [] };
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: hasAi ? AI_MANIFEST : MANIFEST });
    if (path === `/containers/${COLLECTION.id}:summarize` && request.method() === 'POST') {
      state.asked = true;
      const now = new Date(Date.now() + 1000).toISOString();
      state.summary = suggestion(SUMMARY_ID, 'FIELDS', { notes: SUMMARY }, 'summarize-collection', 'PROPOSED', now);
      return route.fulfill({ status: 202, json: { job_id: JOB, status: 'QUEUED', progress: null, result_url: null } });
    }
    if (path === `/jobs/${JOB}`) {
      return route.fulfill({ json: { job_id: JOB, status: 'SUCCEEDED', outcome_code: 'ai.proposed', progress: null, result_url: null, error_code: null, created_at: new Date().toISOString(), finished_at: new Date().toISOString() } });
    }
    if (path === '/suggestions' && url.searchParams.get('target_type') === 'CONTAINER') {
      const draft = suggestion(DRAFT_ID, 'TEMPLATE', { scope_type: 'COLLECTION', name: 'Room by room', root_type: 'TASK', nodes: [{ type: 'WORK_PACKAGE', title: 'Strip the walls' }] }, 'generate-template');
      const items = [draft, ...(state.summary ? [state.summary] : [])].filter((each) => !state.dismissed.includes(each.id));
      return route.fulfill({ json: { items, has_more: false, next_cursor: null } });
    }
    if (path === `/suggestions/${SUMMARY_ID}:dismiss` && request.method() === 'POST') {
      state.dismissed.push(SUMMARY_ID);
      return route.fulfill({ json: { ...state.summary, status: 'DISMISSED' } });
    }
    if (path === '/items' && request.method() === 'POST') {
      const body = request.postDataJSON();
      state.created.push(body);
      return route.fulfill({ status: 201, json: { ...ITEMS[0], id: '01a0e2e0-0000-7000-8000-0000000009c4', ...body } });
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
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.getByRole('heading', { name: COLLECTION.name, level: 1 }).waitFor({ state: 'attached', timeout: 15_000 });
  await page.waitForTimeout(600);
  return { context, page, failures, state };
}

test('chromium: the ✦ at the figure asks how the collection stands, and the answer is kept as an entry', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures, state } = await at(browser, true);
  t.after(() => context.close());

  // The ask stands in the head, where what counts is said; no button of its own below it.
  const ask = page.locator('header').getByRole('button', { name: 'Ask AI how this collection stands' });
  assert.equal(await ask.count(), 1);
  assert.equal(await page.getByRole('button', { name: 'Ask AI how this collection stands' }).count(), 1, 'a second ask');
  // A template's draft is the Templates tab's, not the page's.
  // (The *Structure* panel, closed, keeps its tabs mounted: what counts is what is shown.)
  const shown = await page.getByText('Room by room', { exact: true }).evaluateAll((all) => all.filter((each) => each.checkVisibility()).length);
  assert.equal(shown, 0, 'the draft is drawn on the page');

  await ask.click();
  const answer = page.locator('[data-ai="summary"]').getByText(SUMMARY);
  await answer.waitFor({ timeout: 10_000 });
  assert.ok(state.asked);
  await page.getByRole('button', { name: 'Keep as an entry' }).click();
  await answer.waitFor({ state: 'detached', timeout: 5_000 });
  assert.equal(state.created.length, 1);
  assert.equal(state.created[0].collection_id, COLLECTION.id);
  assert.equal(state.created[0].notes, SUMMARY);
  assert.match(state.created[0].title, /^How this collection stood on /);
  assert.deepEqual(state.dismissed, [SUMMARY_ID], 'the kept answer is closed');
  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbedSoFar(), []);
});

test('chromium: a template drafted for the collection stands in Structure → Templates', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await at(browser, true);
  t.after(() => context.close());
  await page.getByRole('button', { name: `Actions for ${COLLECTION.name}` }).click();
  await page.getByRole('menuitem', { name: 'Structure' }).click();
  const panel = page.locator('dialog[open]');
  await panel.getByRole('tab', { name: 'Templates' }).click();
  const draft = panel.locator('[data-ai="template"]');
  await draft.getByText('Room by room', { exact: true }).waitFor({ timeout: 10_000 });
  assert.equal(await draft.getByText('Strip the walls').count(), 1, 'the draft without its tree');
  assert.equal(await draft.getByRole('button', { name: 'Keep as a template' }).count(), 1);
  assert.deepEqual(failures, []);
});

test('chromium: without AI, no ✦ at the figure and no AI area', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await at(browser, false);
  t.after(() => context.close());
  assert.equal(await page.getByRole('button', { name: 'Ask AI how this collection stands' }).count(), 0);
  assert.equal(await page.locator('[data-ai]').count(), 0);
  assert.deepEqual(failures, []);
});
