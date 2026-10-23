// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *Ask* (UC-AI-08), driven the way a person drives it, against the shared fixture:
//
// * check 1 - with AI the palette offers *Ask* for typed words; without it no such row and nothing
//   about AI is drawn;
// * check 2 - the answer is headed as a proposal, its sources are links (one the reader can no longer
//   open is left out), its provenance sits behind one collapsed line;
// * check 4 - the filter it implies is chips, changed and removed, and *Save as view* saves what the
//   chips hold; without a place the place chip is marked rather than the button switched off;
// * check 5 - a second question is sent as nothing but its words, and nothing is applied: the only
//   writes are the question, the view the person saved, and *Dismiss*.
//
// Check 3, which entries are read, is the server's: `AskMaterial_test.go` and
// `test/integration/ai_answer_test.go`. *Stop* and the nearly-spent budget note are walked on a
// question whose job does not end. Chromium only, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { AI_MANIFEST, ASK_GONE, ASK_JOB, ASK_PLACE, ASK_SOURCES, COLLECTION, ITEMS, MANIFEST, OTHER, signedIn } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const QUESTION = 'What is open for Müller GmbH this week?';
const FOLLOW_UP = 'Who painted the hallway?';

/**
 * The workspace with AI, and the item query's fields as the server declares them (`view/Field.go`):
 * the shared manifest names three, and a chip's condition is sent only for a field and an operator
 * the manifest declares.
 */
const ASK_MANIFEST = {
  ...AI_MANIFEST,
  query_fields: [
    ...AI_MANIFEST.query_fields.filter((each) => each.field !== 'due_at'),
    { field: 'due_at', operators: ['LT', 'LTE', 'GT', 'GTE', 'BETWEEN', 'IS_NULL'], sortable: true, groupable: false, nullable: true },
    { field: 'is_completed', operators: ['EQ', 'NEQ'], sortable: false, groupable: false, nullable: false },
    { field: 'assignee_id', operators: ['EQ', 'NEQ', 'IN', 'NOT_IN', 'IS_NULL'], sortable: false, groupable: true, nullable: true },
    { field: 'labels', operators: ['CONTAINS', 'CONTAINS_ANY', 'CONTAINS_ALL'], sortable: false, groupable: false, nullable: false },
  ],
};

/** A signed-in tab on *Kitchen*, every write it sends recorded. `routes` answers before the fixture. */
async function open(browser, width, { ai = true, scheme = 'light', routes } = {}) {
  const { page, context, failures } = await signedIn(browser, width, 900);
  await page.emulateMedia({ colorScheme: scheme });
  const writes = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    // A query is a read sent as a POST; everything else that is not a GET changes something.
    if (!['GET', 'HEAD'].includes(request.method()) && !path.endsWith(':query') && !['/sync:pull', '/sync:snapshot', '/search'].includes(path)) {
      let body = null;
      try { body = request.postDataJSON(); } catch { body = null; }
      writes.push({ method: request.method(), path, body });
    }
    if (path === '/meta/capabilities') return route.fulfill({ json: ai ? ASK_MANIFEST : MANIFEST });
    if (path === '/views' && request.method() === 'POST') {
      const body = request.postDataJSON();
      return route.fulfill({ status: 201, json: { id: '01a0e2e0-0000-7000-8000-0000000007d8', owner_id: 'me', grouping: null, visible_fields: [], sharing: 'PRIVATE', version: 1, ...body } });
    }
    if (routes && (await routes(route, path))) return undefined;
    return route.fallback();
  });
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.getByText(ITEMS[2].title).first().waitFor({ timeout: 15_000 });
  return { page, failures, writes, close: () => context.close() };
}

/** Types `words` into the palette and returns its rows' text. */
async function typeInPalette(page, words) {
  await page.keyboard.press('Control+k');
  const field = page.getByRole('combobox', { name: 'Search everything' }).last();
  await field.waitFor();
  await field.fill(words);
  await page.locator('#search-menu').getByRole('option').first().waitFor({ timeout: 5_000 });
  return page.locator('#search-menu').getByRole('option').allTextContents();
}

async function evidence(page, name) {
  if (!process.env.EVIDENCE) return;
  await page.screenshot({ path: join(process.env.EVIDENCE, name), fullPage: false, animations: 'disabled' });
}

