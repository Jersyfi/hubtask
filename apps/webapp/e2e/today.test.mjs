// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *Today* (UC-WRK-34), the page the app opens on, arranged as the work-surface scene `today` (one row
// of cards, *Your day* and *Needs you*, tiles under them), over the shared fixture's day: what is due on
// the reader across hubs with the hub's colour, ticked off where it stands (UC-WRK-34 check 1); a list as
// one tile with its figure (UC-WRK-34 check 2); a project's next due step (UC-WRK-34 check 3); the jumble's count and
// first three (UC-WRK-34 check 4); what this device opened (UC-WRK-34 check 5); and no word about what the person did
// not do (UC-WRK-34 check 6, since changed: what is late is said as late). Also UC-WRK-20's line between overdue and next, in the reader's zone, and its
// empty states. Chromium: a layout and a set of requests, with no engine-specific part.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { serve } from './serve.mjs';
import {
  ACCOUNT, COLLECTION, HUB, PAGE, TODAY_ARRIVALS, TODAY_COLLECTIONS, TODAY_HUB, TODAY_ITEMS, TODAY_STEP,
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
  // The page's heading is the greeting, by the hour where the person is (§11.7).
  await tab.page.getByRole('heading', { name: /^(Good morning|Hello|Good evening)/u, level: 1 }).waitFor({ state: 'attached', timeout: 15_000 });
  return { ...tab, searches };
}

const region = (page, name) => page.getByRole('region', { name });

/** Waits until the fixture has been sent this many completions: the request is the proof. */
async function sent(page, count) {
  for (let tries = 0; tries < 50 && dayCompletionsSoFar().length < count; tries++) await page.waitForTimeout(100);
}

// The two walks without the day come first: `showToday` holds for the rest of the process.
test('chromium: nothing on the reader is a sentence, and an empty workspace is one action', async (t) => {
  const browser = await sharedChromium(t);

  // The shared fixture without the day: nothing is on the reader, but its hub holds entries - an
  // owner of a busy workspace. Not the first steps, which would ask for the hub that exists, but
  // the way into what is there.
  const used = await signedIn(browser, 1280, 900);
  t.after(() => used.context.close());
  await used.page.goto(`${served.origin}/`);
  const inUse = used.page.getByRole('region', { name: 'In use here' });
  await inUse.waitFor({ timeout: 15_000 });
  assert.equal(await inUse.getByRole('link', { name: HUB.name }).getAttribute('href'), `/hubs/${HUB.id}`);
  assert.equal(await used.page.getByRole('region', { name: 'First steps' }).count(), 0, 'a used workspace is offered its first steps');
  await used.page.getByText('Nothing assigned to you has a date on it.').waitFor();
  assert.deepEqual(used.failures, []);

  // The same tree with no entry in any hub: a new workspace.
  const empty = await signedIn(browser, 1280, 900);
  t.after(() => empty.context.close());
  await empty.context.route('**/api/v1/items:query', async (route) => {
    const body = route.request().postDataJSON();
    if (body?.scope?.container_id === HUB.id && body.page?.size === 1) return route.fulfill({ json: { data: [], groups: [], page: { next_cursor: null, has_more: false } } });
    return route.fallback();
  });
  await empty.page.goto(`${served.origin}/`);
  // Nothing on the reader, no list, no project, nothing opened, nothing waiting: not empty cards
  // but the first steps, each with its act (concept round 2, §1).
  const steps = empty.page.getByRole('region', { name: 'First steps' });
  await steps.waitFor({ timeout: 15_000 });
  assert.equal(await empty.page.getByRole('region', { name: 'Your collections' }).count(), 0, 'an empty collections tile is drawn');
  assert.equal(await empty.page.getByRole('region', { name: 'Recently opened' }).count(), 0, 'an empty Recently opened panel is drawn');
  assert.equal(await steps.getByRole('button', { name: 'Capture' }).count(), 1, 'the first steps offer no capture');
  assert.match((await steps.textContent()) ?? '', /Create a hub/u);
  assert.deepEqual(empty.failures, []);

  // A workspace with no hub at all: one sentence and one action, not empty panels.
  const fresh = await signedIn(browser, 1280, 900);
  t.after(() => fresh.context.close());
  await fresh.context.route('**/api/v1/containers?type=HUB**', (route) => route.fulfill({ json: { ...PAGE, data: [] } }));
  await fresh.page.goto(`${served.origin}/`);
  await fresh.page.locator('main').getByText('No hubs yet. A hub holds the collections you work in.').waitFor({ timeout: 15_000 });
  assert.equal(await fresh.page.getByRole('region', { name: 'In the inbox' }).count(), 0, 'an empty workspace still draws the panels');
  assert.equal(await fresh.page.locator('main').getByRole('button', { name: /^Create (private )?hub$/ }).count(), 1, 'no way to start a workspace');
  assert.deepEqual(fresh.failures, []);
});

