// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-INS-21 check 14: switching Workflows off where a workflow runs says, before anything is
// written, which places run one, how many open entries stand in a workflow status and how many of
// them could then not even be completed, and asks to convert those places to columns or only to
// pause - nothing chosen, the switch-off waiting for the choice. Converting removes each scheme row
// through the existing scheme write before the switch; pausing writes the switch alone; where no
// workflow runs the question stays as short as before. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { COLLECTION, HUB } from './fixture.mjs';
import { productWalk } from './productwalk.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const status = (key, name, category) => ({ key, name, category, color_token: null });
const OFFER = {
  key: 'offer', system: false, name: 'Offer approval', hub_id: null, draft: null, published_at: '2026-11-01T09:00:00Z', version: 1,
  published: {
    initial: 'draft',
    statuses: [status('draft', 'Draft', 'TODO'), status('check', 'In review', 'IN_PROGRESS'), status('released', 'Released', 'IN_PROGRESS'), status('sent', 'Sent', 'DONE')],
    steps: [
      { key: 'submit', name: 'Submit', from: ['draft'], to: 'check' },
      { key: 'release', name: 'Release', from: ['check'], to: 'released' },
      { key: 'send', name: 'Send', from: ['released'], to: 'sent' },
    ],
  },
};
const column = (id, name, order, statuses, done = false) => ({ id, collection_id: COLLECTION.id, name, order_key: order, wip_limit: null, is_done_bucket: done, color_token: null, version: 1, statuses });
const COLUMNS = [
  column('01a0e2e0-0000-7000-8000-0000000000c1', 'Open', 'a0', ['draft']),
  column('01a0e2e0-0000-7000-8000-0000000000c2', 'Working', 'a1', ['check', 'released']),
  column('01a0e2e0-0000-7000-8000-0000000000c3', 'Done', 'a2', ['sent'], true),
];
const move = (from, count, completed = false) => ({ collection_id: COLLECTION.id, entry_type: 'task', from, to: null, completed, count });
const PREVIEW = {
  moves: [move('draft', 6), move('check', 7), move('released', 4), move('sent', 23, true)],
  entries: 40, removed: [], columns_before: [], columns_after: [],
  boards: [{
    collection_id: COLLECTION.id, level_before: 'WORKFLOW', level_after: 'COLUMNS', done_bucket_id: COLUMNS[2].id,
    columns: COLUMNS.map((each) => ({ bucket_id: each.id, name: each.name, is_done_bucket: each.is_done_bucket, statuses: [], completes: each.is_done_bucket })),
    unmapped: [], moved: 0, column_only: 0,
  }],
};

/** The workspace with Workflows on, its scheme and what each write does, kept in memory. */
function backend({ scheme = [{ container_id: COLLECTION.id, entry_type: null, workflow: OFFER.key }], refuseScheme = false } = {}) {
  const states = [{ module: 'WORKFLOWS', on: true, source: 'WORKSPACE' }];
  const rows = [...scheme];
  return (method, path) => {
    if (path === '/tenant/modules' && method === 'GET') return { json: { modules: states } };
    if (path === '/tenant/modules/WORKFLOWS/reach' && method === 'GET') return { json: { module: 'WORKFLOWS', on: true, people: 14, rules: 3 } };
    if (path === '/tenant/modules/WORKFLOWS' && method === 'PUT') {
      states[0] = { ...states[0], on: false };
      return { json: { modules: states } };
    }
    if (path === '/workflows' && method === 'GET') return { json: { data: [OFFER], scheme: rows } };
    if (path === '/workflow-scheme:preview' && method === 'POST') return { json: PREVIEW };
    if (path === '/workflow-scheme' && method === 'PUT') {
      if (refuseScheme) return { status: 403, json: { type: 'about:blank', title: 'forbidden', status: 403, code: 'forbidden', detail_code: 'auth.forbidden' } };
      rows.splice(0, rows.length);
      return { json: { container_id: COLLECTION.id, entry_type: null, workflow: null, entries_moved: 17 } };
    }
    if (path === `/containers/${COLLECTION.id}/buckets` && method === 'GET') return { json: COLUMNS };
    if (path === `/containers/${COLLECTION.id}` && method === 'GET') return { json: COLLECTION };
    if (path === `/containers/${HUB.id}` && method === 'GET') return { json: HUB };
    return undefined;
  };
}