for (const [width, size] of [[1280, 'large'], [800, 'medium'], [375, 'compact']]) {
  for (const scheme of ['light', 'dark']) {
    test(`chromium: ${size}, ${scheme} — a question in the palette comes back as a proposed answer, its filter saved as a view`, { timeout: 90_000 }, async () => {
      const browser = await chromium.launch();
      const { page, failures, writes, close } = await open(browser, width, { scheme });
      try {
        // Check 1: the row for the words typed.
        const rows = await typeInPalette(page, QUESTION);
        assert.ok(rows.some((row) => row.includes(`Ask: “${QUESTION}”`)), `no Ask row among ${rows.join(' | ')}`);
        await evidence(page, `${size}-${scheme}-palette.png`);
        await page.locator('#search-menu').getByRole('option', { name: /^Ask: “/ }).click();

        // Check 2: headed as a proposal, its sources as links, the provenance collapsed.
        const dialog = page.getByRole('dialog', { name: 'Ask the workspace' });
        await dialog.locator('[data-answer]').waitFor({ timeout: 15_000 });
        await dialog.getByText('Proposed answer', { exact: true }).waitFor();
        for (const source of ASK_SOURCES) {
          const link = dialog.getByRole('link', { name: source.title });
          await link.waitFor({ timeout: 5_000 });
          assert.equal(await link.getAttribute('href'), `/items/${source.id}`);
        }
        await page.waitForResponse((response) => response.url().endsWith(`/items/${ASK_GONE}`), { timeout: 2_000 }).catch(() => {});
        assert.equal(await dialog.locator(`[data-source="${ASK_GONE}"]`).count(), 0, 'an entry the reader cannot open is left out');
        const provenance = dialog.locator('details.provenance');
        assert.equal(await provenance.getAttribute('open'), null, 'the provenance starts collapsed');
        assert.equal(await provenance.locator('summary').count(), 1, 'behind one line');
        await evidence(page, `${size}-${scheme}-answer.png`);
        await provenance.locator('summary').click();
        await provenance.getByText(/local-model-7b · .* · prompt answer-question v1/).waitFor();

        // Check 4: the chips, each changeable or removable, and the view they save.
        await dialog.locator('[data-chip="place"]').getByText(ASK_PLACE.name).waitFor();
        await dialog.locator('[data-chip="assignee"]').getByText(OTHER.display_name).waitFor();
        assert.equal(await dialog.getByRole('combobox', { name: 'State' }).inputValue(), 'false');
        assert.equal(await dialog.getByRole('combobox', { name: 'Due' }).inputValue(), 'next_7_days');
        await dialog.locator('[data-chip="place"]').scrollIntoViewIfNeeded();
        await evidence(page, `${size}-${scheme}-chips.png`);
        await dialog.getByRole('combobox', { name: 'Due' }).selectOption('today');
        await dialog.getByRole('button', { name: 'Remove A person' }).click();
        assert.equal(await dialog.locator('[data-chip="assignee"]').count(), 0);
        await dialog.getByLabel('Name of the view').fill('Müller GmbH today');
        await dialog.getByRole('button', { name: 'Save as view' }).click();
        await page.getByText('The view was saved.').first().waitFor({ state: 'attached', timeout: 5_000 });
        const saved = writes.filter((each) => each.path === '/views');
        assert.equal(saved.length, 1, 'one view saved');
        assert.equal(saved[0].body.scope_type, 'ACCOUNT');
        assert.equal(saved[0].body.name, 'Müller GmbH today');
        assert.equal(saved[0].body.query.scope_container_id, ASK_PLACE.id);
        const filter = JSON.stringify(saved[0].body.query.filter);
        assert.match(filter, /is_completed/, 'the state chip is saved');
        assert.match(filter, /"value":"@today"/, 'the changed due chip is saved');
        assert.match(filter, /@today\+P1D/, 'the changed due chip is saved, to its end');
        assert.doesNotMatch(filter, new RegExp(OTHER.id), 'the removed person chip is not');

        // Without a place, the place chip is marked on submit - nothing is switched off before.
        await dialog.getByRole('button', { name: 'Remove A place' }).click();
        await dialog.getByLabel('Name of the view').fill('Anywhere today');
        await dialog.getByRole('button', { name: 'Save as view' }).click();
        await dialog.locator('[data-missing="place"]').waitFor();
        assert.equal(writes.filter((each) => each.path === '/views').length, 1, 'nothing saved without a place');
        if (size === 'large') await evidence(page, `${size}-${scheme}-place-missing.png`);

        // Check 5: dismissed, then a new question that carries nothing of the first.
        await dialog.getByRole('button', { name: 'Dismiss' }).click();
        await dialog.locator('[data-answer]').waitFor({ state: 'detached' });
        await dialog.getByLabel('Your question').fill(FOLLOW_UP);
        await dialog.getByRole('button', { name: 'Ask', exact: true }).click();
        await dialog.locator('[data-answer]').getByText(/painted the hallway/).waitFor({ timeout: 15_000 });
        assert.equal(await dialog.locator('[data-chip]').count(), 0, 'an answer that is no filter shows no chip');
        assert.equal(await dialog.getByRole('button', { name: 'Save as view' }).count(), 0);
        const asked = writes.filter((each) => each.path === '/ask').map((each) => each.body);
        assert.deepEqual(asked, [{ question: QUESTION }, { question: FOLLOW_UP }], 'each question is its words alone');
        if (size === 'large') await evidence(page, `${size}-${scheme}-second.png`);

        // Nothing applied: the writes are the questions, the view, and the dismissal.
        const kinds = writes.map((each) => `${each.method} ${each.path}`);
        for (const kind of kinds) {
          assert.ok(kind === 'POST /ask' || kind === 'POST /views' || /^POST \/suggestions\/[^/]+:dismiss$/.test(kind), `an unexpected write: ${kind}`);
        }
        assert.deepEqual(failures, []);
      } finally {
        await close();
        await browser.close();
      }
    });
  }
}

