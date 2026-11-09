// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Time clicked through as a person and their lead do it (UC-TIM-01, UC-TIM-02, UC-TIM-03): a timer
// started and stopped on an entry, the week with that record handed in, and a colleague's week
// approved - with no console error and no refused or failed answer on the way. The screen as
// user-experience.md §7.6 draws it: the week's figure and its days in the head, a timer left running
// past a day recorded until a time on its start's day, an empty week that says what to do, and a
// phone that scrolls nothing sideways.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, COLLECTION, HUB, ITEMS, OTHER } from './fixture.mjs';
import { ids, productWalk } from './productwalk.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const DAY = 86_400_000;
const today = new Date().toISOString().slice(0, 10);
const mondayOf = (day) => {
  const at = Date.parse(`${day}T00:00:00Z`);
  return new Date(at - ((new Date(at).getUTCDay() + 6) % 7) * DAY).toISOString().slice(0, 10);
};
const MONDAY = mondayOf(today);
const LAST_MONDAY = new Date(Date.parse(`${MONDAY}T00:00:00Z`) - 7 * DAY).toISOString().slice(0, 10);
const [ENTRY] = ITEMS;

/** The time module's routes, kept in memory as the server would keep them. */
function timeBackend({ running, kinds, rates, absences } = {}) {
  const id = ids('00000b');
  const defined = kinds ?? [{ key: 'work', name: 'Work', billable: true, archived: false, position: 0, version: 1 }];
  let timer = running ?? { running: false };
  const records = [];
  const sheet = (account, week, state) => ({
    account_id: account, week_start: week, state, submitted_minutes: 0, submitted_at: null, decided_at: null, decided_by: null, reason: '', version: 1,
  });
  const mine = sheet(ACCOUNT.id, MONDAY, 'OPEN');
  const theirs = { ...sheet(OTHER.id, LAST_MONDAY, 'SUBMITTED'), submitted_minutes: 450, submitted_at: `${LAST_MONDAY}T17:00:00Z` };
  const week = (of, held) => {
    const days = Array.from({ length: 7 }, (_, i) => new Date(Date.parse(`${of.week_start}T00:00:00Z`) + i * DAY).toISOString().slice(0, 10));
    return { ...of, records: held, days: days.map((day) => ({ day, minutes: held.filter((each) => each.day === day).reduce((sum, each) => sum + each.minutes, 0), locked: false })), minutes: held.reduce((sum, each) => sum + each.minutes, 0) };
  };
  return (method, path, query, body) => {
    if (path === '/time-work-types' && method === 'GET') return { json: { data: defined } };
    const kind = /^\/time-work-types\/([a-z_]+)$/.exec(path);
    if (kind && method === 'PUT') {
      const made = { key: kind[1], archived: false, version: 1, ...body };
      defined.push(made);
      return { json: made };
    }
    if (path === '/time-policy' && method === 'GET') return { json: { locked_through: null, visibility: 'FULL', version: 1 } };
    // A week drawn before its kinds arrive says whom to ask (user-experience.md §5.5): the owner here.
    if (path === '/access/explain' && method === 'GET') {
      return { json: { account_id: ACCOUNT.id, scope_type: query.get('scope_type') ?? 'TENANT', roles: ['OWNER'], permissions: [], sources: [] } };
    }
    if (path === '/timer' && method === 'GET') return { json: timer };
    if (path === '/time-capacities' && method === 'GET') return { json: { data: [] } };
    if (path === '/cost-rates' && method === 'GET') return { json: { data: rates ?? [] } };
    if (path === '/time-absences' && method === 'GET') return { json: { data: absences ?? [] } };
    if (path === '/timer:start' && method === 'POST') {
      timer = { running: true, entry_id: body.entry_id ?? null, work_type: body.work_type ?? 'work', note: '', billable: true, started_at: new Date(Date.now() - 25 * 60_000).toISOString(), elapsed_minutes: 25 };
      return { json: timer };
    }
    if (path === '/timer:stop' && method === 'POST') {
      const made = {
        id: id(), account_id: ACCOUNT.id, entry_id: timer.entry_id, work_type: timer.work_type, day: today, started_at: timer.started_at,
        minutes: body?.stopped_at ? Math.ceil((Date.parse(body.stopped_at) - Date.parse(timer.started_at)) / 60_000) : 25,
        note: '', billable: true, source: 'TIMER', corrects: null, created_at: new Date().toISOString(), updated_at: new Date().toISOString(), version: 1,
        collection_id: COLLECTION.id, hub_id: HUB.id, entry_title: ENTRY.title,
      };
      records.push(made);
      timer = { running: false };
      return { status: 201, json: made };
    }
    const one = /^\/timesheets\/(\d{4}-\d{2}-\d{2})(:submit|:approve|:reject)?$/.exec(path);
    if (one && !one[2] && method === 'GET') {
      if (query.get('account_id') === OTHER.id) return { json: week(theirs, []) };
      return { json: week(one[1] === MONDAY ? mine : sheet(ACCOUNT.id, one[1], 'OPEN'), one[1] === MONDAY ? records : []) };
    }
    if (one && one[2] === ':submit' && method === 'POST' && one[1] === MONDAY) {
      Object.assign(mine, { state: 'SUBMITTED', submitted_minutes: records.reduce((sum, each) => sum + each.minutes, 0), submitted_at: new Date().toISOString(), version: mine.version + 1 });
      return { json: mine };
    }
    if (one && one[2] === ':approve' && method === 'POST' && body?.account_id === OTHER.id && one[1] === LAST_MONDAY) {
      Object.assign(theirs, { state: 'APPROVED', decided_at: new Date().toISOString(), decided_by: ACCOUNT.id, version: theirs.version + 1 });
      return { json: theirs };
    }
    if (one && one[2] === ':reject' && method === 'POST' && body?.account_id === OTHER.id && one[1] === LAST_MONDAY && body?.reason) {
      Object.assign(theirs, { state: 'REJECTED', reason: body.reason, decided_at: new Date().toISOString(), decided_by: ACCOUNT.id, version: theirs.version + 1 });
      return { json: theirs };
    }
    if (path === '/timesheets' && method === 'GET' && query.get('state') === 'SUBMITTED') {
      return { json: { data: [theirs, mine].filter((each) => each.state === 'SUBMITTED') } };
    }
    return undefined;
  };
}

