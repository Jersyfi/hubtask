// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The entry as head, subtree, details and tabs (ADR-0061 decision 4): the trail through
// the levels, completion from the page, the title and the notes edited in place with the same
// write, every details row opening its editor and giving focus back, the whole subtree with its
// counts and a child created from it, and the tabs. Chromium only, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { serve } from './serve.mjs';
import { ACCOUNT, CHILDREN, COLLECTION, HUB, ITEMS, signedIn, stub } from './fixture.mjs';
import { pressArea } from './press.mjs';
import { reveal, unfoldFields } from './reveal.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const ENTRY = ITEMS[0];

const served = await serve(DIST);
test.after(() => served.close());

/**
 * The rows a **task** holds, by their ids and the heading of the editor each opens.
 *
 * A task, because the column is the capability matrix drawn: a work package carries
 * neither a cover nor a repeat, and an activity carries five capabilities of fourteen. The dates
 * are one row, because `DuePanel` is one editor.
 */
const ROWS = [
  ['assignee', 'Assignee'],
  ['due', 'Dates'],
  ['labels', 'Labels'],
  ['reminders', 'Reminders'],
  ['recurrence', 'Repeats'],
  ['language', 'Written in'],
  ['cover', 'Cover'],
  ['attachments', 'Attachments'],
];

async function openEntry(browser, width, written) {
  const { page, failures, context, unstubbed } = await signedIn(browser, width, 1000);
  if (written) {
    await context.unroute('**/api/v1/**');
    await context.route('**/api/v1/**', async (route) => {
      const request = route.request();
      // Every request, by method: the writes are asserted on, and one read is asserted absent.
      written.push({ method: request.method(), path: new URL(request.url()).pathname, body: request.method() === 'GET' ? undefined : request.postDataJSON() });
      return stub(route);
    });
  }
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });
  return { page, failures, close: () => context.close(), unstubbed };
}


/**
 * Types one entry into capture, opened by an add control: the kind chosen first where the control
 * may add several, then Enter - the field closes once the entry is made.
 */
async function captureOne(page, title, kind) {
  const dialog = page.getByRole('dialog', { name: 'Capture' });
  const field = dialog.getByRole('textbox', { name: 'What do you want to remember?' });
  await field.waitFor({ timeout: 5_000 });
  if (kind) {
    await dialog.locator('[data-chip="kind"]').click();
    await page.getByRole('menuitem', { name: kind }).click();
  }
  await field.fill(title);
  await field.press('Enter');
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
}

/** Adds a part on the capture line at the foot of the parts (C-4): the same field, drawn in place. */
async function captureInPlace(page, title, kind) {
  const line = page.locator('[data-parts-capture]');
  const field = line.getByRole('textbox', { name: 'What do you want to remember?' });
  await field.waitFor({ timeout: 5_000 });
  if (kind) {
    await line.locator('[data-chip="kind"]').click();
    await page.getByRole('menuitem', { name: kind }).click();
  }
  await field.fill(title);
  await field.press('Enter');
  await page.waitForFunction((each) => each?.value === '', await field.elementHandle(), { timeout: 5_000 });
}

