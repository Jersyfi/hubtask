// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A hub's *Working hours* (UC-SVC-01 check 5): beside the workspace's calendars it lists the
// installation's offered ones, read with `include_offered`, and *Make a version here* keeps the
// hub's own version of one under its key - with no console error and no refused or failed answer.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { HUB } from './fixture.mjs';
import { productWalk } from './productwalk.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const hours = [{ day: 'MONDAY', from: '08:00', to: '16:00' }];
const OFFICE = { key: 'office', name: 'Office hours', zone: 'Europe/Berlin', hours, holidays: [], version: 1 };
const FEDERAL = { key: 'federal', name: 'Federal holidays', zone: 'Europe/Berlin', hours, holidays: ['2026-12-25'], version: 1, offered: true };

function calendarsBackend() {
  const own = [];
  const asked = [];
  const answer = (method, path, query, body) => {
    if (path === '/business-calendars' && method === 'GET') {
      asked.push(query.get('include_offered'));
      return { json: { data: query.get('include_offered') === 'true' ? [FEDERAL, OFFICE] : [OFFICE] } };
    }
    if (path === `/hubs/${HUB.id}/business-calendars` && method === 'GET') return { json: { data: own } };
    const one = new RegExp(`^/hubs/${HUB.id}/business-calendars/([a-z_]+)$`).exec(path);
    if (one && method === 'PUT') {
      const made = { key: one[1], hub_id: HUB.id, version: 1, holidays: [], ...body };
      own.push(made);
      return { json: made };
    }
    if (path === '/sla-policies' && method === 'GET') return { json: { data: [] } };
    if (path === '/approvals' && method === 'GET') return { json: { data: [], page: { next_cursor: null, has_more: false } } };
    return undefined;
  };
  return { answer, asked };
}

test('chromium: a hub keeps its own version of an installation calendar from its settings - without an error', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const backend = calendarsBackend();
  const walk = await productWalk(browser, { modules: ['SERVICE_DESK'], answer: backend.answer });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/hubs/${HUB.id}/settings/calendars`);
  const federal = page.locator('li').filter({ hasText: 'Federal holidays' });
  await federal.getByText('As the installation').waitFor({ timeout: 10_000 });
  await page.locator('li').filter({ hasText: 'Office hours' }).getByText('As the workspace').waitFor();
  assert.ok(backend.asked.includes('true'), `the list was asked as ${JSON.stringify(backend.asked)}`);

  await federal.getByRole('button', { name: 'Make a version here' }).click();
  await federal.getByRole('button', { name: 'Keep this version' }).click();
  await federal.getByText('This hub\'s own').waitFor({ timeout: 5_000 });
  const kept = walk.written.find((each) => each.method === 'PUT');
  assert.equal(kept?.path, `/hubs/${HUB.id}/business-calendars/federal`);
  assert.equal(kept?.body.name, 'Federal holidays');
  assert.deepEqual(kept?.body.holidays, ['2026-12-25']);
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});
