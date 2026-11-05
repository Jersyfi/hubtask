// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The service desk clicked through as its administrator and a handler do it (UC-SVC-01, UC-SVC-02,
// UC-SVC-03, UC-SVC-09): a calendar and an SLA policy on it defined, a request read with its
// promised times, who raised it and its approval, and in *Waiting for your yes* one approval approved
// and another declined - not before a reason is given - with no console error and no refused or
// failed answer on the way.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, COLLECTION, DEFAULT_STRUCTURE, HUB, ITEMS, OTHER, PAGE, WORKSPACE } from './fixture.mjs';
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
  const answer = (method, path, query, body) => {
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
    // The pickers' values (user-experience.md §5.4): the workspace's types, choice fields and statuses.
    // A desk: its collection offers the request type, which the portal takes (UC-SVC-05).
    if (path === '/entry-types' && method === 'GET') {
      return { json: { data: [{ key: 'request', level: 'TASK', name: 'Request', system: false, position: 1, archived: false, form: { channels: [{ channel: 'PORTAL' }] } }] } };
    }
    const desk = { ...COLLECTION, structure: { ...COLLECTION.structure, settings: { ...COLLECTION.structure.settings, entry_types: { TASK: ['request'] } } } };
    if (path === `/containers/${COLLECTION.id}` && method === 'GET') return { json: desk };
    if (path === '/containers' && method === 'GET' && query.get('parent_id') === HUB.id) return { json: { ...PAGE, data: [desk] } };
    if (path === '/custom-fields' && method === 'GET') {
      return { json: [{ id: id(), collection_id: null, hub_id: null, key: 'priority', name: 'Priority', kind: 'SELECT', options: ['P1', 'P2'], is_required: false, restricted: false, read_restricted: false, applies_to: ['TASK'] }] };
    }
    if (path === '/requester-organisations' && method === 'GET') return { json: { data: [] } };
    if (path === `/items/${REQUEST.id}/sla` && method === 'GET') {
      return { json: { data: [goal('FIRST_RESPONSE', 'MET', null), goal('RESOLUTION', 'RUNNING', at(2 * HOUR))] } };
    }
    // The request was raised by the requester, so the panel asks for their other open ones.
    if (path === `/items/${REQUEST.id}` && method === 'GET') return { json: { ...REQUEST, created_by: OTHER.id } };
    // The requester's other open requests: a search by creator (UC-SVC-03 check 7).
    if (path === '/search' && method === 'POST' && JSON.stringify(body?.filter ?? {}).includes('created_by')) {
      return { json: { ...PAGE, data: [REQUEST, OTHER_REQUEST] } };
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
  answer.policies = policies;
  return answer;
}

test('chromium: service desk - a calendar and a policy defined, a request\'s promised times read, approvals decided - without an error', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const backend = deskBackend();
  const walk = await productWalk(browser, { modules: ['SERVICE_DESK', 'WORKFLOWS'], answer: backend });
  t.after(walk.close);
  const { page } = walk;

  // The desk is set up at its home's last tab, each form in a dialog opened by its *Add* (§7.4).
  await page.goto(`${served.origin}/desk/settings`);
  // One settings anatomy (§5.4): the shell's sections at the start, not a second row of tabs, and
  // the head keeps the home's create act.
  await page.locator('[data-settings-page] [data-settings-sections]').waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('[data-settings-page] [role="tablist"]').count(), 0, 'the desk settings draw a second row of tabs');
  await page.getByRole('button', { name: 'Add the calendar' }).click();
  const calendarForm = page.getByRole('dialog').locator('form');
  await calendarForm.getByLabel('Name').fill('Office hours');
  // A zone is picked from the platform's list, never typed (user-experience.md §5.4).
  await calendarForm.getByLabel('Time zone').selectOption('Europe/Berlin');
  await calendarForm.getByRole('button', { name: 'Add the calendar' }).click();
  await page.getByText('Office hours').first().waitFor({ timeout: 5_000 });

  await page.locator('[data-settings-sections]').getByRole('link', { name: 'SLA policies' }).click();
  await page.getByRole('button', { name: 'Add the policy' }).click();
  const policyForm = page.getByRole('dialog').locator('form');
  await policyForm.getByLabel('Name').fill('Standard');
  // Types, the priority field and its targets are picked, never typed as keys (§5.4).
  await policyForm.getByRole('checkbox', { name: 'Request' }).check();
  await policyForm.getByLabel('Priority field').selectOption('priority');
  await policyForm.getByLabel('First response at P1').fill('15');
  await policyForm.getByLabel('Calendar').selectOption({ label: 'Office hours' });
  // A goal is written in hours and held in minutes (user-experience.md §5.4, DurationInput).
  const firstResponse = policyForm.locator('.duration', { has: page.getByLabel('First response', { exact: true }) });
  await firstResponse.getByLabel('First response', { exact: true }).fill('4');
  await firstResponse.getByLabel('Unit').selectOption('HOURS');
  await policyForm.getByRole('button', { name: 'Add the policy' }).click();
  await page.getByText('Standard').first().waitFor({ timeout: 5_000 });
  assert.equal(backend.policies[0]?.goals.find((goal) => goal.kind === 'FIRST_RESPONSE')?.minutes, 240);

  await page.goto(`${served.origin}/items/${REQUEST.id}`);
  const promised = page.getByRole('list', { name: 'Promised times' });
  await promised.waitFor({ timeout: 5_000 });
  assert.match(await promised.innerText(), /First reply/);
  // §5.6: a desk entry's promised times sit under the head, above the notes.
  assert.equal(await promised.evaluate((node) => {
    const notes = document.querySelector('[aria-label="Notes"]');
    return notes !== null && Boolean(node.compareDocumentPosition(notes) & Node.DOCUMENT_POSITION_FOLLOWING);
  }), true, 'the promised times stand above the notes');
  const requester = page.getByRole('region', { name: 'Requester' });
  await requester.getByText('Acme Plumbing').waitFor({ timeout: 5_000 });
  assert.match(await requester.innerText(), new RegExp(OTHER.display_name));
  // §5.10: the other requests are the design system's rows, not the browser's blue underline.
  const other = requester.getByRole('link', { name: OTHER_REQUEST.title });
  await other.waitFor({ timeout: 5_000 });
  assert.equal(await other.getAttribute('href'), `/items/${OTHER_REQUEST.id}`);
  assert.equal(await requester.getByRole('link', { name: REQUEST.title }).count(), 0, 'the entry itself is listed among the others');
  const drawn = await other.evaluate((node) => ({ line: getComputedStyle(node).textDecorationLine, color: getComputedStyle(node).color, row: node.closest('.row') !== null }));
  assert.equal(drawn.row, true, 'the other request is not a ListRow');
  assert.equal(drawn.line, 'none', 'the other request is underlined as a browser link');
  assert.notEqual(drawn.color, 'rgb(0, 0, 238)', 'the other request is the browser\'s blue');
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
  assert.equal(policy.priority_field, 'priority');
  assert.deepEqual(policy.goals.find((goal) => goal.kind === 'FIRST_RESPONSE')?.by_priority, { P1: 15 });
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});

