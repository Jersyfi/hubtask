// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The list, board and workflow model (build-plan §5) on screen: the state control by level on the
// list row and the card - L0 the mark, L1 the mark and a column chip, L2 the mark and a status
// chip (UC-WRK-05 check 11), a board moving its cards with Workflows off (UC-FLW-06 check 5) -; at L2 the columns
// name the statuses they show, the statuses no column shows stand folded in *Ohne Spalte*; a drop
// several steps reach asks which (`CHOOSE`); a column a carried card may not enter dims with its
// reason; the done column stands folded. Columns a collection keeps without offering a board are
// named in its settings with *Remove columns*, confirmed as final (UC-WRK-05 check 13).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { pressArea } from './press.mjs';
import { serve } from './serve.mjs';
import { BUCKETS, COLLECTION, ITEMS, MANIFEST, PERMISSIONS, signedIn, stub, chooseView } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const WORKFLOW = {
  key: 'team', name: 'Team', system: false, hub_id: null, version: 1,
  published: {
    initial: 'open',
    statuses: [
      { key: 'open', name: 'Open', category: 'TODO' },
      { key: 'doing', name: 'Doing', category: 'IN_PROGRESS' },
      { key: 'review', name: 'Review', category: 'IN_PROGRESS' },
      { key: 'parked', name: 'Parked', category: 'TODO' },
      { key: 'done', name: 'Done', category: 'DONE' },
    ],
    steps: [
      { key: 'start', name: 'Start', from: ['open'], to: 'doing' },
      { key: 'ask', name: 'Ask for review', from: ['open'], to: 'review' },
      { key: 'finish', name: 'Finish', from: ['doing', 'review'], to: 'done' },
    ],
  },
};

/** The fixture's collection at a level, with every write recorded and `:drop` answered by `drop`. */
async function at(browser, level, { width = 1280, view = 'List', drop, workflowsOn = level === 'WORKFLOW' } = {}) {
  const written = [];
  const tab = await signedIn(browser, width, 900);
  const columns = level === 'WORKFLOW'
    ? [{ ...BUCKETS[0], statuses: ['open'] }, { ...BUCKETS[1], statuses: ['doing', 'review'] }, { ...BUCKETS[2], statuses: ['done'] }]
    : BUCKETS.map((bucket) => ({ ...bucket, statuses: [] }));
  const entries = ITEMS.map((item, n) => ({ ...item, bucket_id: n === 3 && level === 'WORKFLOW' ? null : BUCKETS[n % 2].id, ...(level === 'WORKFLOW' ? { status: n === 3 ? 'parked' : 'open' } : {}) }));
  await tab.context.unroute('**/api/v1/**');
  await tab.context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api\/v1/, '');
    if (request.method() !== 'GET') written.push({ method: request.method(), path, query: new URL(request.url()).search, body: request.postDataJSON() });
    if (request.method() === 'GET' && path === `/containers/${COLLECTION.id}`) return route.fulfill({ json: { ...COLLECTION, state_mode: level } });
    if (workflowsOn && request.method() === 'GET' && path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, modules_on: [...(MANIFEST.modules_on ?? []), 'WORKFLOWS'], workspace_permissions: PERMISSIONS.OWNER } });
    }
    if (workflowsOn && /\/containers\/[^/]+\/modules$/.test(path)) return route.fulfill({ json: { modules_on: [...(MANIFEST.modules_on ?? []), 'WORKFLOWS'] } });
    if (request.method() === 'GET' && path === `/containers/${COLLECTION.id}/buckets`) return route.fulfill({ json: columns });
    if (request.method() === 'GET' && path === '/workflows' && level === 'WORKFLOW') {
      return route.fulfill({ json: { data: [WORKFLOW], scheme: [{ container_id: COLLECTION.id, entry_type: null, workflow: WORKFLOW.key }] } });
    }
    if (request.method() === 'DELETE' && path === `/containers/${COLLECTION.id}/buckets/${BUCKETS[2].id}` && !new URL(request.url()).searchParams.get('new_done_bucket_id')) {
      return route.fulfill({ status: 409, contentType: 'application/problem+json', json: { type: 'about:blank', title: 'conflict', status: 409, code: 'conflict', detail_code: 'buckets.done_successor_required', params: { suggested_bucket_id: BUCKETS[1].id } } });
    }
    if (request.method() === 'DELETE' && path.startsWith(`/containers/${COLLECTION.id}/buckets/`)) return route.fulfill({ status: 204, body: '' });
    if (request.method() === 'POST' && path.endsWith('/board:switch')) {
      const to = request.postDataJSON()?.to;
      return route.fulfill({ json: { dry_run: request.postDataJSON()?.dry_run ?? false, level_before: level, level_after: to, columns_created: to === 'COLUMNS' ? 2 : 0, columns_removed: to === 'NONE' ? 3 : 0, entries_placed: 4 } });
    }
    if (request.method() === 'POST' && path.endsWith('/board:workflow')) {
      return route.fulfill({ json: { dry_run: request.postDataJSON()?.dry_run ?? false, workflow: 'board_kitchen', entries: 4, statuses: BUCKETS.map((bucket, n) => ({ key: `s${n + 1}`, name: bucket.name, category: n === 0 ? 'TODO' : bucket.is_done_bucket ? 'DONE' : 'IN_PROGRESS', bucket_id: bucket.id, count: n + 1 })) } });
    }
    if (request.method() === 'POST' && path.endsWith(':drop') && drop) return route.fulfill({ json: drop(request.postDataJSON(), entries) });
    // The entries as they stand at this level: their column and, under the workflow, their status.
    const byId = new Map(entries.map((each) => [each.id, each]));
    const placed = (value) => Array.isArray(value)
      ? value.map(placed)
      : value && typeof value === 'object'
        ? Object.fromEntries(Object.entries(byId.has(value.id) ? { ...value, bucket_id: byId.get(value.id).bucket_id, status: byId.get(value.id).status } : value).map(([key, each]) => [key, placed(each)]))
        : value;
    return stub(new Proxy(route, { get: (target, name) => name === 'fulfill' ? (answer) => target.fulfill(answer?.json ? { ...answer, json: placed(answer.json) } : answer) : target[name].bind(target) }));
  });
  await tab.page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await chooseView(tab.page, view);
  return { ...tab, written, entries };
}