test('chromium: 1280 px — the trail, the head in place, the details rows, the subtree, the tabs', async (t) => {
  const browser = await sharedChromium(t);
  const written = [];
  const { page, failures, close, unstubbed } = await openEntry(browser, 1280, written);
  t.after(close);

  // One heading, the title drawn in the head (§6.9); the trail through the levels.
  const h1 = page.getByRole('heading', { level: 1 });
  assert.equal(await h1.count(), 1);
  assert.equal((await h1.textContent()).trim(), ENTRY.title);
  const trail = page.getByRole('navigation', { name: 'Where you are' });
  assert.deepEqual((await trail.getByRole('link').allTextContents()).map((each) => each.trim()), [HUB.name, COLLECTION.name]);

  // Completion from the head: the status button's next step, *Done*, makes the same write the row
  // makes; the title is struck through, and the status's *To do* reopens and takes the stroke away
  // (UC-WRK-04 checks 1, 2 - the checkbox stays the row's).
  // The client re-reads the entry after either write, so the entry answers what was last written.
  let current = ENTRY;
  await page.route(new RegExp(`/api/v1/items/${ENTRY.id}(:complete|:reopen)?(\\?.*)?$`), (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() === 'POST') {
      written.push({ method: 'POST', path, body: undefined });
      current = { ...current, completion: path.endsWith(':complete') ? { is_completed: true, completed_at: new Date().toISOString() } : { is_completed: false }, version: current.version + 1 };
      return route.fulfill({ json: current });
    }
    if (request.method() === 'GET') {
      written.push({ method: 'GET', path, body: undefined });
      return route.fulfill({ json: current });
    }
    return route.fallback();
  });
  const head = page.locator('[data-tour="entry"]');
  const struck = () => h1.locator('.title-text').evaluate((el) => getComputedStyle(el).textDecorationLine);
  assert.equal(await struck(), 'none', 'an open entry is struck through');
  await head.getByRole('button', { name: 'Done', exact: true }).click();
  await page.waitForTimeout(300);
  assert.ok(written.some((w) => w.method === 'POST' && w.path.endsWith(`/items/${ENTRY.id}:complete`)), `no completion was written: ${JSON.stringify(written)}`);
  await page.waitForFunction(() => getComputedStyle(document.querySelector('h1 .title-text')).textDecorationLine === 'line-through', undefined, { timeout: 5_000 })
    .catch(() => assert.fail('a completed entry is not struck through'));
  await head.getByRole('button', { name: /^Status: / }).click();
  await page.getByRole('menuitemradio', { name: /To do/ }).first().click();
  await page.waitForTimeout(300);
  assert.ok(written.some((w) => w.method === 'POST' && w.path.endsWith(`/items/${ENTRY.id}:reopen`)), `the status did not reopen: ${JSON.stringify(written)}`);
  assert.equal(await struck(), 'none', 'a reopened entry is still struck through');

  // The title in place: the pencil opens it, Enter saves what moved and nothing else.
  const edit = async () => {
    await head.getByRole('button', { name: 'Edit title' }).click();
    const field = page.getByRole('textbox', { name: 'Title', exact: true });
    await field.waitFor({ timeout: 5_000 });
    return field;
  };
  let title = await edit();
  assert.equal(await title.inputValue(), ENTRY.title);
  await title.fill('Order the tiles, both rooms');
  const beforeTitle = written.length;
  await page.keyboard.press('Enter');
  await page.waitForTimeout(300);
  const titleWrite = written.find((w) => w.method === 'PATCH' && w.path.endsWith(`/items/${ENTRY.id}`) && 'title' in (w.body ?? {}));
  assert.deepEqual(titleWrite?.body, { title: 'Order the tiles, both rooms' });
  // What a title costs: the write, the entry, its history and the subtree in one
  // query - and not the thread, the reminders or the attachments, none of which moved. The live
  // stream and the sync pull run on their own clock, so they are not what the title cost: under
  // load one of them lands inside the window and would be counted against it.
  const background = (w) => w.path.endsWith('/stream') || w.path.endsWith('/sync:pull');
  const afterTitle = written.slice(beforeTitle).filter((w) => !background(w)).map((w) => `${w.method} ${w.path.replace(/^.*\/api\/v1/, '')}`).sort();
  assert.deepEqual(afterTitle, [
    `GET /items/${ENTRY.id}`,
    `GET /items/${ENTRY.id}/activity`,
    `PATCH /items/${ENTRY.id}`,
    'POST /items:query',
  ], 'a title costs more than the write, the entry, its history and the subtree: ' + JSON.stringify(written.slice(beforeTitle).filter((w) => w.path.endsWith(':query')).map((w) => w.body.scope)));
  // Escape restores an unsaved edit.
  title = await edit();
  await title.fill('Not this');
  await page.keyboard.press('Escape');
  assert.notEqual((await h1.textContent()).trim(), 'Not this');
  assert.ok(!written.some((w) => w.body?.title === 'Not this'), 'Escape in the title saved it');
  // The notes: leaving the field saves them.
  const notes = page.getByRole('textbox', { name: 'Notes' });
  await notes.fill('Check the delivery date first.');
  await head.getByRole('button', { name: 'Edit title' }).focus();
  await page.waitForTimeout(300);
  assert.ok(written.some((w) => w.method === 'PATCH' && w.body?.notes === 'Check the delivery date first.'), `the notes were not written: ${JSON.stringify(written.map((w) => w.body))}`);
  // UC-WRK-02 check 3: the title saves on Enter (above) and on leaving the field, the notes on
  // leaving theirs, and Escape in either restores the text as it was.
  title = await edit();
  await title.fill('Order the tiles, all rooms');
  await notes.focus();
  await page.waitForTimeout(300);
  assert.ok(written.some((w) => w.method === 'PATCH' && w.body?.title === 'Order the tiles, all rooms'), `leaving the title did not save it: ${JSON.stringify(written.map((w) => w.body))}`);
  const notesBefore = await notes.textContent();
  await notes.fill('Not these notes');
  await page.keyboard.press('Escape');
  await page.waitForTimeout(300);
  assert.equal(await notes.textContent(), notesBefore, 'Escape kept the unsaved notes');
  assert.ok(!written.some((w) => w.body?.notes === 'Not these notes'), 'Escape in the notes saved them');

  // There is one way to edit, and it is where the field is shown (ADR-0063 decision 9): the
  // menu offers no second form over the same three fields.
  await reveal(page.getByRole('button', { name: `Actions for ${ENTRY.title}`, exact: true }));
  const menu = page.getByRole('menu');
  await menu.waitFor({ timeout: 5_000 });
  const offered = (await menu.getByRole('menuitem').evaluateAll((items) => items.map((item) => item.querySelector('.label')?.textContent?.trim() ?? '')));
  assert.ok(offered.includes('Share this entry') && !offered.some((label) => /^(Edit|Rename)/.test(label)), offered.join());
  await page.keyboard.press('Escape');

  await unfoldFields(page);
  // Every details row opens its editor beside it and gives focus back on Escape.
  for (const [id, heading] of ROWS) {
    const row = page.locator(`[data-detail="${id}"]`);
    await row.click();
    const editor = page.getByRole('dialog', { name: heading });
    await editor.waitFor({ timeout: 5_000 }).catch(() => assert.fail(`${id} opened no editor named ${heading}`));
    // Focus moves into the editor where it has a control; a gate with nothing to press leaves it on the row.
    const landed = await page.evaluate(() => document.activeElement?.closest('[role="dialog"]') !== null || document.activeElement?.hasAttribute('data-detail'));
    assert.equal(landed, true, `${id}: focus went to ${await page.evaluate(() => document.activeElement?.outerHTML.slice(0, 60))}`);
    await page.keyboard.press('Escape');
    await editor.waitFor({ state: 'hidden', timeout: 5_000 });
    await page.waitForFunction((rowId) => document.activeElement?.getAttribute('data-detail') === rowId, id, { timeout: 5_000 })
      .catch(async () => assert.fail(`${id}: focus did not return to the row but sits on ${await page.evaluate(() => document.activeElement?.outerHTML.slice(0, 80))}`));
  }
  // The set value reads on its row; the empty one says "add".
  assert.equal((await page.locator('[data-detail="labels"]').textContent()).includes('Materials'), true);
  assert.equal((await page.locator('[data-detail="due"]').textContent()).includes('Add'), true);
  // An entry that repeats never is not asked for its series: the row says so, the
  // editor says so, and no GET went out to answer 404 in the console.
  await page.locator('[data-detail="recurrence"]').click();
  await page.getByText('This entry does not repeat.').waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');
  assert.deepEqual(written.filter((w) => w.method === 'GET' && w.path.endsWith('/recurrence')), [], 'the series was read for an entry whose row says it has none');

  // The subtree: a task holding work packages and an activity of its own is headed by neither
  // name alone, and its count is every direct child of either kind - completed out of how many
  // (UC-WRK-03 check 5); every level, and a count on a branch.
  // The count stands beside the section's heading (ET-2).
  assert.equal(await page.getByRole('heading', { level: 2, name: /Under this entry/ }).count(), 1);
  assert.equal((await page.locator('#entry-parts .count').first().textContent()).trim(), '1/3', 'the section head does not count every kind');
  const rows = page.locator('.level .row [data-row], .level [data-row]');
  assert.equal(await rows.count(), 5, 'direct children open and their activities shown');
  assert.equal(await page.getByText('1/2', { exact: true }).count() >= 1, true);
  // Close every level, then open again - the choice is kept on this device.
  await page.getByRole('button', { name: 'Close every level' }).click();
  assert.equal(await page.locator('.level [data-row]').count(), 3);
  assert.equal(await page.evaluate((id) => sessionStorage.getItem(`hubtask.subtree.${id}`), ENTRY.id), '[]');
  await page.getByRole('button', { name: 'Open every level' }).click();
  assert.equal(await page.locator('.level [data-row]').count(), 5);
  // A child from the entry's page: the capture line at the parts' foot (C-4), an activity inside a row.
  assert.equal(await page.getByRole('dialog', { name: 'Capture' }).count(), 0);
  await captureInPlace(page, 'Grout');
  await page.waitForTimeout(300);
  const created = written.find((w) => w.method === 'POST' && w.path.endsWith('/items') && w.body?.title === 'Grout');
  assert.deepEqual({ type: created?.body?.type, parent_id: created?.body?.parent_id }, { type: 'WORK_PACKAGE', parent_id: ENTRY.id });
  assert.equal(await page.locator('[data-parts-capture] textarea').evaluate((field) => field === document.activeElement), true, 'the line keeps the focus for the next part');
  await reveal(page.getByRole('button', { name: `Add an activity inside ${CHILDREN[ENTRY.id][0].title}` }));
  await captureOne(page, 'Cut the tiles');
  await page.waitForTimeout(300);
  const child = written.find((w) => w.method === 'POST' && w.body?.title === 'Cut the tiles');
  assert.deepEqual({ type: child?.body?.type, parent_id: child?.body?.parent_id }, { type: 'ACTIVITY', parent_id: CHILDREN[ENTRY.id][0].id });
  // UC-WRK-03 check 1: the line offers both kinds the manifest permits under the task, by name,
  // and what is chosen is created directly in the task.
  await page.locator('[data-parts-capture] [data-chip="kind"]').click();
  assert.deepEqual((await page.getByRole('menuitem').allTextContents()).map((each) => each.trim()), ['Work package', 'Activity']);
  await page.keyboard.press('Escape');
  await captureInPlace(page, 'Ask about grout colours', 'Activity');
  await page.waitForTimeout(300);
  const direct = written.find((w) => w.method === 'POST' && w.body?.title === 'Ask about grout colours');
  assert.deepEqual({ type: direct?.body?.type, parent_id: direct?.body?.parent_id }, { type: 'ACTIVITY', parent_id: ENTRY.id });
  // The task's own activity cannot move out to the collection, and the menu says so in the
  // server's words rather than offering a refusal (P-05); an activity of a work package may move
  // out to the task. Both rows have the subtree's root as their parent's parent or parent.
  const ownActivity = CHILDREN[ENTRY.id][2];
  await reveal(page.getByRole('button', { name: `Actions for ${ownActivity.title}` }));
  const out = page.getByRole('menuitem', { name: /Move out one level/ });
  assert.ok((await out.textContent()).includes('An activity has to sit inside another entry.'), 'moving the task\'s activity out is offered without its reason');
  assert.equal(await out.getAttribute('aria-disabled'), 'true');
  await page.keyboard.press('Escape');
  const packActivity = CHILDREN[CHILDREN[ENTRY.id][0].id][0];
  await reveal(page.getByRole('button', { name: `Actions for ${packActivity.title}` }));
  const up = page.getByRole('menuitem', { name: /Move out one level/ });
  assert.notEqual(await up.getAttribute('aria-disabled'), 'true', 'an activity may move out of its work package to the task');
  await page.keyboard.press('Escape');

  // The conversation as tabs (§6.9): everything, the comments, the history.
  const tabs = page.getByRole('tablist', { name: 'Conversation' });
  assert.deepEqual((await tabs.getByRole('tab').allTextContents()).map((each) => each.trim().replace(/\s*·.*$/, '')), ['All', 'Comments', 'History']);
  await tabs.getByRole('tab', { name: 'History' }).click();
  await page.getByRole('feed').or(page.getByText(/Nothing has happened|No history|nothing/i)).first().waitFor({ timeout: 5_000 }).catch(() => {});
  assert.equal(await page.getByRole('tab', { name: 'History' }).getAttribute('aria-selected'), 'true');

  // The trail leads somewhere. Last, because it leaves the page: the entry screen is the one that
  // is rendered without the frame's navigator unless it is handed one, and a crumb that only
  // looks like a link is the trap.
  await trail.getByRole('link', { name: COLLECTION.name }).click();
  await page.waitForFunction((id) => location.pathname === `/collections/${id}`, COLLECTION.id, { timeout: 5_000 }).catch(() => {});
  assert.equal(new URL(page.url()).pathname, `/collections/${COLLECTION.id}`, 'the trail did not navigate');

  assert.deepEqual(failures, []);
  // Nothing was answered by a guess: a shape this fixture never prepared is a shape the walk
  // cannot claim to have exercised (`fixture.mjs`).
  assert.deepEqual(unstubbed(), []);
});

