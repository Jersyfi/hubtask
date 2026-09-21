// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The rule editor, driven the way a person drives it (F8-05): a card moved by drag and by keyboard
// and the stored order asserted on the write that leaves; a trigger let go on a gap refused with
// its sentence; and the phone-width layout, where the details come to the canvas as a sheet and a
// branch shows one arm at a time. Chromium only: the browser's own drag and drop is what the
// editor uses, and Playwright drives it there; the layout has no engine-specific part.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const ACCOUNT = { id: '01a0e2e0-0000-7000-8000-000000000001', kind: 'USER', display_name: 'Rule Walker', email: 'rules@example.invalid', status: 'ACTIVE', locale: 'en', onboarding_completed_at: '2026-09-01T00:00:00Z' };
const HUB = { id: '01a0e2e0-0000-7000-8000-000000000002', type: 'HUB', parent_id: null, name: 'Marketing', order_key: 'a0', version: 1 };
const COLLECTION = { id: '01a0e2e0-0000-7000-8000-000000000003', type: 'COLLECTION', parent_id: HUB.id, name: 'Q4 campaign', order_key: 'a0', version: 1 };
const SERVICE_ACCOUNT = { id: '01a0e2e0-0000-7000-8000-000000000004', display_name: 'Automation', kind: 'SERVICE' };
const LABELS = [
  { id: '01a0e2e0-0000-7000-8000-000000000005', name: 'Approval', color_token: 'label.blue' },
  { id: '01a0e2e0-0000-7000-8000-000000000006', name: 'Escalated', color_token: 'label.red' },
];
const RULE = {
  id: '01a0e2e0-0000-7000-8000-000000000010',
  name: 'Escalate overdue approvals',
  scope: { type: 'HUB', id: HUB.id },
  enabled: true,
  run_as: SERVICE_ACCOUNT.id,
  trigger: { kind: 'EVENT', event_type: 'de.hubtask.work.item.overdue.v1' },
  conditions: [{ expr: "item.type == 'TASK'" }],
  actions: [
    { kind: 'ADD_LABEL', params: { label_id: LABELS[1].id } },
    {
      kind: 'BRANCH',
      params: { condition: 'has(item.due_at)' },
      then: [{ kind: 'ADD_COMMENT', params: { body: 'Overdue' } }],
      else: [{ kind: 'WAIT', params: { duration: 'P1D' } }, { kind: 'STOP' }],
    },
    { kind: 'SEND_WEBHOOK', params: { subscription_id: '01a0e2e0-0000-7000-8000-000000000007' } },
  ],
  throttle: { max_runs_per_hour: 100 },
  on_error: 'CONTINUE',
  failure_count: 0,
  version: 2,
  created_by: ACCOUNT.id,
  created_at: '2026-09-01T00:00:00Z',
  updated_at: '2026-09-01T00:00:00Z',
  findings: [],
};
const MANIFEST = {
  product_version: '0.9.0', api_version: 'v1', tenancy_mode: 'single',
  item_types: [{ type: 'TASK', capabilities: ['COMPLETION'], allowed_child_types: [], max_depth: 3 }],
  supported_locales: [{ locale: 'en', direction: 'ltr' }],
  query_fields: [], view_layouts: ['LIST'], roles: [], limits: {}, features: {},
  event_types: ['de.hubtask.work.item.created.v1', 'de.hubtask.work.item.overdue.v1'],
  automation: {
    triggers: ['EVENT', 'SCHEDULE', 'RELATIVE_DATE', 'INBOUND_WEBHOOK', 'MANUAL', 'JUMBLE_ENTRY'],
    actions: ['ADD_COMMENT', 'ADD_LABEL', 'SEND_WEBHOOK'],
    action_fields: {
      ADD_COMMENT: [{ name: 'item_id', kind: 'id', required: true }, { name: 'body', kind: 'string', required: true }],
      ADD_LABEL: [{ name: 'item_id', kind: 'id', required: true }, { name: 'label_id', kind: 'id', required: true }],
      SEND_WEBHOOK: [{ name: 'subscription_id', kind: 'id', required: true }],
    },
  },
};
const PAGE = { data: [], items: [], page: { next_cursor: null, has_more: false } };