test('chromium: time - a timer on an entry, the week handed in, a colleague\'s week approved - without an error', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const walk = await productWalk(browser, { modules: ['TIME'], answer: timeBackend() });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByRole('button', { name: 'Start timer' }).click();
  await page.getByRole('button', { name: 'Stop timer' }).first().click();
  await page.getByRole('status').filter({ hasText: 'recorded.' }).first().waitFor({ timeout: 5_000 });

  await page.goto(`${served.origin}/time`);
  await page.getByText(ENTRY.title).first().waitFor({ timeout: 5_000 });
  await page.getByRole('button', { name: 'Hand in' }).click();
  await page.getByRole('button', { name: 'Take back' }).waitFor({ timeout: 5_000 });

  await page.getByText(/^0:25 this week · Handed in$/).waitFor({ timeout: 5_000 });
  assert.equal(await page.locator('[data-page-kind="work"]').count(), 1, 'the time is no work page');
  await page.getByRole('link', { name: 'To decide' }).click();
  await page.getByRole('button', { name: 'Approve' }).first().click();
  await page.waitForTimeout(400);

  const wrote = walk.written.map((each) => `${each.method} ${each.path}`);
  for (const want of ['POST /timer:start', 'POST /timer:stop', `POST /timesheets/${MONDAY}:submit`, `POST /timesheets/${LAST_MONDAY}:approve`]) {
    assert.ok(wrote.includes(want), `${want} was not written: ${JSON.stringify(wrote)}`);
  }
  const start = walk.written.find((each) => each.path === '/timer:start');
  assert.equal(start.body.entry_id, ENTRY.id);
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});

// UC-TIM-03 check 7: the approver is shown the queue of weeks to decide - whose, which week and
// how long - opens one whole and sends it back with a reason, after which the queue holds nothing.
test('chromium: time - a colleague\'s week is opened from the queue and sent back with a reason', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const walk = await productWalk(browser, { modules: ['TIME'], answer: timeBackend() });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/time`);
  await page.getByRole('link', { name: 'To decide' }).click();
  const queue = page.getByRole('table', { name: 'Weeks waiting for your decision' });
  const row = queue.getByRole('row').filter({ hasText: OTHER.display_name });
  await row.waitFor({ timeout: 5_000 });
  await row.getByText('7:30').waitFor();
  // The row opens the week to decide: its title is the press (PS-6).
  await row.getByRole('button', { name: OTHER.display_name }).click();
  await page.getByRole('button', { name: 'Send back' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Why').fill('Tuesday is missing.');
  await dialog.getByRole('button', { name: 'Send back' }).click();
  await page.getByText(`${OTHER.display_name}'s week sent back.`).first().waitFor({ timeout: 5_000 });

  const sent = walk.written.find((each) => each.path === `/timesheets/${LAST_MONDAY}:reject`);
  assert.deepEqual(sent?.body, { account_id: OTHER.id, reason: 'Tuesday is missing.' });
  await page.getByText('No week waits for your decision.').waitFor({ timeout: 5_000 });
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});