test('chromium: what was opened is remembered on this device, and only here', async (t) => {
  const browser = await sharedChromium(t);
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
  const browser = await sharedChromium(t);
  const { page, context, failures, unstubbed, searches } = await open(browser);
  t.after(() => context.close());

  // It is the page the app opens on, and the navigation says so.
  await page.locator('[aria-current="page"]', { hasText: 'Today' }).first().waitFor({ state: 'attached', timeout: 5_000 });

  const mine = page.getByRole('region', { name: 'Your day', exact: true });
  await mine.getByRole('link', { name: TODAYS.title }).waitFor({ timeout: 15_000 });

  // Mine, open, dated - and `@me` resolved on the server rather than by this client.
  assert.deepEqual(searches[0]?.filter, {
    op: 'AND',
    nodes: [
      { op: 'EQ', field: 'assignee_id', value: '@me' },
      { op: 'EQ', field: 'is_completed', value: false },
    ],
  });

  // Capture is the one way to add an entry (UC-WRK-26/7): the field on top opens it.
  await mine.locator('[data-capture-field]').click();
  await page.getByRole('dialog').first().waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');

  // The day's bands, in the reader's zone, each with what belongs in it; upcoming folded.
  const bands = await mine.locator('h3').allTextContents();
  assert.deepEqual(bands.map((band) => band.replace(/\s+/gu, ' ').trim()), ['Overdue 1', 'Today 1']);
  const titles = await mine.locator('a[href^="/items/"] .title').allTextContents();
  assert.deepEqual(titles.map((title) => title.trim()), [PAST.title, TODAYS.title], 'the bands hold the entries out of order');
  await mine.getByText('Coming up').click();
  await mine.getByRole('link', { name: LATER.title }).waitFor({ timeout: 5_000 });

  // A title keeps the room to be read beside its hub and collection: where both do not fit, the
  // place goes under the title rather than squeezing it to a few words.
  const room = await mine.locator('[data-entry-row] .content').evaluateAll((nodes) => nodes.map((node) => Math.round(node.getBoundingClientRect().width)));
  assert.ok(room.length > 0 && room.every((width) => width >= 200), `a title is squeezed: ${room}`);

  // A list's entry is the list's tile, not a row (check 2).
  assert.equal(await mine.getByText(LISTED.title).count(), 0, 'a list entry is listed one by one');

  // Check 6, as the owner changed it: what is late is said as late - the band, and the overdue
  // mark on its row - and nothing blames ("missed").
  const said = (await page.locator('main').textContent()) ?? '';
  assert.equal(/missed/i.test(said), false, `the page says missed: ${said}`);
  const late = mine.locator('[class*="task-row"]').filter({ hasText: PAST.title }).first();
  assert.equal(await late.locator('[data-tone="danger"]').count(), 1, 'a late entry does not say so');
  // The late band's heading is red and says it in its word (§3); the day's band is not red.
  const tones = await mine.locator('h3').evaluateAll((nodes) => nodes.map((node) => getComputedStyle(node).color));
  assert.notEqual(tones[0], tones[1], 'the late band is not set apart');
  // Each row carries its key before the title where its collection gives one (work-surface §7 Today).
  assert.equal((await late.locator('[data-entry-key]').textContent())?.trim(), 'CHR-21');

  // Each row says its hub in the hub's own colour, beside the hub's name (UC-WRK-33 check 1).
  const pastRow = mine.locator('li, [class*="task-row"]').filter({ hasText: PAST.title }).first();
  assert.equal(await pastRow.getByText(TODAY_HUB.name, { exact: true }).count(), 1, 'the row does not name its hub');
  assert.equal(await pastRow.locator(`[data-token="${TODAY_HUB.color_token}"]`).count(), 1, 'the row does not carry its hub\'s colour');

  // A list as one row with its figure in *Your collections* (check 2).
  const collections = region(page, 'Your collections');
  const tile = collections.locator('[data-list]').filter({ hasText: TODAY_COLLECTIONS.list.name });
  assert.equal(await tile.count(), 1, 'the list has no row');
  assert.match((await tile.textContent()) ?? '', /5 open/);
  assert.equal(await collections.locator('[data-list]').filter({ hasText: TODAY_COLLECTIONS.chores.name }).count(), 0, 'a collection that is not a list is a list row');

  // A project, its progress and its next due step (check 3).
  await collections.getByRole('link', { name: TODAY_STEP.title }).waitFor({ timeout: 5_000 });
  assert.match((await collections.textContent()) ?? '', /37% done · 3 of 8/);
  assert.equal(await collections.getByText('Next step').count(), 1);

  // The jumble as a small inbox: how many wait and the first three, each with its decisions (check 4).
  const jumble = region(page, 'In the inbox');
  await jumble.getByRole('link', { name: TODAY_ARRIVALS[0].raw_subject }).waitFor({ timeout: 5_000 });
  assert.equal(await jumble.getByRole('link', { name: 'No subject' }).count(), 1);
  assert.equal(await jumble.getByRole('button', { name: 'Open', exact: true }).count(), 1);
  assert.match((await jumble.textContent()) ?? '', /2 waiting/u);

  // The two columns end level (§6.1).
  const columns = await page.locator('.columns > *').evaluateAll((nodes) => nodes.map((node) => Math.round(node.getBoundingClientRect().bottom)));
  assert.ok(columns.length < 2 || Math.abs(columns[0] - columns[1]) <= 1, `the columns do not end level: ${columns}`);

  // No points, streaks or rankings on the person's day (UC-WRK-29).
  assert.equal(/streak|ranking|leaderboard|high score/iu.test(said), false, `the page keeps a score: ${said}`);

  // Recently opened is not drawn while nothing was opened on this device (check 5): an empty card
  // is not a card.
  assert.equal(await page.getByRole('region', { name: 'Recently opened' }).count(), 0);

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
  const browser = await sharedChromium(t);
  const { page, context, failures } = await open(browser, 390);
  t.after(() => context.close());

  // A phone shows *Your day* or the overview, one at a time (§7): the collections are the overview's.
  await page.getByRole('tab', { name: 'Overview' }).click();
  const projects = region(page, 'Your collections');
  await projects.getByRole('link', { name: TODAY_STEP.title }).waitFor({ timeout: 15_000 });
  await projects.getByRole('checkbox', { name: `Mark ${TODAY_STEP.title} as done` }).click();
  // A completion makes every entry list stale, so the project's step is read again - and this
  // project has no dated step left.
  await projects.getByText('No open step has a date yet.').waitFor({ timeout: 10_000 });
  assert.deepEqual(dayCompletionsSoFar(), [`complete ${TODAY_STEP.id}`]);
  assert.deepEqual(failures, []);
});

