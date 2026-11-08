// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Governed agents clicked through at their home as an administrator does it (UC-AGT-01, UC-AGT-02,
// UC-AGT-03, UC-AGT-07; user-experience.md §7.8): a model registered under Settings, an agent added
// from a shipped role on it as a draft with the default budget, a colleague's draft approved and put
// live, its card's held count opening the act it waits on, the act approved, and its run read in
// words - with no console error and no refused or failed answer on the way. An empty workspace
// offers the roles.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, ITEMS, OTHER } from './fixture.mjs';
import { ids, productWalk } from './productwalk.mjs';
import { chooseInRow } from './rowmenu.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const now = () => new Date().toISOString();

/** The agents' routes, kept in memory as the server would keep them. */
function agentsBackend({ empty = false } = {}) {
  const id = ids('00000a');
  const models = [];
  const agent = (fields) => ({
    id: id(), account_id: id(), kind: 'TICKET_TRIAGE', owner_id: ACCOUNT.id, risk_class: 'MINIMAL', scopes: [], model_id: null,
    policy: { reversible: 'APPROVAL', irreversible: 'APPROVAL' }, budget: { tokens_per_day: 0, actions_per_hour: 0, concurrent_runs: 0 },
    min_accuracy: 0, status: 'DRAFT', needs_approval: false, approved_by: null, approved_at: null, suspended_by: null, suspended_at: null,
    created_at: now(), version: 1, ...fields,
  });
  const triage = agent({ name: 'Triage helper', purpose: 'Sorts new requests into their queue.', owner_id: OTHER.id });
  const all = empty ? [] : [triage];
  const requests = empty ? [] : [{
    id: id(), agent_id: triage.id, use_case: 'AddComment', class: 'REVERSIBLE', input: { item_id: ITEMS[0].id, body: 'Belongs to the facilities queue.', internal: true },
    status: 'PENDING', decided_by: null, decided_at: null, code: '', created_at: now(),
  }];
  return (method, path, query, body) => {
    if (path === '/agents' && method === 'GET') return { json: { data: all } };
    if (path === '/agents' && method === 'POST') {
      const made = agent({
        name: body.name, purpose: body.purpose, kind: body.kind, owner_id: body.owner_id ?? ACCOUNT.id, risk_class: body.risk_class,
        scopes: body.scopes, model_id: body.model_id ?? null, policy: body.policy, budget: body.budget, min_accuracy: body.min_accuracy,
      });
      all.push(made);
      return { status: 201, json: made };
    }
    const press = /^\/agents\/([^/:]+):(approve|activate)$/.exec(path);
    if (press && method === 'POST') {
      const one = all.find((each) => each.id === press[1]);
      if (press[2] === 'approve') Object.assign(one, { status: 'APPROVED', approved_by: ACCOUNT.id, approved_at: now() });
      else Object.assign(one, { status: 'ACTIVE' });
      one.version += 1;
      return { json: one };
    }
    const jobs = /^\/agents\/([^/:]+)\/jobs$/.exec(path);
    if (jobs && method === 'GET') {
      return { json: { data: jobs[1] === triage.id ? [{
        id: id(), agent_id: triage.id, target_type: 'ENTRY', target_id: ITEMS[0].id, status: 'DONE', code: '', proposals: 2, tokens: 1840,
        started_by: ACCOUNT.id, started_at: now(), ended_at: now(),
      }] : [] } };
    }
    if (path === '/agents:halt' && method === 'GET') return { json: { halted: false, set_by: null, set_at: null } };
    if (path === '/agents:policy' && method === 'GET') return { json: { 'agents.registered_only': { value: false, source: 'DEFAULT' } } };
    if (path === '/agents:metering' && method === 'GET') return { json: { data: [] } };
    if (path === '/agent-models' && method === 'GET') return { json: { data: models } };
    if (path === '/agent-models' && method === 'POST') {
      const made = {
        id: id(), key: body.key, name: body.name, source: body.source, offered_id: null, model: body.model, region: body.region,
        retention_days: body.retention_days ?? null, consent_by: ACCOUNT.id, consent_at: now(), revision: 1, retired: false, version: 1,
      };
      models.push(made);
      return { status: 201, json: made };
    }
    if (path === '/agent-requests' && method === 'GET') {
      return { json: { data: requests.filter((each) => each.status === query.get('status') && (!query.get('agent_id') || each.agent_id === query.get('agent_id'))) } };
    }
    const decide = /^\/agent-requests\/([^/:]+):decide$/.exec(path);
    if (decide && method === 'POST') {
      const one = requests.find((each) => each.id === decide[1]);
      Object.assign(one, { status: body.approve ? 'APPROVED' : 'DECLINED', decided_by: ACCOUNT.id, decided_at: now() });
      return { json: one };
    }
    return undefined;
  };
}

