// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *Waiting for my yes* as the one inbox (UC-SVC-09 check 1; user-experience.md §7.8): an agent's held
// act stands beside the approvals with its own presses and is approved there, and a desk with more
// approvals than a page shows the first fifty and the rest on *Show more*, asked by the page's cursor
// - with no console error and no refused or failed answer on the way.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, ITEMS, OTHER } from './fixture.mjs';
import { ids, productWalk } from './productwalk.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const HOUR = 3_600_000;

function inboxBackend() {
  const id = ids('00000e');
  const at = (offset) => new Date(Date.now() + offset).toISOString();
  const waiting = Array.from({ length: 52 }, (_, n) => ({
    id: id(), item_id: ITEMS[0].id, item_title: `Laptop order ${n + 1}`, item_number: n + 1, on_behalf_of: null, workflow: 'requests', step: 'approve',
    item_version: 1, state: 'PENDING', current_level: 1,
    levels: [{ rule: 'ANY', count: 1, approvers: [ACCOUNT.id], roles: [], needed: 1 }],
    decisions: [], requested_by: OTHER.id, requested_at: at((n - 60) * HOUR), decided_at: null, due_at: null, may_decide: true, version: 1,
  }));
  const triage = {
    id: id(), account_id: id(), name: 'Triage helper', purpose: 'Sorts new requests.', kind: 'TICKET_TRIAGE', owner_id: ACCOUNT.id, risk_class: 'MINIMAL',
    scopes: [], model_id: null, policy: { reversible: 'APPROVAL', irreversible: 'APPROVAL' }, budget: { tokens_per_day: 0, actions_per_hour: 0, concurrent_runs: 0 },
    min_accuracy: 0, status: 'ACTIVE', needs_approval: false, approved_by: null, approved_at: null, suspended_by: null, suspended_at: null, created_at: at(-HOUR), version: 1,
  };
  const requests = [{
    id: id(), agent_id: triage.id, use_case: 'AddComment', class: 'REVERSIBLE', input: { item_id: ITEMS[0].id, body: 'Belongs to the facilities queue.', internal: true },
    status: 'PENDING', decided_by: null, decided_at: null, code: '', created_at: at(-HOUR),
  }];
  return (method, path, query) => {
    if (path === '/approvals' && method === 'GET') {
      const from = query.get('cursor') ? waiting.findIndex((each) => each.id === query.get('cursor')) + 1 : 0;
      const shown = waiting.slice(from, from + 50);
      const more = from + 50 < waiting.length;
      return { json: { data: shown, page: { next_cursor: more ? shown[shown.length - 1].id : null, has_more: more } } };
    }
    if (path === '/agents' && method === 'GET') return { json: { data: [triage] } };
    if (path === '/agent-requests' && method === 'GET') return { json: { data: requests.filter((each) => each.status === query.get('status')) } };
    const decide = /^\/agent-requests\/([^/:]+):decide$/.exec(path);
    if (decide && method === 'POST') {
      const one = requests.find((each) => each.id === decide[1]);
      Object.assign(one, { status: 'APPROVED', decided_by: ACCOUNT.id, decided_at: at(0) });
      return { json: one };
    }
    return undefined;
  };
}

test('chromium: approvals - an agent\'s held act approved in the one inbox, and the approvals paged by cursor - without an error', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const walk = await productWalk(browser, { modules: ['SERVICE_DESK', 'AGENTS'], answer: inboxBackend() });
  t.after(walk.close);
  const { page } = walk;
  const asked = [];
  page.on('request', (request) => {
    const url = new URL(request.url());
    if (url.pathname.endsWith('/api/v1/approvals')) asked.push(url.searchParams.get('cursor'));
  });

  await page.goto(`${served.origin}/approvals`);
  const acts = page.locator('[data-kind="agent_acts"]');
  await acts.getByRole('heading', { name: 'Agents\' acts (1)' }).waitFor({ timeout: 5_000 });
  const held = acts.locator('[data-held-act="AddComment"]');
  await held.getByText('Triage helper').waitFor();
  await held.getByText('Add an internal comment to an entry').waitFor();
  await held.getByText('Belongs to the facilities queue.').waitFor();

  const entries = page.locator('[data-kind="approvals"]');
  assert.equal(await entries.locator('[data-approval]').count(), 50, 'the first page holds fifty');
  await entries.getByRole('button', { name: 'Show more - 50 shown' }).click();
  await entries.getByText('Laptop order 52').waitFor({ timeout: 5_000 });
  assert.equal(await entries.locator('[data-approval]').count(), 52);
  assert.equal(await entries.getByRole('button', { name: /Show more/ }).count(), 0, 'the last page offers no more');
  assert.ok(asked.length >= 2 && asked[0] === null && asked.at(-1) !== null, `the pages were asked as ${JSON.stringify(asked)}`);

  await held.getByRole('button', { name: 'Approve and do it' }).click();
  await acts.waitFor({ state: 'detached', timeout: 5_000 });
  const decided = walk.written.find((each) => each.path.endsWith(':decide'));
  assert.match(decided?.path ?? '', /^\/agent-requests\/[^/]+:decide$/);
  assert.deepEqual(decided?.body, { approve: true });
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});
