// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// An entry's head by its type (user-experience.md §5.6): where a workflow runs, the head carries the
// status with the steps from it, above the notes, and draws neither the level nor *No column*
// beside the type; where none runs, the level and the column mark stay as they were.
//
// A collection's Workflow settings name the workflow its tasks run, as the collection's head does.
//
// Chromium, large and at a phone's width.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, COLLECTION, ITEMS, LABELS, MANIFEST, OTHER, PERMISSIONS, signedIn, stub } from './fixture.mjs';
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
const BOARD = [
  { id: '00000000-0000-4000-8000-00000000c001', name: 'Ideas', completes: false },
  { id: '00000000-0000-4000-8000-00000000c002', name: 'Building', completes: false },
  { id: '00000000-0000-4000-8000-00000000c003', name: 'Shipped', completes: true },
];

async function open(width, { runs, asks = false, requires = false, path = `/items/${ENTRY.id}`, title, board, patches = [], extra }) {
  const browser = await chromium.launch();
  const { context, page } = await signedIn(browser, width, 900);
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const read = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (runs && read === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, modules_on: [...(MANIFEST.modules_on ?? []), 'WORKFLOWS'], workspace_permissions: PERMISSIONS.OWNER } });
    }
    // The server answers an entry's steps only where the workflows module is on (`module.off`
    // otherwise), so a board whose steps it answers is such a place.
    if (board === true && !runs && read === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, modules_on: [...(MANIFEST.modules_on ?? []), 'WORKFLOWS'] } });
    if (board === true && !runs && /\/containers\/[^/]+\/modules$/.test(read)) return route.fulfill({ json: { modules_on: [...(MANIFEST.modules_on ?? []), 'WORKFLOWS'] } });
    const answered = extra?.(read, request.method());
    if (answered) return route.fulfill({ json: answered });
    if (title && read === `/items/${ENTRY.id}`) return route.fulfill({ json: { ...ENTRY, title } });
    if (runs && /\/containers\/[^/]+\/modules$/.test(read)) return route.fulfill({ json: { modules_on: ['WORKFLOWS'] } });
    if (runs && read === `/items/${ENTRY.id}`) return route.fulfill({ json: { ...ENTRY, entry_type: 'incident', status: 'new' } });
    if (runs && read === `/items/${ENTRY.id}/steps`) {
      const steps = [{ key: 'start', name: 'Start work', to: 'in_progress', asks: [], allowed: true }];
      if (requires) Object.assign(steps[0], { asks: ['custom_fields.reviewer'], requires: ['due_date', 'custom_fields.points'], missing: ['custom_fields.points'] });
      if (asks) steps.push({ key: 'resolve', name: 'Resolve', to: 'resolved', asks: ['custom_fields.resolution'], allowed: true });
      const blocked = asks ? [] : [{ key: 'resolve', name: 'Resolve', to: 'resolved', reason: 'workflows.step_who_unmet', roles: ['ADMIN'], people: [], groups: [] }];
      const picture = { ...SHAPE, steps: SHAPE.steps.map((step) => ({ name: null, limited: step.key === 'resolve', approval: false, ...step })) };
      return route.fulfill({ json: { workflow: 'incident', status: 'new', steps, blocked, done_route: [{ key: 'resolve', name: 'Resolve', to: 'resolved' }], picture, by_default: false, may_design: false } });
    }
    if (board && read === `/items/${ENTRY.id}/steps`) {
      return route.fulfill({ json: { state_mode: 'COLUMNS', columns: BOARD, column_id: BOARD[0].id, workflow: null, status: null, steps: [], blocked: [], done_route: [], by_default: true, may_design: false } });
    }
    // A column chosen in the status menu is the base product's column write (`PATCH bucket_id`),
    // with the Workflows module on or off; the server completes it into the done column.
    if (board && read === `/items/${ENTRY.id}` && request.method() === 'PATCH') {
      patches.push(request.postDataJSON());
      return route.fulfill({ json: { ...ENTRY, bucket_id: BOARD[2].id, version: ENTRY.version + 1, completion: { is_completed: true } } });
    }
    if (board && read.endsWith(':drop')) patches.push({ drop: request.postDataJSON() });
    if (!runs && read === `/items/${ENTRY.id}/steps`) {
      return route.fulfill({ json: { workflow: null, status: null, steps: [], blocked: [], done_route: [], picture: SHAPE, by_default: true, may_design: false } });
    }
    if (asks && read === `/items/${ENTRY.id}:step` && request.method() === 'POST') {
      return route.fulfill({
        status: 422,
        contentType: 'application/problem+json',
        json: {
          type: 'about:blank', title: 'validation_failed', status: 422, code: 'validation_failed', detail_code: 'workflows.step_refused',
          params: { step: 'resolve', count: '2' },
          field_errors: [
            { path: '/custom_fields/resolution', code: 'fields.value_required', params: { key: 'resolution' } },
            { path: '', code: 'workflows.check_failed', params: { check: 'tested', message: 'Attach the test report.' } },
          ],
        },
      });
    }
    if (runs && read === '/workflows') return route.fulfill({ json: { data: [WORKFLOW], scheme: [{ container_id: ENTRY.collection_id, entry_type: null, workflow: 'incident' }] } });
    if (runs && read === '/entry-types') return route.fulfill({ json: { data: [TYPE] } });
    return stub(route);
  });
  await page.goto(`${served.origin}${path}`);
  await page.getByRole('main').waitFor();
  await page.waitForTimeout(800);
  return { page, close: async () => { await context.close(); await browser.close(); } };
}

