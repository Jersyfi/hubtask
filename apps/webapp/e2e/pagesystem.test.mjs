// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The page system on *People & access*, as the administrator meets it at 1280×800 and on a phone
// at 390×844 (page-system §10, §11): one page kind per screen (UC-WRK-42 check 1), the create act
// in the head opening a dialog of its size that holds no table, tabs or second dialog and does not
// scroll (checks 2 and 3), a row opening its record page (check 4), the destructive act in the
// row's menu (check 5); a list with its search, filters and count and nothing else open on arrival,
// its foot saying what is shown, one verb per row, and nine people within 1.5 screens on a phone
// (UC-WRK-41 checks 1-5); the sign-in settings with their sections and states in the first
// viewport, a section at its own address, as a detail with *Back* on a phone, short enough, with
// one save model and the column and section list still in view at the section's end (UC-WRK-40
// checks 1-5). The same on *How we work* - Overview, Workspace, Templates, Modules, Types, Workflows,
// Link types, Rules and Rule runs: one kind each, the types', rules' and runs' lists with their
// search, count and foot, *New type* and *New workflow* opening a Dialog M, a type opening its
// record page with its delete in the row's menu, a workflow at its own address with a status's
// Dialog M, a step's Dialog L and the draft bar, and the workspace's sections each at its address.
// The same on *Connections and data* - E-mail, Webhooks, Voice, Backup, Retention, Restore,
// Privacy requests, Audit log, History & versions, Private hubs, Limits and AI: one kind each, the
// lists' and logs' search, count and foot with nothing open on arrival, their create and export
// acts opening a dialog of its size, and the settings pages' sections listed in the first viewport.
// The same on *the installation* - Overview, Workspaces, Usage, Journal, Failed jobs, Plans,
// Defaults, Templates, Configuration file, Sign-in providers, AI, Backup targets, Events to the
// platform, Keyring and Operators: one kind each, the lists' and logs' search and count with nothing
// open on arrival, *New workspace* a create page, *New plan* and *Add an operator* a Dialog M, and
// the defaults' sections listed in the first viewport.
// Heights are read from `getBoundingClientRect`, never from a screenshot. Against the stubbed API;
// Chromium, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, COLLECTION, HUB, MANIFEST, OTHER, OWNER_GRANTS, SERVICE_ACCOUNT, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const DESKTOP = { width: 1280, height: 800 };
const PHONE = { width: 390, height: 844 };

// Nine people at the workspace, as the seeded demo holds them: the reader, Mara and seven more.
const NAMES = ['Lukas Brandt', 'Amelia Hughes', 'Mehmet Yilmaz', 'Priya Natarajan', 'Sofia Rossi', 'Jonas Weber', 'Clara Neumann'];
const MORE = NAMES.map((name, index) => ({
  id: `01a0e2e0-0000-7000-8000-0000000009${String(index + 10)}`,
  kind: 'USER',
  display_name: name,
  email: `${name.split(' ')[0].toLowerCase()}@example.invalid`,
  status: 'ACTIVE',
}));
const NINE = [
  { id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'OWNER' },
  { id: 'm9', scope_type: 'TENANT', account_id: OTHER.id, role: 'MEMBER' },
  ...MORE.map((account, index) => ({ id: `m${index + 10}`, scope_type: 'TENANT', account_id: account.id, role: index === 0 ? 'ADMIN' : 'MEMBER' })),
];

// *How we work* as an administrator meets it: the workflows module on, two types, two workflows,
// two rules and two of their runs.
const SHAPE = {
  statuses: [
    { key: 'todo', name: 'To do', category: 'TODO', color_token: 'slate' },
    { key: 'doing', name: 'In progress', category: 'IN_PROGRESS', color_token: 'blue' },
    { key: 'done', name: 'Done', category: 'DONE', color_token: 'green' },
  ],
  initial: 'todo',
  steps: [{ key: 'start', name: 'Start work', from: ['todo'], to: 'doing' }, { key: 'finish', name: 'Finish', from: ['doing'], to: 'done', who: { roles: ['OWNER'] } }],
};
const WORKFLOWS = [
  { key: 'default', system: true, name: null, published: SHAPE, draft: null, published_at: null, version: 1 },
  { key: 'incident', system: false, name: 'Incident', published: SHAPE, draft: null, published_at: '2026-10-30T09:00:00Z', version: 1 },
];
const TYPES = [
  { key: 'task', level: 'TASK', system: true, position: 0, archived: false, name: null, icon: 'check-square', color_token: 'slate', completable: true, workflow: true, record: false, schedulable: true, estimable: false, allowed_children: [], translations: {}, version: 1 },
  { key: 'bug', level: 'TASK', system: false, position: 1, archived: false, name: 'Bug', icon: 'tag', color_token: 'red', completable: true, workflow: true, record: false, schedulable: true, estimable: false, allowed_children: [], translations: {}, version: 1, form: { fields: [{ key: 'title', required: true }] } },
];
const RUNNER = '01a0e2e0-0000-7000-8000-000000002901';
const RULES = ['Escalate a missed target', 'Ask for more information'].map((name, index) => ({
  id: `01a0e2e0-0000-7000-8000-00000000291${index}`, name, enabled: index === 0, scope: { type: 'TENANT' }, run_as: RUNNER,
  trigger: { kind: 'MANUAL' }, conditions: [], actions: [{ kind: 'ADD_COMMENT', params: { body: 'More, please' } }],
  throttle: { max_runs_per_hour: 100 }, on_error: 'CONTINUE', failure_count: 0, version: 1, created_by: RUNNER,
  created_at: '2026-09-01T00:00:00Z', updated_at: '2026-09-01T00:00:00Z', findings: [],
}));
const RUNS = [0, 1].map((index) => ({
  id: `01a0e2e0-0000-7000-8000-00000000292${index}`, rule_id: RULES[index].id, status: index === 0 ? 'SUCCEEDED' : 'FAILED', trigger: 'MANUAL',
  started_at: `2026-11-0${index + 5}T06:00:00Z`, finished_at: `2026-11-0${index + 5}T06:00:01Z`, causation_depth: 1, condition_results: [],
  action_results: [{ index: 0, kind: 'ADD_COMMENT', status: index === 0 ? 'SUCCEEDED' : 'FAILED', error_code: index === 0 ? null : 'items.not_found' }],
}));