// UC-WRK-05 check 11 and UC-FLW-06 check 5, with the Workflows module off (it is off in this
// suite unless a level runs a workflow): the row's mark and its column chip; a column is the base
// product's write, `PATCH bucket_id`, never the module's `:drop`; the mark completes (E6).
test('chromium: L1 - a task row shows the mark and its column chip; a column is written by PATCH with Workflows off', async (t) => {
  const browser = await sharedChromium(t);
  const { page, written, context } = await at(browser, 'COLUMNS');
  t.after(() => context.close());
  const row = page.locator(`[data-row="${ITEMS[0].id}"]`);
  const chip = row.locator('[data-state-chip]');
  await chip.waitFor({ timeout: 15_000 });
  assert.match((await chip.textContent()) ?? '', new RegExp(BUCKETS[0].name));
  const mark = row.getByRole('checkbox', { name: `Mark ${ITEMS[0].title} as done` });
  assert.equal(await mark.count(), 1, 'the mark beside the column chip');
  await chip.click();
  const choices = await page.getByRole('menuitem').allTextContents();
  assert.deepEqual(choices.map((each) => each.trim()), BUCKETS.map((bucket) => bucket.name));
  await page.getByRole('menuitem', { name: BUCKETS[2].name }).click();
  await page.waitForTimeout(300);
  const move = written.find((each) => each.method === 'PATCH' && each.path === `/items/${ITEMS[0].id}`);
  assert.equal(move?.body?.bucket_id, BUCKETS[2].id);
  assert.equal(written.filter((each) => each.path.endsWith(':drop')).length, 0, 'the column went through the module\'s :drop');
  assert.equal(written.filter((each) => each.path.endsWith(':complete')).length, 0, 'the client completed it itself');

  const completed = page.waitForResponse((response) => response.url().endsWith(`/items/${ITEMS[0].id}:complete`));
  await row.getByRole('checkbox', { name: `Mark ${ITEMS[0].title} as done` }).click();
  await completed;
  assert.equal(written.filter((each) => each.path === `/items/${ITEMS[0].id}:complete`).length, 1, 'the mark did not complete the entry');
});