test('chromium: time - a timer left running past a day is recorded until a time on its start\'s day', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  // Started two days ago at 09:00 UTC (the walk's zone): a stop now would be refused.
  const started = `${new Date(Date.now() - 2 * DAY).toISOString().slice(0, 10)}T09:00:00.000Z`;
  const running = { running: true, entry_id: null, work_type: 'work', note: '', billable: true, started_at: started, elapsed_minutes: 2880 };
  const walk = await productWalk(browser, { modules: ['TIME'], answer: timeBackend({ running }) });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/time`);
  await page.getByText(/Timer running since .*, 09:00: more than a day/).waitFor({ timeout: 10_000 });
  assert.equal(await page.getByRole('button', { name: 'Stop timer' }).count(), 0, 'a stop now is not offered');
  await page.getByRole('button', { name: 'Record until…' }).click();
  const until = page.getByLabel('Until', { exact: true });
  // Eight hours on without a capacity: the usual end of work on the start's day.
  assert.equal(await until.inputValue(), '17:00');
  await until.fill('08:30');
  await page.getByRole('dialog').getByRole('button', { name: 'Record', exact: true }).click();
  await page.getByText('Choose a time after 09:00.').waitFor({ timeout: 5_000 });
  await until.fill('16:30');
  await page.getByRole('dialog').getByRole('button', { name: 'Record', exact: true }).click();
  await page.getByRole('status').filter({ hasText: '7:30 recorded on' }).first().waitFor({ timeout: 5_000 });

  const stop = walk.written.find((each) => each.path === '/timer:stop');
  assert.equal(stop.body.stopped_at, `${started.slice(0, 10)}T16:30:00.000Z`);
  assert.equal(stop.body.zone, 'UTC');
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});

test('chromium: time - 375 px, an empty week says what to do and nothing scrolls sideways', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  // The reader away on the 3rd of this month, so the absence calendar draws a person's month.
  const thisMonth = new Date().toISOString().slice(0, 7);
  const away = { id: '01a0e2e0-0000-7000-8000-00000000ab01', account_id: ACCOUNT.id, from: `${thisMonth}-03`, to: `${thisMonth}-03`, kind: 'LEAVE', half: false, version: 1 };
  const walk = await productWalk(browser, { modules: ['TIME'], answer: timeBackend({ absences: [away] }), width: 375, height: 812 });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/time`);
  await page.getByText('No time recorded this week.').waitFor({ timeout: 10_000 });
  assert.equal(await page.getByRole('button', { name: 'Start timer' }).count(), 1, 'the timer starts from the head');
  // The head pins *Record time* where the thumb is; the empty week draws no second one under it.
  assert.equal(await page.getByRole('button', { name: 'Record time', exact: true }).count(), 1, 'one Record time on a phone');
  const covered = await page.evaluate(() => {
    const thumb = [...document.querySelectorAll('[data-folded] .primary')].find((each) => getComputedStyle(each).position === 'fixed');
    const at = thumb?.getBoundingClientRect();
    if (!at) return ['no pinned primary'];
    return [...document.querySelectorAll('main button, main a')].filter((each) => !thumb.contains(each)).map((each) => ({ each, r: each.getBoundingClientRect() }))
      .filter(({ r }) => r.width > 0 && r.left < at.right && r.right > at.left && r.top < at.bottom && r.bottom > at.top)
      .map(({ each }) => each.textContent.trim());
  });
  assert.deepEqual(covered, [], 'the pinned primary covers a control');
  assert.equal(await page.getByRole('list', { name: /^The days of / }).getByRole('listitem').count(), 7);
  assert.equal(await page.getByRole('table').count(), 0, 'an empty week draws no empty table');
  const wide = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(wide <= 0, `the page scrolls ${wide} px sideways`);

  // A day of the strip records on that day.
  await page.getByRole('button', { name: /^Record time on / }).first().click();
  await page.getByRole('dialog').getByLabel('Day').waitFor({ timeout: 5_000 });
  assert.equal(await page.getByRole('dialog').getByLabel('Day').inputValue(), MONDAY);
  await page.keyboard.press('Escape');

  // The absence calendar names its month and days in the reader's words, never `2026-11`.
  await page.goto(`${served.origin}/time?tab=absences`);
  const month = new Intl.DateTimeFormat('en', { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date());
  await page.getByText(month, { exact: true }).waitFor({ timeout: 5_000 });
  assert.equal(await page.getByText(/^\d{4}-\d{2}$/).count(), 0);
  // A person's month keeps its day numbers: each day says its date to a screen reader, and a phone
  // shows every week's first day above its mark - without scrolling sideways.
  const strip = page.locator('ol.month:visible').first();
  await strip.waitFor({ timeout: 5_000 });
  assert.match((await strip.getByRole('listitem').nth(2).getAttribute('aria-label')) ?? '', /3.*: Leave$/);
  const shown = await strip.locator('.number').evaluateAll((nodes) => nodes.filter((node) => getComputedStyle(node).visibility === 'visible').map((node) => node.textContent));
  assert.deepEqual(shown, ['1', '8', '15', '22', '29'].slice(0, Math.ceil((await strip.getByRole('listitem').count()) / 7)));
  const wider = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(wider <= 0, `the absence calendar scrolls ${wider} px sideways`);
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});

