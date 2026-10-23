// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *Today* (UC-WRK-34), the page the app opens on, over the shared fixture's day: what is due on
// the reader across hubs with the hub's colour, ticked off where it stands (check 1); a list as
// one tile with its figure (check 2); a project's next due step (check 3); the jumble's count and
// first three (check 4); what this device opened (check 5); and no word about what the person did
// not do (check 6). Also UC-WRK-20's line between overdue and next, in the reader's zone, and its
// empty states. Chromium: a layout and a set of requests, with no engine-specific part.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import {
  ACCOUNT, COLLECTION, PAGE, TODAY_ARRIVALS, TODAY_COLLECTIONS, TODAY_HUB, TODAY_ITEMS, TODAY_STEP,
  dayCompletionsSoFar, showToday, signedIn,
} from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const [PAST, LISTED, TODAYS, LATER] = TODAY_ITEMS;

/** A tab on `/`, in a zone fixed so the day's line falls where the fixture put it. */
async function open(browser, width = 1280) {
  const tab = await signedIn(browser, width, 900, { timezoneId: 'UTC' });
  const searches = [];
  tab.page.on('request', (request) => {
    if (request.url().endsWith('/api/v1/search')) searches.push(request.postDataJSON());
  });
  await tab.page.goto(`${served.origin}/`);
  await tab.page.getByRole('heading', { name: 'Today', level: 1 }).waitFor({ state: 'attached', timeout: 15_000 });
  return { ...tab, searches };
}

const region = (page, name) => page.getByRole('region', { name });

/** Waits until the fixture has been sent this many completions: the request is the proof. */
async function sent(page, count) {
  for (let tries = 0; tries < 50 && dayCompletionsSoFar().length < count; tries++) await page.waitForTimeout(100);
}

// The two walks without the day come first: `showToday` holds for the rest of the process.
test('chromium: nothing on the reader is a sentence, and an empty workspace is one action', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  // The shared fixture without the day: nothing is on the reader.
  const empty = await signedIn(browser, 1280, 900);
  t.after(() => empty.context.close());
  await empty.page.goto(`${served.origin}/`);
  await region(empty.page, 'On you').getByText('Nothing assigned to you has a date on it.').waitFor({ timeout: 15_000 });
  assert.equal(await empty.page.getByRole('region', { name: 'Lists' }).count(), 0, 'an empty Lists panel is drawn');
  assert.deepEqual(empty.failures, []);

  // A workspace with no hub at all: one sentence and one action, not empty panels.
  const fresh = await signedIn(browser, 1280, 900);
  t.after(() => fresh.context.close());
  await fresh.context.route('**/api/v1/containers?type=HUB**', (route) => route.fulfill({ json: { ...PAGE, data: [] } }));
  await fresh.page.goto(`${served.origin}/`);
  await fresh.page.locator('main').getByText('No hubs yet. A hub holds the collections you work in.').waitFor({ timeout: 15_000 });
  assert.equal(await fresh.page.getByRole('region', { name: 'In the jumble' }).count(), 0, 'an empty workspace still draws the panels');
  assert.equal(await fresh.page.locator('main').getByRole('button', { name: /^Create (private )?hub$/ }).count(), 1, 'no way to start a workspace');
  assert.deepEqual(fresh.failures, []);
});

test('chromium: what was opened is remembered on this device, and only here', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());

  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.waitForFunction(
    (id) => Object.keys(localStorage).some((key) => key.startsWith('hubtask.recent.') && (localStorage.getItem(key) ?? '').includes(id)),
    COLLECTION.id,
    { timeout: 10_000 },
  );
  await page.goto(`${served.origin}/`);
  await region(page, 'Recently opened').getByRole('link', { name: COLLECTION.name }).waitFor({ timeout: 10_000 });

  // The account's, kept on this device, under a key nothing else reads.
  const kept = await page.evaluate(() => Object.keys(localStorage).filter((key) => key.startsWith('hubtask.recent.')));
  assert.deepEqual(kept, [`hubtask.recent.${ACCOUNT.id}`]);
  assert.deepEqual(failures, []);
});

