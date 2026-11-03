// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The service desk clicked through as its administrator and a handler do it (UC-SVC-01, UC-SVC-02,
// UC-SVC-03, UC-SVC-09): a calendar and an SLA policy on it defined, a request read with its
// promised times, who raised it and its approval, and in *Waiting for my yes* one approval approved
// and another declined - not before a reason is given - with no console error and no refused or
// failed answer on the way.

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

const [REQUEST, OTHER_REQUEST] = ITEMS;
const HOUR = 3_600_000;

/** The desk's routes, kept in memory as the server would keep them. */
function deskBackend() {
  const id = ids('00000d');
  const calendars = [];
  const policies = [];
  const at = (offset) => new Date(Date.now() + offset).toISOString();
  const goal = (kind, standing, due) => ({
    goal: kind, kind: 'SLA', policy_key: 'standard', assignee_group: null, standing, target_seconds: 28_800,
    armed_at: at(-6 * HOUR), due_at: due, at_risk_at: standing === 'RUNNING' ? at(-HOUR) : null, met_at: standing === 'MET' ? at(-5 * HOUR) : null, breached_at: null,
  });
  const approval = (item, number) => ({
    id: id(), item_id: item.id, item_title: item.title, item_number: number, on_behalf_of: null, workflow: 'requests', step: 'approve',
    item_version: 1, state: 'PENDING', current_level: 1,
    levels: [{ rule: 'ANY', count: 1, approvers: [ACCOUNT.id], roles: [], needed: 1 }],
    decisions: [], requested_by: OTHER.id, requested_at: at(-3 * HOUR), decided_at: null, due_at: at(21 * HOUR), may_decide: true, version: 1,
  });
  const waiting = [approval(REQUEST, 1), approval(OTHER_REQUEST, 2)];
  return (method, path, query, body) => {
    if (path === '/business-calendars' && method === 'GET') return { json: { data: calendars } };
    const calendar = /^\/business-calendars\/([a-z][a-z0-9_]*)$/.exec(path);
    if (calendar && method === 'PUT') {
      const made = { key: calendar[1], name: body.name, zone: body.zone, hours: body.hours, holidays: body.holidays ?? [], version: 1 };
      calendars.push(made);
      return { json: made };
    }
    if (path === '/sla-policies' && method === 'GET') return { json: { data: policies } };
    const policy = /^\/sla-policies\/([a-z][a-z0-9_]*)$/.exec(path);
    if (policy && method === 'PUT') {
      const made = {
        key: policy[1], name: body.name, calendar: body.calendar ?? null, entry_types: body.entry_types, organisations: body.organisations ?? [],
        priority_field: body.priority_field ?? null, assignee_group: null, goals: body.goals, position: body.position ?? 0, enabled: body.enabled !== false, version: 1,
      };
      policies.push(made);
      return { json: made };
    }
    if (path === '/reply-templates' && method === 'GET') return { json: { data: [] } };
    if (path === '/requester-organisations' && method === 'GET') return { json: { data: [] } };
    if (path === `/items/${REQUEST.id}/sla` && method === 'GET') {
      return { json: { data: [goal('FIRST_RESPONSE', 'MET', null), goal('RESOLUTION', 'RUNNING', at(2 * HOUR))] } };
    }
    if (path === `/items/${REQUEST.id}/requester` && method === 'GET') {
      return { json: { account_id: OTHER.id, organisation: { key: 'acme', name: 'Acme Plumbing' }, assets: [] } };
    }
    if (/^\/items\/[^/]+\/sla$/.test(path) && method === 'GET') return { json: { data: [] } };
    const entryApprovals = /^\/items\/([^/]+)\/approvals$/.exec(path);
    if (entryApprovals && method === 'GET') {
      return { json: { data: waiting.filter((each) => each.item_id === entryApprovals[1]).map(({ item_title, item_number, ...rest }) => rest) } };
    }
    if (path === '/approvals' && method === 'GET') return { json: { data: waiting.filter((each) => each.state === 'PENDING') } };
    const decide = /^\/approvals\/([^/:]+):decide$/.exec(path);
    if (decide && method === 'POST') {
      const one = waiting.find((each) => each.id === decide[1]);
      if (!one) return { status: 404, json: { type: 'about:blank', title: 'approvals.not_found', status: 404, code: 'approvals.not_found' } };
      if (body.verdict === 'DECLINE' && !body.reason) return { status: 422, json: { type: 'about:blank', title: 'approvals.reason_required', status: 422, code: 'approvals.reason_required' } };
      Object.assign(one, {
        state: body.verdict === 'APPROVE' ? 'APPROVED' : 'DECLINED', decided_at: new Date().toISOString(), may_decide: false, version: one.version + 1,
        decisions: [{ level: 1, approver_id: ACCOUNT.id, on_behalf_of: null, verdict: body.verdict, reason: body.reason ?? '', item_version: 1, decided_at: new Date().toISOString() }],
      });
      return { json: one };
    }
    return undefined;
  };
}