// A locked day is drawn as a card that is not a button, so both kinds are measured.
for (const isLocked of [false, true]) test(`chromium: time - 1280 px, each ${isLocked ? 'locked' : 'open'} day card keeps inside its own cell and the content edge`, async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const backend = timeBackend();
  const answer = (method, path, query, body) => {
    const answered = backend(method, path, query, body);
    if (isLocked && answered?.json?.days) return { json: { ...answered.json, days: answered.json.days.map((day) => ({ ...day, locked: true })) } };
    return answered;
  };
  const walk = await productWalk(browser, { modules: ['TIME'], answer, width: 1280, height: 900 });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/time`);
  const days = page.getByRole('list', { name: /^The days of / });
  await days.getByRole('listitem').nth(6).waitFor({ timeout: 10_000 });
  const overruns = await days.evaluate((list) => {
    const edge = list.getBoundingClientRect().right;
    return [...list.querySelectorAll(':scope > li')].map((cell) => {
      const card = cell.querySelector('.day').getBoundingClientRect();
      const own = cell.getBoundingClientRect();
      return Math.max(card.right - own.right, card.right - edge, 0);
    }).filter((over) => over > 0.5);
  });
  assert.deepEqual(overruns, [], 'a day card overruns its cell or the content edge');
  assert.deepEqual(walk.errors(), []);
});

// §4: a workspace's kinds of work are there on first use - the first reader who may write them
// gets the usual ones, in their words - so the timer starts and a day records.
test('chromium: time - a workspace without a kind of work is given the usual ones, and the timer starts', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const walk = await productWalk(browser, { modules: ['TIME'], answer: timeBackend({ kinds: [] }) });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/time`);
  const start = page.getByRole('button', { name: 'Start timer' });
  await start.waitFor({ timeout: 10_000 });
  for (let waited = 0; await start.isDisabled() && waited < 5000; waited += 100) await page.waitForTimeout(100);
  assert.equal(await start.isDisabled(), false, 'the timer still cannot start');
  await page.getByRole('button', { name: 'Record time', exact: true }).first().click();
  await page.getByRole('dialog').getByLabel('Day').waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');

  await page.goto(`${served.origin}/time/settings/kinds`);
  await page.getByRole('cell', { name: 'Development', exact: true }).waitFor({ timeout: 10_000 });
  assert.equal(await page.getByRole('columnheader', { name: 'Key' }).count(), 0, 'a key column is drawn');
  // A settings page of its own (page-system §9.6): the home's *Record time* is not shown on it, and
  // adding a kind of work is its section's own act.
  assert.equal(await page.getByRole('button', { name: 'Record time', exact: true }).count(), 0, 'the home\'s act is shown on Settings');
  assert.equal(await page.locator('[data-time-part="kinds"]').getByRole('button', { name: 'Add a kind of work' }).count(), 1, 'adding a kind is not in its section');

  // The policy's settings are rows as on every settings screen (PS-11): inside the region's
  // padding, never on its edge.
  await page.goto(`${served.origin}/time/settings/policy`);
  const policyRow = page.locator('[data-setting="time.locked_through"]');
  await policyRow.waitFor({ timeout: 10_000 });
  const inset = await policyRow.evaluate((row) => {
    const region = row.closest('.region')?.getBoundingClientRect();
    const name = row.querySelector('.name')?.getBoundingClientRect();
    return region && name ? Math.round(name.left - region.left) : -1;
  });
  assert.ok(inset >= 16, `the policy's first row starts ${inset} px inside its region`);
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});