// UC-WRK-34 check 7: a greeting by the hour and figures that lead to their entries - the cards'
// row, each card with its one act.
test('chromium: the day at a glance - a greeting by the hour, cards that lead to their search, the overdue among them (check 7)', async (t) => {
  showToday();
  const browser = await sharedChromium(t);
  const { page, context, failures } = await open(browser);
  t.after(() => context.close());

  const glance = region(page, 'Your day at a glance');
  await glance.waitFor({ timeout: 15_000 });
  const due = glance.locator('[data-card="DUE_TODAY"]');
  await due.getByText('1', { exact: true }).waitFor({ timeout: 15_000 });
  // What is late, counted, said as late beside its figure, and a way to it (check 6, as changed).
  const overdue = glance.locator('[data-card="OVERDUE"]');
  assert.match((await overdue.textContent()) ?? '', /1\s*overdue/u);
  // The cards stand in one row, equally wide.
  const widths = await glance.locator('[data-card]').evaluateAll((all) => all.map((card) => Math.round(card.getBoundingClientRect().width)));
  assert.ok(widths.length >= 2 && widths.every((width) => Math.abs(width - widths[0]) <= 1), `the cards are not equal: ${widths}`);
  await overdue.getByRole('button', { name: 'Show' }).click();
  await page.waitForURL(/\/search\?/u, { timeout: 5_000 });
  assert.equal(new URL(page.url()).searchParams.get('f'), 'who:me is:open due:overdue');
  await page.goBack();
  await glance.locator('[data-card="DUE_TODAY"]').getByRole('button', { name: 'Show' }).click();
  await page.waitForURL(/due%3Atoday|due:today/u, { timeout: 5_000 });
  assert.equal(new URL(page.url()).searchParams.get('f'), 'who:me is:open due:today');
  await page.goBack();
  // The greeting by the hour where the person is, as the page's heading.
  const subtitle = await page.locator('main').getByText(/^(Good morning|Hello|Good evening)/).first().textContent();
  assert.ok(subtitle.includes(ACCOUNT.display_name.split(' ')[0]), `the greeting does not name the person: ${subtitle}`);
  assert.deepEqual(failures, []);
});