test('chromium: Today lists what is due on the reader, plainly, and ticks it off there', async (t) => {
  showToday();
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures, unstubbed, searches } = await open(browser);
  t.after(() => context.close());

  // It is the page the app opens on, and the navigation says so.
  await page.locator('[aria-current="page"]', { hasText: 'Today' }).first().waitFor({ state: 'attached', timeout: 5_000 });

  const mine = region(page, 'On you');
  await mine.getByRole('link', { name: TODAYS.title }).waitFor({ timeout: 15_000 });

  // Mine, open, dated - and `@me` resolved on the server rather than by this client.
  assert.deepEqual(searches[0]?.filter, {
    op: 'AND',
    nodes: [
      { op: 'EQ', field: 'assignee_id', value: '@me' },
      { op: 'EQ', field: 'is_completed', value: false },
      { op: 'NOT', nodes: [{ op: 'IS_NULL', field: 'due_at' }] },
    ],
  });

  // The day's three bands, in the reader's zone, each with what belongs in it.
  const bands = await mine.locator('h3').allTextContents();
  assert.deepEqual(bands, ['Earlier', 'Today', 'Coming up']);
  const titles = await mine.locator('a[href^="/items/"]').allTextContents();
  assert.deepEqual(titles.map((title) => title.trim()), [PAST.title, TODAYS.title, LATER.title], 'the bands hold the entries out of order');

  // A list's entry is the list's tile, not a row (check 2).
  assert.equal(await mine.getByText(LISTED.title).count(), 0, 'a list entry is listed one by one');

  // Check 6: what is past is listed plainly - no overdue word, no warning, anywhere on the page.
  const said = (await page.locator('main').textContent()) ?? '';
  for (const word of [/overdue/i, /missed/i]) assert.equal(word.test(said), false, `the page says ${word}: ${said}`);
  assert.equal(await mine.locator('[data-tone="danger"], [data-tone="warning"]').count(), 0, 'a past entry is drawn as a warning');

  // Each row says its hub in the hub's own colour, beside the hub's name (UC-WRK-33 check 1).
  const pastRow = mine.locator('li, [class*="task-row"]').filter({ hasText: PAST.title }).first();
  assert.equal(await pastRow.getByText(TODAY_HUB.name, { exact: true }).count(), 1, 'the row does not name its hub');
  assert.equal(await pastRow.locator(`[data-token="${TODAY_HUB.color_token}"]`).count(), 1, 'the row does not carry its hub\'s colour');

  // A list as one tile with its figure (check 2).
  const tile = region(page, 'Lists').getByRole('link', { name: new RegExp(TODAY_COLLECTIONS.list.name) });
  assert.equal(await tile.count(), 1, 'the list has no tile');
  assert.match((await tile.textContent()) ?? '', /5 open/);
  assert.equal(await region(page, 'Lists').getByRole('link', { name: new RegExp(TODAY_COLLECTIONS.chores.name) }).count(), 0, 'a collection that is not a list is a tile');

  // A project, its progress and its next due step (check 3).
  const projects = region(page, 'Projects');
  await projects.getByRole('link', { name: TODAY_STEP.title }).waitFor({ timeout: 5_000 });
  assert.match((await projects.textContent()) ?? '', /37% done · 3 of 8/);
  assert.equal(await projects.getByText('Next step').count(), 1);

  // The jumble: how many wait and the first three (check 4).
  const jumble = region(page, 'In the jumble');
  await jumble.getByRole('link', { name: TODAY_ARRIVALS[0].raw_subject }).waitFor({ timeout: 5_000 });
  assert.equal(await jumble.getByRole('link', { name: 'No subject' }).count(), 1);
  assert.equal(await jumble.getByRole('link', { name: 'Open the jumble (2)' }).count(), 1);

  // Recently opened, said in a sentence while there is nothing yet (check 5).
  assert.equal(await region(page, 'Recently opened').getByText('What you open is listed here, on this device only.').count(), 1);

  // Ticking off where it stands is the ordinary completion (check 1): the request, and the row
  // keeps its place, ticked.
  const box = mine.getByRole('checkbox', { name: `Mark ${TODAYS.title} as done` });
  await box.click();
  await sent(page, 1);
  assert.deepEqual(dayCompletionsSoFar(), [`complete ${TODAYS.id}`]);
  assert.equal(await box.isChecked(), true, 'the tick did not stay');
  assert.equal(await mine.getByRole('link', { name: TODAYS.title }).count(), 1, 'the row moved away under the reader');

  // And it is undone where it was made.
  await box.click();
  await sent(page, 2);
  assert.deepEqual(dayCompletionsSoFar(), [`complete ${TODAYS.id}`, `reopen ${TODAYS.id}`]);

  // The row carries the menu every surface draws, by its ⋯ and by a right click (UC-WRK-28 check 1).
  const menu = page.getByRole('menu', { name: `Actions for ${TODAYS.title}` });
  await mine.getByRole('button', { name: `Actions for ${TODAYS.title}`, exact: true }).click();
  await menu.waitFor({ timeout: 5_000 });
  const byButton = await menu.getByRole('menuitem').allTextContents();
  assert.ok(byButton.some((label) => label.includes('Open')), `no Open: ${byButton}`);
  await page.keyboard.press('Escape');
  await menu.waitFor({ state: 'detached', timeout: 5_000 });
  await mine.getByRole('link', { name: TODAYS.title }).click({ button: 'right' });
  await menu.waitFor({ timeout: 5_000 });
  assert.deepEqual(await menu.getByRole('menuitem').allTextContents(), byButton);
  await page.keyboard.press('Escape');

  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

test('chromium: a project\'s step ticked off on Today gives way to the next', async (t) => {
  showToday();
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, 390);
  t.after(() => context.close());

  const projects = region(page, 'Projects');
  await projects.getByRole('link', { name: TODAY_STEP.title }).waitFor({ timeout: 15_000 });
  await projects.getByRole('checkbox', { name: `Mark ${TODAY_STEP.title} as done` }).click();
  // A completion makes every entry list stale, so the project's step is read again - and this
  // project has no dated step left.
  await projects.getByText('No open step has a date yet.').waitFor({ timeout: 10_000 });
  assert.deepEqual(dayCompletionsSoFar(), [`complete ${TODAY_STEP.id}`]);
  assert.deepEqual(failures, []);
});
