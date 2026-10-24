// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// AI where its subject is (UC-AI-04 checks 6-9), walked against the shared fixture with the AI
// reads routed here:
//
// * with AI on and nothing standing, the entry shows a ✦ per subject and no suggestion area;
// * each standing proposal is drawn under its own subject, the look-alikes by their titles;
// * a reader is offered the translation and the summary of the discussion, and closes a proposal
//   rather than deciding it;
// * a nearly spent budget is said in the menu's head, a spent one on every item;
// * a running question is stopped from its card, and the job is cancelled;
// * with AI off, nothing of it is drawn - the rule editor's AI blocks included.
//
// Chromium, at 375, 768 and 1280 px; EVIDENCE=<dir> keeps the screenshots, light and dark.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, BUCKETS, CHILDREN, ITEMS, LABELS, MANIFEST, OWNER_GRANTS, PAGE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const ENTRY = ITEMS[0];
const NEIGHBOUR = ITEMS[1];
const UNKNOWN = '01a0e2e0-0000-7000-8000-0000000009ff';
const JOB = '01a0e2e0-0000-7000-8000-0000000009a1';

const proposal = (n, kind, payload, prompt = 'suggest-item-fields') => ({
  id: `01a0e2e0-0000-7000-8000-0000000009b${n}`, target_type: 'WORK_ITEM', target_id: ENTRY.id, kind, status: 'PROPOSED',
  payload, source: 'AI', model: 'local-model-7b', prompt_id: prompt, prompt_version: 'v3',
  produced_at: '2030-01-01T00:00:00Z', created_at: '2030-01-01T00:00:01Z', version: 1,
});

/** One proposal per subject. */
const STANDING = [
  proposal(1, 'FIELDS', { title: 'Book the tiler for Thursday' }),
  proposal(2, 'FIELDS', { due_date: '2030-02-01' }),
  proposal(3, 'FIELDS', { label_ids: [LABELS[1].id], bucket_id: BUCKETS[1].id }, 'classify'),
  proposal(4, 'DECOMPOSITION', { children: [{ title: 'Pick the grout', type: 'ACTIVITY' }] }, 'decompose'),
  proposal(5, 'FIELDS', { notes: 'We agreed on white tiles.' }, 'summarize-thread'),
  proposal(6, 'DUPLICATES', { duplicates: [{ item_id: NEIGHBOUR.id, similarity: 0.91 }, { item_id: UNKNOWN, similarity: 0.88 }] }, ''),
];

const COMMENT = { id: '01a0e2e0-0000-7000-8000-0000000009c1', item_id: ENTRY.id, author_id: ACCOUNT.id, body: 'White tiles, then.', created_at: '2030-01-01T00:00:00Z', version: 1 };

/**
 * A signed-in tab on the entry: AI on or off, the reader's role on the workspace, what stands, and
 * how the budget stands. Answers the page and what the walk saw asked.
 */
async function openEntry(browser, options = {}) {
  try {
    return await reachEntry(browser, options);
  } catch (error) {
    // A browser left open keeps the walk's process alive after its last test.
    await browser.close();
    throw error;
  }
}