test('chromium: 375 px — the key in the bar, the details behind All properties, a row opens a drawer', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, close, unstubbed } = await openEntry(browser, 375);
  t.after(close);

  // The bar names the entry by its key (task.m); the head carries the whole title.
  assert.match((await page.getByRole('banner', { name: 'Application bar' }).locator('[data-bar="title"]').textContent()).trim(), /^KITCHEN-\d+$/);
  assert.equal(await page.getByRole('link', { name: COLLECTION.name }).count(), 1, 'the parent as the way up');
  // On a phone the rail is a sheet behind *All properties* (§7): closed until asked for.
  assert.equal(await page.locator('[data-detail]').filter({ visible: true }).count(), 0, 'the details stand open on a phone');
  await page.getByRole('button', { name: /^All properties/ }).click();
  await unfoldFields(page);
  await page.getByRole('dialog').locator('[data-detail="due"]').click();
  // The row's editor opens over the sheet as a drawer of its own.
  const drawer = page.locator('dialog.drawer[open]').filter({ has: page.getByRole('heading', { name: 'Dates', exact: true }) });
  await drawer.waitFor({ timeout: 5_000 }).catch(() => assert.fail('the editor is not a drawer on a phone'));
  await page.keyboard.press('Escape');
  await drawer.waitFor({ state: 'hidden', timeout: 5_000 });
  const scroll = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: window.innerWidth }));
  assert.equal(scroll.width, scroll.viewport, 'the entry scrolls sideways');
  // The subtree's indent is capped in the narrow tree - read where it is drawn: on a phone one
  // section shows at a time (§7). The sheet closes first.
  await page.keyboard.press('Escape');
  await page.getByRole('dialog').waitFor({ state: 'hidden', timeout: 5_000 });
  await page.getByRole('navigation', { name: /^Sections of/ }).getByRole('link', { name: /^Parts/ }).click();
  await page.locator('.task-row').first().waitFor();
  const indents = await page.locator('.task-row').evaluateAll((rows) => rows.map((row) => getComputedStyle(row).paddingInlineStart));
  assert.deepEqual([...new Set(indents)].sort(), ['0px', '16px'], `the indent steps are ${indents}`);
  assert.deepEqual(failures, []);
  // Nothing was answered by a guess: a shape this fixture never prepared is a shape the walk
  // cannot claim to have exercised (`fixture.mjs`).
  assert.deepEqual(unstubbed(), []);
});