for (const width of [1280, 390]) {
  test(`chromium ${width}: one head - key, type, title, the status as a button with where it can go next (UC-WRK-44/1, /6, UC-FLW-06/1, /3, /4)`, { timeout: 60_000 }, async () => {
    const { page, close } = await open(width, { runs: true });
    try {
      const status = page.locator('[data-steps]');
      await status.waitFor({ timeout: 10_000 });
      const head = page.locator('[data-tour="entry"]');
      assert.equal(await head.locator('[data-steps]').count(), 1, 'the status sits in the head');
      assert.equal(await head.locator('[data-steps]').getAttribute('data-status'), 'new');
      assert.ok((await head.innerText()).includes('Incident'), 'the type is named');
      assert.equal(await page.locator('[data-column-mark]').count(), 0, 'no column mark where a workflow runs');
      if (width > 600) assert.ok(await head.getByRole('button', { name: 'Start work' }).isVisible(), 'the next step is the primary button');
      // The menu: the allowed step with its target, the blocked one as information with its reason.
      await status.getByRole('button', { name: /Change status/ }).click();
      const surface = page.locator('[role=menu]:visible, dialog[open]').last();
      await surface.waitFor({ timeout: 5_000 });
      const text = await surface.innerText();
      assert.ok(text.includes('In progress'), `the step names its target: ${text}`);
      assert.ok(/Resolve/.test(text) && /only/.test(text), `the blocked step says why: ${text}`);
      assert.ok(await page.getByText('Done is reached by: Resolve').first().isVisible(), 'how done is reached');
      assert.equal(await surface.locator('[data-blocked]').count(), 1, 'one blocked step');
      await page.getByRole('button', { name: 'View workflow' }).filter({ visible: true }).first().click();
      const picture = page.locator('[data-workflow-picture]');
      await picture.waitFor({ timeout: 5_000 });
      assert.equal(await picture.locator('[data-current]').count(), 1, 'the entry\'s status is outlined');
      assert.equal(await page.getByRole('button', { name: 'Open in designer' }).count(), 0, 'no designer without the right to the structure');
      const scroll = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      assert.ok(scroll <= 0, `nothing scrolls sideways: ${scroll}`);
    } finally {
      await close();
    }
  });
}

for (const width of [1280, 390]) {
  test(`chromium ${width}: a step says in the status menu what it asks and what it needs, ✓ where it is filled (E-23)`, { timeout: 60_000 }, async () => {
    const { page, close } = await open(width, { runs: true, requires: true });
    try {
      const status = page.locator('[data-steps]');
      await status.waitFor({ timeout: 10_000 });
      await status.getByRole('button', { name: /Change status/ }).click();
      const surface = page.locator('[role=menu]:visible, dialog[open]').last();
      await surface.waitFor({ timeout: 5_000 });
      const row = surface.locator('button.row').filter({ hasText: 'Start work' });
      const line = await row.innerText();
      assert.ok(line.includes('asks: reviewer'), `what it asks: ${line}`);
      assert.ok(line.includes('needs Due date ✓, points'), `what it needs, ✓ where filled: ${line}`);
    } finally {
      await close();
    }
  });
}

for (const width of [1280, 390]) {
  test(`chromium ${width}: the workflow picture draws each step as an arrow, a legend and a card per step - who, what it asks, blocked or not (E-23)`, { timeout: 60_000 }, async () => {
    const { page, close } = await open(width, { runs: true, requires: true });
    try {
      const status = page.locator('[data-steps]');
      await status.waitFor({ timeout: 10_000 });
      await status.getByRole('button', { name: /Change status/ }).click();
      await page.getByRole('button', { name: 'View workflow' }).filter({ visible: true }).first().click();
      const picture = page.locator('[data-workflow-picture]');
      await picture.waitFor({ timeout: 5_000 });
      assert.equal(await picture.locator('[data-picture-status="new"][data-current]').count(), 1, 'the entry\'s status is outlined');
      // The step from New is an arrow, drawn in the accent: the reader can take it now.
      const arrow = picture.locator('path[data-arrow="start:new"]');
      await arrow.waitFor({ state: 'attached', timeout: 5_000 });
      assert.equal(await arrow.getAttribute('data-kind'), 'now');
      assert.match(await arrow.getAttribute('d') ?? '', /^M [\d.]+ [\d.]+ C /, 'the arrow is measured between the boxes');
      assert.ok((await arrow.getAttribute('marker-end'))?.startsWith('url(#'), 'it ends in an arrowhead');
      // A step from any status is an arrow from the entry's status, dashed where it is blocked for the reader.
      const any = picture.locator('path[data-arrow="resolve:new"]');
      await any.waitFor({ state: 'attached', timeout: 5_000 });
      assert.equal(await any.getAttribute('data-kind'), 'blocked');
      assert.notEqual(await any.evaluate((path) => getComputedStyle(path).strokeDasharray), 'none', 'a blocked step is dashed');
      assert.equal(await picture.locator('[data-label="resolve:new"]').textContent(), 'Resolved · from any status');
      const legend = await picture.locator('[data-picture-legend]').innerText();
      for (const words of ['the entry\'s status', 'steps you can take now', 'other steps', 'blocked for you']) assert.ok(legend.includes(words), `the legend: ${legend}`);
      const start = await picture.locator('[data-step-card="start"]').innerText();
      assert.ok(/you can/.test(start) && start.includes('New → In progress') && start.includes('Who: everyone who may change the entry'), `who may take it: ${start}`);
      assert.ok(start.includes('asks: reviewer') && start.includes('needs Due date ✓, points'), `what it asks: ${start}`);
      const resolve = picture.locator('[data-step-card="resolve"]');
      assert.equal(await resolve.getAttribute('data-kind'), 'blocked');
      const blocked = await resolve.innerText();
      assert.ok(/blocked/.test(blocked) && /only /.test(blocked), `blocked for the reader, with the reason: ${blocked}`);
      const scroll = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      assert.ok(scroll <= 0, `nothing scrolls sideways: ${scroll}`);
      const box = await picture.boundingBox();
      assert.ok(box && box.x >= 0 && box.x + box.width <= width + 1, `the picture fits the width: ${JSON.stringify(box)}`);
    } finally {
      await close();
    }
  });
}

