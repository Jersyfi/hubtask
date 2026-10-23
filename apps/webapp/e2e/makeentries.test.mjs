// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *Make entries* in the capture field (UC-AI-09), walked against the shared fixture with the
// server's answers for this walk routed here:
//
// * check 1 - with AI on, pasted text of more than one line offers *Make entries*; one line does
//   not, and without AI nothing is drawn;
// * check 2 - the proposal lists the entries with the fields the server proposed, and marks the one
//   whose title looks like an existing entry;
// * check 3 - nothing is created before a press: *Take* on one creates that one and leaves the rest,
//   *Take all* creates the rest and closes the field, *Dismiss* creates nothing;
// * check 4 is the server's (the material carries nobody's content who keeps it out), proved by
//   `core/application/service/suggestion/MakeEntries_test.go`.
//
// Chromium only, as the other walks; compact and large, light and dark.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, AI_MANIFEST, COLLECTION, GROCERIES, HUB, MANIFEST, PAGE, signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const FIELD = 'What do you want to remember?';
const JOB = '01a0e2e0-0000-7000-8000-0000000020a1';
const PROPOSAL = '01a0e2e0-0000-7000-8000-0000000020a2';
const MILK_THERE = '01a0e2e0-0000-7000-8000-0000000020a3';
const RECIPE = 'Pancakes for Sunday\n200 g flour\n2 eggs\n1 l milk';

const dialogOf = (page) => page.getByRole('dialog', { name: 'Capture' });
const fieldOf = (page) => dialogOf(page).getByRole('textbox', { name: FIELD });

/** The proposal the server holds for the walk, narrowed as entries are taken. */
function proposalWith(entries, status = 'PROPOSED') {
  const at = new Date().toISOString();
  return {
    id: PROPOSAL, target_type: 'ACCOUNT', target_id: ACCOUNT.id, kind: 'ENTRIES', status,
    payload: { collection_id: GROCERIES.id, type: 'TASK', entries },
    source: 'AI', model: 'local-model-7b', prompt_id: 'make-entries', prompt_version: 'v1',
    produced_at: at, created_at: at, version: 1, dropped_nodes: 0,
  };
}

const PROPOSED = [
  { key: '1', title: 'Flour', amount: '200 g' },
  { key: '2', title: 'Eggs', amount: '2' },
  { key: '3', title: 'Milk', amount: '1 l' },
];

async function open(browser, width, { ai, scheme = 'light' }) {
  const { context, page, failures } = await signedIn(browser, width, 900);
  await page.emulateMedia({ colorScheme: scheme });
  const writes = [];
  let entries = [...PROPOSED];
  let decided = 'PROPOSED';
  let made = 0;
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (request.method() !== 'GET') writes.push({ call: `${request.method()} ${path}`, body: request.postDataJSON?.() ?? null });
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: ai ? { ...AI_MANIFEST, features: { ...AI_MANIFEST.features, semantic_search: true } } : MANIFEST });
    }
    if (path === `/containers/${GROCERIES.id}`) return route.fulfill({ json: GROCERIES });
    if (path === '/containers' && url.searchParams.get('parent_id') === HUB.id) return route.fulfill({ json: { ...PAGE, data: [COLLECTION, GROCERIES] } });
    if (path === `/containers/${GROCERIES.id}:make-entries`) {
      return route.fulfill({ status: 202, json: { job_id: JOB, status: 'QUEUED', progress: null, result_url: null } });
    }
    if (path === `/jobs/${JOB}`) {
      const at = new Date().toISOString();
      return route.fulfill({ json: { job_id: JOB, status: 'SUCCEEDED', outcome_code: 'ai.proposed', progress: null, result_url: null, error_code: null, created_at: at, finished_at: at } });
    }
    if (path === '/suggestions' && url.searchParams.get('target_type') === 'ACCOUNT') {
      const asked = writes.some((each) => each.call === `POST /containers/${GROCERIES.id}:make-entries`);
      return route.fulfill({ json: { items: asked ? [proposalWith(entries, decided)] : [], has_more: false, next_cursor: null } });
    }
    if (path === `/suggestions/${PROPOSAL}:accept`) {
      const keys = request.postDataJSON()?.take;
      const taken = keys ? entries.filter((each) => keys.includes(each.key)) : entries;
      entries = entries.filter((each) => !taken.includes(each));
      if (entries.length === 0) decided = 'ACCEPTED';
      const created_ids = taken.map(() => `01a0e2e0-0000-7000-8000-0000000020b${made++}`);
      return route.fulfill({ json: { ...proposalWith(entries.length > 0 ? entries : taken, decided), created_ids } });
    }
    if (path === `/suggestions/${PROPOSAL}:dismiss`) {
      decided = 'DISMISSED';
      return route.fulfill({ json: proposalWith(entries, decided) });
    }
    // The resemblance read: an entry called Milk is already on the list.
    if (path === '/items:duplicates') {
      const title = request.postDataJSON()?.title ?? '';
      const items = title === 'Milk' ? [{ item_id: MILK_THERE, title: 'Milk', collection_id: GROCERIES.id, similarity: 0.97 }] : [];
      return route.fulfill({ json: { items } });
    }
    return stub(route);
  });
  await page.goto(`${served.origin}/collections/${GROCERIES.id}`);
  await page.getByRole('main').waitFor();
  await page.waitForTimeout(600);
  await page.locator('body').click({ position: { x: 1, y: 300 } });
  await page.keyboard.press('q');
  await fieldOf(page).waitFor({ timeout: 5_000 });
  return { page, failures, writes, close: () => context.close() };
}