async function askToSwitchOff(browser, options) {
  const walk = await productWalk(browser, { modules: ['WORKFLOWS'], answer: backend(options) });
  await walk.page.goto(`${served.origin}/administration/modules`);
  const row = walk.page.locator('[data-module="WORKFLOWS"]');
  await row.getByRole('switch').waitFor({ timeout: 10_000 });
  await row.getByRole('switch').click();
  // The APG Alert Dialog: a consequence that interrupts on purpose.
  const dialog = walk.page.getByRole('alertdialog', { name: 'Switch Workflows off?' });
  await dialog.waitFor({ timeout: 5_000 });
  return { ...walk, dialog };
}

const switchedOff = (written) => written.filter((each) => each.method === 'PUT' && each.path === '/tenant/modules/WORKFLOWS');

test('chromium: switching Workflows off lists what it strands and waits for a choice; pausing writes the switch alone', async (t) => {
  const browser = await sharedChromium(t);
  const { page, dialog, written, close, errors } = await askToSwitchOff(browser);
  t.after(close);
  // The focus starts on the safe answer, never on the switch.
  await page.waitForFunction(() => document.activeElement?.textContent?.trim() === 'Keep it on', undefined, { timeout: 5_000 });
  const open = dialog.locator('[data-off-open]');
  await open.waitFor({ timeout: 10_000 });
  const text = await dialog.innerText();
  assert.match(text, /1 place runs a workflow\./);
  assert.match(text, /Kitchen \(House\)/, 'the place is named with its hub');
  assert.match(text, /17 open entries stand in a status that exists only under a workflow\./);
  assert.match(text, /13 of them could then not even be completed/);
  assert.match(text, /3 switched-on rules use it and pause/);
  // The rules are named by count; where they are managed is one link away (no filter by product there).
  assert.equal(await dialog.getByRole('link', { name: 'View rules' }).getAttribute('href'), '/administration/rules');
  // Nothing is chosen, and the switch waits for the choice with its reason.
  const options = dialog.getByRole('radio');
  assert.equal(await options.count(), 2);
  assert.equal(await dialog.getByRole('radio', { checked: true }).count(), 0, 'a choice was made for the person');
  const confirm = dialog.getByRole('button', { name: 'Switch off', exact: true });
  assert.equal(await confirm.getAttribute('aria-disabled'), 'true');
  await confirm.click({ force: true });
  assert.deepEqual(switchedOff(written), [], 'switched off without a choice');

  // The consequences are one list, a line each; the two answers are cards of one named radio group,
  // chosen with the arrow keys as any radio group.
  assert.equal(await dialog.getByRole('list', { name: 'What switching off reaches' }).getByRole('listitem').count(), 4);
  const choice = dialog.getByRole('radiogroup', { name: 'What should happen to the 17 entries?' });
  await choice.getByRole('radio', { name: /Convert to columns/ }).focus();
  await page.keyboard.press('ArrowDown');
  assert.equal(await choice.getByRole('radio', { name: /Only pause/ }).isChecked(), true, 'the arrow key did not choose the next card');
  await dialog.getByText(/the cards of these boards do not move/).waitFor({ timeout: 5_000 });
  const answered = page.waitForResponse((response) => response.url().endsWith('/tenant/modules/WORKFLOWS'));
  await dialog.getByRole('button', { name: 'Pause and switch off' }).click();
  await answered;
  assert.equal(switchedOff(written).length, 1);
  assert.equal(written.filter((each) => each.path === '/workflow-scheme').length, 0, 'pausing converted a place');
  assert.deepEqual(errors(), []);
});