// A story's workflow, as the demo runs it: a step there and one back between two boxes of one lane,
// and a step from any status limited to a role.
const STORY = {
  initial: 'backlog',
  statuses: [
    { key: 'backlog', name: 'Backlog', category: 'BACKLOG', color_token: 'slate' },
    { key: 'todo', name: 'To do', category: 'TODO', color_token: 'slate' },
    { key: 'in_progress', name: 'In progress', category: 'IN_PROGRESS', color_token: 'blue' },
    { key: 'in_review', name: 'In review', category: 'IN_PROGRESS', color_token: 'violet' },
    { key: 'done', name: 'Done', category: 'DONE', color_token: 'green' },
    { key: 'dropped', name: 'Dropped', category: 'BACKLOG', color_token: 'slate' },
  ],
  steps: [
    { key: 'plan', name: 'Plan', from: ['backlog'], to: 'todo' },
    { key: 'start', name: 'Begin', from: ['todo'], to: 'in_progress' },
    { key: 'review', name: 'Ask for review', from: ['in_progress'], to: 'in_review' },
    { key: 'rework', name: 'Rework', from: ['in_review'], to: 'in_progress' },
    { key: 'finish', name: 'Finish', from: ['in_review'], to: 'done' },
    { key: 'reopen', name: 'Reopen', from: ['done'], to: 'todo' },
    { key: 'drop', name: 'Drop', from: [], to: 'dropped' },
  ].map((step) => ({ approval: false, limited: step.key === 'drop', ...step })),
};

for (const width of [1280, 1024]) {
  test(`chromium ${width}: the workflow picture keeps every step's name clear of the others and draws a blocked step dashed (E-23)`, { timeout: 60_000 }, async () => {
    const { page, close } = await open(width, {
      runs: true,
      extra: (read) => {
        if (read === `/items/${ENTRY.id}`) return { ...ENTRY, entry_type: 'incident', status: 'in_progress' };
        if (read !== `/items/${ENTRY.id}/steps`) return undefined;
        return {
          workflow: 'incident', status: 'in_progress', by_default: false, may_design: false, picture: STORY,
          steps: [{ key: 'review', name: 'Ask for review', to: 'in_review', asks: [], allowed: true }],
          blocked: [{ key: 'drop', name: 'Drop', to: 'dropped', reason: 'workflows.step_who_unmet', roles: ['ADMIN'], people: [], groups: [] }],
          done_route: [{ key: 'review', name: 'Ask for review', to: 'in_review' }, { key: 'finish', name: 'Finish', to: 'done' }],
        };
      },
    });
    try {
      const status = page.locator('[data-steps]');
      await status.waitFor({ timeout: 10_000 });
      await status.getByRole('button', { name: /Change status/ }).click();
      await page.getByRole('button', { name: 'View workflow' }).filter({ visible: true }).first().click();
      const picture = page.locator('[data-workflow-picture]');
      await picture.waitFor({ timeout: 5_000 });
      const drop = picture.locator('path[data-arrow="drop:in_progress"]');
      await drop.waitFor({ state: 'attached', timeout: 5_000 });
      assert.equal(await drop.getAttribute('data-kind'), 'blocked');
      assert.notEqual(await drop.evaluate((path) => getComputedStyle(path).strokeDasharray), 'none', 'the step blocked for the reader is dashed');
      await page.waitForTimeout(300);
      const names = await picture.locator('text[data-label]').evaluateAll((texts) => texts.map((text) => {
        const r = text.getBoundingClientRect();
        return { name: text.textContent, left: r.left, right: r.right, top: r.top, bottom: r.bottom };
      }));
      assert.ok(names.some((each) => each.name === 'Ask for review') && names.some((each) => each.name === 'Rework'), JSON.stringify(names));
      for (const [i, a] of names.entries()) {
        for (const b of names.slice(i + 1)) {
          const apart = a.right <= b.left || b.right <= a.left || a.bottom <= b.top || b.bottom <= a.top;
          assert.ok(apart, `"${a.name}" and "${b.name}" overlap: ${JSON.stringify([a, b])}`);
        }
      }
    } finally {
      await close();
    }
  });
}

test('chromium 1280x800: the head\'s menu of a workflow entry fits or scrolls to its last item, with no tick off', { timeout: 60_000 }, async () => {
  const { page, close } = await open(1280, { runs: true });
  try {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.locator('[data-steps]').waitFor({ timeout: 10_000 });
    await page.locator('[data-tour="entry"]').getByRole('button', { name: /^Actions for/ }).first().click();
    const menu = page.getByRole('menu').last();
    await menu.waitFor({ timeout: 5_000 });
    assert.equal(await menu.getByRole('menuitem', { name: 'Mark as done' }).count(), 0, 'tick off steps around the workflow');
    const trash = menu.getByRole('menuitem', { name: 'Move to the trash' });
    await trash.scrollIntoViewIfNeeded();
    const box = await trash.boundingBox();
    assert.ok(box && box.y >= 0 && box.y + box.height <= 800, `the trash is out of reach: ${JSON.stringify(box)}`);
    const surface = await menu.evaluate((list) => { const r = list.parentElement.getBoundingClientRect(); return { bottom: r.bottom }; });
    assert.ok(surface.bottom <= 800, `the menu runs past the screen: ${surface.bottom}`);
  } finally {
    await close();
  }
});