// E5 and UC-WRK-05 check 11 at L2: the mark is the next step - an arrow named by it where it does
// not finish the entry -, the chip beside it lists every step.
test('chromium: L2 - the row\'s mark takes the next step towards done, an arrow where it does not finish', async (t) => {
  const browser = await sharedChromium(t);
  const { page, written, context } = await at(browser, 'WORKFLOW');
  t.after(() => context.close());
  const row = page.locator(`[data-row="${ITEMS[0].id}"]`);
  await row.locator('[data-state-chip]').waitFor({ timeout: 15_000 });
  const step = row.getByRole('button', { name: `${ITEMS[0].title}: Move on – Start` });
  assert.equal(await step.count(), 1, 'the mark is not the next step');
  const answered = page.waitForResponse((response) => response.url().endsWith(`/items/${ITEMS[0].id}:step`));
  await step.click();
  await answered;
  const taken = written.find((each) => each.path === `/items/${ITEMS[0].id}:step`);
  assert.deepEqual(taken?.body, { step: 'start' });
});

// A workflow stays in force on the server with its module off (`WorkflowGate`): the row says it
// is paused instead of pretending to be a list, offers no step, and its mark completes through the
// base product's `:complete`, which the server answers.
test('chromium: L2 with Workflows off - the chip says the workflow is paused, the mark completes', async (t) => {
  const browser = await sharedChromium(t);
  const { page, written, context } = await at(browser, 'WORKFLOW', { workflowsOn: false });
  t.after(() => context.close());
  const row = page.locator(`[data-row="${ITEMS[0].id}"]`);
  const chip = row.locator('[data-state-chip]');
  await chip.waitFor({ timeout: 15_000 });
  assert.match((await chip.textContent()) ?? '', /Workflow paused/);
  assert.equal(await chip.getByRole('button').count(), 0, 'a paused chip offers a menu');
  const completed = page.waitForResponse((response) => response.url().endsWith(`/items/${ITEMS[0].id}:complete`));
  await row.getByRole('checkbox', { name: `Mark ${ITEMS[0].title} as done` }).click();
  await completed;
  assert.equal(written.filter((each) => each.path === `/items/${ITEMS[0].id}:complete`).length, 1);
  assert.equal(written.filter((each) => each.path.endsWith(':step') || each.path.endsWith(':drop')).length, 0, 'a module door was asked');
});

// A thumb's target (user-experience.md §7): on a phone the status chip, drawn small, takes a press
// across layout.target (44 px) each way.
test('chromium: L2 - at 375 px the status chip of a row takes a 44 px press', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context } = await at(browser, 'WORKFLOW', { width: 375 });
  t.after(() => context.close());
  const chip = page.locator(`[data-row="${ITEMS[0].id}"] [data-state-chip]`);
  await chip.waitFor({ timeout: 15_000 });
  const area = await pressArea(chip);
  assert.ok(area.width >= 44 && area.height >= 44, `the status chip takes ${JSON.stringify(area)}`);
});

// UC-WRK-31 check 5: under a workflow each column names its statuses, the rest fold into Ohne Spalte.
test('chromium: L2 - the status chip lists the steps; the board names its statuses and folds Ohne Spalte', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context } = await at(browser, 'WORKFLOW');
  t.after(() => context.close());
  const chip = page.locator(`[data-row="${ITEMS[0].id}"] [data-state-chip]`);
  await chip.waitFor({ timeout: 15_000 });
  assert.match((await chip.textContent()) ?? '', /Open/);
  await chip.click();
  assert.deepEqual((await page.getByRole('menuitem').allTextContents()).map((each) => each.trim()), ['Start', 'Ask for review']);
  await page.keyboard.press('Escape');

  await chooseView(page, 'Board');
  const doing = page.locator(`[data-column-zone="${BUCKETS[1].id}"]`);
  await doing.waitFor({ timeout: 15_000 });
  assert.equal((await doing.locator('.statuses').textContent())?.trim(), 'Doing · Review');
  assert.equal((await page.locator(`[data-column-zone="${BUCKETS[0].id}"] .statuses`).textContent())?.trim(), 'Open', 'a column names a status called otherwise');
  assert.equal(await page.locator(`[data-column-zone="${BUCKETS[2].id}"] .statuses`).count(), 0, 'a column repeats the name of its one status');
  const unmapped = page.locator('[data-column-zone="none"]');
  assert.equal(await unmapped.locator('[data-folded]').count(), 1, 'Ohne Spalte is not folded');
  assert.match((await unmapped.textContent()) ?? '', /Without column/);
  const zones = await page.locator('[data-column-zone]').evaluateAll((all) => all.map((each) => each.getAttribute('data-column-zone')));
  assert.equal(zones.at(-1), 'none', 'Ohne Spalte stands after the columns');
  assert.equal(await page.locator(`[data-column-zone="${BUCKETS[2].id}"] [data-folded]`).count(), 1, 'the done column is not folded');
});