async function reachEntry(browser, { width = 1280, ai = true, role = 'OWNER', standing = [], budget = { state: 'OK', renews_at: '2030-01-02T00:00:00Z' }, scheme = 'light', jobs = () => ({ status: 'RUNNING' }) } = {}) {
  const context = await browser.newContext({ viewport: { width, height: 1100 }, colorScheme: scheme });
  const asked = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, features: { ...MANIFEST.features, ai_suggestions: ai, semantic_search: ai } } });
    }
    if (path === '/memberships') {
      const scope = url.searchParams.get('scope_type');
      const rows = scope === 'TENANT' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role }] : [];
      return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: role === 'OWNER' ? OWNER_GRANTS : [] } });
    }
    if (path === '/suggestions') return route.fulfill({ json: { items: standing, has_more: false } });
    if (path === '/ai-budget-state') return route.fulfill({ json: budget });
    if (path === `/items/${ENTRY.id}/comments`) return route.fulfill({ json: { ...PAGE, data: [COMMENT] } });
    if (path === `/items/${UNKNOWN}`) return route.fulfill({ status: 404, contentType: 'application/problem+json', body: JSON.stringify({ status: 404, code: 'not_found', detail_code: 'items.not_found' }) });
    if (request.method() === 'POST' && path.startsWith(`/items/${ENTRY.id}:`)) {
      asked.push({ path, body: request.postDataJSON() });
      return route.fulfill({ status: 202, json: { job_id: JOB, status: 'QUEUED', progress: null, result_url: null } });
    }
    if (path === `/jobs/${JOB}`) return route.fulfill({ json: { job_id: JOB, progress: null, result_url: null, error_code: null, created_at: '2030-01-01T00:00:00Z', finished_at: null, ...jobs() } });
    if (path === `/jobs/${JOB}:cancel`) {
      asked.push({ path });
      return route.fulfill({ json: { job_id: JOB, status: 'CANCELLED', progress: null, result_url: null, error_code: null, created_at: '2030-01-01T00:00:00Z', finished_at: '2030-01-01T00:00:01Z' } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.emulateMedia({ colorScheme: scheme });
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByText(CHILDREN[ENTRY.id][0].title).first().waitFor({ timeout: 15_000 });
  // The role and the manifest arrive with their reads; give the screen the moment to draw them.
  await page.waitForTimeout(600);
  return { page, failures, asked, close: () => context.close() };
}

async function evidence(page, name) {
  if (!process.env.EVIDENCE) return;
  await page.screenshot({ path: join(process.env.EVIDENCE, name), fullPage: true });
}

const menu = (page, name) => page.getByRole('button', { name, exact: true });

for (const width of [375, 768, 1280]) {
  test(`chromium: ${width} px — a ✦ per subject, and no suggestion area while nothing stands`, { timeout: 90_000 }, async () => {
    const browser = await chromium.launch();
    const { page, failures, close } = await openEntry(browser, { width });
    try {
      for (const name of ['Ask AI about this entry', 'Ask AI about the notes', 'Ask AI about the steps']) {
        assert.equal(await menu(page, name).count(), 1, `${name} is offered`);
      }
      assert.equal(await page.getByText('Nothing has been suggested', { exact: false }).count(), 0, 'no empty area');
      assert.equal(await page.getByRole('heading', { name: 'Suggestions' }).count(), 0, 'no section heading');
      assert.equal(await page.locator('section[data-ai]').count(), 0, 'no proposal drawn');
      await evidence(page, `places-0-nothing-${width}.png`);
      assert.deepEqual(failures, []);
    } finally {
      await close();
      await browser.close();
    }
  });

  for (const scheme of width === 1280 ? ['light', 'dark'] : ['light']) {
    test(`chromium: ${width} px ${scheme} — each proposal stands under its subject, look-alikes by title`, { timeout: 90_000 }, async () => {
      const browser = await chromium.launch();
      const { page, failures, close } = await openEntry(browser, { width, standing: STANDING, scheme });
      try {
        const placeOf = (text) => page.getByText(text, { exact: false }).first().evaluate((node) => {
          const row = node.closest('.with-ai');
          if (row) return row.querySelector('[data-detail]')?.getAttribute('data-detail') ?? 'row';
          return node.closest('[role="tabpanel"]') ? 'comments' : node.closest('.main-column') ? 'steps' : 'head';
        });
        assert.equal(await placeOf('Book the tiler for Thursday'), 'head');
        assert.equal(await placeOf('Pick the grout'), 'steps');
        assert.equal(await placeOf('We agreed on white tiles.'), 'comments');
        // The details fold opens on a phone while a proposal stands in it.
        assert.equal(await page.locator('details.details-fold').evaluate((node) => node.open), true);
        assert.equal(await placeOf('Doing'), 'labels');
        // Named by title, read from the entry; one that cannot be read is said to be one.
        await page.getByRole('link', { name: NEIGHBOUR.title }).first().waitFor({ timeout: 5_000 });
        await page.getByText('An entry you cannot open').waitFor({ timeout: 5_000 });
        assert.equal(await page.getByText(NEIGHBOUR.id).count(), 0, 'no identifier shown');
        await evidence(page, `places-1-standing-${width}-${scheme}.png`);
        assert.deepEqual(failures, []);
      } finally {
        await close();
        await browser.close();
      }
    });
  }
}

test('chromium: a reader is offered the reading aids and closes rather than decides (check 8)', { timeout: 90_000 }, async () => {
  const browser = await chromium.launch();
  const { page, failures, asked, close } = await openEntry(browser, { role: 'VIEWER', standing: [STANDING[4]] });
  try {
    for (const name of ['Ask AI about the notes', 'Ask AI about the steps', 'Ask AI about the due date', 'Ask AI about the labels and the column']) {
      assert.equal(await menu(page, name).count(), 0, `${name} is not offered to a reader`);
    }
    await menu(page, 'Ask AI about this entry').click();
    assert.deepEqual((await page.getByRole('menuitem').allTextContents()).map((text) => text.trim()), ['Read in another language']);
    await page.keyboard.press('Escape');

    assert.equal(await page.getByRole('button', { name: 'Put into the notes' }).count(), 0);
    assert.equal(await page.getByRole('button', { name: 'Dismiss' }).count(), 0);
    await page.getByRole('button', { name: 'Close', exact: true }).click();
    assert.equal(await page.getByText('We agreed on white tiles.').count(), 0, 'closed on this page');

    // The discussion's ✦ asks the server, which serves a reader (D4).
    await menu(page, 'Ask AI about the discussion').click();
    await page.getByRole('menuitem', { name: 'Summarise the discussion' }).click();
    await page.getByText('Suggesting…', { exact: true }).waitFor({ timeout: 5_000 });
    assert.ok(asked.some((each) => each.path.endsWith(':summarize-thread')));
    await evidence(page, 'places-2-reader-1280.png');
    assert.deepEqual(failures, []);
  } finally {
    await close();
    await browser.close();
  }
});

test('chromium: a question on its way is stopped from its card, and its job is cancelled (check 9)', { timeout: 90_000 }, async () => {
  const browser = await chromium.launch();
  const { page, failures, asked, close } = await openEntry(browser);
  try {
    await menu(page, 'Ask AI about the steps').click();
    await page.getByRole('menuitem', { name: 'Break this down' }).click();
    await page.getByText('Suggesting…', { exact: true }).waitFor({ timeout: 5_000 });
    await evidence(page, 'places-3-running-1280.png');
    await page.getByRole('button', { name: 'Stop', exact: true }).click();
    await page.getByText('Suggesting…', { exact: true }).waitFor({ state: 'detached', timeout: 5_000 });
    assert.ok(asked.some((each) => each.path === `/jobs/${JOB}:cancel`), 'the job was cancelled');
    assert.deepEqual(failures, []);
  } finally {
    await close();
    await browser.close();
  }
});

test('chromium: a field question asks for its one field (check 6)', { timeout: 90_000 }, async () => {
  const browser = await chromium.launch();
  const { page, failures, asked, close } = await openEntry(browser);
  try {
    await menu(page, 'Ask AI about this entry').click();
    await page.getByRole('menuitem', { name: 'Suggest a title' }).click();
    await page.getByText('Suggesting…', { exact: true }).waitFor({ timeout: 5_000 });
    const sent = asked.find((each) => each.path.endsWith(':suggest-fields'));
    assert.deepEqual(sent?.body, { fields: ['title'] });
    assert.deepEqual(failures, []);
  } finally {
    await close();
    await browser.close();
  }
});

test('chromium: the budget is said before asking - nearly spent in the head, spent on every item (check 9)', { timeout: 90_000 }, async () => {
  const browser = await chromium.launch();
  try {
    for (const [state, check] of [
      ['NEARLY_SPENT', async (page) => page.getByText("Today's AI budget is nearly used up.").waitFor({ timeout: 5_000 })],
      ['SPENT', async (page) => {
        const items = page.getByRole('menuitem');
        for (const each of await items.all()) assert.equal(await each.getAttribute('aria-disabled'), 'true');
      }],
    ]) {
      const { page, failures, close } = await openEntry(browser, { budget: { state, renews_at: '2030-01-02T00:00:00Z' } });
      try {
        await menu(page, 'Ask AI about this entry').click();
        await check(page);
        await evidence(page, `places-4-budget-${state.toLowerCase()}-1280.png`);
        assert.deepEqual(failures, []);
      } finally {
        await close();
      }
    }
  } finally {
    await browser.close();
  }
});

test('chromium: with AI off nothing of it is drawn (UC-AI-01 check 1)', { timeout: 90_000 }, async () => {
  const browser = await chromium.launch();
  const { page, failures, close } = await openEntry(browser, { ai: false, standing: STANDING });
  try {
    assert.equal(await page.locator('[data-ai]').count(), 0, 'no AI element');
    assert.equal(await page.getByText('Book the tiler for Thursday').count(), 0);
    await evidence(page, 'places-5-ai-off-1280.png');
    assert.deepEqual(failures, []);
  } finally {
    await close();
    await browser.close();
  }
});

test('chromium: while an entry is being created, what already looks like it is named by title (check 7)', { timeout: 90_000 }, async () => {
  const browser = await chromium.launch();
  try {
  for (const ai of [true, false]) {
    const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    const asked = [];
    await context.route('**/api/v1/**', async (route) => {
      const request = route.request();
      const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
      if (path === '/meta/capabilities') {
        return route.fulfill({ json: { ...MANIFEST, features: { ...MANIFEST.features, ai_suggestions: ai, semantic_search: ai } } });
      }
      if (path === '/items:duplicates') {
        asked.push({ body: request.postDataJSON(), key: request.headers()['idempotency-key'] });
        return route.fulfill({ json: { items: [{ item_id: NEIGHBOUR.id, title: NEIGHBOUR.title, collection_id: NEIGHBOUR.collection_id, similarity: 0.9 }] } });
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
    try {
      await page.goto(`${served.origin}/items/${ENTRY.id}`);
      await page.getByRole('button', { name: 'Add an activity', exact: true }).click();
      await page.getByRole('dialog', { name: 'Capture' }).getByRole('textbox', { name: 'What do you want to remember?' }).fill('Book an electrician');
      await page.waitForTimeout(900);
      if (ai) {
        await page.getByText('Possibly already here:').waitFor({ timeout: 5_000 });
        assert.equal(await page.getByRole('link', { name: NEIGHBOUR.title }).count() >= 1, true);
        assert.deepEqual(asked.at(-1)?.body, { title: 'Book an electrician' });
        assert.equal(asked.at(-1)?.key, undefined, 'a read carries no idempotency key');
        await evidence(page, 'places-6-lookalikes-1280.png');
      } else {
        assert.equal(asked.length, 0, 'nothing is asked without AI');
        assert.equal(await page.getByText('Possibly already here:').count(), 0);
      }
      assert.deepEqual(failures, []);
    } finally {
      await context.close();
    }
  }
  } finally {
    await browser.close();
  }
});