// UC-WRK-34 check 9 (E-17): *Sections* hides and orders the cards and tiles, written to the account so another device
// draws the same; a card the person's modules do not give is not offered at all (P-17).
test('chromium: Sections hides and orders Today\'s cards and tiles, on the account', async (t) => {
  showToday();
  const browser = await sharedChromium(t);
  const { page, context, failures } = await open(browser);
  t.after(() => context.close());
  const written = [];
  let stored = {};
  await context.route(`**/api/v1/accounts/${ACCOUNT.id}/preferences`, (route) => {
    const body = route.request().postDataJSON();
    written.push(body);
    stored = { ...stored, ...body };
    return route.fulfill({ json: { ...ACCOUNT, ...stored } });
  });
  await context.route(`**/api/v1/accounts/${ACCOUNT.id}`, (route) => route.fulfill({ json: { ...ACCOUNT, ...stored } }));
  await context.route('**/api/v1/accounts/me', (route) => route.fulfill({ json: { ...ACCOUNT, ...stored } }));

  const glance = region(page, 'Your day at a glance');
  await glance.locator('[data-card="OVERDUE"]').waitFor({ timeout: 15_000 });
  await page.getByRole('button', { name: 'Sections' }).click();
  const dialog = page.getByRole('dialog', { name: 'Sections of Today' });
  await dialog.waitFor({ timeout: 5_000 });
  // Only the cards and tiles this person has: no time card without the time module.
  const offered = await dialog.locator('[data-section]').evaluateAll((rows) => rows.map((row) => row.getAttribute('data-section')));
  assert.ok(offered.includes('OVERDUE') && offered.includes('RECENT'), `offered ${offered}`);
  assert.equal(offered.includes('TIME'), false, 'a card of a module that is off is offered');

  await dialog.locator('[data-section="OVERDUE"]').getByRole('checkbox').click();
  for (let tries = 0; tries < 50 && written.length === 0; tries++) await page.waitForTimeout(100);
  assert.deepEqual(written[0]?.today_sections_hidden, ['OVERDUE']);
  await dialog.locator('[data-section="DUE_TODAY"]').getByRole('button', { name: 'Move Due today up' }).click();
  for (let tries = 0; tries < 50 && written.length < 2; tries++) await page.waitForTimeout(100);
  assert.deepEqual(written[1]?.today_sections?.slice(0, 2), ['DUE_TODAY', 'OVERDUE']);
  await page.keyboard.press('Escape');
  await glance.locator('[data-card="OVERDUE"]').waitFor({ state: 'detached', timeout: 5_000 });
  assert.equal(await glance.locator('[data-card="DUE_TODAY"]').count(), 1);
  assert.deepEqual(failures, []);
});

// §7: on a phone the cards swipe and *Your day | Overview* is a switch, reached within one and a
// half screens.
// UC-WRK-20 check 8: the phone's cards are one row it swipes, and it switches between your day and the overview.
test('chromium 390 px: the phone swipes the cards and switches between your day and the overview', async (t) => {
  showToday();
  const browser = await sharedChromium(t);
  const { page, context, failures } = await open(browser, 390);
  t.after(() => context.close());
  const glance = region(page, 'Your day at a glance');
  await glance.locator('[data-card="OVERDUE"]').waitFor({ timeout: 15_000 });
  const strip = await glance.evaluate((node) => ({ scroll: node.scrollWidth, width: node.clientWidth, overflow: getComputedStyle(node).overflowX }));
  assert.equal(strip.overflow, 'auto');
  const tab = page.getByRole('tab', { name: 'Overview' });
  const bottom = await tab.evaluate((node) => node.getBoundingClientRect().bottom + window.scrollY);
  assert.ok(bottom <= 844 * 1.5, `the switch stands at ${bottom} px`);
  const day = page.getByRole('region', { name: 'Your day', exact: true });
  assert.equal(await day.count(), 1);
  await tab.click();
  await region(page, 'Needs you').or(region(page, 'Your collections')).first().waitFor({ timeout: 5_000 });
  assert.equal(await day.count(), 0, 'both halves stand at once');
  const wide = await page.evaluate(() => [...document.querySelectorAll('main *')].filter((node) => node.getBoundingClientRect().right > window.innerWidth + 1).slice(0, 6).map((node) => `${node.tagName}.${node.className}`));
  const scroll = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: window.innerWidth }));
  assert.equal(scroll.width, scroll.viewport, `the page scrolls sideways (${scroll.width} of ${scroll.viewport}): ${wide}`);
  assert.deepEqual(failures, []);
});