// user-experience.md §5.12: on a phone the running timer, a long entry title among it, wraps
// rather than widening the page.
test('chromium: time - 390 px, a running timer on a long title scrolls nothing sideways', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const title = 'Als Kundin möchte ich meine offenen Rechnungen im Portal sehen und einzeln herunterladen können';
  const running = { running: true, entry_id: ENTRY.id, entry_title: title, work_type: 'work', note: '', billable: true, started_at: new Date(Date.now() - 25 * 60_000).toISOString(), elapsed_minutes: 25 };
  const walk = await productWalk(browser, { modules: ['TIME'], answer: timeBackend({ running }), width: 390, height: 844 });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/time`);
  await page.locator('[data-timer="running"]').getByRole('button', { name: `On ${title}` }).waitFor({ timeout: 10_000 });
  await page.getByRole('button', { name: 'Stop timer' }).waitFor({ timeout: 5_000 });
  const wide = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(wide <= 0, `the page scrolls ${wide} px sideways`);
  assert.deepEqual(walk.errors(), []);
});

// user-experience.md §5.10: a rate's days are the reader's dates, never ISO text.
test('chromium: time - the cost rates name their days in words', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const rate = { id: '01a0e2e0-0000-7000-8000-00000000b701', account_id: null, role: 'MEMBER', hour_minor: 9000, currency: 'EUR', from: '2025-11-05', to: '2026-03-31', version: 1 };
  const walk = await productWalk(browser, { modules: ['TIME'], answer: timeBackend({ rates: [rate] }) });
  t.after(walk.close);
  const { page } = walk;

  // The old address of a section lands on its own (page-system PS-9).
  await page.goto(`${served.origin}/time?tab=settings&section=rates`);
  await page.waitForFunction(() => location.pathname === '/time/settings/rates', null, { timeout: 15_000 });
  const rates = page.getByRole('table', { name: 'Cost rates' });
  await rates.getByText('Nov 5, 2025', { exact: true }).waitFor({ timeout: 10_000 });
  await rates.getByText('Mar 31, 2026', { exact: true }).waitFor({ timeout: 5_000 });
  assert.equal(await page.getByText('2025-11-05').count(), 0, 'no ISO day reaches the reader');
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});

// The time head names the entry the timer runs on (user-experience.md §7.6), and the name opens it.
test('chromium: time - the running timer names its entry in the head and opens it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const started = new Date(Date.now() - 25 * 60_000).toISOString();
  const running = { running: true, entry_id: ENTRY.id, entry_title: ENTRY.title, work_type: 'work', note: '', billable: true, started_at: started, elapsed_minutes: 25 };
  const walk = await productWalk(browser, { modules: ['TIME'], answer: timeBackend({ running }) });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/time`);
  const named = page.locator('[data-timer="running"]').getByRole('button', { name: `On ${ENTRY.title}` });
  await named.waitFor({ timeout: 5_000 });
  await named.click();
  await page.waitForURL(new RegExp(`/items/${ENTRY.id}$`));
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});

// The product's first mark keeps the width of its words where its target ends the head: it turns to
// end at the create act instead of squeezing into the strip beside it (user-experience.md §3.4).
test('chromium: time - the first visit\'s mark at the head\'s create act keeps its width', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { products_seen: _seen, ...newcomer } = ACCOUNT;
  const backend = timeBackend();
  const answer = (method, path, query, body) =>
    (path === '/accounts/me' || path === `/accounts/${ACCOUNT.id}`) && method === 'GET' ? { json: newcomer } : backend(method, path, query, body);
  const walk = await productWalk(browser, { modules: ['TIME'], answer });
  t.after(walk.close);
  const { page } = walk;
  await page.goto(`${served.origin}/time`);
  const mark = page.getByRole('dialog').filter({ hasText: /Step \d of 3/u });
  await mark.waitFor({ timeout: 15_000 });
  assert.equal(await page.locator('header.head .primary button[data-hbt-spotlit]').count(), 1, 'the mark does not point at the head\'s create act');
  const box = await mark.boundingBox();
  const fontSize = await mark.evaluate((element) => Number.parseFloat(getComputedStyle(element).fontSize));
  assert.ok(box && box.width >= fontSize * 20, `the mark is squeezed to ${box?.width}px`);
  assert.ok(box.x >= 0 && box.x + box.width <= 1280, 'the mark runs off the screen');
  const next = await mark.getByRole('button', { name: 'Next' }).boundingBox();
  const done = await mark.getByRole('button', { name: 'Got it' }).boundingBox();
  assert.ok(next && done && Math.abs(next.y - done.y) < 2, 'the mark\'s buttons are not on one row');
});
