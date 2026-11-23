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

import { sharedChromium } from './browser.mjs';

import { serve } from './serve.mjs';
import { ACCOUNT, COLLECTION, DEFAULT_STRUCTURE, HUB, ITEMS, OTHER, PAGE, WORKSPACE } from './fixture.mjs';
import { ids, productWalk } from './productwalk.mjs';
import { chipsOf, focused, pick, valueOf } from './several.mjs';

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
    step_name: 'Get the manager\'s yes', item_version: 1, state: 'PENDING', current_level: 1,
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
  const browser = await sharedChromium(t);
  const backend = deskBackend();
  const walk = await productWalk(browser, { modules: ['SERVICE_DESK', 'WORKFLOWS'], answer: backend });
  t.after(walk.close);
  const { page } = walk;

  // The desk is set up at its home's last tab, each form in a dialog opened by its *Add* (§7.4).
  await page.goto(`${served.origin}/desk/settings`);
  // The home's last tab (the navigation concept §4): its view bar stays with *Settings* current,
  // the section list beside the section - never a second strip.
  const sections = page.getByRole('navigation', { name: 'Sections of Service desk settings' });
  await sections.waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('[data-page-kind="settings"] [role="tablist"]').count(), 0, 'the desk settings draw a second row of tabs');
  const bar = page.getByRole('navigation', { name: 'Service desk', exact: true });
  assert.equal(await bar.getByRole('link', { name: 'Settings' }).getAttribute('aria-current'), 'page', 'the home\'s view bar is gone from its settings');
  assert.equal(await page.getByRole('button', { name: 'New request' }).count(), 0, 'the settings tab offers the home\'s create act');
  await page.getByRole('button', { name: 'Add the calendar' }).click();
  const calendarForm = page.getByRole('dialog').locator('form');
  await calendarForm.getByLabel('Name').fill('Office hours');
  // A zone is picked from the platform's list, never typed (user-experience.md §5.4).
  await calendarForm.getByLabel('Time zone').selectOption('Europe/Berlin');
  await calendarForm.getByRole('button', { name: 'Add the calendar' }).click();
  await page.getByText('Office hours').first().waitFor({ timeout: 5_000 });

  await sections.getByRole('link', { name: 'SLA policies' }).click();
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
  // The concept's ticket (ET-1): the promised times are the rail's first section.
  const promised = page.locator('[data-entry-sla]');
  await promised.waitFor({ timeout: 5_000 });
  assert.match(await promised.innerText(), /First reply/);
  assert.equal(await page.getByRole('complementary', { name: 'Details' }).locator('section').first().getByText('Deadlines', { exact: true }).count(), 1, 'the deadlines are not the rail\'s first section');
  const requester = page.getByRole('region', { name: 'Requester' });
  await requester.getByText('Acme Plumbing').waitFor({ timeout: 5_000 });
  assert.match(await requester.innerText(), new RegExp(OTHER.display_name));
  // The other requests are one line, by their keys (deskentry.d), each a link named with its title -
  // never the browser's blue underline (§5.10).
  const other = requester.getByRole('link', { name: OTHER_REQUEST.title });
  await other.waitFor({ timeout: 5_000 });
  assert.equal(await other.getAttribute('href'), `/items/${OTHER_REQUEST.id}`);
  assert.equal((await other.innerText()).trim(), `KITCHEN-${OTHER_REQUEST.number}`, 'the other request is not named by its key');
  assert.equal(await requester.getByRole('link', { name: REQUEST.title }).count(), 0, 'the entry itself is listed among the others');
  const line = requester.locator('[data-requester-open]');
  assert.match((await line.innerText()).trim(), new RegExp(`^\\d+ other open requests?: KITCHEN-${OTHER_REQUEST.number}`));
  assert.equal(await requester.getByRole('listitem').filter({ has: other }).count(), 0, 'the other requests are rows, not one line');
  const drawn = await other.evaluate((node) => ({ line: getComputedStyle(node).textDecorationLine, color: getComputedStyle(node).color }));
  assert.equal(drawn.line, 'none', 'the other request is underlined as a browser link');
  assert.notEqual(drawn.color, 'rgb(0, 0, 238)', 'the other request is the browser\'s blue');
  const beside = page.locator('[data-approval]');
  await beside.waitFor({ timeout: 5_000 });
  assert.match(await beside.innerText(), /Waiting[\s\S]*Mara Lind asked on [\s\S]*Level 1/);
  // The team reads the step by the workflow's name for it, never its key (P-12).
  assert.match(await beside.innerText(), /to take “Get the manager's yes”/);
  assert.doesNotMatch(await beside.innerText(), /“approve”/);

  await page.goto(`${served.origin}/approvals`);
  const first = page.locator('article').filter({ hasText: REQUEST.title });
  await first.getByText('to take “Get the manager\'s yes”', { exact: false }).waitFor({ timeout: 5_000 });
  await first.getByRole('button', { name: 'Approve' }).click();
  // Decided, it leaves the list: only what still waits for the reader's yes is listed.
  await first.waitFor({ state: 'detached', timeout: 5_000 });

  const second = page.locator('article').filter({ hasText: OTHER_REQUEST.title });
  // The reason is written in the Dialog M the card's *Decline* opens: a card holds no form (PS-4).
  await second.getByRole('button', { name: 'Decline' }).click();
  const answer = page.getByRole('dialog');
  await answer.getByRole('button', { name: 'Decline' }).click();
  await answer.getByRole('alert').filter({ hasText: 'Say why you decline first.' }).waitFor({ timeout: 5_000 });
  await answer.getByLabel('Reason').fill('Not in this quarter\'s budget.');
  await answer.getByRole('button', { name: 'Decline' }).click();
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
  const browser = await sharedChromium(t);
  const desk = deskBackend();
  const long = { ...OTHER_REQUEST, title: 'Drucker im zweiten Obergeschoss druckt seit Montag nur noch leere Seiten und meldet einen Papierstau im Fach drei' };
  const answer = (method, path, query, body) => (path === '/search' && method === 'POST' && JSON.stringify(body?.filter ?? {}).includes('created_by')
    ? { json: { ...PAGE, data: [REQUEST, long] } }
    : desk(method, path, query, body));
  const walk = await productWalk(browser, { modules: ['SERVICE_DESK', 'WORKFLOWS'], answer, width: 390, height: 844 });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/items/${REQUEST.id}`);
  // The `deskentry` phone scene: the conversation first among four marks, none cut off; no copy
  // icons under the head - the link's and the key's copy are rows of the bar's ⋯.
  const marks = page.getByRole('navigation', { name: /^Sections of / });
  await marks.waitFor({ timeout: 10_000 });
  const labels = await marks.getByRole('link').allInnerTexts();
  assert.ok(labels.length <= 4 && /^Conversation/u.test(labels[0]), `the marks are ${labels.join(' · ')}`);
  const cut = await marks.evaluate((nav) => [...nav.querySelectorAll('a')].filter((a) => a.getBoundingClientRect().right > innerWidth + 1).map((a) => a.textContent));
  assert.deepEqual(cut, [], 'a mark is cut off');
  const icons = await page.locator('[data-copy-link]').evaluateAll((all) => all.filter((each) => each.getClientRects().length > 0).length);
  assert.equal(icons, 0, 'a copy icon stands under the head');
  // On a phone the rail is the sheet behind *All properties* (§7).
  await page.getByRole('button', { name: /^All properties/ }).click();
  await page.getByRole('dialog').getByRole('region', { name: 'Requester' }).getByRole('link', { name: long.title }).waitFor({ timeout: 10_000 });
  const wide = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(wide <= 0, `the page scrolls ${wide} px sideways`);
});

test('chromium: service desk - an empty section starts from the ITIL defaults through the template\'s plan', async (t) => {
  const browser = await sharedChromium(t);
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
  await page.waitForURL(/\/administration\/templates\?template=itil_service_desk$/, { timeout: 10_000 });
  await page.getByRole('dialog').waitFor({ timeout: 10_000 });
  assert.deepEqual(plans, [''], 'the ITIL plan was not asked for the workspace once');
  assert.deepEqual(walk.errors(), []);
});

// A desk entry's detail by type (the work surface's ticket, ET-1/ET-6): its triage fields in the rail
// as *Classification*, not folded among the details; no done checkbox where its workflow decides
// completion; who raised it in the rail after the details.
test('chromium: service desk - a request shows its triage under the head, no done checkbox, the requester before its parts', async (t) => {
  const browser = await sharedChromium(t);
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
  assert.equal(await page.locator('[data-details]').getByText('Priority').count(), 0, 'the priority is still among the details');
  const before = (first, second) => page.evaluate(([a, b]) => {
    const x = document.querySelector(a);
    const y = document.querySelector(b);
    return Boolean(x && y && x.compareDocumentPosition(y) & Node.DOCUMENT_POSITION_FOLLOWING);
  }, [first, second]);
  // The concept's ticket (ET-1, ET-6): *Classification* in the rail above the details, the requester after them.
  assert.ok(await before('[data-entry-triage]', '[data-details]'), 'the triage is not above the details');
  assert.equal(await page.locator('.head').getByRole('checkbox').count(), 0, 'a done checkbox is drawn where the workflow decides');
  await page.locator('[data-requester-panel]').waitFor({ timeout: 10_000 });
  assert.ok(await before('[data-details]', '[data-requester-panel]'), 'the requester stands before the details');
});

// UC-SVC-06 (ET-7): the reply offers the desk's snippets - by its button and by "/" - and its macros, each saying what it
// does; a snippet is rendered for the ticket and inserted, a macro presses the rule with the trigger *by hand*.
test('chromium: service desk - the reply inserts a snippet by its button and by /, and presses a macro that says what it does', async (t) => {
  const browser = await sharedChromium(t);
  const desk = deskBackend();
  const runner = '01a0e2e0-0000-7000-8000-000000002901';
  const macro = {
    id: '01a0e2e0-0000-7000-8000-000000002910', name: 'Ask for details', enabled: true, scope: { type: 'TENANT' }, run_as: runner,
    trigger: { kind: 'MANUAL' }, conditions: [], actions: [{ kind: 'ADD_COMMENT', params: { body: 'More, please' } }],
    throttle: { max_runs_per_hour: 100 }, on_error: 'CONTINUE', failure_count: 0, version: 1, created_by: runner,
    created_at: '2026-09-01T00:00:00Z', updated_at: '2026-09-01T00:00:00Z', findings: [],
  };
  const event = { ...macro, id: '01a0e2e0-0000-7000-8000-000000002911', name: 'Escalate', trigger: { kind: 'EVENT', event_type: 'de.hubtask.work.item.overdue.v1' } };
  const asked = { rendered: 0, pressed: [] };
  const answer = (method, path, query, body) => {
    if (path === '/entry-types' && method === 'GET') {
      return { json: { data: [{ key: 'request', level: 'TASK', name: 'Request', system: false, position: 1, archived: false, completable: true, workflow: true, record: false, schedulable: true, estimable: false, allowed_children: [], form: { channels: [{ channel: 'PORTAL' }] }, version: 1 }] } };
    }
    if (path === `/items/${REQUEST.id}` && method === 'GET') return { json: { ...REQUEST, created_by: OTHER.id, entry_type: 'request' } };
    if (path === '/reply-templates' && method === 'GET') return { json: { data: [{ key: 'greeting', name: 'Greeting', body: 'Hello {{requester.name}}', version: 1 }] } };
    if (path === `/items/${REQUEST.id}/reply-templates/greeting` && method === 'GET') {
      asked.rendered += 1;
      return { json: { key: 'greeting', body: 'Hello Olivia, thank you for writing.' } };
    }
    if (path === '/automation/rules' && method === 'GET') return { json: { data: [macro, event], page: { has_more: false, next_cursor: null } } };
    if (path === `/automation/rules/${macro.id}:trigger` && method === 'POST') {
      asked.pressed.push(body?.item_id);
      return { status: 202, json: { run_id: '01a0e2e0-0000-7000-8000-000000002920' } };
    }
    return desk(method, path, query, body);
  };
  const walk = await productWalk(browser, { modules: ['SERVICE_DESK', 'WORKFLOWS'], answer });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/items/${REQUEST.id}`);
  const composer = page.locator('[data-composer]');
  // At rest the reply is one line; its bar comes forward with the focus (§6.9).
  await composer.getByRole('textbox').first().click();
  const snippets = composer.getByRole('button', { name: 'Snippets', exact: true });
  await snippets.waitFor({ timeout: 10_000 });
  await snippets.click();
  await page.getByRole('menuitem', { name: /Greeting/ }).click();
  await composer.getByText('Hello Olivia, thank you for writing.').waitFor({ timeout: 10_000 });

  // "/" at the start of a line opens the same snippets.
  const editor = composer.getByRole('textbox').first();
  await editor.click();
  await page.keyboard.press('End');
  await page.keyboard.press('Enter');
  await page.keyboard.press('/');
  await page.getByRole('menuitem', { name: /Greeting/ }).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');

  // The macro menu lists only the rules pressed by hand, each with what it does.
  await composer.getByRole('button', { name: 'Macros', exact: true }).click();
  const item = page.getByRole('menuitem', { name: /Ask for details/ });
  await item.waitFor({ timeout: 5_000 });
  assert.match(await item.innerText(), /does: /, 'the macro does not say what it does');
  assert.equal(await page.getByRole('menuitem', { name: /Escalate/ }).count(), 0, 'a rule with an event trigger is offered as a macro');
  await item.click();
  await page.getByText('The macro runs on this entry.').first().waitFor({ timeout: 5_000 });
  assert.deepEqual(asked.pressed, [REQUEST.id], 'the macro was not pressed on this entry');
  assert.ok(asked.rendered >= 1, 'the snippet was not rendered for the ticket');
});