const ITEM = { id: '01a0e2e0-0000-7000-8000-000000000020', type: 'TASK', title: 'Campaign approval Q4', collection_id: COLLECTION.id, order_key: 'a0', version: 1 };

/**
 * What the dry run answers for a sample about the entry, for the definition the probe sends - the
 * canvas's, with the webhook moved up: the gate held, the branch went the other way.
 */
const TEST_HELD = {
  matched: true,
  condition_results: [{ index: 0, matched: true }],
  actions: [
    { path: '0', kind: 'ADD_LABEL', would_run: true },
    { path: '1', kind: 'SEND_WEBHOOK', would_run: true },
    { path: '2', kind: 'BRANCH', would_run: true, matched: false },
    { path: '2/then/0', kind: 'ADD_COMMENT', would_run: false },
    { path: '2/else/0', kind: 'WAIT', would_run: true },
    { path: '2/else/1', kind: 'STOP', would_run: true },
  ],
};
const TEST_NOT_HELD = { matched: false, condition_results: [{ index: 0, matched: false }], actions: [] };
const RUNS = [
  { id: '01a0e2e0-0000-7000-8000-000000000031', rule_id: RULE.id, trigger: 'EVENT', status: 'SUCCEEDED', started_at: '2026-09-20T14:32:00Z', causation_depth: 1, condition_results: [{ index: 0, matched: true }], action_results: [{ index: 0, kind: 'ADD_LABEL', path: '0', status: 'SUCCEEDED' }, { index: 1, kind: 'BRANCH', path: '1', matched: true, status: 'SUCCEEDED' }, { index: 2, kind: 'ADD_COMMENT', path: '1/then/0', status: 'SUCCEEDED' }, { index: 3, kind: 'SEND_WEBHOOK', path: '2', status: 'SUCCEEDED' }] },
  { id: '01a0e2e0-0000-7000-8000-000000000032', rule_id: RULE.id, trigger: 'EVENT', status: 'FAILED', started_at: '2026-09-20T13:05:00Z', causation_depth: 1, condition_results: [{ index: 0, matched: true }], action_results: [{ index: 0, kind: 'ADD_LABEL', path: '0', status: 'FAILED', error_code: 'labels.not_found' }] },
  { id: '01a0e2e0-0000-7000-8000-000000000033', rule_id: RULE.id, trigger: 'EVENT', status: 'SKIPPED', started_at: '2026-09-19T20:16:00Z', causation_depth: 1, condition_results: [{ index: 0, matched: false }], action_results: [] },
];

/** A second rule, as the check found it: a label gone, and an action kind this version no longer serves. */
const STALE = {
  ...RULE,
  id: '01a0e2e0-0000-7000-8000-000000000011',
  name: 'Flag blocked work',
  enabled: false,
  actions: [{ kind: 'ADD_LABEL', params: { label_id: '01a0e2e0-0000-7000-8000-0000000000ff' } }, { kind: 'ADD_ATTACHMENT_FROM_URL' }],
  findings: [
    { level: 'ATTENTION', path: '/actions/0/params/label_id', code: 'automation.finding.reference_gone', params: { kind: 'label', id: '01a0e2e0-0000-7000-8000-0000000000ff' } },
    { level: 'BROKEN', path: '/actions/1/kind', code: 'automation.finding.action_unknown', params: { kind: 'ADD_ATTACHMENT_FROM_URL' } },
  ],
  checked_at: '2026-09-20T15:00:00Z',
};