test('chromium: a question on its way shows the budget note and stops with Stop', { timeout: 60_000 }, async () => {
  const browser = await chromium.launch();
  const cancelled = [];
  const { page, failures, close } = await open(browser, 1280, {
    routes: async (route, path) => {
      if (path === '/ai-budget-state') await route.fulfill({ json: { state: 'NEARLY_SPENT' } });
      else if (path === `/jobs/${ASK_JOB}`) await route.fulfill({ json: { job_id: ASK_JOB, status: 'RUNNING', progress: null, result_url: null } });
      else if (path === `/jobs/${ASK_JOB}:cancel`) {
        cancelled.push(path);
        await route.fulfill({ json: { job_id: ASK_JOB, status: 'CANCELLED', progress: null, result_url: null } });
      } else return false;
      return true;
    },
  });
  try {
    await typeInPalette(page, QUESTION);
    await page.locator('#search-menu').getByRole('option', { name: /^Ask: “/ }).click();
    const dialog = page.getByRole('dialog', { name: 'Ask the workspace' });
    await dialog.getByText("Today's AI budget is nearly used up.").waitFor({ timeout: 10_000 });
    const pending = dialog.locator('section.proposal[data-state="pending"]');
    await pending.waitFor({ timeout: 10_000 });
    await evidence(page, 'large-light-pending.png');
    await pending.getByRole('button', { name: 'Stop' }).click();
    await pending.waitFor({ state: 'detached', timeout: 5_000 });
    assert.deepEqual(cancelled, [`/jobs/${ASK_JOB}:cancel`], 'Stop cancels the job');
    assert.equal(await dialog.locator('[data-answer]').count(), 0);
    assert.deepEqual(failures, []);
  } finally {
    await close();
    await browser.close();
  }
});

test('chromium: without AI the palette offers no Ask and draws nothing of AI', { timeout: 60_000 }, async () => {
  const browser = await chromium.launch();
  const { page, failures, writes, close } = await open(browser, 1280, { ai: false });
  try {
    const rows = await typeInPalette(page, QUESTION);
    assert.ok(!rows.some((row) => row.includes('Ask:')), `an Ask row without AI: ${rows.join(' | ')}`);
    assert.equal(await page.locator('[data-ai]').count(), 0, 'no AI area drawn at all');
    assert.equal(writes.filter((each) => each.path === '/ask').length, 0);
    assert.deepEqual(failures, []);
  } finally {
    await close();
    await browser.close();
  }
});