// UC-WRK-10 check 1: Responsible and Also on it, two questions; an activity has the first alone.
test('chromium: 1280 px — assignment is two named questions, and the policy is offered only where there is one', async (t) => {
  const browser = await sharedChromium(t);

  /** Opens the entry's assignee editor with the collection served as given, and reads the panel. */
  const panelWith = async (policies, itemId = ENTRY.id) => {
    const { page, context, close, unstubbed } = await signedIn(browser, 1280, 1000);
    t.after(close);
    await context.route(`**/api/v1/containers/${COLLECTION.id}`, (route) =>
      route.fulfill({ json: { ...COLLECTION, policies } }));
    // The hub's level as well: the tree opens the hub of the entry's collection, and a list that
    // served the collection without its policies would take them back.
    await context.route(`**/api/v1/containers?parent_id=${HUB.id}*`, (route) =>
      route.fulfill({ json: { data: [{ ...COLLECTION, policies }], page: { next_cursor: null, has_more: false } } }));
    await page.goto(`${served.origin}/items/${itemId}`);
    await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });
    await unfoldFields(page);
    await page.locator('[data-detail="assignee"]').click();
    const editor = page.getByRole('dialog', { name: 'Assignee' });
    await editor.waitFor({ timeout: 5_000 });
    const read = {
      // The two parts, by their headings and the sentence under each.
      parts: (await editor.getByRole('heading', { level: 3 }).allTextContents()).map((each) => each.trim()),
      says: (await editor.textContent()) ?? '',
      // Each part names its own list, so a reader arriving by keyboard hears which one they are in.
      // Named lists a reader may actually reach: a refused gate leaves its control in the page
      // and `inert`, so the DOM still holds it and the accessibility tree does not.
      lists: await editor
        .locator('[role="listbox"]:not([inert] [role="listbox"])')
        .evaluateAll((lists) => lists.map((list) => list.getAttribute('aria-label'))),
      refusals: (await editor.locator('[data-status="refused"] .reason').allTextContents()).map((each) => each.trim()),
      button: await editor.getByRole('button', { name: 'Let the policy choose' }).count(),
      link: await editor.getByRole('link', { name: 'Open the collection' }).count() === 1
        ? await editor.getByRole('link', { name: 'Open the collection' }).getAttribute('href')
        : null,
    };
    await context.close();
    return read;
  };

  // Two named parts, each with the sentence that distinguishes it (ADR-0063 decision 10).
  const without = await panelWith({ completion_policy: 'MANUAL' });
  assert.deepEqual(without.parts, ['Responsible', 'Also on it']);
  assert.deepEqual(without.lists, ['Responsible', 'Also on it']);
  assert.match(without.says, /One person\. The entry is theirs/);
  assert.match(without.says, /Several people\. They follow the entry/);

  // No policy: no button at all, the sentence saying where one is set, and the way there for a
  // reader who may set it - this reader owns the hub, so they may.
  assert.equal(without.button, 0, 'the policy was offered where the collection has none');
  assert.match(without.says, /No policy chooses for this collection yet/);
  assert.equal(without.link, `/collections/${COLLECTION.id}`);

  const with_ = await panelWith({
    completion_policy: 'MANUAL',
    auto_assign: { strategy: 'FIXED', candidates: [{ kind: 'ACCOUNT', id: ENTRY.id }], enabled: true },
  });
  assert.equal(with_.button, 1, 'the policy was not offered where the collection has one');
  assert.equal(with_.link, null, 'the way to the policy is offered beside a policy that exists');

  const off = await panelWith({
    completion_policy: 'MANUAL',
    auto_assign: { strategy: 'FIXED', candidates: [{ kind: 'ACCOUNT', id: ENTRY.id }], enabled: false },
  });
  assert.equal(off.button, 0, 'a policy that is switched off still offered to choose');

  // A type that carries no member list has no such part: a capability the type refuses is absent,
  // not drawn with its reason (design-system.md §11).
  const activityId = CHILDREN[CHILDREN[ENTRY.id][0].id][0].id;
  const activity = await panelWith({ completion_policy: 'MANUAL' }, activityId);
  assert.deepEqual(activity.parts, ['Responsible']);
  assert.deepEqual(activity.lists, ['Responsible'], 'an activity was offered a member list it can reach');
  assert.deepEqual(activity.refusals, [], 'a part the type refuses was drawn with its reason');
  assert.deepEqual(without.refusals, [], 'a task was refused one of the two');
});

test('chromium: 1280 px — the details column is the capability matrix, and nothing the type refuses is offered', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, close, unstubbed } = await signedIn(browser, 1280, 1000);
  t.after(close);

  const rowsOf = async (id) => {
    await page.goto(`${served.origin}/items/${id}`);
    await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });
    // What is set stands in Details, the rest under More fields: the rows offered are both.
    await unfoldFields(page);
    return page.locator('[data-detail]').evaluateAll((rows) => rows.map((row) => row.getAttribute('data-detail')).sort());
  };

  // A task carries all fourteen; a work package neither a cover nor a repeat; an activity five of
  // the fourteen, and among them neither notes nor labels. `domain-model.md` §2, which the
  // manifest answers and this client may not second-guess.
  const task = await rowsOf(ENTRY.id);
  assert.deepEqual(task, ['assignee', 'due', 'labels', 'reminders', 'recurrence', 'language', 'cover', 'attachments'].sort());

  const pack = await rowsOf(CHILDREN[ENTRY.id][0].id);
  assert.deepEqual(pack, ['assignee', 'due', 'labels', 'reminders', 'language', 'attachments'].sort());
  // The pages' editor arrives a moment after the head (E-3, a lazy chunk).
  await page.getByRole('textbox', { name: 'Notes', exact: true }).waitFor({ timeout: 10_000 }).catch(() => assert.fail('a work package carries notes'));

  const activity = await rowsOf(CHILDREN[CHILDREN[ENTRY.id][0].id][0].id);
  assert.deepEqual(activity, ['assignee', 'due', 'reminders', 'language'].sort());
  assert.equal(await page.locator('#entry-description').count(), 0, 'an activity was offered notes it cannot keep');
  // And no conversation either: no tab standing there with a gate inside it saying so.
  assert.deepEqual(
    (await page.getByRole('tablist', { name: 'Conversation' }).getByRole('tab').allTextContents()).map((each) => each.trim()),
    ['History'],
  );

  // And the dates are one row: pressing it opens the one editor that writes both, which is why
  // two rows for it showed the reader the other date wherever they pressed.
  await unfoldFields(page);
  await page.locator('[data-detail="due"]').click();
  const editor = page.getByRole('dialog', { name: 'Dates' });
  await editor.waitFor({ timeout: 5_000 });
  const inside = (await editor.textContent()) ?? '';
  assert.ok(inside.includes('Date') && inside.includes('Starts'), `the dates editor holds ${inside.slice(0, 120)}`);

  assert.deepEqual(failures, []);
  // Nothing was answered by a guess: a shape this fixture never prepared is a shape the walk
  // cannot claim to have exercised (`fixture.mjs`).
  assert.deepEqual(unstubbed(), []);
  await context.close();
});