/** The API at the network edge, and a place the last write's body is kept for the assertions. */
function stubFor(written, tested = TEST_HELD) {
  return async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    if (path.endsWith('/api/v1/stream')) return route.abort();
    if (path.endsWith('/api/v1/sync:snapshot')) {
      const record = JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: HUB.id, container_id: null, payload: HUB });
      return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${record}\n{"cursor":"c-e2e"}\n` });
    }
    if (path.endsWith('/api/v1/sync:pull')) return route.fulfill({ json: { changes: [], cursor: 'c-e2e', has_more: false, tombstone_window_days: 90 } });
    if (path.endsWith('/api/v1/meta/capabilities')) return route.fulfill({ json: MANIFEST });
    if (path.endsWith('/api/v1/accounts/me')) return route.fulfill({ json: ACCOUNT });
    if (path.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: [HUB] } });
    if (path.endsWith('/api/v1/containers')) return route.fulfill({ json: { ...PAGE, data: [COLLECTION] } });
    if (path.endsWith('/labels')) return route.fulfill({ json: LABELS });
    if (path.endsWith('/buckets')) return route.fulfill({ json: [] });
    if (path.endsWith('/api/v1/auth/service-accounts')) return route.fulfill({ json: [SERVICE_ACCOUNT] });
    if (path.endsWith('/api/v1/integrations/webhooks')) return route.fulfill({ json: [] });
    if (path.endsWith(`/api/v1/automation/rules/${RULE.id}`) && request.method() === 'PATCH') {
      written.push(request.postDataJSON());
      return route.fulfill({ json: { ...RULE, ...request.postDataJSON(), version: RULE.version + written.length } });
    }
    if (path.endsWith('/api/v1/automation/rules')) return route.fulfill({ json: { ...PAGE, data: [RULE, STALE] } });
    if (path.endsWith('/api/v1/automation/rules:check')) {
      written.push({ check: true });
      return route.fulfill({ json: { data: [RULE, STALE] } });
    }
    if (path.endsWith('/api/v1/automation/rules:test')) {
      written.push(request.postDataJSON());
      return route.fulfill({ json: tested });
    }
    if (path.endsWith('/api/v1/automation/runs')) {
      written.push({ runs: Object.fromEntries(url.searchParams) });
      return route.fulfill({ json: { ...PAGE, data: RUNS } });
    }
    if (path.endsWith('/api/v1/search')) return route.fulfill({ json: { data: [ITEM], items: [ITEM], page: { next_cursor: null, has_more: false } } });
    return route.fulfill({ json: PAGE });
  };
}

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, written, viewport, tested) {
  const context = await browser.newContext({ viewport });
  await context.route('**/api/v1/**', stubFor(written, tested));
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/administration/rules/${RULE.id}`);
  await page.locator('[data-card="2"]').waitFor();
  return page;
}

test('chromium: a card moves by keyboard and by drag, and the write carries the new order', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const written = [];
  const page = await open(browser, written, { width: 1400, height: 900 });

  // The three cards of the chain, in the order the fixture stores them.
  const titles = async () => page.locator('[data-canvas] [data-card="0"] .title, [data-canvas] [data-card="1"] .title, [data-canvas] [data-card="2"] .title').allTextContents();
  assert.deepEqual(await titles(), ['Add a label', 'Branch', 'Deliver to a webhook']);

  // By drag: the second card onto the gap at the top of the chain. Source and target are both in
  // view: the browser's own drag does not survive a scroll between the two.
  await page.locator('[data-card="1"] .kind').dragTo(page.locator('.gap[data-list=""][data-index="0"] [data-slot]'));
  await page.waitForTimeout(200);
  assert.deepEqual(await titles(), ['Branch', 'Add a label', 'Deliver to a webhook']);

  // By keyboard: the last card's "move up" tool, reached by focus, does what a drag would.
  await page.locator('[data-card="2"] button[aria-label="Move up"]').focus();
  await page.keyboard.press('Enter');
  assert.deepEqual(await titles(), ['Branch', 'Deliver to a webhook', 'Add a label']);

  // The write carries the order the canvas shows.
  await page.getByRole('button', { name: 'Save the rule' }).click();
  await page.waitForFunction(() => document.querySelector('[data-canvas]') !== null);
  await page.waitForTimeout(500);
  const patches = written.filter((body) => body.actions);
  assert.equal(patches.length, 1, 'one PATCH left');
  assert.deepEqual(patches[0].actions.map((action) => action.kind), ['BRANCH', 'SEND_WEBHOOK', 'ADD_LABEL']);
  assert.deepEqual(patches[0].actions[0].then.map((action) => action.kind), ['ADD_COMMENT']);
  // And the check right after it: the write leaves the rule unchecked, and the writer is told at
  // the card whether the repair held rather than at the next opening of the list.
  assert.ok(written.some((body) => body.check), 'the editor checked after the save');
});