test('chromium: converting to columns shows where the entries go, removes each scheme row, then switches off', async (t) => {
  const browser = await sharedChromium(t);
  const { page, dialog, written, close, errors } = await askToSwitchOff(browser);
  t.after(close);
  await dialog.locator('[data-off-open]').waitFor({ timeout: 10_000 });
  await dialog.getByRole('radio', { name: /Convert to columns/ }).check();
  const where = dialog.locator('[data-off-where]');
  await where.waitFor({ timeout: 5_000 });
  assert.match(await where.innerText(), /Kitchen/);
  // A fold, open at once, with one list per collection: each status keeps the column that showed it,
  // with how many; done entries stay done and are not listed.
  assert.equal(await where.getByRole('button', { name: 'Where the entries go' }).getAttribute('aria-expanded'), 'true');
  const kitchen = where.getByRole('list', { name: 'Kitchen' });
  const lines = await kitchen.locator('[data-off-line]').evaluateAll((rows) => rows.map((row) => [...row.children].map((cell) => cell.textContent.trim())));
  assert.deepEqual(lines, [
    ['Draft → Open', '6 entries'], ['In review → Working', '7 entries'], ['Released → Working', '4 entries'],
  ]);
  // The scheme write held until the run is read: the question gives way to where it is and how far.
  let release = () => {};
  const held = new Promise((done) => (release = done));
  await page.route('**/api/v1/workflow-scheme', async (route) => {
    if (route.request().method() === 'PUT') await held;
    return route.fallback();
  });
  const answered = page.waitForResponse((response) => response.url().endsWith('/tenant/modules/WORKFLOWS'));
  await dialog.getByRole('button', { name: 'Convert and switch off' }).click();
  const run = page.getByRole('alertdialog', { name: 'Converting …' });
  await run.getByText('Converting 1 of 1: Kitchen (House)').waitFor({ timeout: 5_000 });
  assert.equal(await run.getByRole('progressbar', { name: 'Converting' }).getAttribute('value'), '0');
  assert.match(await run.innerText(), /If something stops, Workflows stays on/);
  assert.equal(await run.getByRole('button', { name: 'Keep it on' }).count(), 0, 'the run offers a way out it cannot keep');
  release();
  await answered;
  const order = written.filter((each) => each.method === 'PUT').map((each) => each.path);
  assert.deepEqual(order, ['/workflow-scheme', '/tenant/modules/WORKFLOWS'], 'the places are converted before the switch');
  const removal = written.find((each) => each.path === '/workflow-scheme');
  assert.deepEqual(removal?.body, { container_id: COLLECTION.id, confirm: true }, 'the removal names a workflow');
  assert.deepEqual(errors(), []);
});

test('chromium: a conversion that stops keeps Workflows on and says how far it came', async (t) => {
  const browser = await sharedChromium(t);
  const { dialog, written, close } = await askToSwitchOff(browser, { refuseScheme: true });
  t.after(close);
  await dialog.locator('[data-off-open]').waitFor({ timeout: 10_000 });
  await dialog.getByRole('radio', { name: /Convert to columns/ }).check();
  await dialog.getByRole('button', { name: 'Convert and switch off' }).click();
  await dialog.getByText(/Converting stopped: 0 of 1 places run on columns now/).waitFor({ timeout: 5_000 });
  // The progress the conversion drew stays where it stopped: no place done of one, and the switch.
  assert.equal(await dialog.getByRole('progressbar', { name: 'Converting' }).getAttribute('value'), '0');
  assert.deepEqual(switchedOff(written), [], 'switched off after a conversion that stopped');
});

test('chromium: where no workflow runs, switching Workflows off asks as briefly as before', async (t) => {
  const browser = await sharedChromium(t);
  const { page, dialog, written, close } = await askToSwitchOff(browser, { scheme: [] });
  t.after(close);
  await dialog.getByText(/every entry's history stay stored/).waitFor({ timeout: 10_000 });
  assert.equal(await dialog.getByRole('radio').count(), 0, 'a choice is asked with nothing to choose for');
  const answered = page.waitForResponse((response) => response.url().endsWith('/tenant/modules/WORKFLOWS'));
  await dialog.getByRole('button', { name: 'Switch off', exact: true }).click();
  await answered;
  assert.equal(switchedOff(written).length, 1);
});