test('chromium: 1280 px — the cover row offers both kinds, and no cover takes no room above the title', async (t) => {
  const browser = await sharedChromium(t);
  const written = [];
  const { page, failures, close, unstubbed } = await openEntry(browser, 1280, written);
  t.after(close);

  // Nothing above the title for a cover that is not there (ADR-0063 decision 9). Measured rather
  // than read from the markup: an element with no height is still an element, and what the
  // decision is about is the room it takes.
  const above = await page.evaluate(() => {
    const head = document.querySelector('[data-tour="entry"]');
    // The object's head (trail, key, title) starts where the head does: no cover is drawn above it.
    const title = head?.querySelector('.object-head');
    if (!head || !title) return 'missing';
    return Math.round(title.getBoundingClientRect().top - head.getBoundingClientRect().top);
  });
  assert.equal(above, 0, 'the head keeps room above the title for a cover that is not set');

  await unfoldFields(page);
  await page.locator('[data-detail="cover"]').click();
  const editor = page.getByRole('dialog', { name: 'Cover' });
  await editor.waitFor({ timeout: 5_000 });

  // The row says where a cover goes, rather than only what is missing. The editor's body is
  // loaded on first use (`LazyView`), so the dialog stands before its words do.
  await editor.getByText(/drawn at the top of this entry/).waitFor({ timeout: 5_000 }).catch(() => {});
  assert.match((await editor.textContent()) ?? '', /drawn at the top of this entry and on its card/);

  // Both kinds are offered: the ten colours of the design system, and a picture.
  assert.equal(await editor.locator('button[aria-pressed]').count(), 10, 'the ten colours are not all offered');
  assert.equal(await editor.getByText('A picture').count(), 1, 'the picture half of the row is missing');

  // One press is one write, and it is a COLOR cover - the kind no client could set before.
  await editor.locator('button[aria-pressed][data-token="amber"]').click();
  await page.waitForTimeout(300);
  const write = written.find((w) => w.method === 'PUT' && w.path.endsWith('/cover'));
  assert.deepEqual(write?.body, { kind: 'COLOR', color_token: 'amber', media_id: null });

  assert.deepEqual(failures, []);
  // Nothing was answered by a guess: a shape this fixture never prepared is a shape the walk
  // cannot claim to have exercised (`fixture.mjs`).
  assert.deepEqual(unstubbed(), []);
});

test('chromium: the history announces a page only after the reader asked for it', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 1000);
  t.after(() => context.close());

  // Two pages of history: the first arrives with the tab, the second only on *Show more history*.
  const step = (n) => ({
    id: `01a0e2e0-0000-7000-8000-0000000007${String(n).padStart(2, '0')}`,
    code: 'activity.item_updated',
    actor: { kind: 'USER', id: ACCOUNT.id },
    occurred_at: `2026-10-0${n}T08:00:00Z`,
    change_set: {},
  });
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith(`/items/${ENTRY.id}/activity`)) {
      if (url.searchParams.get('cursor') === 'h2') {
        return route.fulfill({ json: { data: [step(3)], page: { next_cursor: null, has_more: false } } });
      }
      return route.fulfill({ json: { data: [step(1), step(2)], page: { next_cursor: 'h2', has_more: true } } });
    }
    return stub(route);
  });
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });
  await page.getByRole('tablist', { name: 'Conversation' }).getByRole('tab', { name: /^History/ }).click();

  const more = page.getByRole('button', { name: 'Show more history' });
  await more.waitFor({ timeout: 5_000 });
  // The live region is there and empty: nobody asked for the first page, so nothing is said about it.
  const status = page.locator('#entry-conversation .load-more [role="status"]');
  assert.equal(await status.count(), 1);
  assert.equal((await status.textContent()).trim(), '', 'the history announced a page nobody asked for');

  // Asked for, the page is said - and by the same region, which stays when the last page has
  // taken the button away: a region removed with the button is one whose sentence nobody hears.
  await more.click();
  await page.waitForFunction(() => (document.querySelector('#entry-conversation .load-more [role="status"]')?.textContent ?? '').trim() !== '', null, { timeout: 5_000 }).catch(() => {});
  assert.equal((await status.textContent()).trim(), 'History loaded: 3.');
  assert.equal(await more.count(), 0, 'the button stayed after the last page');
  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

test('chromium: the attachments announce a page only after the reader asked for it', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 1000);
  t.after(() => context.close());

  const file = (n) => ({
    id: `01a0e2e0-0000-7000-8000-0000000008${String(n).padStart(2, '0')}`,
    file_name: `plan-${n}.pdf`,
    content_type: 'application/pdf',
    size: 2048,
    status: 'READY',
    created_at: `2026-10-0${n}T08:00:00Z`,
  });
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith(`/items/${ENTRY.id}/attachments`)) {
      if (url.searchParams.get('cursor') === 'a2') {
        return route.fulfill({ json: { data: [file(3)], page: { next_cursor: null, has_more: false } } });
      }
      return route.fulfill({ json: { data: [file(1), file(2)], page: { next_cursor: 'a2', has_more: true } } });
    }
    // A PDF card asks for its first page through the file's record; without a download the card keeps its mark.
    const one = /\/media\/([^/:]+)$/.exec(url.pathname);
    if (one && route.request().method() === 'GET') return route.fulfill({ json: { ...file(Number(one[1].slice(-2))), id: one[1] } });
    return stub(route);
  });
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });
  await unfoldFields(page);
  // The attachments stand in the files section (ET-4): no tab to open first.

  const more = page.getByRole('button', { name: 'Older attachments' });
  await more.waitFor({ timeout: 5_000 });
  const status = page.locator('[data-entry-attachments] .load-more [role="status"]');
  assert.equal(await status.count(), 1);
  assert.equal((await status.textContent()).trim(), '', 'the attachments announced a page nobody asked for');

  await more.click();
  await page.waitForFunction(() => (document.querySelector('[data-entry-attachments] .load-more [role="status"]')?.textContent ?? '').trim() !== '', null, { timeout: 5_000 }).catch(() => {});
  assert.equal((await status.textContent()).trim(), '3 attachments are listed.');
  assert.equal(await more.count(), 0, 'the button stayed after the last page');
  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