// §7.4: an empty calendars section offers the ITIL defaults to whoever may shape the workspace,
// which open the ITIL template's plan at the templates - shown in words before anything changes.
// user-experience.md §5.12: on a phone a requester's other request with a long title wraps
// inside the panel rather than widening the page.
test('chromium: service desk - 390 px, a requester\'s long other request scrolls nothing sideways', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const desk = deskBackend();
  const long = { ...OTHER_REQUEST, title: 'Drucker im zweiten Obergeschoss druckt seit Montag nur noch leere Seiten und meldet einen Papierstau im Fach drei' };
  const answer = (method, path, query, body) => (path === '/search' && method === 'POST' && JSON.stringify(body?.filter ?? {}).includes('created_by')
    ? { json: { ...PAGE, data: [REQUEST, long] } }
    : desk(method, path, query, body));
  const walk = await productWalk(browser, { modules: ['SERVICE_DESK', 'WORKFLOWS'], answer, width: 390, height: 844 });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/items/${REQUEST.id}`);
  await page.getByRole('region', { name: 'Requester' }).getByRole('link', { name: long.title }).waitFor({ timeout: 10_000 });
  const wide = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(wide <= 0, `the page scrolls ${wide} px sideways`);
});

test('chromium: service desk - an empty section starts from the ITIL defaults through the template\'s plan', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const backend = deskBackend();
  const itil = {
    key: 'itil_service_desk', version: 6, name: 'itil_service_desk', origin: 'HUBTASK', provenance: 'itil_service_desk@6', created_at: '2026-09-01T00:00:00Z',
    update_available: false, taken_version: null, hub_document: {}, document: { modules: { service_desk: { switch: 'ON' } } }, questions: [],
  };
  const plans = [];
  const walk = await productWalk(browser, {
    modules: ['SERVICE_DESK', 'WORKFLOWS'],
    answer: (method, path, query, body) => {
      if (path === '/tenant' && method === 'GET') return { json: { ...WORKSPACE, structure: { ...DEFAULT_STRUCTURE, default_preset: null, default_preset_id: null } } };
      if (path === '/config/templates' && method === 'GET') return { json: { data: [itil] } };
      if (path === `/config/templates/${itil.key}:plan`) {
        plans.push(body?.hub_id ?? '');
        return { json: {
          template: itil.key, upgrade: [], document: {}, mode: 'MERGE', base_hash: 'b1', plan_hash: 'p1', refusals: [], operations: [], hub_id: null,
          flags: { hides_data: false, changes_permissions: false }, counts: { added: 1, changed: 0, removed: 0 },
          changes: [{ path: 'business_calendars.office', section: 'business_calendars', key: 'office', op: 'ADD', fields: [] }],
        } };
      }
      return backend(method, path, query, body);
    },
  });
  t.after(walk.close);
  const { page } = walk;
  await page.goto(`${served.origin}/desk/settings`);
  await page.locator('[data-itil-defaults]').click();
  await page.waitForURL(/\/administration\/presets\?template=itil_service_desk$/, { timeout: 10_000 });
  await page.getByRole('dialog').waitFor({ timeout: 10_000 });
  assert.deepEqual(plans, [''], 'the ITIL plan was not asked for the workspace once');
  assert.deepEqual(walk.errors(), []);
});

// A desk entry's detail by type (user-experience.md §5.6): its triage fields directly under the head,
// above the notes and not folded among the details; no done checkbox where its workflow decides
// completion; who raised it before what sits under it.
test('chromium: service desk - a request shows its triage under the head, no done checkbox, the requester before its parts', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const desk = deskBackend();
  const form = {
    sections: [{ key: 'request', name: 'Request' }, { key: 'triage', name: 'Triage' }],
    fields: [{ key: 'title', section: 'request', required: true }, { key: 'notes', section: 'request' }, { key: 'custom_fields.priority', section: 'triage' }],
    channels: [{ channel: 'PORTAL' }],
  };
  const shape = {
    statuses: [{ key: 'new', name: 'New', category: 'TODO', color_token: 'slate' }, { key: 'resolved', name: 'Resolved', category: 'DONE', color_token: 'green' }],
    initial: 'new', steps: [{ key: 'resolve', name: 'Resolve', from: ['new'], to: 'resolved' }],
  };
  const answer = (method, path, query, body) => {
    if (path === '/entry-types' && method === 'GET') {
      return { json: { data: [{ key: 'request', level: 'TASK', name: 'Request', system: false, position: 1, archived: false, completable: true, workflow: true, record: false, schedulable: true, estimable: false, allowed_children: [], form, version: 1 }] } };
    }
    if (path === `/items/${REQUEST.id}` && method === 'GET') return { json: { ...REQUEST, created_by: OTHER.id, entry_type: 'request', status: 'new' } };
    if (path === `/items/${REQUEST.id}/steps` && method === 'GET') return { json: { workflow: 'requests', status: 'new', steps: [{ key: 'resolve', name: 'Resolve', to: 'resolved', asks: [] }] } };
    if (path === '/workflows' && method === 'GET') {
      return { json: { data: [{ key: 'requests', system: false, name: 'Requests', published: shape, draft: null, published_at: '2026-10-30T09:00:00Z', version: 1 }], scheme: [{ container_id: COLLECTION.id, entry_type: null, workflow: 'requests' }] } };
    }
    if (path === '/entry-types/request/form:evaluate' && method === 'POST') return { json: { fields: {} } };
    return desk(method, path, query, body);
  };
  const walk = await productWalk(browser, { modules: ['SERVICE_DESK', 'WORKFLOWS'], answer });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/items/${REQUEST.id}`);
  await page.locator('[data-steps]').waitFor({ timeout: 10_000 });
  const triage = page.locator('[data-entry-triage]');
  await triage.waitFor({ timeout: 10_000 });
  assert.equal(await triage.getByText('Priority').count(), 1, 'the priority is not among the triage fields');
  assert.equal(await page.locator('aside.details').getByText('Priority').count(), 0, 'the priority is still among the details');
  const before = (first, second) => page.evaluate(([a, b]) => {
    const x = document.querySelector(a);
    const y = document.querySelector(b);
    return Boolean(x && y && x.compareDocumentPosition(y) & Node.DOCUMENT_POSITION_FOLLOWING);
  }, [first, second]);
  assert.ok(await before('[data-entry-triage]', '.notes-field'), 'the triage is not above the notes');
  assert.equal(await page.locator('.head').getByRole('checkbox').count(), 0, 'a done checkbox is drawn where the workflow decides');
  await page.locator('[data-requester-panel]').waitFor({ timeout: 10_000 });
  assert.ok(await before('[data-requester-panel]', '[data-tour="levels"]'), 'what sits under it comes before who raised it');
});