test('chromium: All mixes the history between the posts by time, the reader\'s own steps in the second person', { timeout: 60_000 }, async () => {
  const comment = (id, at, body, author) => ({ id, item_id: ENTRY.id, author_id: author, body, parent_comment_id: null, internal: false, created_at: at, edited_at: null, deleted_at: null, version: 1 });
  // The comments in UTC, the history in another zone: an hour later as text, the same morning as instants.
  const comments = [comment('01a0e2e0-0000-7000-8000-00000000cc01', '2026-11-10T08:00:00Z', 'First post', OTHER.id), comment('01a0e2e0-0000-7000-8000-00000000cc02', '2026-11-10T10:00:00Z', 'Second post', OTHER.id)];
  const history = [{ id: '01a0e2e0-0000-7000-8000-00000000ac01', code: 'activity.item_updated', occurred_at: '2026-11-10T10:00:00+01:00', actor: { type: 'USER', id: ACCOUNT.id }, change_set: {} }];
  const { page, close } = await open(1280, {
    runs: false,
    extra: (read, method) => {
      if (method !== 'GET') return undefined;
      if (read === '/accounts/me') return { ...ACCOUNT, locale: 'de' };
      if (read === `/items/${ENTRY.id}/comments`) return { data: comments, page: { has_more: false, next_cursor: null } };
      if (read === `/items/${ENTRY.id}/activity`) return { data: history, page: { has_more: false, next_cursor: null } };
      return undefined;
    },
  });
  try {
    const thread = page.locator('#entry-conversation');
    await thread.locator('[data-thread-event]').first().waitFor({ timeout: 10_000 });
    const order = await thread.evaluate((root) => [...root.querySelectorAll('[data-thread-event], .entry')].map((row) => (row.matches('[data-thread-event]') ? 'event' : row.textContent.includes('First post') ? 'first' : 'second')));
    assert.deepEqual(order, ['first', 'event', 'second'], 'the history row stands between the posts it happened between');
    assert.match(await thread.locator('[data-thread-event]').first().innerText(), /^Du hast diesen Eintrag bearbeitet/, 'the reader\'s own step in the second person');
    // Each tab says how many it holds (the scene's *Alles · 21 · Kommentare · 13 · Verlauf · 8*).
    const tabs = (await thread.getByRole('tab').allInnerTexts()).map((text) => text.replace(/\s+/g, ' ').trim());
    assert.deepEqual(tabs, ['Alles 3', 'Kommentare 2', 'Verlauf 1']);
  } finally {
    await close();
  }
});