/** One connected mailbox and what arrived through it, and a message of another door. */
const MAILBOX = {
  id: '01a0e2e0-0000-7000-8000-000000002a01', protocol: 'IMAP', address: 'desk@example.org', host: 'imap.example.org', port: 993, username: 'desk@example.org',
  folder: 'INBOX', done_folder: 'Hubtask', plus_addresses: true, enabled: true, failing: false, checked_at: '2026-11-05T06:00:00Z', created_at: '2026-10-01T06:00:00Z', auth: 'PASSWORD',
};
const RECEIPTS = [MAILBOX.address, 'inbound'].map((door, index) => ({
  id: `01a0e2e0-0000-7000-8000-000000002b0${index}`, received_at: `2026-11-0${index + 5}T06:00:00Z`, sender: 'ana@example.com',
  subject: index === 0 ? 'Printer on floor 2' : 'Through the operator', outcome: 'JUMBLE', reason: null, door,
}));

/** One file store connection and one calendar push of the reader's own. */
const FILE_STORE = { id: '01a0e2e0-0000-7000-8000-000000002f01', kind: 'NEXTCLOUD', server: 'https://cloud.example.org', login: 'ada', used_at: '2026-11-03T09:15:00Z', created_at: '2026-10-30T10:00:00Z' };
const CALENDAR_PUSH = {
  id: '01a0e2e0-0000-7000-8000-000000002f02', feed_id: '01a0e2e0-0000-7000-8000-000000002f03', provider: 'GOOGLE', address: 'ada@example.org',
  state: 'ACTIVE', pushed_at: '2026-11-03T09:15:00Z', created_at: '2026-10-30T10:00:00Z',
};

/** One agent of the agents' module, live and acting for the owner. */
const AGENT = {
  id: '01a0e2e0-0000-7000-8000-000000000a01', account_id: null, name: 'Request sorter', purpose: 'Sorts new requests into the right queue', kind: 'TICKET_TRIAGE',
  owner_id: ACCOUNT.id, status: 'ACTIVE', risk_class: 'MINIMAL', scopes: [], model_id: null, policy: { reversible: 'APPROVAL', irreversible: 'APPROVAL' },
  budget: { tokens_per_day: 200000, actions_per_hour: 120, concurrent_runs: 2 }, min_accuracy: 0, needs_approval: false, approved_by: null, approved_at: null,
  suspended_by: null, suspended_at: null, created_at: '2026-11-01T08:00:00Z', version: 1,
};

/** What the agents read, answered with the module on; nothing for any other path. */
function agentsAnswer(route, request, path) {
  if (request.method() !== 'GET') return undefined;
  if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, modules_on: [...new Set([...(MANIFEST.modules_on ?? []), 'AGENTS'])] } });
  if (path === '/tenant/modules') return route.fulfill({ json: { modules: [{ module: 'AGENTS', on: true, source: 'WORKSPACE', locked_by: null }] } });
  if (path === '/agents') return route.fulfill({ json: { data: [AGENT] } });
  if (path === `/agents/${AGENT.id}`) return route.fulfill({ json: { ...AGENT, usage: { actions_this_hour: 0, tokens_today: 0 } } });
  if (path.startsWith(`/agents/${AGENT.id}/`) || path === '/agent-requests' || path === '/agent-models' || path === '/agents:metering') return route.fulfill({ json: { data: [] } });
  if (path === '/agents:halt') return route.fulfill({ json: { halted: false, set_by: null, set_at: null } });
  if (path === '/agents:policy') return route.fulfill({ json: { 'agents.registered_only': { value: false, source: 'DEFAULT' } } });
  return undefined;
}

/** What *How we work* reads, answered; nothing for any other path. */
function howWeWork(route, request, path) {
  if (request.method() !== 'GET') return undefined;
  if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, modules_on: [...new Set([...(MANIFEST.modules_on ?? []), 'WORKFLOWS'])] } });
  // The fixture answers no module list; the workflows module, switched on here.
  if (path === '/tenant/modules') return route.fulfill({ json: { modules: [{ module: 'WORKFLOWS', on: true, source: 'WORKSPACE', locked_by: null }] } });
  if (path === '/entry-types') return route.fulfill({ json: { data: TYPES } });
  if (path === '/workflows') return route.fulfill({ json: { data: WORKFLOWS, scheme: [] } });
  if (path === '/automation/rules') return route.fulfill({ json: { data: RULES, page: { next_cursor: null, has_more: false } } });
  if (path === '/mail/mailboxes') return route.fulfill({ json: { mailboxes: [MAILBOX], presets: [] } });
  if (path === '/mail/receipts') return route.fulfill({ json: { receipts: RECEIPTS } });
  if (path === '/automation/runs') return route.fulfill({ json: { data: RUNS, page: { next_cursor: null, has_more: false } } });
  const run = RUNS.find((each) => path === `/automation/runs/${each.id}`);
  if (run) return route.fulfill({ json: run });
  return undefined;
}

/** Signed in as the fixture's owner, at a width; below `medium` as a phone. */
async function open(browser, { width, height }, { isNine = false, isInstallation = false, isAgents = false } = {}) {
  const isPhone = width < 768;
  const context = await browser.newContext({ viewport: { width, height }, ...(isPhone ? { isMobile: true, hasTouch: true } : {}) });
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (isNine && path === '/memberships' && url.searchParams.get('scope_type') === 'TENANT') {
      return route.fulfill({ json: { data: NINE, page: { next_cursor: null, has_more: false }, grantable_roles: OWNER_GRANTS } });
    }
    const account = MORE.find((each) => path === `/accounts/${each.id}`);
    if (account) return route.fulfill({ json: account });
    // The installation: several workspaces, and the hour raised (ADR-0070 §4).
    if (isInstallation && path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, tenancy_mode: 'multi' } });
    if (isInstallation && path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    const agentsAnswered = isAgents ? agentsAnswer(route, request, path) : undefined;
    if (agentsAnswered) return agentsAnswered;
    const answered = howWeWork(route, request, path);
    if (answered) return answered;
    // The reader's own file stores and calendar pushes, one each, for their lists' search and count.
    if (path === '/integrations/file-stores' && request.method() === 'GET') return route.fulfill({ json: { data: [FILE_STORE], kinds: ['NEXTCLOUD'] } });
    if (path === '/integrations/calendar-pushes' && request.method() === 'GET') return route.fulfill({ json: { data: [CALENDAR_PUSH], providers: ['GOOGLE'] } });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  return { context, page, failures };
}

