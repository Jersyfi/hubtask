// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The entry as head, subtree, details and tabs (F9-08, ADR-0061 decision 4): the trail through
// the levels, completion from the page, the title and the notes edited in place with the same
// write, every details row opening its editor and giving focus back, the whole subtree with its
// counts and a child created from it, and the tabs. Chromium only, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { CHILDREN, COLLECTION, HUB, ITEMS, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const ENTRY = ITEMS[0];

const served = await serve(DIST);
test.after(() => served.close());

/**
 * The rows a **task** holds, by their ids and the heading of the editor each opens.
 *
 * A task, because the column is the capability matrix drawn (issue 916): a work package carries
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
  const { page, failures, context } = await signedIn(browser, width, 1000);
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
  await page.getByRole('textbox', { name: 'Title' }).first().waitFor({ timeout: 15_000 });
  return { page, failures, close: () => context.close() };
}

test('chromium: 1280 px — the trail, the head in place, the details rows, the subtree, the tabs', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const written = [];
  const { page, failures, close } = await openEntry(browser, 1280, written);
  t.after(close);

  // One heading, read and not drawn; the trail through the levels.
  assert.equal(await page.getByRole('heading', { level: 1 }).count(), 1);
  assert.equal(await page.getByRole('heading', { level: 1 }).evaluate((el) => el.getBoundingClientRect().width <= 1), true, 'the h1 is drawn as well as read');
  const trail = page.getByRole('navigation', { name: 'Where you are' });
  assert.deepEqual((await trail.getByRole('link').allTextContents()).map((each) => each.trim()), [HUB.name, COLLECTION.name]);

  // Completion from the page: the same write the row makes, announced.
  const done = page.getByRole('checkbox', { name: `Mark ${ENTRY.title} as done` });
  await done.click();
  await page.waitForTimeout(300);
  assert.ok(written.some((w) => w.method === 'POST' && w.path.endsWith(`/items/${ENTRY.id}:complete`)), `no completion was written: ${JSON.stringify(written)}`);

  // The title in place: text until it has focus, Enter saves what moved and nothing else.
  const title = page.getByRole('textbox', { name: 'Title' }).first();
  assert.equal(await title.inputValue(), ENTRY.title);
  assert.equal(await title.evaluate((el) => getComputedStyle(el).borderTopColor), 'rgba(0, 0, 0, 0)', 'the title field draws a border at rest');
  await title.focus();
  await title.fill('Order the tiles, both rooms');
  const beforeTitle = written.length;
  await page.keyboard.press('Enter');
  await page.waitForTimeout(300);
  const titleWrite = written.find((w) => w.method === 'PATCH' && w.path.endsWith(`/items/${ENTRY.id}`) && 'title' in (w.body ?? {}));
  assert.deepEqual(titleWrite?.body, { title: 'Order the tiles, both rooms' });
  // What a title costs (issue 877): the write, the entry, its history and the subtree in one
  // query - and not the thread, the reminders or the attachments, none of which moved.
  const afterTitle = written.slice(beforeTitle).map((w) => `${w.method} ${w.path.replace(/^.*\/api\/v1/, '')}`).sort();
  assert.deepEqual(afterTitle, [
    `GET /items/${ENTRY.id}`,
    `GET /items/${ENTRY.id}/activity`,
    `PATCH /items/${ENTRY.id}`,
    'POST /items:query',
  ], 'a title costs more than the write, the entry, its history and the subtree: ' + JSON.stringify(written.slice(beforeTitle).filter((w) => w.path.endsWith(':query')).map((w) => w.body.scope)));
  // Escape restores an unsaved edit.
  await title.focus();
  await title.fill('Not this');
  await page.keyboard.press('Escape');
  assert.notEqual(await title.inputValue(), 'Not this');
  // The notes: leaving the field saves them.
  const notes = page.getByRole('textbox', { name: 'Notes' });
  await notes.fill('Check the delivery date first.');
  await page.getByRole('textbox', { name: 'Title' }).first().focus();
  await page.waitForTimeout(300);
  assert.ok(written.some((w) => w.method === 'PATCH' && w.body?.notes === 'Check the delivery date first.'), `the notes were not written: ${JSON.stringify(written.map((w) => w.body))}`);

  // There is one way to edit, and it is where the field is shown (ADR-0063 decision 9): the
  // menu offers no second form over the same three fields.
  await page.getByRole('button', { name: /Actions for/ }).click();
  const menu = page.getByRole('menu');
  await menu.waitFor({ timeout: 5_000 });
  assert.deepEqual((await menu.getByRole('menuitem').allTextContents()).map((each) => each.trim()), ['Share this entry']);
  await page.keyboard.press('Escape');

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
  // An entry that repeats never is not asked for its series (issue 882): the row says so, the
  // editor says so, and no GET went out to answer 404 in the console.
  await page.locator('[data-detail="recurrence"]').click();
  await page.getByText('This entry does not repeat.').waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');
  assert.deepEqual(written.filter((w) => w.method === 'GET' && w.path.endsWith('/recurrence')), [], 'the series was read for an entry whose row says it has none');

  // The subtree: the child type's name with its count, every level, and a count on a branch.
  const heading = page.getByRole('heading', { level: 2, name: /Work package/ });
  assert.ok((await heading.textContent()).replace(/\s+/g, ' ').includes('1/2'), 'the section head carries no count');
  const rows = page.locator('.level .row [data-row], .level [data-row]');
  assert.equal(await rows.count(), 4, 'direct children open and their activities shown');
  assert.equal(await page.getByText('1/2', { exact: true }).count() >= 1, true);
  // Close every level, then open again - the choice is kept on this device.
  await page.getByRole('button', { name: 'Close every level' }).click();
  assert.equal(await page.locator('.level [data-row]').count(), 2);
  assert.equal(await page.evaluate((id) => sessionStorage.getItem(`hubtask.subtree.${id}`), ENTRY.id), '[]');
  await page.getByRole('button', { name: 'Open every level' }).click();
  assert.equal(await page.locator('.level [data-row]').count(), 4);
  // A child from the entry's page: "+ Work package" at the end, and "+ Activity" inside a row.
  await page.getByRole('button', { name: 'Add Work package' }).click();
  const field = page.getByRole('textbox', { name: 'Title' }).last();
  await field.waitFor({ timeout: 5_000 });
  assert.equal(await page.evaluate(() => document.activeElement?.tagName), 'INPUT');
  await field.fill('Grout');
  await page.getByRole('button', { name: 'Create', exact: true }).click();
  await page.waitForTimeout(300);
  const created = written.find((w) => w.method === 'POST' && w.path.endsWith('/items') && w.body?.title === 'Grout');
  assert.deepEqual({ type: created?.body?.type, parent_id: created?.body?.parent_id }, { type: 'WORK_PACKAGE', parent_id: ENTRY.id });
  await page.getByRole('button', { name: `Add Activity inside ${CHILDREN[ENTRY.id][0].title}` }).click();
  await page.getByRole('textbox', { name: 'Title' }).last().fill('Cut the tiles');
  await page.getByRole('button', { name: 'Create', exact: true }).click();
  await page.waitForTimeout(300);
  const child = written.find((w) => w.method === 'POST' && w.body?.title === 'Cut the tiles');
  assert.deepEqual({ type: child?.body?.type, parent_id: child?.body?.parent_id }, { type: 'ACTIVITY', parent_id: CHILDREN[ENTRY.id][0].id });

  // Comments and activity as tabs.
  const tabs = page.getByRole('tablist', { name: "The entry's history" });
  assert.deepEqual((await tabs.getByRole('tab').allTextContents()).map((each) => each.trim()), ['Comments · 0', 'History']);
  await tabs.getByRole('tab', { name: 'History' }).click();
  await page.getByRole('feed').or(page.getByText(/Nothing has happened|No history|nothing/i)).first().waitFor({ timeout: 5_000 }).catch(() => {});
  assert.equal(await page.getByRole('tab', { name: 'History' }).getAttribute('aria-selected'), 'true');

  // The trail leads somewhere. Last, because it leaves the page: the entry screen is the one that
  // is rendered without the frame's navigator unless it is handed one, and a crumb that only
  // looked like a link is what issue 914 was.
  await trail.getByRole('link', { name: COLLECTION.name }).click();
  await page.waitForFunction((id) => location.pathname === `/collections/${id}`, COLLECTION.id, { timeout: 5_000 }).catch(() => {});
  assert.equal(new URL(page.url()).pathname, `/collections/${COLLECTION.id}`, 'the trail did not navigate');

  assert.deepEqual(failures, []);
});

test('chromium: 375 px — the title in the bar, the details folded under the head, a row opens a drawer', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await openEntry(browser, 375);
  t.after(close);

  assert.equal((await page.getByRole('banner', { name: 'Application bar' }).locator('[data-bar="title"]').textContent()).trim(), ENTRY.title);
  assert.equal(await page.getByRole('link', { name: COLLECTION.name }).count(), 1, 'the parent as the way up');
  const fold = page.locator('details.details-fold');
  assert.equal(await fold.evaluate((el) => el.open), false, 'the details stand open on a phone');
  // The details come before the subtree in the document, so a reader meets them after the head.
  const order = await page.evaluate(() => {
    const details = document.querySelector('details.details-fold');
    const subtree = document.querySelector('.level');
    return details && subtree ? details.compareDocumentPosition(subtree) & Node.DOCUMENT_POSITION_FOLLOWING ? 'details first' : 'subtree first' : 'missing';
  });
  assert.equal(order, 'details first');
  await page.locator('summary.details-summary').click();
  await page.locator('[data-detail="due"]').click();
  const drawer = page.locator('dialog[open]');
  await drawer.waitFor({ timeout: 5_000 });
  assert.equal(await drawer.getByRole('heading', { name: 'Date' }).count(), 1, 'the editor is not a drawer on a phone');
  await page.keyboard.press('Escape');
  await drawer.waitFor({ state: 'hidden', timeout: 5_000 });
  const scroll = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: window.innerWidth }));
  assert.equal(scroll.width, scroll.viewport, 'the entry scrolls sideways');
  // The subtree's indent is capped in the narrow tree.
  const indents = await page.locator('.task-row').evaluateAll((rows) => rows.map((row) => getComputedStyle(row).paddingInlineStart));
  assert.deepEqual([...new Set(indents)].sort(), ['0px', '16px'], `the indent steps are ${indents}`);
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — the policy chooses only where there is one to choose', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  /** Opens the entry's assignee editor and reads the auto-assign button, with the collection served as given. */
  const buttonWith = async (policies) => {
    const { page, context, close } = await signedIn(browser, 1280, 1000);
    t.after(close);
    await context.route(`**/api/v1/containers/${COLLECTION.id}`, (route) =>
      route.fulfill({ json: { ...COLLECTION, policies } }));
    await page.goto(`${served.origin}/items/${ENTRY.id}`);
    await page.getByRole('textbox', { name: 'Title' }).first().waitFor({ timeout: 15_000 });
    await page.locator('[data-detail="assignee"]').click();
    await page.getByRole('dialog', { name: 'Assignee' }).waitFor({ timeout: 5_000 });
    const state = await page.evaluate(() => {
      const button = [...document.querySelectorAll('button')].find((each) => each.textContent?.includes('Let the policy choose'));
      return button ? { disabled: button.disabled, reason: button.nextElementSibling?.textContent ?? null } : null;
    });
    await context.close();
    return state;
  };

  // Offered where a policy exists, and carrying its reason where none does — rather than
  // disappearing, because automatic assignment is exactly something somebody might want and be
  // missing (issue 917, and `domain-model.md` §2's rule that a refusal is never silent).
  const without = await buttonWith({ completion_policy: 'MANUAL' });
  assert.equal(without?.disabled, true, 'the policy was offered where the collection has none');
  assert.match(without?.reason ?? '', /No policy chooses/);

  const with_ = await buttonWith({
    completion_policy: 'MANUAL',
    auto_assign: { strategy: 'FIXED', candidates: [{ kind: 'ACCOUNT', id: ENTRY.id }], enabled: true },
  });
  assert.equal(with_?.disabled, false, 'the policy was refused where the collection has one');
  assert.equal(with_?.reason, null);

  const off = await buttonWith({
    completion_policy: 'MANUAL',
    auto_assign: { strategy: 'FIXED', candidates: [{ kind: 'ACCOUNT', id: ENTRY.id }], enabled: false },
  });
  assert.equal(off?.disabled, true, 'a policy that is switched off still offered to choose');
});

