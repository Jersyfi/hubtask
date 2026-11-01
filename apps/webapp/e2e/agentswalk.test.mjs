// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Governed agents clicked through as an administrator does it (UC-AGT-01, UC-AGT-02, UC-AGT-03): a
// model registered, an agent registered on it as a draft, a colleague's draft approved and put
// live, and an act it was held on approved - with no console error and no refused or failed answer
// on the way.

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

const now = () => new Date().toISOString();

/** The agents' routes, kept in memory as the server would keep them. */
function agentsBackend() {
  const id = ids('00000a');
  const models = [];
  const agent = (fields) => ({
    id: id(), account_id: id(), kind: 'TICKET_TRIAGE', owner_id: ACCOUNT.id, risk_class: 'MINIMAL', scopes: [], model_id: null,
    policy: { reversible: 'APPROVAL', irreversible: 'APPROVAL' }, budget: { tokens_per_day: 0, actions_per_hour: 0, concurrent_runs: 0 },
    min_accuracy: 0, status: 'DRAFT', needs_approval: false, approved_by: null, approved_at: null, suspended_by: null, suspended_at: null,
    created_at: now(), version: 1, ...fields,
  });
  const triage = agent({ name: 'Triage helper', purpose: 'Sorts new requests into their queue.', owner_id: OTHER.id });
  const all = [triage];
  const requests = [{
    id: id(), agent_id: triage.id, use_case: 'items.update', class: 'REVERSIBLE', input: { item_id: ITEMS[0].id, title: 'Tiles ordered' },
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

test('chromium: agents - a model and an agent registered, a draft approved and put live, a held act approved - without an error', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const walk = await productWalk(browser, { modules: ['AGENTS'], answer: agentsBackend() });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/administration/agents`);
  await page.getByRole('tab', { name: 'Models' }).click();
  await page.getByRole('button', { name: 'Register a model' }).click();
  const modelDialog = page.getByRole('dialog');
  await modelDialog.getByLabel('Key').fill('local-small');
  await modelDialog.getByLabel('Name').fill('Local small');
  await modelDialog.getByLabel('Model', { exact: true }).fill('small-7b');
  await modelDialog.getByRole('button', { name: 'Save' }).click();
  await modelDialog.waitFor({ state: 'detached', timeout: 5_000 });
  await page.getByRole('cell', { name: /^Local small local-small/ }).waitFor({ timeout: 5_000 });

  await page.getByRole('tab', { name: 'Agents' }).click();
  await page.getByRole('button', { name: 'Register an agent' }).click();
  const agentDialog = page.getByRole('dialog');
  await agentDialog.getByLabel('Name').fill('Draft writer');
  await agentDialog.getByLabel('What it is for').fill('Drafts knowledge articles from solved requests.');
  await agentDialog.getByLabel('Model', { exact: true }).selectOption({ label: 'Local small' });
  await agentDialog.getByRole('button', { name: 'Register an agent' }).click();
  await agentDialog.waitFor({ state: 'detached', timeout: 5_000 });
  await page.getByRole('heading', { name: 'Draft writer' }).waitFor({ timeout: 5_000 });

  const triage = page.locator('li').filter({ has: page.getByRole('heading', { name: 'Triage helper' }) });
  await triage.getByRole('button', { name: 'Approve' }).click();
  await triage.getByRole('button', { name: 'Put live' }).click();
  await triage.getByText('Live', { exact: true }).waitFor({ timeout: 5_000 });

  await page.getByRole('tab', { name: 'Waiting for a person' }).click();
  await page.getByRole('button', { name: 'Approve and do it' }).click();
  // Decided, it leaves what waits: the list says nothing waits any more.
  await page.getByText('Nothing waits for a person.').waitFor({ timeout: 5_000 });

  const wrote = walk.written.map((each) => `${each.method} ${each.path}`);
  for (const want of ['POST /agent-models', 'POST /agents']) {
    assert.ok(wrote.includes(want), `${want} was not written: ${JSON.stringify(wrote)}`);
  }
  assert.ok(wrote.some((each) => /^POST \/agents\/[^/]+:approve$/.test(each)), `no approval: ${JSON.stringify(wrote)}`);
  assert.ok(wrote.some((each) => /^POST \/agents\/[^/]+:activate$/.test(each)), `not put live: ${JSON.stringify(wrote)}`);
  const decided = walk.written.find((each) => each.path.endsWith(':decide'));
  assert.deepEqual(decided?.body, { approve: true });
  const registered = walk.written.find((each) => each.method === 'POST' && each.path === '/agents').body;
  assert.equal(registered.name, 'Draft writer');
  assert.ok(registered.model_id, 'the agent was registered without its model');
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});