for (const width of [1280, 820]) {
  test(`chromium: L2 - folded Ohne Spalte keeps its width beside the column before it at ${width}`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, context } = await at(browser, 'WORKFLOW', { width, view: 'Board' });
    t.after(() => context.close());
    const unmapped = page.locator('[data-column-zone="none"]');
    await unmapped.locator('[data-folded]').waitFor({ timeout: 15_000 });
    // Unfolding the done column widens the board past the screen, where a zone used to shrink.
    await page.locator(`[data-column-zone="${BUCKETS[2].id}"] .unfold`).click();
    const zone = await unmapped.boundingBox();
    const column = await unmapped.locator('[data-folded]').boundingBox();
    const before = await page.locator(`[data-column-zone="${BUCKETS[2].id}"]`).boundingBox();
    assert.ok(zone && column && before, 'a column is not drawn');
    assert.ok(zone.width >= column.width - 0.5, `the zone (${zone.width}) is narrower than its folded column (${column.width})`);
    assert.ok(before.x + before.width <= column.x + 0.5, `the column before ends at ${before.x + before.width}, Ohne Spalte starts at ${column.x}`);
    assert.equal(await unmapped.getByRole('button', { name: /Entry in/ }).count(), 0, 'a folded column shows its add button');
  });
}

// UC-FLW-03 check 8: a drop several steps reach offers them and writes nothing until one is chosen.
test('chromium: L2 - a drop several steps reach asks which, and sends the one chosen', async (t) => {
  const browser = await sharedChromium(t);
  const drop = (body, entries) => {
    const item = entries.find((each) => each.id === ITEMS[0].id);
    return body.step
      ? { outcome: 'STEP_TAKEN', steps: [{ key: body.step, name: null, to: 'review' }], item: { ...item, status: 'review', bucket_id: body.bucket_id, version: item.version + 1 } }
      : { outcome: 'CHOOSE', steps: [{ key: 'start', name: 'Start', to: 'doing' }, { key: 'ask', name: 'Ask for review', to: 'review' }], item };
  };
  const { page, written, context } = await at(browser, 'WORKFLOW', { view: 'Board', drop });
  t.after(() => context.close());
  const card = page.locator(`[data-card="${ITEMS[0].id}"]`);
  await card.waitFor({ timeout: 15_000 });
  await card.click({ button: 'right' });
  await page.getByRole('menuitem', { name: `Move ${ITEMS[0].title} to ${BUCKETS[1].name}`, exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.waitFor({ timeout: 5_000 });
  assert.match((await dialog.textContent()) ?? '', /Which step should/);
  await dialog.getByRole('button', { name: /Ask for review/ }).click();
  await page.waitForTimeout(300);
  const drops = written.filter((each) => each.path === `/items/${ITEMS[0].id}:drop`);
  assert.deepEqual(drops.map((each) => each.body), [{ bucket_id: BUCKETS[1].id }, { bucket_id: BUCKETS[1].id, step: 'ask' }]);
});

test('chromium: L1 - Alt and an arrow carry a card into the next column by keyboard (WCAG 2.5.7)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, written, context } = await at(browser, 'COLUMNS', { view: 'Board' });
  t.after(() => context.close());
  const card = page.locator(`[data-card="${ITEMS[0].id}"]`);
  await card.waitFor({ timeout: 15_000 });
  await card.getByRole('link').first().focus();
  await page.keyboard.press('Alt+ArrowRight');
  await page.waitForTimeout(300);
  // With Workflows off a board moves its cards by the base product's write (UC-FLW-06 check 5).
  const move = written.find((each) => each.method === 'PATCH' && each.path === `/items/${ITEMS[0].id}`);
  assert.equal(move?.body?.bucket_id, BUCKETS[1].id);
  assert.equal(written.filter((each) => each.path.endsWith(':drop')).length, 0);
  // The card carries the mark and its column chip in its foot (UC-WRK-05 check 11).
  assert.equal(await card.getByRole('checkbox', { name: `Mark ${ITEMS[0].title} as done` }).count(), 1, 'the card has no mark');
});

/** Opens the collection's own menu and one of its items. */
async function fromPlaceMenu(page, item) {
  await page.getByRole('button', { name: `Actions for ${COLLECTION.name}` }).click();
  await page.getByRole('menuitem', { name: item }).click();
}

test('chromium: L0 - "Show as board" previews the server\'s dry run, then switches', async (t) => {
  const browser = await sharedChromium(t);
  const { page, written, context } = await at(browser, 'NONE');
  t.after(() => context.close());
  await page.locator(`[data-row="${ITEMS[0].id}"]`).waitFor({ timeout: 15_000 });
  assert.equal(await page.locator(`[data-row="${ITEMS[0].id}"]`).getByRole('checkbox').count(), 1, 'L0 completes with the checkbox');
  await fromPlaceMenu(page, 'Show as board');
  const preview = page.locator('[data-level-preview]');
  await preview.waitFor({ timeout: 5_000 });
  assert.match((await preview.textContent()) ?? '', /2 columns are created.*4 entries change column/s);
  const switches = () => written.filter((each) => each.path.endsWith('/board:switch')).map((each) => each.body);
  assert.deepEqual(switches(), [{ to: 'COLUMNS', dry_run: true }], 'the preview wrote');
  await page.getByRole('dialog').getByRole('button', { name: 'Show as board' }).click();
  await page.waitForTimeout(300);
  assert.deepEqual(switches().at(-1), { to: 'COLUMNS', dry_run: false });
});

// UC-FLW-02 check 10: *Workflow from these columns* previews the server's dry run, then makes it.
test('chromium: L1 - "Back to the list" and "Workflow from these columns" each preview first', async (t) => {
  const browser = await sharedChromium(t);
  const { page, written, context } = await at(browser, 'COLUMNS', { workflowsOn: true });
  t.after(() => context.close());
  await page.locator(`[data-row="${ITEMS[0].id}"] [data-state-chip]`).waitFor({ timeout: 15_000 });
  await fromPlaceMenu(page, 'Back to the list');
  const preview = page.locator('[data-level-preview]');
  await preview.waitFor({ timeout: 5_000 });
  assert.match((await preview.textContent()) ?? '', /3 columns are removed.*Done stays done/s);
  await page.keyboard.press('Escape');
  assert.equal(written.filter((each) => each.body?.dry_run === false).length, 0, 'closing the preview switched');
  await fromPlaceMenu(page, 'Workflow from these columns');
  await page.locator('[data-level-preview] [data-mapping-row]').first().waitFor({ timeout: 5_000 });
  assert.equal(await page.locator('[data-level-preview] [data-mapping-row]').count(), BUCKETS.length, 'a status per column');
  await page.getByRole('dialog').getByRole('button', { name: 'Make the workflow' }).click();
  await page.waitForTimeout(300);
  assert.deepEqual(written.filter((each) => each.path.endsWith('/board:workflow')).map((each) => each.body), [{ dry_run: true }, { dry_run: false }]);
});

test('chromium: L1 - deleting the done column asks which column means done, the server\'s suggestion chosen', async (t) => {
  const browser = await sharedChromium(t);
  const { page, written, context } = await at(browser, 'COLUMNS', { view: 'Board' });
  t.after(() => context.close());
  const done = page.locator(`[data-column-zone="${BUCKETS[2].id}"]`);
  await done.locator('button.unfold').click({ timeout: 15_000 });
  await done.getByRole('button', { name: `Actions for ${BUCKETS[2].name}` }).click();
  await page.getByRole('menuitem', { name: 'Delete column' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('button', { name: `Actions for ${BUCKETS[2].name}` }).click();
  const chosen = dialog.getByRole('radio', { name: BUCKETS[1].name });
  await chosen.waitFor({ timeout: 5_000 });
  assert.equal(await chosen.isChecked(), true, 'the server\'s suggestion is not preselected');
  await dialog.getByRole('button', { name: `Actions for ${BUCKETS[2].name}` }).click();
  await page.waitForTimeout(300);
  const deletes = written.filter((each) => each.method === 'DELETE');
  assert.deepEqual(deletes.map((each) => each.query), ['', `?new_done_bucket_id=${BUCKETS[1].id}`]);
});

/**
 * The collection's settings at L1 while it offers no board: the level, the columns and a
 * `board:switch` that empties both once it is sent without `dry_run`, each switch recorded.
 */
async function settingsWithoutBoard(browser, width) {
  const written = [];
  let removed = false;
  const tab = await signedIn(browser, width, 900);
  const structure = { ...COLLECTION.structure, settings: { ...COLLECTION.structure.settings, views: { offered: ['LIST_COLLAPSED', 'LIST_EXPANDED', 'TIMELINE'], default: 'LIST_COLLAPSED' } } };
  await tab.context.unroute('**/api/v1/**');
  await tab.context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api\/v1/, '');
    if (request.method() === 'GET' && path === `/containers/${COLLECTION.id}`) return route.fulfill({ json: { ...COLLECTION, structure, state_mode: removed ? 'NONE' : 'COLUMNS' } });
    if (request.method() === 'GET' && path === `/containers/${COLLECTION.id}/buckets`) return route.fulfill({ json: removed ? [] : BUCKETS.map((bucket) => ({ ...bucket, statuses: [] })) });
    if (request.method() === 'POST' && path === `/containers/${COLLECTION.id}/board:switch`) {
      const body = request.postDataJSON();
      written.push(body);
      if (body?.dry_run === false) removed = true;
      return route.fulfill({ json: { dry_run: body?.dry_run ?? false, level_before: 'COLUMNS', level_after: 'NONE', columns_created: 0, columns_removed: BUCKETS.length, entries_placed: 4 } });
    }
    // The collection offers no board wherever it is answered - the hub's list of collections too.
    const shaped = (value) => Array.isArray(value)
      ? value.map(shaped)
      : value && typeof value === 'object'
        ? Object.fromEntries(Object.entries(value.id === COLLECTION.id && value.type === 'COLLECTION' ? { ...value, structure } : value).map(([key, each]) => [key, shaped(each)]))
        : value;
    return stub(new Proxy(route, { get: (target, name) => name === 'fulfill' ? (answer) => target.fulfill(answer?.json ? { ...answer, json: shaped(answer.json) } : answer) : target[name].bind(target) }));
  });
  await tab.page.goto(`${served.origin}/collections/${COLLECTION.id}/settings/views`);
  return { ...tab, written };
}

