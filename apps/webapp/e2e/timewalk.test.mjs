// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Time clicked through as a person and their lead do it (UC-TIM-01, UC-TIM-02, UC-TIM-03): a timer
// started and stopped on an entry, the week with that record handed in, and a colleague's week
// approved - with no console error and no refused or failed answer on the way.

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
function timeBackend() {
  const id = ids('00000b');
  let timer = { running: false };
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
    if (path === '/time-work-types' && method === 'GET') {
      return { json: { data: [{ key: 'work', name: 'Work', billable: true, archived: false, position: 0, version: 1 }] } };
    }
    if (path === '/time-policy' && method === 'GET') return { json: { locked_through: null, visibility: 'FULL', version: 1 } };
    if (path === '/timer' && method === 'GET') return { json: timer };
    if (path === '/timer:start' && method === 'POST') {
      timer = { running: true, entry_id: body.entry_id ?? null, work_type: body.work_type ?? 'work', note: '', billable: true, started_at: new Date(Date.now() - 25 * 60_000).toISOString(), elapsed_minutes: 25 };
      return { json: timer };
    }
    if (path === '/timer:stop' && method === 'POST') {
      const made = {
        id: id(), account_id: ACCOUNT.id, entry_id: timer.entry_id, work_type: timer.work_type, day: today, started_at: timer.started_at, minutes: 25,
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

  await page.getByRole('tab', { name: 'To approve' }).click();
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