test('chromium: 1280 px — the details column is the capability matrix, and nothing the type refuses is offered', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, close } = await signedIn(browser, 1280, 1000);
  t.after(close);

  const rowsOf = async (id) => {
    await page.goto(`${served.origin}/items/${id}`);
    await page.getByRole('textbox', { name: 'Title' }).first().waitFor({ timeout: 15_000 });
    return page.locator('[data-detail]').evaluateAll((rows) => rows.map((row) => row.getAttribute('data-detail')));
  };

  // A task carries all fourteen; a work package neither a cover nor a repeat; an activity five of
  // the fourteen, and among them neither notes nor labels. `domain-model.md` §2, which the
  // manifest answers and this client may not second-guess (issue 916).
  const task = await rowsOf(ENTRY.id);
  assert.deepEqual(task, ['assignee', 'due', 'labels', 'reminders', 'recurrence', 'language', 'cover', 'attachments']);

  const pack = await rowsOf(CHILDREN[ENTRY.id][0].id);
  assert.deepEqual(pack, ['assignee', 'due', 'labels', 'reminders', 'language', 'attachments']);
  assert.equal(await page.locator('.notes-field').count(), 1, 'a work package carries notes');

  const activity = await rowsOf(CHILDREN[CHILDREN[ENTRY.id][0].id][0].id);
  assert.deepEqual(activity, ['assignee', 'due', 'reminders', 'language']);
  assert.equal(await page.locator('.notes-field').count(), 0, 'an activity was offered notes it cannot keep');
  // And no conversation either: the tab used to stand there with a gate inside it saying so.
  assert.deepEqual(
    (await page.getByRole('tablist', { name: "The entry's history" }).getByRole('tab').allTextContents()).map((each) => each.trim()),
    ['History'],
  );

  // And the dates are one row: pressing it opens the one editor that writes both, which is why
  // two rows for it showed the reader the other date wherever they pressed.
  await page.locator('[data-detail="due"]').click();
  const editor = page.getByRole('dialog', { name: 'Dates' });
  await editor.waitFor({ timeout: 5_000 });
  const inside = (await editor.textContent()) ?? '';
  assert.ok(inside.includes('Date') && inside.includes('Starts'), `the dates editor holds ${inside.slice(0, 120)}`);

  assert.deepEqual(failures, []);
  await context.close();
});

test('chromium: 1280 px — the cover row offers both kinds, and no cover takes no room above the title', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const written = [];
  const { page, failures, close } = await openEntry(browser, 1280, written);
  t.after(close);

  // Nothing above the title for a cover that is not there (ADR-0063 decision 9). Measured rather
  // than read from the markup: an element with no height is still an element, and what the
  // decision is about is the room it takes.
  const above = await page.evaluate(() => {
    const head = document.querySelector('[data-tour="entry"]');
    const title = head?.querySelector('.title-row');
    if (!head || !title) return 'missing';
    return Math.round(title.getBoundingClientRect().top - head.getBoundingClientRect().top);
  });
  assert.equal(above, 0, 'the head keeps room above the title for a cover that is not set');

  await page.locator('[data-detail="cover"]').click();
  const editor = page.getByRole('dialog', { name: 'Cover' });
  await editor.waitFor({ timeout: 5_000 });

  // The row says where a cover goes, rather than only what is missing.
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
});