test('chromium: without a workflow the default holds and the primary button is Done (UC-FLW-06/5)', { timeout: 60_000 }, async () => {
  const { page, close } = await open(1280, { runs: false });
  try {
    await page.locator('[data-steps]').waitFor({ timeout: 10_000 });
    assert.equal(await page.locator('[data-steps]').getAttribute('data-status'), 'todo');
    assert.ok(await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Done', exact: true }).isVisible(), 'Done is the primary button');
    assert.equal(await page.locator('[data-column-mark]').count(), 0, 'a second control beside the status sets the column');
  } finally {
    await close();
  }
});

test('chromium: on a board without a workflow the menu lists its columns, the done one completing (UC-FLW-06/5)', { timeout: 60_000 }, async () => {
  const patches = [];
  const { page, close } = await open(1280, { runs: false, board: true, patches });
  try {
    const status = page.locator('[data-steps]');
    await status.waitFor({ timeout: 10_000 });
    assert.equal(await status.getAttribute('data-status'), BOARD[0].id);
    assert.equal(await page.locator('[data-column-mark]').count(), 0, 'the status is the one control for the column');
    await status.getByRole('button', { name: /Change status/ }).click();
    const rows = page.getByRole('menuitemradio');
    assert.deepEqual((await rows.allInnerTexts()).map((text) => text.split('\n')[0].trim()), ['Ideas', 'Building', 'Shipped']);
    assert.match(await rows.nth(2).innerText(), /completes the entry/, 'the done column is marked as completing');
    assert.ok(await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Done', exact: true }).isVisible(), 'Done stays the main action');
    await rows.nth(2).click();
    await page.waitForTimeout(400);
    assert.deepEqual(patches, [{ bucket_id: BOARD[2].id }], 'choosing a column writes it by PATCH, the server keeping the done column');
  } finally {
    await close();
  }
});

// UC-FLW-06 check 5 with the Workflows module off: the board's columns are the states all the same,
// built from the collection's level and its column list (the steps are the module's and are not
// asked), the menu is headed *Column*, a column is written by PATCH and never by `:drop`.
test('chromium: on a board with Workflows off the entry still shows its columns and writes one by PATCH (UC-FLW-06/5)', { timeout: 60_000 }, async () => {
  const patches = [];
  const columns = BOARD.map((column, n) => ({ id: column.id, collection_id: ENTRY.collection_id, name: column.name, order_key: `a${n}`, wip_limit: null, is_done_bucket: column.completes, color_token: null, version: 1 }));
  const asked = [];
  const extra = (read, method) => {
    if (read.endsWith('/steps')) asked.push(read);
    if (method === 'GET' && read === `/containers/${ENTRY.collection_id}`) return { ...COLLECTION, state_mode: 'COLUMNS', state_modes: { task: 'COLUMNS' } };
    if (method === 'GET' && read === `/containers/${ENTRY.collection_id}/buckets`) return columns;
    if (method === 'GET' && read === `/items/${ENTRY.id}`) return { ...ENTRY, bucket_id: BOARD[1].id };
    return undefined;
  };
  const { page, close } = await open(1280, { runs: false, board: 'off', patches, extra });
  try {
    const status = page.locator('[data-steps]');
    await status.waitFor({ timeout: 10_000 });
    assert.equal(await status.getAttribute('data-status'), BOARD[1].id);
    await status.getByRole('button', { name: /Change status/ }).click();
    assert.ok(await page.getByText('Column', { exact: true }).first().isVisible(), 'the menu is headed Column, no module\'s word');
    const rows = page.getByRole('menuitemradio');
    assert.deepEqual((await rows.allInnerTexts()).map((text) => text.split('\n')[0].trim()), ['Ideas', 'Building', 'Shipped']);
    const written = page.waitForResponse((response) => response.request().method() === 'PATCH' && response.url().endsWith(`/items/${ENTRY.id}`));
    await rows.nth(0).click();
    await written;
    assert.deepEqual(patches, [{ bucket_id: BOARD[0].id }], 'the column went through PATCH');
    assert.deepEqual(asked, [], 'the module\'s steps were asked with the module off');
    assert.ok(await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Done', exact: true }).isVisible(), 'Done is the main action');
  } finally {
    await close();
  }
});

// The original complaint (D3): in a list an entry is completed from inside it, and once done the
// same place offers *Reopen* - the state reads *Open* / *Done* (E7).
test('chromium: in a list the state reads Open, Done completes, and a completed entry keeps Reopen (UC-FLW-06/5)', { timeout: 60_000 }, async () => {
  const written = [];
  const extra = (read, method) => {
    if (method === 'GET' && read === `/containers/${ENTRY.collection_id}`) return { ...COLLECTION, state_mode: 'NONE', state_modes: { task: 'NONE' } };
    if (method === 'GET' && read === `/items/${ENTRY.id}`) return { ...ENTRY, completion: { is_completed: true, completed_at: '2026-11-20T09:00:00Z', completed_by: ACCOUNT.id } };
    if (method === 'POST' && read.endsWith(':reopen')) { written.push(read); return { ...ENTRY, version: ENTRY.version + 1, completion: { is_completed: false } }; }
    return undefined;
  };
  const { page, close } = await open(1280, { runs: false, extra });
  try {
    const head = page.locator('[data-tour="entry"]');
    await page.locator('[data-steps]').waitFor({ timeout: 10_000 });
    assert.match(await page.locator('[data-steps]').innerText(), /Done/);
    const reopen = head.getByRole('button', { name: 'Reopen', exact: true });
    assert.ok(await reopen.isVisible(), 'a completed entry offers Reopen in the head');
    const reopened = page.waitForResponse((response) => response.url().endsWith(':reopen'));
    await reopen.click();
    await reopened;
    assert.deepEqual(written, [`/items/${ENTRY.id}:reopen`]);
  } finally {
    await close();
  }
});

// A workflow in force while the module is off (`WorkflowGate` knows no module): the head says the
// workflow is paused, with the reason, offers no step, and Done goes to the base product's
// `:complete`, which the server answers with the step to done or a refusal in words.
test('chromium: a workflow paused with Workflows off says so, and Done asks the server to complete (UC-FLW-06/5)', { timeout: 60_000 }, async () => {
  const written = [];
  const extra = (read, method) => {
    if (method === 'GET' && read === `/containers/${ENTRY.collection_id}`) return { ...COLLECTION, state_mode: 'WORKFLOW', state_modes: { task: 'WORKFLOW' } };
    if (method === 'POST' && read.endsWith(':complete')) { written.push(read); return { ...ENTRY, version: ENTRY.version + 1, completion: { is_completed: true } }; }
    if (read.endsWith('/steps') || read.endsWith(':step')) written.push(read);
    return undefined;
  };
  const { page, close } = await open(1280, { runs: false, extra });
  try {
    const status = page.locator('[data-steps]');
    await status.waitFor({ timeout: 10_000 });
    assert.match(await status.innerText(), /Workflow paused/);
    assert.ok(await page.getByText(/Workflows is switched off here/).first().isVisible(), 'the reason is said');
    const completed = page.waitForResponse((response) => response.url().endsWith(':complete'));
    await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Done', exact: true }).click();
    await completed;
    assert.deepEqual(written, [`/items/${ENTRY.id}:complete`]);
  } finally {
    await close();
  }
});

test('chromium: jump marks with counts, the conversation opens on All (UC-WRK-44/2, /3)', { timeout: 60_000 }, async () => {
  const { page, close } = await open(1280, { runs: false });
  try {
    const marks = page.getByRole('navigation', { name: /Sections of/ });
    await marks.waitFor({ timeout: 10_000 });
    // From the head the focus goes into the content before the properties beside it (2.4.3).
    const order = await page.evaluate(() => {
      const focusable = [...document.querySelectorAll('a[href], button, [tabindex="0"]')].filter((each) => each.getClientRects().length > 0);
      const at = (selector) => focusable.findIndex((each) => each.closest(selector));
      return { marks: at('[data-show]'), rail: at('aside.rail') };
    });
    assert.ok(order.marks >= 0 && order.rail >= 0 && order.marks < order.rail, `the rail comes before the content in the tab order: ${JSON.stringify(order)}`);
    const words = await marks.innerText();
    for (const word of ['Description', 'Links', 'Files', 'Conversation']) assert.ok(words.includes(word), `${word} is a mark: ${words}`);
    await marks.getByText('Conversation').click();
    await page.waitForTimeout(400);
    const box = await page.locator('#entry-conversation').boundingBox();
    assert.ok(box && box.y < 900, `the conversation is in view: ${box?.y}`);
    const tabs = page.getByRole('tablist', { name: 'Conversation' });
    assert.deepEqual((await tabs.getByRole('tab').allInnerTexts()).map((each) => each.trim()), ['All', 'Comments', 'History']);
    assert.equal(await tabs.getByRole('tab', { selected: true }).innerText(), 'All');
  } finally {
    await close();
  }
});

test('chromium 390: the marks are a switch and the properties a sheet (UC-WRK-44/7)', { timeout: 60_000 }, async () => {
  const { page, close } = await open(390, { runs: false });
  try {
    const marks = page.getByRole('navigation', { name: /Sections of/ });
    await marks.waitFor({ timeout: 10_000 });
    // On a phone *All properties* stands under the head, above the marks (task.m).
    const above = await page.evaluate(() => {
      const all = [...document.querySelectorAll('aside.rail button')].find((each) => each.getClientRects().length > 0)?.getBoundingClientRect();
      const nav = document.querySelector('[data-show] nav')?.getBoundingClientRect();
      return all && nav ? all.bottom <= nav.top : undefined;
    });
    assert.equal(above, true, 'All properties is not under the head');
    await marks.getByText('Conversation').click();
    await page.waitForTimeout(300);
    assert.ok(await page.locator('#entry-conversation').isVisible(), 'the chosen section shows');
    assert.ok(!(await page.locator('#entry-description').isVisible()), 'the others do not');
    // The reply is docked right on the bottom bar - mid-thread and at the end of the page alike.
    const gap = () => page.evaluate(() => {
      const dock = document.querySelector('[data-docked]')?.getBoundingClientRect();
      const bar = [...document.querySelectorAll('nav.bar')].map((each) => each.getBoundingClientRect()).find((box) => box.height > 0);
      return dock && bar ? Math.round(bar.top - dock.bottom) : undefined;
    });
    const near = (value) => value !== undefined && Math.abs(value) <= 1;
    assert.ok(near(await gap()), `the docked reply floats above the bottom bar: ${await gap()}`);
    await page.mouse.wheel(0, 4000);
    await page.waitForTimeout(300);
    assert.ok(near(await gap()), `at the end of the page the docked reply floats above the bottom bar: ${await gap()}`);
    await page.getByRole('button', { name: /All properties/ }).click();
    const sheet = page.getByRole('dialog');
    await sheet.waitFor({ timeout: 5_000 });
    // The sheet is named as the button that opened it, not after its own first section.
    assert.equal(await sheet.getByRole('heading', { level: 2 }).first().innerText(), 'All properties');
  } finally {
    await close();
  }
});

test('chromium: a refused step says at the asked field what it needs, and the failing check inside the dialog', { timeout: 60_000 }, async () => {
  const { page, close } = await open(1280, { runs: true, asks: true });
  try {
    await page.locator('[data-steps]').getByRole('button', { name: /Change status/ }).click();
    await page.getByRole('menuitem', { name: /Resolve/ }).click();
    const dialog = page.getByRole('dialog');
    await dialog.waitFor({ timeout: 5_000 });
    await dialog.locator('[data-confirm-step]').click();
    const banner = dialog.locator('.banner[data-tone=danger]');
    await banner.waitFor({ timeout: 5_000 });
    const field = dialog.getByLabel('resolution');
    assert.equal(await field.getAttribute('aria-invalid'), 'true', 'the asked field carries its error');
    assert.ok(await field.evaluate((input) => input === document.activeElement), 'focus is on the field');
    assert.ok((await banner.innerText()).includes('Attach the test report.'), `the check is said: ${await banner.innerText()}`);
    assert.equal(await page.locator('[data-steps] .refusal').count(), 0, 'nothing is said under the dialog');
  } finally {
    await close();
  }
});

test('chromium: a collection\'s Workflow settings name the workflow in force', { timeout: 60_000 }, async () => {
  for (const width of [1280, 390]) {
    const { page, close } = await open(width, { runs: true, path: `/collections/${ENTRY.collection_id}/settings/workflow` });
    try {
      const line = page.locator('[data-collection-workflow="incident"]');
      await line.waitFor({ timeout: 15_000 });
      assert.equal(await line.getByRole('link', { name: 'Incident' }).getAttribute('href'), '/administration/workflows');
    } finally {
      await close();
    }
  }
});

// §5.6 on a phone: the key stands on its own line above the title, so a long title takes the
// head's width rather than a column between the checkbox and the key, one word a line.
test('chromium 375: a long title takes the head\'s width beside its key', { timeout: 60_000 }, async () => {
  const title = 'VPN-Gateway Frankfurt antwortet sporadisch nicht (Paketverlust 30 %)';
  const { page, close } = await open(375, { runs: false, title });
  try {
    const key = page.locator('.object-head .key');
    await key.waitFor({ timeout: 10_000 });
    const sizes = await page.evaluate(() => {
      const head = document.querySelector('.object-head').getBoundingClientRect();
      const field = document.querySelector('.object-head .title').getBoundingClientRect();
      const key = document.querySelector('.object-head .key').getBoundingClientRect();
      return { head: head.width, field: field.width, fieldTop: field.top, keyBottom: key.bottom };
    });
    assert.ok(sizes.field >= sizes.head * 0.8, `the title takes ${Math.round(sizes.field)} of ${Math.round(sizes.head)} px`);
    assert.ok(sizes.keyBottom <= sizes.fieldTop + 1, 'the key stands above the title');
    // The phone bar names the entry by its key, beside a whole way back (task.m): the title is the head's.
    const bar = page.locator('[data-phone-bar]');
    assert.match(await bar.locator('[data-bar="title"]').innerText(), /^KITCHEN-\d+$/, 'the bar carries the entry\'s key');
    const fit = await page.evaluate(() => {
      const box = (selector) => document.querySelector(`[data-phone-bar] ${selector}`)?.getBoundingClientRect();
      const back = box('[data-back]');
      const title = box('[data-bar="title"]');
      const end = box('.end');
      return { back: back?.width ?? 0, overlaps: title && end ? title.right > end.left + 1 : false };
    });
    assert.ok(fit.back >= 44, `the way back is ${Math.round(fit.back)} px wide`);
    assert.ok(!fit.overlaps, 'the bar\'s title runs under its marks');
  } finally {
    await close();
  }
});

test('chromium 1280: links are tight rows by relation, each far end in its own status (§6.9 links)', { timeout: 60_000 }, async () => {
  const far = (n, title, extra) => ({
    kind: 'ENTRY', id: `01a0e2e0-0000-7000-8000-0000000001f${n}`, anchor: null, available: true, title, key: `TAMA-${n}`,
    level: 'TASK', entry_type: 'task', completed: false, archived: false, collection_id: ENTRY.collection_id, ...extra,
  });
  const link = (n, type, outward, other) => ({
    id: `01a0e2e0-0000-7000-8000-0000000002f${n}`, type, source_id: ENTRY.id, target_id: other.id, created_at: '2026-11-01T09:00:00Z', outward, other,
  });
  const answer = {
    links: [
      link(1, 'blocks', true, far(1, 'Passkey recovery through a second device with a long title that runs past the line, and on past the column into the rail beside it', { status_name: 'In review', status_category: 'IN_PROGRESS' })),
      link(2, 'depends_on', true, far(2, 'Rate limiting per API key', { completed: true })),
      link(3, 'depends_on', true, far(3, 'Webhooks retried', { status_name: null, status_category: 'TODO' })),
    ],
    backlinks: [], page_backlinks: [], whiteboard_backlinks: [],
  };
  const { page, close } = await open(1280, { runs: false, extra: (read) => (read === `/items/${ENTRY.id}/links` ? answer : undefined) });
  try {
    const section = page.locator('[data-links]');
    await section.locator('[data-link]').first().waitFor({ timeout: 10_000 });
    const heads = await section.locator('.relation').allInnerTexts();
    assert.equal(heads.length, 2, `one head per relation: ${heads}`);
    assert.ok(heads.some((head) => /\b2$/.test(head.trim())), `the relation of two counts them: ${heads}`);
    const states = await section.locator('[data-link] .state').allInnerTexts();
    assert.deepEqual(states.map((each) => each.trim()), ['In review', 'Done', 'To do'], 'each far end in its own state');
    assert.equal(await section.locator('[data-link-state="progress"]').count(), 1, 'a status in progress in its tone');
    const look = await section.locator('[data-link] a').first().evaluate((anchor) => {
      const style = getComputedStyle(anchor);
      const row = anchor.closest('[data-link]').getBoundingClientRect();
      const column = anchor.closest('[data-links]').getBoundingClientRect();
      return { underline: style.textDecorationLine, height: row.height, past: row.right - column.right };
    });
    assert.equal(look.underline, 'none', 'a link row is not underlined at rest');
    assert.ok(look.height <= 36, `a long title stays on one line: ${look.height} px`);
    assert.ok(look.past <= 0.5, `a long title is cut inside the column, not pushed past it: ${look.past} px over`);
    // The section and its jump mark carry the same count (ET-2: *Links 7*).
    assert.match(await section.locator('h2.section').innerText(), /^Links\s+3$/);
    assert.match(await page.getByRole('navigation', { name: /Sections of/ }).getByRole('link', { name: /Links/ }).innerText(), /Links\s*3/);
  } finally {
    await close();
  }
});

// On a phone the links stand under the description, so *Description* is the mark that leads to them.
test('chromium 390: Link is reached through the jump mark of the description, nothing covers it, and its dialog fits the phone (§6.9 links)', { timeout: 60_000 }, async () => {
  const blocks = { key: 'blocks', system: true, name: null, inverse_name: null, direction: 'DIRECTED', cardinality: 'MANY', cycles: 'FORBIDDEN', waits: 'NONE', position: 1, archived: false, version: 1 };
  const far = (n) => ({ kind: 'ENTRY', id: `01a0e2e0-0000-7000-8000-0000000003f${n}`, anchor: null, available: true, title: `Far end ${n}`, key: `TAMA-${n}`, level: 'TASK', entry_type: 'task', completed: false, archived: false, collection_id: ENTRY.collection_id });
  const answer = {
    links: [1, 2, 3, 4].map((n) => ({ id: `01a0e2e0-0000-7000-8000-0000000004f${n}`, type: 'blocks', source_id: ENTRY.id, target_id: far(n).id, created_at: '2026-11-01T09:00:00Z', outward: true, other: far(n) })),
    backlinks: [], page_backlinks: [], whiteboard_backlinks: [],
  };
  const { page, close } = await open(390, {
    runs: false,
    extra: (read, method) => (method !== 'GET' ? undefined : read === '/link-types' ? { data: [blocks] } : read === `/items/${ENTRY.id}/links` ? answer : undefined),
  });
  try {
    await page.getByRole('navigation', { name: /^Sections of/ }).getByRole('link', { name: /^Description/ }).click();
    const button = page.locator('[data-links]').getByRole('button', { name: 'Link', exact: true });
    await button.waitFor({ state: 'visible', timeout: 10_000 });
    await button.scrollIntoViewIfNeeded();
    // What a finger lands on at the button's centre is the button, not the bar above or below it.
    const covered = await button.evaluate((element) => {
      const box = element.getBoundingClientRect();
      const hit = document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2);
      return hit && element.contains(hit) ? null : (hit?.outerHTML.slice(0, 120) ?? 'nothing');
    });
    assert.equal(covered, null, `Link is covered by ${covered}`);
    await button.click({ timeout: 5_000 });
    const dialog = page.getByRole('dialog', { name: 'Link this entry to another' });
    await dialog.waitFor({ timeout: 5_000 });
    const confirm = await dialog.getByRole('button', { name: 'Link', exact: true }).boundingBox();
    assert.ok(confirm && confirm.x >= 0 && confirm.x + confirm.width <= 390 && confirm.y + confirm.height <= 900, `the dialog's Link is off the phone: ${JSON.stringify(confirm)}`);
  } finally {
    await close();
  }
});

test('chromium 1280: the affected items are named as their jump mark, both with their count (§6.9)', { timeout: 60_000 }, async () => {
  const ci = (n, name) => ({ ci_id: `01a0e2e0-0000-7000-8000-0000000003f${n}`, hub_id: ENTRY.collection_id, class: 'server', name, stage: 'OPERATIONAL', added_at: '2026-11-01T09:00:00Z' });
  const { page, close } = await open(1280, {
    runs: false,
    extra: (read, method) => {
      if (method !== 'GET') return undefined;
      if (read === '/meta/capabilities') return { ...MANIFEST, modules_on: ['ASSETS'] };
      if (/^\/containers\/[^/]+\/modules$/.test(read)) return { modules_on: ['ASSETS'] };
      if (read === `/items/${ENTRY.id}/cis`) return { item_id: ENTRY.id, cis: [ci(1, 'lb-01'), ci(2, 'db-01')], services: [], related: [], collisions: [] };
      return undefined;
    },
  });
  try {
    const section = page.locator('#entry-affected');
    await section.locator('[data-affected-ci]').first().waitFor({ timeout: 10_000 });
    assert.match(await section.locator('h2.section').innerText(), /^Affected objects\s+2$/, 'the section is named otherwise than its mark');
    assert.match(await page.getByRole('navigation', { name: /Sections of/ }).getByRole('link', { name: /Affected objects/ }).innerText(), /Affected objects\s*2/);
  } finally {
    await close();
  }
});

test('chromium 1280: the rail groups the fields as the type\'s form does, the empty ones under More fields (ET-1, task.d)', { timeout: 60_000 }, async () => {
  const form = {
    sections: [{ key: 'acceptance', name: 'Acceptance' }, { key: 'planning', name: 'Planning' }],
    fields: [
      { key: 'title', section: 'acceptance', required: true }, { key: 'custom_fields.signoff', section: 'acceptance' },
      { key: 'custom_fields.spec', section: 'acceptance' }, { key: 'custom_fields.points', section: 'planning' },
    ],
  };
  const field = (n, key, name, kind) => ({ id: `01a0e2e0-0000-7000-8000-0000000004f${n}`, collection_id: null, hub_id: null, key, name, kind, options: [], is_required: false, restricted: false, read_restricted: false, applies_to: ['TASK'] });
  const { page, close } = await open(1280, {
    runs: true,
    extra: (read, method) => {
      if (read === '/entry-types' && method === 'GET') return { data: [{ ...TYPE, form }] };
      if (read === '/entry-types/incident/form:evaluate' && method === 'POST') return { fields: {} };
      if (read === '/custom-fields' && method === 'GET') return [field(1, 'signoff', 'Signed off by', 'TEXT'), field(2, 'spec', 'Specification', 'TEXT'), field(3, 'points', 'Story points', 'NUMBER')];
      if (read === `/items/${ENTRY.id}` && method === 'GET') {
        return {
          ...ENTRY, entry_type: 'incident', status: 'new', label_ids: [LABELS[0].id, LABELS[1].id], custom_fields: { signoff: 'Clara', points: 8 },
          start_at: '2026-11-03T09:00:00Z', due_at: '2026-11-27T16:00:00Z', due_date_only: false,
        };
      }
      return undefined;
    },
  });
  try {
    const rail = page.locator('aside.rail').first();
    const acceptance = rail.locator('[data-form-section="acceptance"]');
    await acceptance.waitFor({ timeout: 10_000 });
    const titles = await rail.locator('section .title').evaluateAll((all) => all.map((each) => each.textContent.trim()));
    assert.ok(titles.indexOf('Acceptance') > titles.indexOf('Details') && titles.indexOf('Planning') > titles.indexOf('Acceptance'), `the form's sections are not the rail's groups: ${titles}`);
    assert.match(await acceptance.innerText(), /Signed off by\s+Clara/);
    assert.match(await rail.locator('[data-form-section="planning"]').innerText(), /Story points\s+8/);
    assert.equal(await rail.locator('[data-details]:not([data-more-fields]) [data-detail="field-signoff"]').count(), 0, 'a sectioned field stands in Details');
    // What is empty waits under More fields, counted, and is not among the set ones.
    const more = rail.getByRole('button', { name: /^Unfold More fields, \d+$/ });
    assert.ok(await more.isVisible(), `no folded More fields: ${titles}`);
    assert.equal(await acceptance.locator('[data-detail="field-spec"]').count(), 0, 'an empty field stands among the set ones');
    await more.click();
    await rail.locator('[data-more-fields] [data-detail="field-spec"]').waitFor({ timeout: 5_000 });
    // The labels as their chips, the dates short on one line beside their name.
    assert.equal(await rail.locator('[data-detail="labels"] .chip').count(), 2, 'the labels are plain text');
    const due = rail.locator('[data-detail="due"]');
    assert.doesNotMatch(await due.innerText(), /→|2026/, 'the dates are the long form');
    const box = await due.boundingBox();
    assert.ok(box && box.height <= 40, `the dates wrap: ${box?.height}`);
  } finally {
    await close();
  }
});