const callsOf = (writes, call) => writes.filter((each) => each.call === call);

for (const [width, size] of [[1280, 'large'], [375, 'compact']]) {
  for (const scheme of ['light', 'dark']) {
    test(`chromium: ${size}, ${scheme} — pasted lines become a proposal; one taken, then all, nothing before`, { timeout: 90_000 }, async () => {
      const browser = await chromium.launch();
      const { page, failures, writes, close } = await open(browser, width, { ai: true, scheme });
      try {
        // Check 1: one line offers nothing; more than one offers *Make entries*.
        await fieldOf(page).fill('Flour');
        await page.waitForTimeout(300);
        assert.equal(await dialogOf(page).getByRole('button', { name: 'Make entries' }).count(), 0, 'one line offers it');
        await fieldOf(page).fill(RECIPE);
        await dialogOf(page).getByRole('button', { name: 'Make entries' }).click();

        // Check 2: the entries with their amounts, and the one already there marked.
        const proposal = dialogOf(page).getByRole('list', { name: 'Suggested entries' });
        await proposal.waitFor({ timeout: 15_000 });
        const asked = callsOf(writes, `POST /containers/${GROCERIES.id}:make-entries`);
        assert.equal(asked.length, 1);
        assert.equal(asked[0].body.text, RECIPE, 'the text travels whole, in a body');
        for (const text of ['Flour', 'Eggs', 'Milk', '200 g', '1 l']) await proposal.getByText(text, { exact: true }).first().waitFor({ timeout: 5_000 });
        const milk = proposal.getByRole('listitem').filter({ has: page.getByText('Milk', { exact: true }) }).first();
        await milk.getByText('Possibly already here:').waitFor({ timeout: 5_000 });
        assert.equal(await proposal.getByRole('listitem').filter({ hasText: 'Flour' }).getByText('Possibly already here:').count(), 0, 'Flour is not like anything');
        assert.equal(callsOf(writes, 'POST /items').length + callsOf(writes, `POST /suggestions/${PROPOSAL}:accept`).length, 0, 'nothing was created before a press');
        if (process.env.EVIDENCE) await page.screenshot({ path: join(process.env.EVIDENCE, `ux-20-proposal-${size}-${scheme}.png`) });

        // Check 3: one entry taken, the rest still proposed, and the way back offered.
        await proposal.getByRole('button', { name: 'Take “Eggs”' }).click();
        await page.locator('.stack').getByText('Added to Groceries').waitFor({ timeout: 5_000 });
        assert.deepEqual(callsOf(writes, `POST /suggestions/${PROPOSAL}:accept`)[0].body, { take: ['2'] });
        await proposal.getByText('Eggs', { exact: true }).waitFor({ state: 'detached', timeout: 5_000 });
        await proposal.getByText('Flour', { exact: true }).waitFor({ timeout: 5_000 });

        await dialogOf(page).getByRole('button', { name: 'Take all' }).click();
        await dialogOf(page).waitFor({ state: 'detached', timeout: 5_000 });
        const accepts = callsOf(writes, `POST /suggestions/${PROPOSAL}:accept`);
        assert.equal(accepts.length, 2);
        assert.deepEqual(accepts[1].body, {}, 'Take all names no entry');
        await page.locator('.stack').getByText('2 entries added to Groceries').waitFor({ timeout: 5_000 });
        assert.equal(callsOf(writes, 'POST /items').length, 0, 'the entries are the acceptance’s, not the field’s');
        assert.deepEqual(failures, []);
      } finally {
        await close();
        await browser.close();
      }
    });
  }
}

test('chromium: Dismiss creates nothing and keeps the text', { timeout: 60_000 }, async () => {
  const browser = await chromium.launch();
  const { page, failures, writes, close } = await open(browser, 1280, { ai: true });
  try {
    await fieldOf(page).fill(RECIPE);
    await dialogOf(page).getByRole('button', { name: 'Make entries' }).click();
    await dialogOf(page).getByRole('list', { name: 'Suggested entries' }).waitFor({ timeout: 15_000 });
    await dialogOf(page).getByRole('button', { name: 'Dismiss' }).click();
    await dialogOf(page).getByRole('list', { name: 'Suggested entries' }).waitFor({ state: 'detached', timeout: 5_000 });
    assert.equal(callsOf(writes, `POST /suggestions/${PROPOSAL}:dismiss`).length, 1);
    assert.equal(callsOf(writes, `POST /suggestions/${PROPOSAL}:accept`).length + callsOf(writes, 'POST /items').length, 0);
    assert.equal(await fieldOf(page).inputValue(), RECIPE, 'the text stays to capture by hand');
    assert.deepEqual(failures, []);
  } finally {
    await close();
    await browser.close();
  }
});

test('chromium: without AI, pasted lines offer nothing to make entries with (check 1)', { timeout: 60_000 }, async () => {
  const browser = await chromium.launch();
  const { page, failures, close } = await open(browser, 1280, { ai: false });
  try {
    await fieldOf(page).fill(RECIPE);
    await page.waitForTimeout(500);
    assert.equal(await dialogOf(page).getByRole('button', { name: 'Make entries' }).count(), 0);
    assert.equal(await dialogOf(page).locator('[data-ai]').count(), 0, 'no AI area drawn at all');
    assert.deepEqual(failures, []);
  } finally {
    await close();
    await browser.close();
  }
});