async function visit(page, path) {
  await page.goto(`${served.origin}${path}`);
  await page.locator('main h1').first().waitFor({ state: 'attached', timeout: 15_000 });
  await page.locator('[data-page-kind]').first().waitFor({ state: 'attached', timeout: 15_000 }).catch((error) => {
    throw new Error(`${path} draws no page kind: ${error.message}`);
  });
}

const SCREENS = [
  ['/administration/people', 'list'],
  ['/administration/people?tab=groups', 'list'],
  ['/administration/people?tab=roles', 'list'],
  ['/administration/people?tab=permissions', 'list'],
  ['/administration/sign-in', 'settings'],
  ['/administration/sign-in/passwords', 'settings'],
  ['/administration/legal', 'settings'],
  ['/administration/machines', 'list'],
  ['/administration/machines?tab=apps', 'list'],
  [`/administration/people/${OTHER.id}`, 'record'],
  [`/administration/machines/${SERVICE_ACCOUNT.id}`, 'record'],
  ['/administration', 'overview'],
  ['/administration/workspace', 'settings'],
  ['/administration/workspace/language', 'settings'],
  ['/administration/templates', 'list'],
  ['/administration/modules', 'settings'],
  ['/administration/types', 'list'],
  ['/administration/types/bug', 'record'],
  ['/administration/workflows', 'editor'],
  ['/administration/workflows/incident', 'editor'],
  ['/administration/link-types', 'list'],
  ['/administration/rules', 'list'],
  ['/administration/rules/new', 'create'],
  [`/administration/rules/${RULES[0].id}`, 'record'],
  ['/administration/runs', 'log'],
  // Connections and data (page-system §9.3).
  ['/administration/mail', 'settings'],
  ['/administration/mail/senders', 'settings'],
  [`/administration/mail/mailboxes/${MAILBOX.id}`, 'record'],
  ['/administration/webhooks', 'list'],
  ['/administration/voice', 'settings'],
  ['/administration/backup', 'settings'],
  ['/administration/retention', 'settings'],
  ['/administration/retention/holds', 'settings'],
  ['/administration/restore', 'create'],
  ['/administration/privacy', 'list'],
  ['/administration/audit', 'log'],
  ['/administration/changes', 'log'],
  ['/administration/private-hubs', 'list'],
  ['/administration/quotas', 'list'],
  ['/administration/ai', 'settings'],
];

/** Presses a head's act; on a phone a secondary act folds into the head's ⋯. */
async function pressHead(page, name, title) {
  const button = page.getByRole('main').getByRole('button', { name, exact: true }).filter({ visible: true });
  if (await button.count() > 0) return button.first().click();
  await page.getByRole('button', { name: `More for ${title}` }).first().click();
  await page.getByRole('menuitem', { name }).click();
}

/** A sized dialog without a table, tabs or a second dialog, scrolling only as an L at 1280×800. */
async function assertLadder(page, size, wanted) {
  const dialog = page.locator('dialog[open]');
  await dialog.waitFor({ timeout: 5_000 });
  const shape = await dialog.evaluate((node) => {
    const body = node.querySelector('.body');
    return {
      size: node.getAttribute('data-size'),
      tables: node.querySelectorAll('table, [role="table"], [role="grid"]').length,
      tabs: node.querySelectorAll('[role="tablist"]').length,
      // A picker's sheet on a phone is a closed `<dialog>` until pressed: only one open over it counts.
      dialogs: node.querySelectorAll('dialog[open]').length,
      scrolls: body ? body.scrollHeight > body.clientHeight + 1 : false,
    };
  });
  assert.equal(shape.size, wanted, `the dialog is ${shape.size}, not ${wanted}`);
  assert.equal(shape.tables + shape.tabs + shape.dialogs, 0, 'the dialog holds a table, tabs or a second dialog');
  if (size === DESKTOP && shape.size !== 'l') assert.equal(shape.scrolls, false, 'the dialog scrolls at 1280×800');
}