test('chromium: a trigger let go on a gap is refused with its sentence, and the trigger stays', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const page = await open(browser, [], { width: 1400, height: 900 });

  await page.getByRole('button', { name: 'A schedule' }).dragTo(page.locator('.gap[data-list=""][data-index="1"]'));
  await page.getByText('A trigger can only be at the top.').waitFor();
  assert.equal(await page.locator('[data-card="trigger"] .title').textContent(), 'Something happens');
  // The event in the words the sentence above the canvas uses, not its wire name.
  assert.equal(await page.locator('[data-card="trigger"] .meta').textContent(), 'item overdue');

  // And on the trigger card it lands: the kind changes.
  await page.getByRole('button', { name: 'A schedule' }).dragTo(page.locator('[data-card="trigger"]'));
  assert.equal(await page.locator('[data-card="trigger"] .title').textContent(), 'A schedule');
});

test('chromium: at phone width the details come as a sheet and a branch shows one arm at a time', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const page = await open(browser, [], { width: 375, height: 812 });

  // The branch: one arm shown, the switch naming both with their counts.
  const seghead = page.locator('[data-branch="1"] .seghead');
  await seghead.waitFor();
  assert.equal(await page.locator('[data-arm="1/then"]').count(), 1);
  assert.equal(await page.locator('[data-arm="1/else"]').count(), 0);
  await page.locator('[data-arm-pick="1/else"]').click();
  assert.equal(await page.locator('[data-arm="1/else"]').count(), 1);
  assert.equal(await page.locator('[data-arm="1/then"]').count(), 0);

  // Selecting a card opens the sheet over the canvas, with the card's own settings in it.
  assert.equal(await page.locator('dialog[open]').count(), 0);
  await page.locator('[data-card="0"]').click();
  const sheet = page.locator('dialog[open]');
  await sheet.waitFor();
  assert.ok(await sheet.getByText('Add a label').first().isVisible());
  await page.keyboard.press('Escape');
  await page.waitForFunction(() => !document.querySelector('dialog[open]'));
});

test('chromium: the probe runs the canvas\'s definition through the dry run and draws the answer', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const written = [];
  const page = await open(browser, written, { width: 1400, height: 1200 });

  // An unsaved change first, so that what is tested is the canvas and not the stored rule.
  await page.locator('[data-card="2"] button[aria-label="Move up"]').focus();
  await page.keyboard.press('Enter');

  await page.getByRole('tab', { name: 'Probe' }).click();
  await page.getByRole('button', { name: 'Run it through' }).click();
  await page.locator('[data-card="trigger"] .verdict').waitFor();
  await page.getByText('4 steps would run.').waitFor();

  // The definition that left is the canvas's, and the sample names the event.
  const test = written.find((body) => body.rule);
  assert.ok(test, 'the dry run took a definition');
  assert.deepEqual(test.rule.actions.map((action) => action.kind), ['ADD_LABEL', 'SEND_WEBHOOK', 'BRANCH']);
  assert.equal(test.sample_event.type, RULE.trigger.event_type);

  // The drawing: the gate held, the branch went the other way, the arm not taken is skipped.
  assert.equal(await page.locator('[data-card="conditions/0"] .verdict').textContent(), 'held');
  assert.equal(await page.locator('[data-card="0"] .verdict').textContent(), 'would run');
  assert.ok(await page.locator('[data-card="2"].no').count());
  assert.equal(await page.locator('[data-card="2/then/0"].faded').count(), 1);
  assert.equal(await page.locator('[data-card="2/else/1"] .verdict').textContent(), 'ends the run');
});