// UC-WRK-05 check 13: columns kept without a board are named in the settings with their number and
// *Remove columns*; the confirmation, read from the dry run, says how many go, that completion
// stays and that it cannot be undone; confirming sends the switch without `dry_run`, and the
// settings no longer name columns.
for (const width of [1280, 375]) {
  test(`chromium: a collection with columns and no board offers to remove them, and says it is final, at ${width}`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, written, context } = await settingsWithoutBoard(browser, width);
    t.after(() => context.close());
    const hint = page.getByRole('note').filter({ hasText: 'offers no board' });
    await hint.waitFor({ timeout: 15_000 });
    assert.match((await hint.textContent()) ?? '', /This collection has 3 columns that set its entries' state, but offers no board\./);
    assert.deepEqual(written, [], 'the hint asked the server before anybody pressed');

    await hint.getByRole('button', { name: 'Remove columns' }).click();
    const dialog = page.getByRole('dialog', { name: 'Remove columns' });
    const preview = dialog.locator('[data-level-preview]');
    await preview.waitFor({ timeout: 5_000 });
    assert.match((await preview.textContent()) ?? '', /3 columns are removed.*4 entries leave their column.*Done stays done, open stays open\./s);
    assert.match((await dialog.getByRole('note').textContent()) ?? '', /This cannot be undone\. A board switched on later starts with two new columns, Open and Done, not these\./);
    assert.deepEqual(written, [{ to: 'NONE', dry_run: true }], 'the preview is not the dry run');

    const sent = page.waitForRequest((request) => request.url().endsWith('/board:switch') && request.postDataJSON()?.dry_run === false);
    await dialog.getByRole('button', { name: 'Remove columns' }).click();
    await sent;
    assert.deepEqual(written.at(-1), { to: 'NONE', dry_run: false });
    await dialog.waitFor({ state: 'detached', timeout: 5_000 });
    await hint.waitFor({ state: 'detached', timeout: 15_000 });
  });
}