test('chromium: agents - a model registered, an agent added from a role, a draft put live, its held act approved from its card, its run read - without an error', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const walk = await productWalk(browser, { modules: ['AGENTS'], answer: agentsBackend(), manifest: { token_scopes: ['items:read', 'items:write', 'admin:tenants'] } });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/agents?tab=settings&section=models`);
  await page.getByRole('button', { name: 'Register a model' }).click();
  const modelDialog = page.getByRole('dialog');
  await modelDialog.getByLabel('Key').fill('local-small');
  await modelDialog.getByLabel('Name').fill('Local small');
  await modelDialog.getByLabel('Model', { exact: true }).fill('small-7b');
  await modelDialog.getByRole('button', { name: 'Save' }).click();
  await modelDialog.waitFor({ state: 'detached', timeout: 5_000 });
  await page.getByRole('main').getByText('Local small', { exact: true }).waitFor({ timeout: 5_000 });

  await page.getByRole('link', { name: 'Agents', exact: true }).click();
  await page.waitForURL(/\/agents$/);
  await page.getByText('0 live · 1 draft · 1 waiting for a person').waitFor({ timeout: 5_000 });
  await page.getByRole('button', { name: 'Start from a role' }).click();
  await page.getByRole('menuitem', { name: 'Knowledge draft' }).click();
  const agentDialog = page.getByRole('dialog');
  assert.equal(await agentDialog.getByLabel('Name').inputValue(), 'Knowledge draft');
  assert.equal(await agentDialog.getByLabel('Tokens a day').inputValue(), '200000', 'a new agent starts from the default budget');
  await agentDialog.getByLabel('Model', { exact: true }).selectOption({ label: 'Local small' });
  // What it may do is ticked in words (user-experience.md §5.4); the operator's scope is not offered.
  await page.getByRole('dialog', { name: 'Add an agent' }).waitFor({ timeout: 5_000 });
  await agentDialog.getByRole('checkbox', { name: /^Read your entries/ }).check();
  await agentDialog.getByRole('checkbox', { name: /^Create and change entries/ }).check();
  assert.equal(await agentDialog.getByRole('checkbox', { name: /Administer every workspace/ }).count(), 0);
  assert.equal(await agentDialog.getByLabel('Scopes', { exact: true }).count(), 0, 'the typed list is folded under Advanced');
  await agentDialog.getByRole('button', { name: 'Add agent' }).click();
  await agentDialog.waitFor({ state: 'detached', timeout: 5_000 });
  await page.locator('[data-agent-row]').filter({ hasText: 'Knowledge draft' }).waitFor({ timeout: 5_000 });

  const triage = page.getByRole('row').filter({ hasText: 'Triage helper' });
  await chooseInRow(triage, 'Approve');
  await chooseInRow(triage, 'Put live');
  await triage.getByText('Live', { exact: true }).waitFor({ timeout: 5_000 });

  await triage.getByRole('button', { name: '1 waiting' }).click();
  await page.waitForURL(/tab=requests&agent=/);
  await page.getByText('Waiting on Triage helper').waitFor({ timeout: 5_000 });
  // The shipped role's act in words (user-experience.md §7.8): what it would do, what it wrote,
  // where it would land - not the use case's name and the input's keys.
  const held = page.locator('[data-held-act="AddComment"]');
  await held.getByText('Add an internal comment to an entry').waitFor({ timeout: 5_000 });
  await held.getByText('Belongs to the facilities queue.').waitFor();
  assert.equal(await held.getByRole('link', { name: 'Open the entry' }).getAttribute('href'), `/items/${ITEMS[0].id}`);
  assert.equal(await held.locator('code').count(), 0, 'the act is still said by its name');
  await page.getByRole('button', { name: 'Approve and do it' }).click();
  // Decided, it leaves what waits: the list says nothing waits any more.
  await page.getByText('Nothing waits for a person.').waitFor({ timeout: 5_000 });

  await page.getByRole('tab', { name: /^Runs/ }).click();
  await page.getByText('Triage helper on an entry').waitFor({ timeout: 5_000 });
  await page.getByText(/2 proposals · 1,840 tokens/).waitFor({ timeout: 5_000 });

  const wrote = walk.written.map((each) => `${each.method} ${each.path}`);
  assert.deepEqual(walk.written.find((each) => each.method === 'POST' && each.path === '/agents')?.body.scopes, ['items:read', 'items:write']);
  for (const want of ['POST /agent-models', 'POST /agents']) {
    assert.ok(wrote.includes(want), `${want} was not written: ${JSON.stringify(wrote)}`);
  }
  assert.ok(wrote.some((each) => /^POST \/agents\/[^/]+:approve$/.test(each)), `no approval: ${JSON.stringify(wrote)}`);
  assert.ok(wrote.some((each) => /^POST \/agents\/[^/]+:activate$/.test(each)), `not put live: ${JSON.stringify(wrote)}`);
  const decided = walk.written.find((each) => each.path.endsWith(':decide'));
  assert.deepEqual(decided?.body, { approve: true });
  const registered = walk.written.find((each) => each.method === 'POST' && each.path === '/agents').body;
  assert.equal(registered.kind, 'KNOWLEDGE_DRAFT');
  assert.deepEqual(registered.budget, { tokens_per_day: 200000, actions_per_hour: 120, concurrent_runs: 2 });
  assert.ok(registered.model_id, 'the agent was registered without its model');
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});

test('chromium: agents - an empty workspace offers the shipped roles, on a phone too', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const walk = await productWalk(browser, { modules: ['AGENTS'], answer: agentsBackend({ empty: true }), width: 375, height: 812 });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/agents`);
  await page.getByText('No agent works here yet').waitFor({ timeout: 5_000 });
  await page.getByRole('button', { name: 'Ticket triage' }).click();
  const dialog = page.getByRole('dialog');
  assert.equal(await dialog.getByLabel('Name').inputValue(), 'Ticket triage');
  await dialog.getByRole('button', { name: 'Cancel' }).last().click();
  await page.getByRole('tab', { name: /^Runs/ }).click();
  await page.getByText('No agent has run a job yet').waitFor({ timeout: 5_000 });
  const scrolls = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
  assert.equal(scrolls, false, 'the page scrolls sideways on a phone');
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});