// ET-4: the files as a grid of cards or a list, the reader's choice kept on the device; a file
// dropped anywhere on the entry is attached, the button under *Files* the way without a drag.
test('chromium: 1280 px — files as a grid or a list, and a file dropped anywhere is attached', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 1000);
  t.after(() => context.close());
  const uploads = [];
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.pathname.endsWith(`/items/${ENTRY.id}/attachments`) && request.method() === 'GET') {
      return route.fulfill({ json: { data: [
        { id: '01a0e2e0-0000-7000-8000-000000000901', file_name: 'plan.pdf', content_type: 'application/pdf', size: 2048, status: 'READY', created_at: '2026-10-01T08:00:00Z' },
        { id: '01a0e2e0-0000-7000-8000-000000000902', file_name: 'Wallet-Integration.md', content_type: 'text/markdown', size: 600, status: 'READY', created_at: '2026-10-01T08:00:00Z' },
      ], page: { next_cursor: null, has_more: false } } });
    }
    if (url.pathname.endsWith(`/items/${ENTRY.id}/file-links`) && request.method() === 'GET') {
      return route.fulfill({ json: { data: [{
        id: '01a0e2e0-0000-7000-8000-000000000911', item_id: ENTRY.id, collection_id: ENTRY.collection_id, provider: 'NEXTCLOUD',
        address: 'https://cloud.example.org/s/spec', name: 'Checkout spec', added_by: ACCOUNT.id, added_at: '2026-10-01T08:00:00Z',
      }] } });
    }
    if (request.method() === 'POST' && url.pathname.includes('/media')) {
      uploads.push(url.pathname);
      return route.fulfill({ status: 503, json: { type: 'about:blank', title: 'Unavailable', status: 503, code: 'common.unavailable' } });
    }
    // A PDF card asks for its first page through the file's record; without a download the card keeps its mark.
    if (/\/media\/[^/:]+$/.test(url.pathname) && request.method() === 'GET') {
      return route.fulfill({ json: { id: url.pathname.split('/').pop(), file_name: 'brief.pdf', content_type: 'application/pdf', size: 2048, status: 'READY', created_at: '2026-10-01T08:00:00Z' } });
    }
    return stub(route);
  });
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });
  const files = page.locator('[data-entry-attachments] .files');
  await files.waitFor({ timeout: 5_000 });
  assert.equal(await files.getAttribute('data-view'), 'grid', 'files open as the scene\'s grid');
  assert.equal((await page.locator('[data-attachment] .ending').first().textContent()).trim(), 'PDF');
  // A card says what kind of file it is and how big in the reader's words - never its media type.
  assert.deepEqual((await files.locator('[data-attachment] .detail').allInnerTexts()).map((each) => each.trim()), ['PDF · 2 kB', 'Text · 600 B']);
  // Uploaded and linked files are one grid (ET-4): the linked file is a cell beside the uploaded ones, not a row of its own.
  assert.equal(await page.locator('[data-file-link]').count(), 1);
  assert.equal(await files.locator('[data-file-link]').count(), 1, 'the linked file stands outside the files\' grid');
  const tops = await files.evaluate((list) => [...list.children].slice(0, 2).map((cell) => Math.round(cell.getBoundingClientRect().top)));
  assert.equal(tops[0], tops[1], `the linked file is alone in its row: ${tops}`);
  // The grid's last cell is the dashed tile to add one; a list has the button under it instead.
  const tile = files.locator('li.add:last-child');
  assert.equal(await tile.getByRole('button', { name: 'Choose a file' }).count(), 1, 'no add tile as the grid\'s last cell');
  await page.getByRole('button', { name: 'Files as a list' }).click();
  assert.equal(await files.locator('li.add').count(), 0, 'the list kept the tile');
  assert.equal(await page.locator('[data-entry-attachments]').getByRole('button', { name: 'Choose a file' }).count(), 1);
  assert.equal(await files.getAttribute('data-view'), 'list');
  assert.equal(await page.evaluate(() => localStorage.getItem('hubtask.entry.files_view')), 'list');
  assert.equal(await page.getByRole('button', { name: 'Files as a list' }).getAttribute('aria-pressed'), 'true');

  // A drag carrying files over the entry's head says where it goes; the drop sends the file.
  const transfer = await page.evaluateHandle(() => {
    const data = new DataTransfer();
    data.items.add(new File(['tiles'], 'tiles.txt', { type: 'text/plain' }));
    return data;
  });
  const head = page.getByRole('heading', { level: 1 }).first();
  await head.dispatchEvent('dragover', { dataTransfer: transfer });
  await page.getByText('Drop to attach to this entry').waitFor({ timeout: 5_000 });
  await head.dispatchEvent('drop', { dataTransfer: transfer });
  await page.waitForFunction(() => !document.querySelector('.drop-veil'), null, { timeout: 5_000 });
  await page.waitForTimeout(500);
  assert.ok(uploads.length >= 1, 'the dropped file was not sent');
  assert.deepEqual(failures.filter((each) => !/503|Unavailable/.test(each)), []);
  assert.deepEqual(unstubbed(), []);
});

// ET-1: a folded rail section says how many it holds - code, goals, approvals - read from the
// section's own list.
test('chromium: 1280 px — folded rail sections carry their counts', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 1000);
  t.after(() => context.close());
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.pathname.endsWith(`/items/${ENTRY.id}/code-links`) && request.method() === 'GET') {
      return route.fulfill({ json: { data: [{}, {}, {}].map((_, n) => ({ id: `01a0e2e0-0000-7000-8000-00000000c00${n}`, kind: 'COMMIT', ref: `abc${n}`, title: 'Fix', url: 'https://forge.example/c', created_at: '2026-10-01T08:00:00Z' })), page: { has_more: false } } });
    }
    return stub(route);
  });
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });
  const rail = page.locator('.rail');
  await rail.getByRole('button', { name: 'Unfold Code, 3' }).waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

