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
function timeBackend({ running, kinds } = {}) {
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
    if (path === '/timer' && method === 'GET') return { json: timer };
    if (path === '/time-capacities' && method === 'GET') return { json: { data: [] } };
    if (path === '/cost-rates' && method === 'GET') return { json: { data: [] } };
    if (path === '/time-absences' && method === 'GET') return { json: { data: [] } };
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
    const one = /^\/timesheets\/(\d{4}-\d{2}-\d{2})(:submit|:approve)?$/.exec(path);
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
  await page.getByRole('link', { name: 'Weeks to decide' }).click();
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
  const walk = await productWalk(browser, { modules: ['TIME'], answer: timeBackend(), width: 375, height: 812 });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/time`);
  await page.getByText('No time recorded this week.').waitFor({ timeout: 10_000 });
  assert.equal(await page.getByRole('button', { name: 'Start timer' }).count(), 1, 'the timer starts from the head');
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
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
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

  await page.goto(`${served.origin}/time?tab=settings`);
  await page.getByRole('rowheader', { name: 'Development' }).waitFor({ timeout: 10_000 });
  assert.equal(await page.getByRole('columnheader', { name: 'Key' }).count(), 0, 'a key column is drawn');
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});