// The ticket's rail as the scene draws it (ET-1, ET-6), read in German: the triage's values in the
// reader's words, the priority saying it follows impact × urgency, the responding group by its name
// for whoever may read the entry, and each deadline by what is left or how long it is overdue, its
// policy and its group by name, its urgency in a word and a mark beside the colour.
test('chromium: service desk - a ticket\'s rail names its triage and deadlines in the reader\'s words', async (t) => {
  const browser = await sharedChromium(t);
  const desk = deskBackend();
  const MINUTE = 60_000;
  const at = (offset) => new Date(Date.now() + offset).toISOString();
  const GROUP = '01a0e2e0-0000-7000-8000-00000000f0a1';
  const form = {
    sections: [{ key: 'request', name: 'Request' }, { key: 'triage', name: 'Triage' }],
    fields: [
      { key: 'title', section: 'request', required: true }, { key: 'custom_fields.impact', section: 'triage' },
      { key: 'custom_fields.urgency', section: 'triage' }, { key: 'custom_fields.priority', section: 'triage' },
      { key: 'custom_fields.responding_group', section: 'triage' },
    ],
    channels: [{ channel: 'PORTAL' }],
  };
  const fieldId = ids('0000ff');
  const field = (key, name, kind, options = []) => ({ id: fieldId(), collection_id: null, hub_id: null, key, name, kind, options, is_required: false, restricted: false, read_restricted: false, applies_to: ['TASK'] });
  const goal = (kind, policy, group, standing, due, breached = null) => ({
    goal: 'RESOLUTION', kind, policy_key: policy, assignee_group: group, standing, target_seconds: 14_400,
    armed_at: at(-3 * HOUR), due_at: due, at_risk_at: null, met_at: null, breached_at: breached,
  });
  const answer = (method, path, query, body) => {
    if (path === '/accounts/me' && method === 'GET') return { json: { ...ACCOUNT, locale: 'de' } };
    if (path === '/entry-types' && method === 'GET') {
      return { json: { data: [{ key: 'request', level: 'TASK', name: 'Request', system: false, position: 1, archived: false, completable: true, workflow: true, record: false, schedulable: true, estimable: false, allowed_children: [], form, version: 1 }] } };
    }
    if (path === '/entry-types/request/form:evaluate' && method === 'POST') return { json: { fields: {} } };
    // A ticket's head (deskentry.d): the step that resolves as the primary, the one that waits beside it.
    const statuses = [
      { key: 'new', name: 'New', category: 'TODO', color_token: 'slate' }, { key: 'waiting', name: 'Waiting', category: 'IN_PROGRESS', color_token: 'amber' },
      { key: 'resolved', name: 'Resolved', category: 'DONE', color_token: 'green' },
    ];
    if (path === `/items/${REQUEST.id}/steps` && method === 'GET') {
      return { json: {
        workflow: 'requests', status: 'new', picture: { statuses, initial: 'new', steps: [] }, blocked: [], done_route: [{ key: 'resolve', name: 'Lösen', to: 'resolved' }],
        steps: [{ key: 'wait', name: 'Auf anfragende Person warten', to: 'waiting', asks: [], allowed: true }, { key: 'resolve', name: 'Lösen', to: 'resolved', asks: [], allowed: true }],
      } };
    }
    // Two replies to the requester and one note for the team: the conversation's tabs count each.
    if (path === `/items/${REQUEST.id}/comments` && method === 'GET') {
      const post = (n, internal) => ({ id: `01a0e2e0-0000-7000-8000-00000000cd0${n}`, item_id: REQUEST.id, author_id: n === 3 ? ACCOUNT.id : OTHER.id, body: `Post ${n}`, parent_comment_id: null, internal, created_at: `2026-11-10T0${n}:00:00Z`, edited_at: null, deleted_at: null, version: 1 });
      return { json: { data: [post(1, false), post(2, true), post(3, false)], page: { has_more: false, next_cursor: null } } };
    }
    if (path === '/workflows' && method === 'GET') {
      const shape = { statuses, initial: 'new', steps: [] };
      return { json: { data: [{ key: 'requests', system: false, name: 'Requests', published: shape, draft: null, published_at: '2026-10-30T09:00:00Z', version: 1 }], scheme: [{ container_id: COLLECTION.id, entry_type: null, workflow: 'requests' }] } };
    }
    if (path === '/custom-fields' && method === 'GET') {
      return { json: [field('impact', 'Auswirkung', 'SELECT', ['High', 'Medium', 'Low']), field('urgency', 'Dringlichkeit', 'SELECT', ['High', 'Medium', 'Low']), field('priority', 'Priorität', 'SELECT', ['P1', 'P2', 'P3', 'P4']), field('responding_group', 'Zuständige Gruppe', 'GROUP')] };
    }
    if (path === '/groups' && method === 'GET') return { json: { data: [{ id: GROUP, name: 'Plattform-Team', version: 1 }], page: { next_cursor: null, has_more: false, total: 1 } } };
    if (path === '/sla-policies' && method === 'GET') {
      const policy = (key, name, group) => ({ key, name, calendar: null, entry_types: ['request'], organisations: [], priority_field: 'priority', assignee_group: group, goals: [], position: 0, enabled: true, version: 1 });
      return { json: { data: [policy('incidents', 'Störungen', null), policy('ola_plattform', 'Team Plattform', GROUP)] } };
    }
    if (path === `/items/${REQUEST.id}/sla` && method === 'GET') {
      return { json: { data: [goal('SLA', 'incidents', null, 'RUNNING', at(130 * MINUTE)), goal('OLA', 'ola_plattform', GROUP, 'BREACHED', at(-20 * MINUTE), at(-20 * MINUTE))] } };
    }
    if (path === `/items/${REQUEST.id}` && method === 'GET') {
      return { json: { ...REQUEST, created_by: OTHER.id, entry_type: 'request', status: 'new', custom_fields: { impact: 'High', urgency: 'Medium', priority: 'P2', responding_group: GROUP } } };
    }
    return desk(method, path, query, body);
  };
  const walk = await productWalk(browser, { modules: ['SERVICE_DESK', 'WORKFLOWS'], answer });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/items/${REQUEST.id}`);
  const triage = page.locator('[data-entry-triage]');
  await triage.getByText('Plattform-Team').waitFor({ timeout: 10_000 });
  const said = await triage.innerText();
  assert.match(said, /Auswirkung\s*Hoch/, 'the impact is not in the reader\'s words');
  assert.match(said, /Dringlichkeit\s*Mittel/, 'the urgency is not in the reader\'s words');
  assert.doesNotMatch(said, /\bHigh\b|\bMedium\b/, 'a stored English option shows');
  assert.equal(await triage.locator('[data-detail="field-priority"] [data-detail-note]').innerText(), 'aus Auswirkung × Dringlichkeit');
  assert.doesNotMatch(said, /Für dich nicht sichtbar/, 'the responding group reads as out of sight to who may read the entry');

  const deadlines = page.locator('[data-entry-sla]');
  await deadlines.getByText('Plattform-Team · OLA').waitFor({ timeout: 10_000 });
  const rows = deadlines.locator('.deadline');
  assert.equal(await rows.count(), 2);
  // The breached OLA first, in its tone with a mark and a word; the running SLA by what is left.
  const breached = rows.nth(0);
  assert.equal(await breached.getAttribute('data-tone'), 'danger');
  assert.match(await breached.locator('.words').innerText(), /^seit 20 Min\. überfällig$/);
  assert.equal(await breached.locator('.words svg').count(), 1, 'the breach is a colour alone');
  assert.match(await breached.locator('.due').innerText(), /^war fällig /);
  const running = rows.nth(1);
  assert.match(await running.innerText(), /Störungen · SLA/);
  assert.match(await running.locator('.words').innerText(), /^noch 2 Std\. 10 Min\.$/);
  assert.match(await running.locator('.due').innerText(), /^bis /);
  assert.doesNotMatch(await deadlines.innerText(), /incidents|ola_plattform/, 'a policy shows by its key');

  // The head (deskentry.d): *Resolve* the primary, *wait for the requester* beside it, the priority among its facts.
  const head = page.locator('[data-tour="entry"]');
  await head.getByRole('button', { name: 'Lösen', exact: true }).waitFor({ timeout: 10_000 });
  assert.ok(await head.getByRole('button', { name: 'Auf anfragende Person warten' }).isVisible(), 'the step that waits is not beside the primary');
  assert.match(await head.locator('[data-head-priority]').innerText(), /P2/, 'the priority is not in the head');
  // The conversation's tabs say how many each holds (§6.9).
  const tabs = (await page.locator('#entry-conversation').getByRole('tab').allInnerTexts()).map((text) => text.replace(/\s+/g, ' ').trim());
  assert.deepEqual(tabs.slice(1, 3), ['Antworten 2', 'Interne Notizen 1']);

  // The reader's own post: *Antworten ···* (deskentry.d) - edit and remove wait in the ⋯, not as text buttons.
  const own = page.locator('#entry-conversation article').filter({ hasText: 'Post 3' }).first();
  await own.waitFor({ timeout: 10_000 });
  assert.equal(await own.getByRole('button', { name: 'Antworten', exact: true }).count(), 1, 'the post offers no reply');
  assert.equal(await own.getByRole('button', { name: /^(Bearbeiten|Entfernen)$/ }).count(), 0, 'edit and remove are text buttons beside the reply');
  await own.getByRole('button', { name: 'Mehr zu diesem Kommentar' }).click();
  const acts = page.getByRole('menu', { name: 'Mehr zu diesem Kommentar' });
  assert.deepEqual((await acts.getByRole('menuitem').allInnerTexts()).map((text) => text.trim()), ['Bearbeiten', 'Entfernen']);
  await page.keyboard.press('Escape');
});

// A policy's entry types (design-system.md §4): six types are one field named "Entry types".
// Opened by the keyboard on the first type, Space ticks, Escape closes the list and leaves the
// dialog open with focus on the field, and the policy is saved with the types chosen. In English
// and in German.
const SIX = ['request', 'incident', 'change', 'problem', 'question', 'outage'];
for (const locale of ['en', 'de']) {
  test(`chromium: service desk - a policy's six entry types are one field, chosen by the keyboard (${locale})`, async (t) => {
    const browser = await sharedChromium(t);
    const backend = deskBackend();
    const answer = (method, path, query, body) => {
      if (path === '/accounts/me' && method === 'GET') return { json: { ...ACCOUNT, locale } };
      if (path === '/entry-types' && method === 'GET') {
        return { json: { data: SIX.map((key, at) => ({ key, level: 'TASK', name: key[0].toUpperCase() + key.slice(1), system: false, position: at + 1, archived: false })) } };
      }
      return backend(method, path, query, body);
    };
    const walk = await productWalk(browser, { modules: ['SERVICE_DESK', 'WORKFLOWS'], answer });
    t.after(walk.close);
    const { page } = walk;
    const words = locale === 'de'
      ? { tab: 'SLA-Richtlinien', add: 'Richtlinie hinzufügen', name: 'Name', types: 'Eintragstypen', empty: 'Nichts gewählt' }
      : { tab: 'SLA policies', add: 'Add the policy', name: 'Name', types: 'Entry types', empty: 'Nothing chosen' };

    await page.goto(`${served.origin}/desk/settings`);
    if (locale === 'de') await page.waitForFunction(() => document.documentElement.lang === 'de', null, { timeout: 15_000 });
    await page.getByRole('link', { name: words.tab }).first().click({ timeout: 15_000 });
    await page.getByRole('button', { name: words.add }).click();
    const dialog = page.getByRole('dialog');
    const policyForm = dialog.locator('form');
    await policyForm.getByLabel(words.name).first().fill('Standard');

    const types = policyForm.getByRole('button', { name: new RegExp(`^${words.types}`, 'u') });
    await types.waitFor({ timeout: 5_000 }).catch(() => {});
    assert.equal(await types.count(), 1, 'six entry types are not one field');
    assert.equal(await types.getAttribute('aria-haspopup'), 'listbox');
    assert.equal(await valueOf(types), words.empty);

    await types.focus();
    await page.keyboard.press('Enter');
    const list = page.getByRole('listbox', { name: words.types });
    await list.waitFor({ timeout: 5_000 });
    assert.equal(await focused(page), 'option "Request"', 'Enter did not land on the first type');
    await page.keyboard.press('Space');
    await page.keyboard.press('End');
    assert.equal(await focused(page), 'option "Outage"');
    await page.keyboard.press('Space');
    await page.keyboard.press('Escape');
    await list.waitFor({ state: 'detached', timeout: 5_000 });
    assert.equal(await dialog.isVisible(), true, 'Escape closed the dialog, not only the list');
    assert.match(await focused(page), new RegExp(`^button "${words.types} `, 'u'), 'closing the list left focus elsewhere');
    assert.deepEqual(await chipsOf(types), ['Request', 'Outage']);
    assert.deepEqual(await pick(page, types, ['Change']), ['Request', 'Change', 'Outage']);

    await policyForm.getByRole('button', { name: words.add }).click();
    await page.getByRole('main').getByText('Standard').first().waitFor({ timeout: 5_000 });
    assert.deepEqual(backend.policies[0]?.entry_types, ['request', 'outage', 'change']);
  });
}