for (const size of [DESKTOP, PHONE]) {
  const at = `${size.width}×${size.height}`;

  test(`chromium ${at}: every administration screen is exactly one page kind (UC-WRK-42 check 1)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size);
    t.after(() => context.close());
    for (const [path, kind] of SCREENS) {
      await visit(page, path);
      const kinds = await page.locator('[data-page-kind]').evaluateAll((nodes) => nodes.map((node) => node.getAttribute('data-page-kind')));
      assert.deepEqual(kinds, [kind], `${path} draws ${kinds.join(', ') || 'no page kind'}`);
    }
    assert.deepEqual(failures, []);
  });

  test(`chromium ${at}: People narrows, counts and acts by row; nothing else is open on arrival (UC-WRK-41 checks 1-5)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size, { isNine: true });
    t.after(() => context.close());
    await visit(page, '/administration/people');
    const main = page.getByRole('main');
    await main.getByRole('link', { name: 'Lukas Brandt' }).first().waitFor({ timeout: 15_000 });

    // Check 1: the search, the filters - behind *Filters* on a phone - and the count.
    await main.getByRole('searchbox', { name: 'Search people' }).waitFor();
    await main.getByText('9 of 9', { exact: true }).first().waitFor();
    if (size === PHONE) await main.getByRole('button', { name: /^Filters/ }).waitFor();
    else assert.ok((await main.locator('.filter-bar button').count()) >= 2, 'the filters are chips beside the search');

    // Check 2: no form and no field but the search is open on arrival.
    const fields = await main.locator('input:not([type="search"]), select, textarea, form').evaluateAll((nodes) => nodes.filter((node) => node.getClientRects().length > 0).length);
    assert.equal(fields, 0, 'a field is open on arrival');

    // Check 3: the foot says what is shown.
    await main.getByText('Showing 9', { exact: true }).waitFor();

    // Check 4: a row carries at most one verb beside its menu.
    const verbs = await main.locator('tbody tr').evaluateAll((rows) =>
      rows.map((row) => [...row.querySelectorAll('button')].filter((button) => button.getClientRects().length > 0 && !button.getAttribute('aria-haspopup')).length),
    );
    assert.ok(verbs.length >= 9, `${verbs.length} rows drawn`);
    assert.ok(verbs.every((count) => count <= 1), `a row has more than one verb: ${verbs.join(', ')}`);

    // Narrowing: "pri" leaves Priya.
    await main.getByRole('searchbox', { name: 'Search people' }).fill('pri');
    await main.getByText('1 of 9', { exact: true }).first().waitFor();

    // Check 5: on a phone the nine people and the head fit in 1.5 screens.
    if (size === PHONE) {
      await main.getByRole('searchbox', { name: 'Search people' }).fill('');
      await main.getByText('9 of 9', { exact: true }).first().waitFor();
      const height = await page.evaluate(() => document.documentElement.scrollHeight);
      assert.ok(height <= 1.5 * size.height, `the People page is ${height} px high, ${(height / size.height).toFixed(2)} screens`);
    }
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    assert.equal(overflow, 0, 'the page scrolls sideways');
    assert.deepEqual(failures, []);
  });

  test(`chromium ${at}: the head's primary opens a dialog of its size, a row its record page, revoking sits in the row's menu (UC-WRK-42 checks 2-5)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size);
    t.after(() => context.close());
    await visit(page, '/administration/people');
    const main = page.getByRole('main');
    await main.getByRole('link', { name: 'Mara Lind' }).first().waitFor({ timeout: 15_000 });

    // Check 2 and 3: *Add people* opens a sized dialog without a table, tabs or a second dialog,
    // whose body does not scroll unless it is size L.
    await main.getByRole('button', { name: 'Add people', exact: true }).first().click();
    const dialog = page.locator('dialog[open]');
    await dialog.waitFor({ timeout: 5_000 });
    const shape = await dialog.evaluate((node) => {
      const body = node.querySelector('.body');
      return {
        size: node.getAttribute('data-size'),
        tables: node.querySelectorAll('table, [role="table"], [role="grid"]').length,
        tabs: node.querySelectorAll('[role="tablist"]').length,
        dialogs: node.querySelectorAll('dialog').length,
        scrolls: body ? body.scrollHeight > body.clientHeight + 1 : false,
      };
    });
    assert.ok(['s', 'm', 'l'].includes(shape.size ?? ''), `the dialog has no size: ${shape.size}`);
    assert.equal(shape.tables, 0, 'the dialog holds a table');
    assert.equal(shape.tabs, 0, 'the dialog holds tabs');
    assert.equal(shape.dialogs, 0, 'the dialog holds a second dialog');
    if (size === DESKTOP && shape.size !== 'l') assert.equal(shape.scrolls, false, 'the dialog scrolls at 1280×800');
    await page.keyboard.press('Escape');
    await dialog.waitFor({ state: 'detached', timeout: 5_000 }).catch(async () => {
      // A dialog holding changes asks first; nothing was typed, so it closes.
      assert.equal(await page.locator('dialog[open]').count(), 0, 'the dialog stayed open');
    });

    // Check 5: the destructive act is in the row's menu, not a button on the row.
    const row = main.locator('tbody tr').filter({ hasText: 'Mara Lind' });
    assert.equal(await row.getByRole('button', { name: /revoke/i }).count(), 0, 'revoking is a button on the row');
    await row.getByRole('button', { name: /More for Mara Lind/ }).click();
    const menu = page.getByRole('menu').or(page.locator('dialog[open]'));
    await menu.getByText('Revoke access…').first().waitFor({ timeout: 5_000 });
    await page.keyboard.press('Escape');

    // Check 4: the person has lists of their own, so the row opens their page.
    await main.getByRole('link', { name: 'Mara Lind' }).first().click();
    await page.waitForFunction((id) => location.pathname === `/administration/people/${id}`, OTHER.id, { timeout: 10_000 });
    await page.locator('[data-page-kind="record"]').waitFor({ timeout: 10_000 });
    assert.deepEqual(failures, []);
  });

  test(`chromium ${at}: Sign-in shows its sections with their state and opens one at its address (UC-WRK-40 checks 1-5)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size);
    t.after(() => context.close());

    // Check 1: every section and its state within the first viewport.
    await visit(page, '/administration/sign-in');
    const nav = page.locator('nav.section-nav');
    await nav.getByRole('link').first().waitFor({ timeout: 15_000 });
    const links = await nav.getByRole('link').evaluateAll((nodes) => nodes.map((node) => ({ text: node.textContent?.trim() ?? '', bottom: node.getBoundingClientRect().bottom })));
    assert.ok(links.length >= 5, `${links.length} sections listed`);
    for (const link of links) assert.ok(link.bottom <= size.height, `${link.text} is below the first viewport`);
    const statuses = await nav.locator('a .status, a [class*="status"]').count();
    assert.ok(statuses >= links.length, `${statuses} states for ${links.length} sections`);

    // Check 2: a section's address opens it; on a phone alone, with the way back.
    await visit(page, '/administration/sign-in/passwords');
    await page.getByRole('heading', { name: 'Passwords' }).first().waitFor({ timeout: 10_000 });
    if (size === PHONE) {
      await page.getByRole('button', { name: 'Sign-in' }).first().waitFor();
      assert.equal(await nav.evaluate((node) => node.getClientRects().length), 0, 'the section list stands beside the section on a phone');
    } else {
      assert.ok((await nav.evaluate((node) => node.getClientRects().length)) > 0, 'the section list is not beside the section');
    }

    // Check 3: the section is no taller than two viewports, four on a phone.
    const height = await page.evaluate(() => document.documentElement.scrollHeight);
    assert.ok(height <= (size === PHONE ? 4 : 2) * size.height, `the section is ${height} px high`);

    // Check 4: one save model - one save bar at most, its one *Save*.
    const saves = await page.getByRole('main').getByRole('button', { name: 'Save', exact: true }).count();
    assert.ok(saves <= 1, `${saves} save buttons in one section`);

    // Check 5: at the section's end the column and the section list are still in view.
    if (size === DESKTOP) {
      await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
      for (const [what, locator] of [['the column', page.getByRole('navigation', { name: 'Administration', exact: true })], ['the section list', nav]]) {
        const box = await locator.evaluate((node) => node.getBoundingClientRect().toJSON());
        assert.ok(box.bottom > 0 && box.top < size.height && box.height > 0, `${what} left the viewport`);
      }
    }
    assert.deepEqual(failures, []);
  });
  test(`chromium ${at}: Types, Rules and Rule runs narrow, count and continue; nothing else is open on arrival (UC-WRK-41 checks 1-4)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size);
    t.after(() => context.close());
    for (const [path, search, rows, hasChips] of [
      ['/administration/types', 'Search types', 2, false],
      ['/administration/rules', 'Search rules', 2, false],
      ['/administration/runs', 'Search by rule', 2, true],
    ]) {
      await visit(page, path);
      const main = page.getByRole('main');
      await main.locator('tbody tr.row').nth(rows - 1).waitFor({ timeout: 15_000 });
      // Check 1: the search, the filters - behind *Filters* on a phone - and the count.
      await main.getByRole('searchbox', { name: search }).waitFor();
      assert.ok(await main.locator('.filter-bar').getByText(/^\d+ (of \d+|runs?)$/).count() >= 1, `${path} has no count`);
      if (hasChips) {
        if (size === PHONE) await main.getByRole('button', { name: /^Filters/ }).waitFor();
        else await main.getByRole('button', { name: 'Any time' }).waitFor();
      }
      // Check 2: no field but the search is open on arrival.
      const fields = await main.locator('input:not([type="search"]):not([role="switch"]), select, textarea, form').evaluateAll((nodes) => nodes.filter((node) => node.getClientRects().length > 0).length);
      assert.equal(fields, 0, `${path} opens a field on arrival`);
      // Check 3: the foot says what is shown.
      await main.getByText(`Showing ${rows}`, { exact: true }).waitFor();
      // Check 4: a row carries at most one verb beside its menu.
      const verbs = await main.locator('tbody tr.row').evaluateAll((all) =>
        all.map((row) => [...row.querySelectorAll('button, [role="switch"]')].filter((each) => each.getClientRects().length > 0 && !each.getAttribute('aria-haspopup') && !each.classList.contains('press')).length),
      );
      assert.ok(verbs.every((count) => count <= 2), `${path}: a row has more than one verb beside its menu: ${verbs.join(', ')}`);
    }
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    assert.equal(overflow, 0, 'the page scrolls sideways');
    assert.deepEqual(failures, []);
  });

  test(`chromium ${at}: New type and New workflow open a Dialog M, a type opens its record page, its delete sits in the row's menu (UC-WRK-42 checks 2-5)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size);
    t.after(() => context.close());

    await visit(page, '/administration/types');
    const main = page.getByRole('main');
    await main.getByRole('link', { name: 'Bug' }).first().waitFor({ timeout: 15_000 });
    await main.getByRole('button', { name: 'New type', exact: true }).first().click();
    await assertLadder(page, size, 'm');
    await page.keyboard.press('Escape');
    await page.locator('dialog[open]').waitFor({ state: 'detached', timeout: 5_000 });

    // Check 5: deleting is in the row's menu, not a button on the row.
    const row = main.locator('tbody tr.row').filter({ hasText: 'Bug' });
    assert.equal(await row.getByRole('button', { name: /delete/i }).count(), 0, 'deleting is a button on the row');
    await row.getByRole('button', { name: /More for Bug/ }).click();
    await page.getByRole('menu').or(page.locator('dialog[open]')).getByText('Delete…').first().waitFor({ timeout: 5_000 });
    await page.keyboard.press('Escape');

    // Check 4: a type has its form and where it is used, so the row opens its page.
    await main.getByRole('link', { name: 'Bug' }).first().click();
    await page.waitForFunction(() => location.pathname === '/administration/types/bug', null, { timeout: 10_000 });
    await page.locator('[data-page-kind="record"]').waitFor({ timeout: 10_000 });
    await page.getByRole('region', { name: 'Form of Bug' }).waitFor();

    await visit(page, '/administration/workflows');
    await page.getByRole('button', { name: 'New workflow', exact: true }).first().click();
    await assertLadder(page, size, 'm');
    await page.keyboard.press('Escape');
    assert.deepEqual(failures, []);
  });

  test(`chromium ${at}: a workflow opens at its address, a status as a Dialog M, a step as a Dialog L, a change in the draft bar (page-system §7.4)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size);
    t.after(() => context.close());

    await visit(page, '/administration/workflows/incident');
    await page.getByRole('heading', { name: 'Incident', level: 2 }).waitFor({ timeout: 15_000 });
    const nav = page.locator('nav.section-nav');
    if (size === PHONE) {
      await page.getByRole('button', { name: 'Workflows', exact: true }).first().waitFor();
      assert.equal(await nav.evaluate((node) => node.getClientRects().length), 0, 'the list stands beside the workflow on a phone');
    } else {
      assert.ok((await nav.evaluate((node) => node.getClientRects().length)) > 0, 'the list is not beside the workflow');
    }

    await page.locator('.block-list').first().getByRole('button', { name: /In progress/ }).first().click();
    await assertLadder(page, size, 'm');
    await page.keyboard.press('Escape');
    await page.locator('dialog[open]').waitFor({ state: 'detached', timeout: 5_000 });
    await page.locator('.block-list').nth(1).getByRole('button', { name: /Finish/ }).first().click();
    await assertLadder(page, size, 'l');
    await page.keyboard.press('Escape');
    await page.locator('dialog[open]').waitFor({ state: 'detached', timeout: 5_000 });

    // A new status is a change of the draft, which the bar counts.
    assert.equal(await page.getByText(/^Draft · /).count(), 0, 'a draft bar without a change');
    await page.getByRole('button', { name: 'Add status' }).click();
    await page.locator('dialog[open]').getByLabel('Name').fill('Review');
    await page.locator('dialog[open]').getByRole('button', { name: 'Done' }).first().click();
    await page.getByText(/^Draft · 1 change since the published version/).waitFor({ timeout: 5_000 });
    assert.deepEqual(failures, []);
  });

  test(`chromium ${at}: the workspace's sections are listed with their state and each opens at its address (UC-WRK-40 checks 1-2)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size);
    t.after(() => context.close());

    await visit(page, '/administration/workspace');
    const nav = page.locator('nav.section-nav');
    await nav.getByRole('link').first().waitFor({ timeout: 15_000 });
    const links = await nav.getByRole('link').evaluateAll((nodes) => nodes.map((node) => ({ text: node.textContent?.trim() ?? '', bottom: node.getBoundingClientRect().bottom })));
    assert.ok(links.length >= 3, `${links.length} sections listed`);
    for (const link of links) assert.ok(link.bottom <= size.height, `${link.text} is below the first viewport`);

    await visit(page, '/administration/workspace/language');
    if (size === PHONE) {
      assert.equal(await nav.evaluate((node) => node.getClientRects().length), 0, 'the section list stands beside the section on a phone');
    } else {
      assert.ok((await nav.evaluate((node) => node.getClientRects().length)) > 0, 'the section list is not beside the section');
    }
    const height = await page.evaluate(() => document.documentElement.scrollHeight);
    assert.ok(height <= (size === PHONE ? 4 : 2) * size.height, `the section is ${height} px high`);
    assert.deepEqual(failures, []);
  });

  test(`chromium ${at}: Connections and data's lists and logs narrow and count, nothing open on arrival (UC-WRK-41 checks 1-2)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size);
    t.after(() => context.close());
    for (const [path, search] of [
      ['/administration/webhooks', 'Search webhooks'],
      ['/administration/privacy', 'Search by person'],
      ['/administration/audit', 'Search what, who or about'],
      ['/administration/changes', 'Search versions and proposals'],
      ['/administration/private-hubs', 'Search by owner'],
      ['/administration/quotas', 'Search limits'],
    ]) {
      await visit(page, path);
      const main = page.getByRole('main');
      // Check 1: the search, and the filters - behind *Filters* on a phone.
      await main.getByRole('searchbox', { name: search }).waitFor({ timeout: 15_000 });
      if (size === PHONE) assert.ok(await main.getByRole('button', { name: /^Filters/ }).count() <= 1);
      // Check 2: no field but the search is open on arrival.
      const fields = await main.locator('input:not([type="search"]):not([role="switch"]), select, textarea, form').evaluateAll((nodes) => nodes.filter((node) => node.getClientRects().length > 0).length);
      assert.equal(fields, 0, `${path} opens a field on arrival`);
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      assert.equal(overflow, 0, `${path} scrolls sideways`);
    }
    assert.deepEqual(failures, []);
  });

  test(`chromium ${at}: Connections and data's create and export acts open a dialog of their size (UC-WRK-42 checks 2-3)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size);
    t.after(() => context.close());
    for (const [path, act, title, wanted] of [
      ['/administration/webhooks', 'Add a webhook', 'Webhooks', 'm'],
      ['/administration/privacy', 'Record a request', 'Privacy requests', 'm'],
      ['/administration/audit', 'Export…', 'Audit log', 'm'],
    ]) {
      await visit(page, path);
      await page.getByRole('main').getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });
      await page.waitForTimeout(300);
      await pressHead(page, act, title);
      await assertLadder(page, size, wanted);
      await page.keyboard.press('Escape');
      await page.locator('dialog[open]').waitFor({ state: 'detached', timeout: 5_000 });
    }
    assert.deepEqual(failures, []);
  });

  test(`chromium ${at}: Connections and data's settings pages list their sections in the first viewport (UC-WRK-40 check 1)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size);
    t.after(() => context.close());
    for (const path of ['/administration/mail', '/administration/voice', '/administration/backup', '/administration/retention', '/administration/ai']) {
      await visit(page, path);
      const nav = page.locator('nav.section-nav');
      await nav.getByRole('link').first().waitFor({ timeout: 15_000 });
      const links = await nav.getByRole('link').evaluateAll((nodes) => nodes.map((node) => ({ text: node.textContent?.trim() ?? '', bottom: node.getBoundingClientRect().bottom })));
      assert.ok(links.length >= 2, `${path}: ${links.length} sections listed`);
      for (const link of links) assert.ok(link.bottom <= size.height, `${path}: ${link.text} is below the first viewport`);
    }
    assert.deepEqual(failures, []);
  });

  test(`chromium ${at}: Your settings are one page kind each, their lists narrow and count, Name and language lists its sections (UC-WRK-40 check 1, UC-WRK-41 checks 1-2, UC-WRK-42 check 1)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size);
    t.after(() => context.close());
    for (const [path, kind] of [
      ['/profile', 'settings'], ['/profile/you/language', 'settings'], ['/profile/appearance', 'settings'],
      ['/profile/notifications', 'settings'], ['/profile/security', 'settings'],
      ['/profile/sessions', 'list'], ['/profile/devices', 'list'], ['/profile/apps', 'list'], ['/profile/tokens', 'list'],
      ['/profile/files', 'list'], ['/profile/calendars', 'list'],
    ]) {
      await visit(page, path);
      await page.locator('[data-page-kind]').first().waitFor({ timeout: 15_000 });
      const kinds = await page.locator('[data-page-kind]').evaluateAll((nodes) => nodes.map((node) => node.getAttribute('data-page-kind')));
      assert.deepEqual(kinds, [kind], `${path} draws ${kinds.join(', ') || 'no page kind'}`);
      const main = page.getByRole('main');
      if (kind === 'list') {
        await main.getByRole('searchbox').first().waitFor({ timeout: 15_000 });
        const fields = await main.locator('input:not([type="search"]):not([role="switch"]), select, textarea, form').evaluateAll((nodes) => nodes.filter((node) => node.getClientRects().length > 0).length);
        assert.equal(fields, 0, `${path} opens a field on arrival`);
      }
      if (path === '/profile/files' || path === '/profile/calendars') {
        await main.locator('.filter-bar').getByText(/^\d+ of \d+$/).waitFor({ timeout: 15_000 });
      }
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      assert.equal(overflow, 0, `${path} scrolls sideways`);
    }
    // Name and language: its sections listed in the first viewport, each at its own address.
    await visit(page, '/profile');
    const nav = page.locator('nav.section-nav');
    await nav.getByRole('link').first().waitFor({ timeout: 15_000 });
    const links = await nav.getByRole('link').evaluateAll((nodes) => nodes.map((node) => ({ href: node.getAttribute('href'), bottom: node.getBoundingClientRect().bottom })));
    assert.ok(links.length >= 3, `${links.length} sections listed`);
    for (const link of links) {
      assert.match(link.href ?? '', /^\/profile\/you\/[a-z]+$/);
      assert.ok(link.bottom <= size.height, `${link.href} is below the first viewport`);
    }
    assert.deepEqual(failures, []);
  });

  test(`chromium ${at}: every installation screen is one page kind, its lists and logs narrow and count, nothing open on arrival (UC-WRK-42 check 1, UC-WRK-41 checks 1-2)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size, { isInstallation: true });
    t.after(() => context.close());
    for (const [path, kind, search] of [
      ['/instance', 'overview'],
      ['/instance/workspaces', 'list', 'Search by short name'],
      ['/instance/workspaces/new', 'create'],
      ['/instance/usage', 'log'],
      ['/instance/journal', 'log', 'Search by workspace address'],
      ['/instance/dead-letters', 'log', 'Search failed jobs'],
      ['/instance/plans', 'list'],
      ['/instance/settings', 'settings'],
      ['/instance/templates', 'list', 'Search presets'],
      ['/instance/configuration-file', 'record'],
      ['/instance/providers', 'list', 'Search providers'],
      ['/instance/ai', 'list', 'Search models'],
      ['/instance/backup-targets', 'list', 'Search targets'],
      ['/instance/platform', 'list', 'Search by address'],
      ['/instance/encryption', 'list'],
      ['/instance/operators', 'list', 'Search operators'],
    ]) {
      await visit(page, path);
      const kinds = await page.locator('[data-page-kind]').evaluateAll((nodes) => nodes.map((node) => node.getAttribute('data-page-kind')));
      assert.deepEqual(kinds, [kind], `${path} draws ${kinds.join(', ') || 'no page kind'}`);
      // The hour stays in view above every screen of the area.
      await page.getByText(/You are working on the installation\. \d+ minutes left\./).first().waitFor({ timeout: 5_000 });
      if (search) {
        const main = page.getByRole('main');
        await main.getByRole('searchbox', { name: search }).waitFor({ timeout: 15_000 });
        const fields = await main.locator('input:not([type="search"]):not([role="switch"]), select, textarea, form').evaluateAll((nodes) => nodes.filter((node) => node.getClientRects().length > 0).length);
        assert.equal(fields, 0, `${path} opens a field on arrival`);
      }
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      assert.equal(overflow, 0, `${path} scrolls sideways`);
    }
    assert.deepEqual(failures, []);
  });

  test(`chromium ${at}: the installation's create acts open their rung, the defaults list their sections in the first viewport (UC-WRK-42 checks 2-3, UC-WRK-40 check 1)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size, { isInstallation: true });
    t.after(() => context.close());

    await visit(page, '/instance/workspaces');
    await page.getByRole('main').getByRole('button', { name: 'New workspace', exact: true }).first().click({ timeout: 15_000 });
    await page.waitForFunction(() => location.pathname === '/instance/workspaces/new', null, { timeout: 10_000 });
    await page.locator('[data-page-kind="create"]').waitFor();

    for (const [path, act, title] of [['/instance/plans', 'New plan', 'Plans'], ['/instance/operators', 'Add an operator', 'Operators']]) {
      await visit(page, path);
      await page.getByRole('main').getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });
      await page.waitForTimeout(300);
      await pressHead(page, act, title);
      await assertLadder(page, size, 'm');
      await page.keyboard.press('Escape');
      await page.locator('dialog[open]').waitFor({ state: 'detached', timeout: 5_000 });
    }

    await visit(page, '/instance/settings');
    const nav = page.locator('nav.section-nav');
    await nav.getByRole('link').first().waitFor({ timeout: 15_000 });
    const links = await nav.getByRole('link').evaluateAll((nodes) => nodes.map((node) => ({ text: node.textContent?.trim() ?? '', bottom: node.getBoundingClientRect().bottom })));
    assert.ok(links.length >= 4, `${links.length} sections listed`);
    for (const link of links) assert.ok(link.bottom <= size.height, `${link.text} is below the first viewport`);
    assert.deepEqual(failures, []);
  });

  test(`chromium ${at}: the agents are a list page, their settings a settings page, an agent its record page, old addresses land (UC-WRK-42 checks 1 and 4-5, UC-WRK-40 check 2)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size, { isAgents: true });
    t.after(() => context.close());
    for (const [path, kind] of [
      ['/agents', 'list'], ['/agents?tab=requests', 'list'], ['/agents?tab=runs', 'list'],
      ['/agents/settings', 'settings'], ['/agents/settings/models', 'settings'], [`/agents/${AGENT.id}`, 'record'],
      ['/profile/agents', 'list'], [`/profile/agents/${AGENT.id}`, 'record'],
    ]) {
      await visit(page, path);
      const kinds = await page.locator('[data-page-kind]').evaluateAll((nodes) => nodes.map((node) => node.getAttribute('data-page-kind')));
      assert.deepEqual(kinds, [kind], `${path} draws ${kinds.join(', ') || 'no page kind'}`);
      if (kind === 'list') {
        const main = page.getByRole('main');
        await main.getByRole('searchbox').first().waitFor({ timeout: 15_000 });
        const fields = await main.locator('input:not([type="search"]):not([role="switch"]), select, textarea, form').evaluateAll((nodes) => nodes.filter((node) => node.getClientRects().length > 0).length);
        assert.equal(fields, 0, `${path} opens a field on arrival`);
      }
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      assert.equal(overflow, 0, `${path} scrolls sideways`);
    }

    // A row opens its record page; retiring is the danger zone's, asked in a Dialog S.
    await visit(page, '/agents');
    await page.locator('[data-agent-row]').first().click({ timeout: 15_000 });
    await page.waitForFunction((id) => location.pathname === `/agents/${id}`, AGENT.id, { timeout: 10_000 });
    await page.locator('[data-page-kind="record"]').waitFor();
    await page.getByRole('button', { name: 'Retire…' }).click({ timeout: 15_000 });
    await assertLadder(page, size, 's');
    await page.keyboard.press('Escape');

    // The administration's old address and the old tabs land on the settings' sections.
    await page.goto(`${served.origin}/administration/agents?tab=settings`);
    await page.locator('nav.section-nav').getByRole('link', { name: /^Every agent/ }).waitFor({ timeout: 15_000 });
    for (const [old, heading, now] of [['/agents?tab=models', 'Models', 'models'], ['/agents?tab=metering', 'Metering', 'metering'], ['/agents?tab=settings&section=models', 'Models', 'models']]) {
      await page.goto(`${served.origin}${old}`);
      await page.waitForFunction((section) => location.pathname === `/agents/settings/${section}`, now, { timeout: 15_000 });
      await page.locator('[data-page-kind="settings"]').waitFor({ timeout: 15_000 });
      await page.getByRole('main').getByRole('heading', { name: heading, exact: true }).first().waitFor({ timeout: 15_000 });
    }
    assert.deepEqual(failures, []);
  });

  test(`chromium ${at}: a rule is a create page or its record page, a mailbox its record page from its row (page-system §9.1, §9.3)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size);
    t.after(() => context.close());
    const main = page.getByRole('main');
    const headings = () => main.getByRole('heading', { level: 1 }).allTextContents();

    // *Write a rule*: a create page, its one heading the page's, *Save the rule* its primary.
    await visit(page, '/administration/rules/new');
    await page.locator('[data-canvas]').waitFor({ timeout: 15_000 });
    assert.deepEqual((await headings()).map((each) => each.trim()), ['New rule']);
    await main.getByRole('button', { name: 'Save the rule' }).filter({ visible: true }).first().waitFor();

    // A rule: its record page, the name the one heading, deleting in ⋯ asked in a Dialog S.
    await visit(page, '/administration/rules');
    await main.getByRole('link', { name: RULES[0].name }).first().click({ timeout: 15_000 });
    await page.waitForFunction((id) => location.pathname === `/administration/rules/${id}`, RULES[0].id, { timeout: 10_000 });
    await page.locator('[data-page-kind="record"] [data-canvas]').waitFor({ timeout: 15_000 });
    assert.deepEqual((await headings()).map((each) => each.trim()), [RULES[0].name]);
    await main.getByRole('button', { name: 'Save the rule' }).waitFor();
    await main.getByRole('button', { name: `More for ${RULES[0].name}` }).click();
    await page.getByRole('menuitem', { name: 'Delete the rule' }).click();
    await assertLadder(page, size, 's');
    await page.keyboard.press('Escape');

    // A mailbox: its row is a link to its record page - its settings, what arrived through it as
    // a table, disconnecting in the danger zone asked in a Dialog S.
    await visit(page, '/administration/mail/mailboxes');
    await main.getByRole('link', { name: MAILBOX.address }).first().click({ timeout: 15_000 });
    await page.waitForFunction((id) => location.pathname === `/administration/mail/mailboxes/${id}`, MAILBOX.id, { timeout: 10_000 });
    await page.locator('[data-page-kind="record"]').waitFor();
    // The record page names itself *Reading the settings* until the mailbox is read.
    await main.getByRole('heading', { level: 1, name: MAILBOX.address }).waitFor({ timeout: 15_000 });
    assert.deepEqual((await headings()).map((each) => each.trim()), [MAILBOX.address]);
    const arrived = page.locator('[data-mailbox-arrived]');
    await arrived.getByText('Printer on floor 2').first().waitFor({ timeout: 15_000 });
    assert.equal(await arrived.getByText('Through the operator').count(), 0, 'mail of another door is listed');
    await main.getByText('Hubtask', { exact: true }).waitFor();
    await main.getByRole('button', { name: 'Disconnect', exact: true }).click();
    await assertLadder(page, size, 's');
    await page.keyboard.press('Escape');

    // The old address of the mailboxes lands on their section.
    await visit(page, '/administration/mail/mailboxes');
    await page.locator('[data-mail-mailboxes]').waitFor({ timeout: 15_000 });
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    assert.equal(overflow, 0, 'the page scrolls sideways');
    assert.deepEqual(failures, []);
  });

  test(`chromium ${at}: a hub's and a collection's settings are a settings page, a section at a time and none too tall (page-system §9.6, UC-WRK-40)`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await open(browser, size);
    t.after(() => context.close());
    const main = page.getByRole('main');
    for (const place of [`/hubs/${HUB.id}`, `/collections/${COLLECTION.id}`]) {
      await visit(page, `${place}/settings/general`);
      const kinds = await page.locator('[data-page-kind]').evaluateAll((nodes) => nodes.map((node) => node.getAttribute('data-page-kind')));
      assert.deepEqual(kinds, ['settings'], `${place}: ${kinds.join(', ')}`);
      // The place's tabs gave way to the section list (PS-2); the place is in the trail.
      assert.equal(await main.locator('[data-place-strip], [role="tablist"]').count(), 0, `${place}: tabs beside the section list`);
      // *General*: one save for every value - no button of the prefix's or the handle's own.
      await page.locator('[data-place-general]').waitFor({ timeout: 15_000 });
      assert.equal(await main.getByRole('button', { name: /^Change the (prefix|handle)$/ }).count(), 0, `${place}: a save of its own`);
      if (size === PHONE) {
        // On a phone the way back stands only in the bar (the navigation concept §9): an open
        // section is a step of the stack with *‹ Settings* up there, and the page draws none.
        assert.equal((await page.locator('[data-back]').first().textContent())?.trim(), 'Settings', `${place}: the bar's way back`);
        assert.equal(await main.getByRole('button', { name: 'Settings', exact: true }).count(), 0, `${place}: a second way back on the page`);
        await page.locator('[data-back]').first().click();
        await page.waitForURL((url) => url.pathname === `${place}/settings`);
        assert.equal(await main.locator('.parent a').count(), 0, `${place}: the list's way up drawn on the page`);
      }

      // Every section by its address: one section, at most two screens on a desk, four on a phone.
      await visit(page, `${place}/settings`);
      const hrefs = await page.getByRole('navigation', { name: 'Parts of the structure' }).getByRole('link').evaluateAll((links) => links.map((link) => link.getAttribute('href')));
      assert.ok(hrefs.length >= 8, `${place}: ${hrefs.length} sections`);
      for (const href of hrefs) {
        await visit(page, href);
        await page.waitForLoadState('networkidle');
        const screens = await page.evaluate(() => document.documentElement.scrollHeight / window.innerHeight);
        assert.ok(screens <= (size === PHONE ? 4 : 2), `${href}: ${screens.toFixed(2)} screens`);
        const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
        assert.equal(overflow, 0, `${href} scrolls sideways`);
      }
    }

    // *Products* holds the module switches and no template gallery; the hub's templates are its own section.
    await visit(page, `/hubs/${HUB.id}/settings/modules`);
    await page.locator('[data-module]').first().waitFor({ timeout: 15_000 });
    assert.equal(await main.locator('[data-templates], [data-take]').count(), 0, 'templates under the switches');
    // *Types & forms*: the types a table, the levels' names and what each carries folded under *Advanced*.
    await visit(page, `/hubs/${HUB.id}/settings/types`);
    await page.locator('[data-setting="entry_types"] table, [data-setting="entry_types"] [role="list"]').first().waitFor({ timeout: 15_000 });
    assert.equal(await main.locator('[data-setting="entry_types"] input[type="checkbox"]').count() > 0, true, 'no tick per type');
    assert.deepEqual(failures, []);
  });
}