test('chromium: service desk - a calendar and a policy defined, a request\'s promised times read, approvals decided - without an error', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const walk = await productWalk(browser, { modules: ['SERVICE_DESK'], answer: deskBackend() });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/administration/service-desk`);
  const calendarForm = page.locator('form').filter({ has: page.getByRole('button', { name: 'Add the calendar' }) });
  await calendarForm.getByLabel('Name').fill('Office hours');
  // A zone is picked from the platform's list, never typed (user-experience.md §5.4).
  await calendarForm.getByLabel('Time zone').selectOption('Europe/Berlin');
  await calendarForm.getByRole('button', { name: 'Add the calendar' }).click();
  await page.getByText('Office hours').first().waitFor({ timeout: 5_000 });

  await page.getByRole('tab', { name: 'SLA policies' }).click();
  const policyForm = page.locator('form').filter({ has: page.getByRole('button', { name: 'Add the policy' }) });
  await policyForm.getByLabel('Name').fill('Standard');
  await policyForm.getByLabel('Entry types').fill('request');
  await policyForm.getByLabel('Calendar').selectOption({ label: 'Office hours' });
  await policyForm.getByRole('button', { name: 'Add the policy' }).click();
  await page.getByText('Standard').first().waitFor({ timeout: 5_000 });

  await page.goto(`${served.origin}/items/${REQUEST.id}`);
  const promised = page.getByRole('list', { name: 'Promised times' });
  await promised.waitFor({ timeout: 5_000 });
  assert.match(await promised.innerText(), /First reply/);
  const requester = page.getByRole('region', { name: 'Requester' });
  await requester.getByText('Acme Plumbing').waitFor({ timeout: 5_000 });
  assert.match(await requester.innerText(), new RegExp(OTHER.display_name));
  const beside = page.locator('[data-approval]');
  await beside.waitFor({ timeout: 5_000 });
  assert.match(await beside.innerText(), /Waiting[\s\S]*Mara Lind asked on [\s\S]*Level 1/);

  await page.goto(`${served.origin}/approvals`);
  const first = page.locator('article').filter({ hasText: REQUEST.title });
  await first.getByRole('button', { name: 'Approve' }).click();
  // Decided, it leaves the list: only what still waits for the reader's yes is listed.
  await first.waitFor({ state: 'detached', timeout: 5_000 });

  const second = page.locator('article').filter({ hasText: OTHER_REQUEST.title });
  await second.getByRole('button', { name: 'Decline' }).click();
  await second.getByRole('alert').filter({ hasText: 'Say why you decline first.' }).waitFor({ timeout: 5_000 });
  await second.getByLabel('Reason').fill('Not in this quarter\'s budget.');
  await second.getByRole('button', { name: 'Decline' }).click();
  await second.waitFor({ state: 'detached', timeout: 5_000 });
  await page.getByText('Nothing waits for your yes').waitFor({ timeout: 5_000 });

  const wrote = walk.written.map((each) => `${each.method} ${each.path}`);
  for (const want of ['PUT /business-calendars/office_hours', 'PUT /sla-policies/standard']) {
    assert.ok(wrote.includes(want), `${want} was not written: ${JSON.stringify(wrote)}`);
  }
  const decisions = walk.written.filter((each) => each.path.endsWith(':decide')).map((each) => each.body);
  assert.deepEqual(decisions, [{ verdict: 'APPROVE' }, { verdict: 'DECLINE', reason: 'Not in this quarter\'s budget.' }]);
  const policy = walk.written.find((each) => each.path === '/sla-policies/standard').body;
  assert.equal(policy.calendar, 'office_hours');
  assert.deepEqual(policy.entry_types, ['request']);
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});