// ET-3: from twenty files on the section is a list cut to its first twenty, *Show all* the rest.
test('chromium: 1280 px — more than twenty files fold to a list of twenty', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 1000);
  t.after(() => context.close());
  const many = Array.from({ length: 25 }, (_, n) => ({ id: `01a0e2e0-0000-7000-8000-${String(n + 1).padStart(12, '0')}`, file_name: `scan-${n + 1}.pdf`, content_type: 'application/pdf', size: 2048, status: 'READY', created_at: '2026-10-01T08:00:00Z' }));
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.pathname.endsWith(`/items/${ENTRY.id}/attachments`) && request.method() === 'GET') {
      return route.fulfill({ json: { data: many, page: { next_cursor: null, has_more: false } } });
    }
    return stub(route);
  });
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });
  const files = page.locator('[data-entry-attachments] .files');
  await files.waitFor({ timeout: 5_000 });
  assert.equal(await files.getAttribute('data-view'), 'list', 'twenty-five files stayed a grid');
  assert.equal(await files.locator('[data-attachment]').count(), 20);
  assert.equal(await page.getByRole('button', { name: 'Files as a list' }).count(), 0, 'the switch stayed on a folded list');
  await page.locator('#entry-files').getByRole('button', { name: 'Show all 25' }).click();
  assert.equal(await files.locator('[data-attachment]').count(), 25);
  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

// E-3, ET-5: a comment is written in the pages' editor - a mark from the bar, Ctrl+Enter sends -
// and is stored as Markdown; `/` at the start opens what can be inserted.
test('chromium: 1280 px — a comment is written in the pages\' editor and sent as Markdown', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 1000);
  t.after(() => context.close());
  const posted = [];
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    if (request.method() === 'POST' && new URL(request.url()).pathname.endsWith(`/items/${ENTRY.id}/comments`)) {
      posted.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: { id: '01a0e2e0-0000-7000-8000-000000000c01', item_id: ENTRY.id, author_id: ACCOUNT.id, body: request.postDataJSON().body, created_at: '2026-10-01T08:00:00Z', updated_at: '2026-10-01T08:00:00Z' } });
    }
    return stub(route);
  });
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });
  const writer = page.locator('[data-composer] [contenteditable="true"]');
  await writer.waitFor({ timeout: 10_000 });
  await writer.click();
  await page.keyboard.press('/');
  await page.getByRole('menuitem', { name: 'Checklist' }).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');
  await writer.click();
  await page.locator('[data-composer]').getByRole('button', { name: 'Bold' }).click();
  // The editor takes the focus back on the next frame.
  await page.waitForFunction(() => document.activeElement?.getAttribute('contenteditable') === 'true', null, { timeout: 5_000 });
  await page.keyboard.type('grout');
  await page.keyboard.press('ControlOrMeta+Enter');
  await page.waitForFunction((count) => count > 0, posted.length, { timeout: 100 }).catch(() => {});
  await page.waitForTimeout(500);
  assert.equal(posted.at(-1)?.body, '**grout**');
  assert.equal((await writer.textContent()).trim(), '', 'the editor starts over after sending');
  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

test('chromium: a page of history that did not arrive is said beside the button, and the button is the retry', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 1000);
  t.after(() => context.close());

  const step = (n) => ({
    id: `01a0e2e0-0000-7000-8000-0000000007${String(n).padStart(2, '0')}`,
    code: 'activity.item_updated',
    actor: { kind: 'USER', id: ACCOUNT.id },
    occurred_at: `2026-10-0${n}T08:00:00Z`,
    change_set: {},
  });
  // The second page is refused once and then answers: what a reader meets when a permission or
  // the connection fails between two presses.
  let refusals = 0;
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith(`/items/${ENTRY.id}/activity`)) {
      if (url.searchParams.get('cursor') === 'h2') {
        if (refusals === 0) {
          refusals += 1;
          return route.fulfill({ status: 403, contentType: 'application/problem+json', body: JSON.stringify({ status: 403, code: 'forbidden' }) });
        }
        return route.fulfill({ json: { data: [step(3)], page: { next_cursor: null, has_more: false } } });
      }
      return route.fulfill({ json: { data: [step(1), step(2)], page: { next_cursor: 'h2', has_more: true } } });
    }
    return stub(route);
  });
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });
  await page.getByRole('tablist', { name: 'Conversation' }).getByRole('tab', { name: /^History/ }).click();

  const more = page.getByRole('button', { name: 'Show more history' });
  await more.waitFor({ timeout: 5_000 });
  const alert = page.locator('#entry-conversation .load-more [role="alert"]');
  // Mounted empty, so that what fills it later is announced.
  assert.equal(await alert.count(), 1, 'there is no alert region beside the button');
  assert.equal((await alert.textContent()).trim(), '');
  const shown = () => page.locator('#entry-conversation .load-more').locator('xpath=..').locator('li').count();
  const before = await shown();

  await more.click();
  await page.waitForFunction(() => (document.querySelector('.load-more [role="alert"]')?.textContent ?? '').trim() !== '', null, { timeout: 5_000 }).catch(() => {});
  assert.equal((await alert.textContent()).trim(), 'You do not have permission for this action.');
  assert.ok(await alert.isVisible(), 'the refusal is announced but not shown');
  assert.equal(await more.count(), 1, 'the button went away with the failure, so there is no retry');
  assert.equal(await shown(), before, 'the rows already shown were lost');

  // Pressed again, the page arrives and the refusal is gone.
  await more.click();
  await page.waitForFunction(() => (document.querySelector('.load-more [role="status"]')?.textContent ?? '').trim() !== '', null, { timeout: 5_000 }).catch(() => {});
  assert.equal((await alert.textContent()).trim(), '');
  assert.equal((await page.locator('#entry-conversation .load-more [role="status"]').textContent()).trim(), 'History loaded: 3.');
  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

// The subtree on an entry's page has no bulk bar, so nothing there starts selecting: a Ctrl-click
// (⌘ on a Mac) on a row of it follows the row and draws no checkbox nothing would act on.
test('chromium: 1280 px — the subtree on an entry\'s page does not select', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, close, unstubbed } = await openEntry(browser, 1280);
  t.after(close);

  const child = CHILDREN[ENTRY.id][0];
  const row = page.locator(`[data-row="${child.id}"] a`).first();
  await row.waitFor({ timeout: 15_000 });
  await row.click({ modifiers: ['ControlOrMeta'] });
  await page.waitForTimeout(200);
  assert.equal(await page.getByRole('checkbox', { name: `Select ${child.title}` }).count(), 0);
  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