test('chromium: a sample the gate refuses stops at the gate, and a recorded run is drawn from its log', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const written = [];
  const page = await open(browser, written, { width: 1400, height: 1200 }, TEST_NOT_HELD);

  await page.getByRole('tab', { name: 'Probe' }).click();
  await page.getByRole('button', { name: 'Run it through' }).click();
  await page.locator('[data-card="conditions/0"] .verdict').waitFor();
  assert.equal(await page.locator('[data-card="conditions/0"] .verdict').textContent(), 'did not hold');
  await page.getByText('A condition did not hold').waitFor();
  assert.equal(await page.locator('[data-card="0"].faded').count(), 1, 'the chain fades');

  // The runs tab: the health from the last runs, and a run drawn onto the canvas.
  // The runs tab reads the log again when it opens: a run recorded since the editor opened is
  // announced by nothing, so the listing the editor subscribed to at its start would miss it.
  const readBefore = written.filter((body) => body.runs).length;
  await page.getByRole('tab', { name: 'Runs' }).click();
  await page.getByText('Fails sometimes').waitFor();
  await page.getByText('1 of 3 runs failed').waitFor();
  assert.equal(written.filter((body) => body.runs).length, readBefore + 1, 'the tab read the runs again');
  await page.locator('.rows .row').nth(1).click();
  await page.locator('[data-card="0"] .verdict').waitFor();
  assert.equal(await page.locator('[data-card="0"] .verdict').textContent(), 'failed');
  await page.getByText('Failed', { exact: true }).first().waitFor();
});

test('chromium: the list checks the rules when it opens and says what the check found', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const written = [];
  const context = await browser.newContext({ viewport: { width: 1400, height: 1000 } });
  await context.route('**/api/v1/**', stubFor(written));
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/administration/rules`);

  await page.getByText('The check found one rule that needs your attention.').waitFor();
  assert.ok(written.some((body) => body.check), 'the list asked for the check');
  await page.getByText('Broken', { exact: true }).waitFor();
  await page.getByText('Step 0: The label this step points at no longer exists; the step would find nothing.').waitFor();
  await page.getByText('Works', { exact: true }).waitFor();

  // The rule itself: the findings at their cards, and the switch refused with the reason.
  await page.getByRole('link', { name: 'Flag blocked work' }).click();
  await page.locator('[data-card="1"] .flag').waitFor();
  assert.match(await page.locator('[data-card="1"] .flag').textContent(), /no action ADD_ATTACHMENT_FROM_URL/);
  assert.match(await page.locator('[data-card="0"] .flag').textContent(), /no longer exists/);
  const enable = page.getByRole('button', { name: 'Switch it on' });
  assert.equal(await enable.isDisabled(), true);
});

test('chromium: the runs page opens prefiltered on a rule and narrows to a window', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const written = [];
  const context = await browser.newContext({ viewport: { width: 1400, height: 1000 } });
  await context.route('**/api/v1/**', stubFor(written));
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/administration/runs?rule_id=${RULE.id}`);
  await page.locator('[data-strip]').waitFor();
  await page.waitForFunction(() => document.querySelector('[data-strip] dd')?.textContent === '3');
  assert.ok(written.some((body) => body.runs?.rule_id === RULE.id), 'the listing was asked for the rule');

  await page.getByLabel('From').fill('2026-09-20T00:00');
  await page.getByLabel('To').fill('2026-09-21T00:00');
  await page.waitForFunction(() => performance.now() > 0);
  await page.waitForTimeout(500);
  const windowed = written.find((body) => body.runs?.from && body.runs?.to);
  assert.ok(windowed, 'the listing was asked for the window');
  assert.ok(windowed.runs.to > windowed.runs.from, `to ${windowed.runs.to} after from ${windowed.runs.from}`);
  assert.equal(await page.locator('[data-strip] .stat').count(), 5);
});
