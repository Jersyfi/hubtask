// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// An entry's head by its type (user-experience.md §5.6): where a workflow runs, the head carries the
// status with the steps from it, above the notes, and draws neither the level nor *No column*
// beside the type; where none runs, the level and the column mark stay as they were.
//
// Chromium, large and at a phone's width.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ITEMS, MANIFEST, PERMISSIONS, signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const SHAPE = {
  statuses: [
    { key: 'new', name: 'New', category: 'TODO', color_token: 'slate' },
    { key: 'in_progress', name: 'In progress', category: 'IN_PROGRESS', color_token: 'blue' },
    { key: 'resolved', name: 'Resolved', category: 'DONE', color_token: 'green' },
  ],
  initial: 'new',
  steps: [{ key: 'start', name: 'Start work', from: ['new'], to: 'in_progress' }, { key: 'resolve', from: [], to: 'resolved' }],
};
const WORKFLOW = { key: 'incident', system: false, name: 'Incident', published: SHAPE, draft: null, published_at: '2026-10-30T09:00:00Z', version: 1 };
const TYPE = {
  key: 'incident', level: 'TASK', system: false, name: 'Incident', description: '', icon: 'circle-alert', color_token: 'red',
  completable: true, workflow: true, record: false, schedulable: true, estimable: false, allowed_children: [], position: 1, archived: false, version: 1,
};
const ENTRY = ITEMS[3];

async function open(width, { runs }) {
  const browser = await chromium.launch();
  const { context, page } = await signedIn(browser, width, 900);
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const read = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (runs && read === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, modules_on: [...(MANIFEST.modules_on ?? []), 'WORKFLOWS'], workspace_permissions: PERMISSIONS.OWNER } });
    }
    if (runs && /\/containers\/[^/]+\/modules$/.test(read)) return route.fulfill({ json: { modules_on: ['WORKFLOWS'] } });
    if (runs && read === `/items/${ENTRY.id}`) return route.fulfill({ json: { ...ENTRY, entry_type: 'incident', status: 'new' } });
    if (runs && read === `/items/${ENTRY.id}/steps`) {
      return route.fulfill({ json: { workflow: 'incident', status: 'new', steps: [{ key: 'start', name: 'Start work', to: 'in_progress', asks: [] }] } });
    }
    if (runs && read === '/workflows') return route.fulfill({ json: { data: [WORKFLOW], scheme: [{ container_id: ENTRY.collection_id, entry_type: null, workflow: 'incident' }] } });
    if (runs && read === '/entry-types') return route.fulfill({ json: { data: [TYPE] } });
    return stub(route);
  });
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByRole('main').waitFor();
  await page.waitForTimeout(800);
  return { page, close: async () => { await context.close(); await browser.close(); } };
}

for (const width of [1280, 390]) {
  test(`chromium ${width}: a workflow entry's head carries its status and steps, not the level or the column`, { timeout: 60_000 }, async () => {
    const { page, close } = await open(width, { runs: true });
    try {
      const steps = page.locator('[data-steps]');
      await steps.waitFor({ timeout: 10_000 });
      const head = page.locator('.head');
      assert.equal(await head.locator('[data-steps]').count(), 1, 'the status sits in the head');
      assert.ok(await head.locator('[data-status="new"]').isVisible(), 'the status pill is drawn');
      assert.ok(await head.getByRole('button', { name: 'Start work' }).isVisible(), 'the step from it is offered');
      const marks = (await head.locator('.marks').innerText()).trim();
      assert.ok(marks.includes('Incident'), `the type is named: ${marks}`);
      assert.ok(!/\bTask\b/.test(marks), `no level beside the type: ${marks}`);
      assert.equal(await page.locator('[data-column-mark]').count(), 0, 'no column mark');
      assert.ok(!marks.includes('No column'), `no "No column": ${marks}`);
      const scroll = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      assert.ok(scroll <= 0, `nothing scrolls sideways: ${scroll}`);
    } finally {
      await close();
    }
  });
}

test('chromium: an entry without a workflow keeps its level and column mark', { timeout: 60_000 }, async () => {
  const { page, close } = await open(1280, { runs: false });
  try {
    await page.locator('[data-column-mark]').waitFor({ timeout: 10_000 });
    assert.equal(await page.locator('[data-steps]').count(), 0, 'no status where no workflow runs');
    const marks = (await page.locator('.head .marks').innerText()).trim();
    assert.ok(/\bTask\b/.test(marks), `the level is drawn: ${marks}`);
  } finally {
    await close();
  }
});