// UC-WRK-03 check 1 from a task's row in the collection: the one ＋ inside a task opens the form
// with a chooser of the two kinds the manifest permits, named in the catalogue's words rather than
// the contract's, and what is chosen is what is created.
test('chromium: 1280 px — the ＋ inside a task offers a work package and an activity by name', async (t) => {
  const browser = await sharedChromium(t);
  const written = [];
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 1000);
  t.after(() => context.close());
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    written.push({ method: request.method(), body: request.method() === 'GET' ? undefined : request.postDataJSON() });
    return stub(route);
  });
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);

  const inside = page.getByRole('button', { name: `Add something inside ${ENTRY.title}` });
  await inside.waitFor({ state: 'attached', timeout: 15_000 });
  await reveal(inside);
  // Capture under the task: the kind is a chip offering the two the task takes, by name.
  const kind = page.locator('[data-chip="kind"]');
  await kind.waitFor({ timeout: 5_000 });
  await kind.click();
  assert.deepEqual(
    (await page.getByRole('menuitem').allTextContents()).map((each) => each.trim()),
    ['Work package', 'Activity'],
  );
  await page.keyboard.press('Escape');
  await captureOne(page, 'Ring the supplier', 'Activity');
  await page.waitForTimeout(300);
  const created = written.find((w) => w.method === 'POST' && w.body?.title === 'Ring the supplier');
  assert.deepEqual({ type: created?.body?.type, parent_id: created?.body?.parent_id }, { type: 'ACTIVITY', parent_id: ENTRY.id });
  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

// The description is never drawn empty on its way in: an entry with notes shows them, or nothing
// while they load - not the placeholder for a frame first.
test('chromium: 1280 px — the description does not flash empty while the entry loads', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 1280, 1000);
  t.after(() => context.close());
  await context.addInitScript(() => {
    window.__emptySeen = [];
    // Blank (the text not drawn yet and nothing saying so) or the placeholder of an empty one.
    new MutationObserver(() => {
      const body = document.querySelector('#entry-description [id$="-body"]');
      if (!body) return;
      const isBlank = body.textContent.trim() === '' && !body.querySelector('[aria-busy="true"]');
      if (isBlank || body.querySelector('[data-placeholder][data-empty], p.quiet')) window.__emptySeen.push(body.innerHTML.slice(0, 120));
    }).observe(document, { childList: true, subtree: true, attributes: true });
  });
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    if (new URL(request.url()).pathname.endsWith(`/items/${ENTRY.id}`) && request.method() === 'GET') {
      return route.fulfill({ json: { ...ENTRY, notes: 'Written **before** the page opened.' } });
    }
    return stub(route);
  });
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.locator('#entry-description').getByText('before').waitFor({ timeout: 15_000 });
  await page.waitForTimeout(500);
  assert.deepEqual(await page.evaluate(() => window.__emptySeen), [], 'the description stood empty while it had notes');
  assert.deepEqual(failures, []);
});

// The parts as the `task` scene draws them: a head over the columns, a bar beside each work
// package's count, and every work package of the first eight shown - the fold counts work
// packages, not the activities under the open ones.
test('chromium: 1280 px — the parts show their first eight work packages under a column head, each with a bar', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 1280, 1000);
  t.after(() => context.close());
  const id = (n) => `01a0e2e0-0000-7000-8000-0000000009${String(n).padStart(2, '0')}`;
  const pack = (n) => ({ id: id(n), type: 'WORK_PACKAGE', title: `Package ${n}`, collection_id: ENTRY.collection_id, parent_id: ENTRY.id, depth: 1, order_key: `a${String(n).padStart(2, '0')}`, version: 1, number: 100 + n, completion: { is_completed: false }, labels: [], label_ids: [] });
  const step = (p, n, done) => ({ ...pack(50 + p * 5 + n), type: 'ACTIVITY', title: `Step ${p}.${n}`, parent_id: id(p), depth: 2, completion: { is_completed: done } });
  const packs = Array.from({ length: 10 }, (_, n) => pack(n + 1));
  const steps = [1, 2, 3].flatMap((p) => [step(p, 1, true), step(p, 2, false), step(p, 3, false)]);
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    if (new URL(request.url()).pathname.endsWith('/items:query') && request.postDataJSON()?.scope?.item_id === ENTRY.id) {
      const data = [...packs, ...steps];
      return route.fulfill({ json: { data, groups: [], page: { next_cursor: null, has_more: false }, total: data.length } });
    }
    return stub(route);
  });
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  const parts = page.locator('#entry-parts');
  await parts.locator(`[data-row="${id(1)}"]`).waitFor({ timeout: 15_000 });
  await page.waitForTimeout(300);
  for (let n = 1; n <= 8; n += 1) assert.ok(await parts.locator(`[data-row="${id(n)}"]`).isVisible(), `work package ${n} is folded away`);
  assert.equal(await parts.locator(`[data-row="${id(9)}"]`).count(), 0, 'the ninth work package is not folded');
  assert.ok(await parts.locator(`[data-row="${steps[1].id}"]`).isVisible(), 'the first work package does not open on its activities');
  assert.ok(await parts.locator('.fields-head').getByText('Title').isVisible(), 'no column head over the parts');
  assert.ok(await parts.locator(`[data-row="${id(1)}"] progress`).isVisible(), 'no bar beside a work package\'s count');
  assert.deepEqual(failures, []);
});

// A thumb's target (user-experience.md §7): on a phone each box of a task list in the description
// takes a press across layout.target (44 px) each way, the box beside it included.
test('chromium: 375 px — a task list in the description gives each box a 44 px press', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures } = await signedIn(browser, 375, 812);
  t.after(() => context.close());
  const entry = ITEMS[0];
  await page.route(new RegExp(`/api/v1/items/${entry.id}(\\?.*)?$`), (route) => route.request().method() === 'GET'
    ? route.fulfill({ json: { ...entry, notes: '- [ ] Buy the tiles\n- [x] Measure the wall' } })
    : route.fallback());
  await page.goto(`${served.origin}/items/${entry.id}`);
  await page.getByRole('navigation', { name: /^Sections of/ }).getByRole('link', { name: /^Description/ }).click();
  const boxes = page.locator('ul[data-type="taskList"] li > label');
  await boxes.first().waitFor({ timeout: 10_000 });
  assert.equal(await boxes.count(), 2);
  for (const box of await boxes.all()) {
    const area = await pressArea(box);
    assert.ok(area.width >= 44 && area.height >= 44, `a task box takes ${JSON.stringify(area)}`);
  }
  assert.deepEqual(failures, []);
});
