// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Goals clicked through from an empty workspace (UC-GOL-01, UC-GOL-02, UC-GOL-04): the first
// period, the first goal in it, its page, a check-in, its title changed, and the first dashboard -
// with no console error and no refused or failed answer on the way.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT } from './fixture.mjs';
import { ids, productWalk } from './productwalk.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

/** The goals module's routes, kept in memory as the server would keep them. */
function goalsBackend() {
  const id = ids('00000a');
  const periods = [];
  const goals = [];
  const checkIns = [];
  const dashboards = [];
  const page = (data) => ({ data, page: { next_cursor: null, has_more: false } });
  return (method, path, query, body) => {
    if (path === '/goal-periods' && method === 'GET') return { json: page(periods) };
    if (path === '/goal-periods' && method === 'POST') {
      const made = { id: id(), state: 'OPEN', version: 1, ...body };
      periods.push(made);
      return { status: 201, json: made };
    }
    if (path === '/goals' && method === 'GET') {
      const period = query.get('period_id');
      return { json: page(goals.filter((each) => !period || each.period_id === period)) };
    }
    if (path === '/goals' && method === 'POST') {
      const made = {
        id: id(), hub_id: null, parent_id: null, description: '', owner_id: ACCOUNT.id, status: 'NONE', confidence: null, grade: null,
        key_results: [], entries: { total: 0, done: 0 }, child_count: 0, progress: 0, version: 1, linked_entries: [], check_ins: [], ...body,
      };
      goals.push(made);
      return { status: 201, json: made };
    }
    const one = /^\/goals\/([^/]+)(\/check-ins)?$/.exec(path);
    const goal = one && goals.find((each) => each.id === one[1]);
    if (goal && !one[2] && method === 'GET') return { json: goal };
    if (goal && !one[2] && method === 'PUT') {
      Object.assign(goal, body, { version: goal.version + 1 });
      return { json: goal };
    }
    if (goal && one[2] && method === 'GET') return { json: page(checkIns.filter((each) => each.goal_id === goal.id)) };
    if (goal && one[2] && method === 'POST') {
      const made = { id: id(), goal_id: goal.id, author_id: ACCOUNT.id, created_at: '2026-11-01T09:00:00Z', ...body };
      checkIns.unshift(made);
      Object.assign(goal, { status: body.status, confidence: body.confidence, check_ins: [made, ...goal.check_ins], version: goal.version + 1 });
      return { status: 201, json: goal };
    }
    if (path === '/portfolios' && method === 'GET') return { json: page([]) };
    if (path.startsWith('/dashboards') && !path.slice('/dashboards'.length).startsWith('/') && method === 'GET') return { json: page(dashboards) };
    if (path === '/dashboards' && method === 'POST') {
      const made = { id: id(), hub_id: null, owner_id: ACCOUNT.id, sharing: 'PRIVATE', group_ids: [], widgets: [], version: 1, ...body };
      dashboards.push(made);
      return { status: 201, json: made };
    }
    const board = /^\/dashboards\/([^/]+)(\/figures|\/digest)?$/.exec(path);
    const dashboard = board && dashboards.find((each) => each.id === board[1]);
    if (dashboard && !board[2] && method === 'GET') return { json: dashboard };
    if (dashboard && board[2] === '/figures') return { json: { dashboard_id: dashboard.id, computed_at: '2026-11-01T09:00:00Z', widgets: [] } };
    if (dashboard && board[2] === '/digest' && method === 'GET') {
      return { json: { dashboard_id: dashboard.id, cadence: 'NONE', weekday: null, hour: null, time_zone: 'UTC', next_at: null, last_sent_at: null } };
    }
    return undefined;
  };
}

test('chromium: goals from nothing - a period, a goal, a check-in, a change, a dashboard - without an error', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const walk = await productWalk(browser, { modules: ['GOALS'], answer: goalsBackend() });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/goals`);
  await page.getByRole('button', { name: 'New period' }).first().click();
  const periodDialog = page.getByRole('dialog', { name: 'New period' });
  await periodDialog.getByLabel('Name').fill('Q4 2026');
  await periodDialog.getByLabel('First day').fill('2026-10-01');
  await periodDialog.getByLabel('Last day').fill('2026-12-31');
  await periodDialog.getByRole('button', { name: 'Create' }).click();
  await periodDialog.waitFor({ state: 'detached', timeout: 5_000 });

  await page.getByRole('button', { name: 'New goal' }).first().click();
  const goalDialog = page.getByRole('dialog', { name: 'New goal' });
  await goalDialog.getByLabel('Title').fill('Ship the suite');
  await goalDialog.getByRole('button', { name: 'Set goal' }).click();
  await goalDialog.waitFor({ state: 'detached', timeout: 5_000 });
  await page.waitForURL(/\/goals\/[0-9a-f-]+$/, { timeout: 5_000 });
  await page.getByRole('heading', { name: 'Ship the suite' }).first().waitFor({ timeout: 5_000 });

  await page.getByRole('button', { name: 'Check in' }).first().click();
  const checkDialog = page.getByRole('dialog', { name: /Check in/ });
  await checkDialog.getByLabel('Confidence').fill('8');
  await checkDialog.getByLabel('Note').fill('First release out.');
  await checkDialog.getByRole('button', { name: 'Check in' }).click();
  await checkDialog.waitFor({ state: 'detached', timeout: 5_000 });

  await page.getByRole('button', { name: 'Change' }).first().click();
  const changeDialog = page.getByRole('dialog', { name: 'Change' });
  await changeDialog.getByLabel('Title').fill('Ship the suite to everyone');
  await changeDialog.getByRole('button', { name: 'Save' }).click();
  await changeDialog.waitFor({ state: 'detached', timeout: 5_000 });
  await page.getByRole('heading', { name: 'Ship the suite to everyone' }).first().waitFor({ timeout: 5_000 });

  await page.goto(`${served.origin}/goals?tab=dashboards`);
  await page.getByRole('button', { name: 'New dashboard' }).first().click();
  const boardDialog = page.getByRole('dialog', { name: 'New dashboard' });
  await boardDialog.getByLabel('Name').fill('Leadership');
  await boardDialog.getByRole('button', { name: 'Create' }).click();
  await boardDialog.waitFor({ state: 'detached', timeout: 5_000 });
  await page.waitForURL(/\/dashboards\/[0-9a-f-]+$/, { timeout: 5_000 });
  await page.getByRole('heading', { name: 'Leadership' }).first().waitFor({ timeout: 5_000 });

  const wrote = walk.written.map((each) => `${each.method} ${each.path}`);
  for (const want of [/^POST \/goal-periods$/, /^POST \/goals$/, /^POST \/goals\/[^/]+\/check-ins$/, /^PUT \/goals\/[^/]+$/, /^POST \/dashboards$/]) {
    assert.ok(wrote.some((each) => want.test(each)), `nothing matched ${want}: ${JSON.stringify(wrote)}`);
  }
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});
