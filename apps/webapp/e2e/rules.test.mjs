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
      // The arms inside params: what the kind takes, and where the domain reads them (issue 853).
      kind: 'BRANCH',
      params: {
        condition: 'has(item.due_at)',
        then: [{ kind: 'ADD_COMMENT', params: { body: 'Overdue' } }],
        else: [{ kind: 'WAIT', params: { duration: 'P1D' } }, { kind: 'STOP' }],
      },
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
  last_run: { at: '2026-09-20T15:00:00Z', status: 'SUCCEEDED' },
};
const MANIFEST = {
  product_version: '0.9.0', api_version: 'v1', tenancy_mode: 'single',
  item_types: [{ type: 'TASK', capabilities: ['COMPLETION'], allowed_child_types: [], max_depth: 3 }],
  supported_locales: [{ locale: 'en', direction: 'ltr' }],
  query_fields: [], view_layouts: ['LIST'], roles: [], limits: {}, features: {},
  event_types: ['de.hubtask.work.item.created.v1', 'de.hubtask.work.item.overdue.v1'],
  automation: {
    triggers: ['EVENT', 'SCHEDULE', 'RELATIVE_DATE', 'INBOUND_WEBHOOK', 'MANUAL', 'JUMBLE_ENTRY'],
    actions: ['ADD_COMMENT', 'ADD_LABEL', 'CREATE_ACCESS_TOKEN', 'SEND_WEBHOOK'],
    action_fields: {
      CREATE_ACCESS_TOKEN: [],
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
  last_run: null,
  // A stop with a step stored after it: the server accepts the shape, the run never reaches it.
  actions: [{ kind: 'ADD_LABEL', params: { label_id: '01a0e2e0-0000-7000-8000-0000000000ff' } }, { kind: 'STOP' }, { kind: 'ADD_ATTACHMENT_FROM_URL' }, { kind: 'ADD_COMMENT', params: {} }],
  findings: [
    { level: 'ATTENTION', path: '/run_as', code: 'automation.finding.runner_without_role', params: { account_id: RULE.run_as, scope: 'TENANT' } },
    { level: 'ATTENTION', path: '/actions/0/params/label_id', code: 'automation.finding.reference_gone', params: { kind: 'label', id: '01a0e2e0-0000-7000-8000-0000000000ff' } },
    { level: 'ATTENTION', path: '/actions/3/params/body', code: 'automation.finding.parameter_missing', params: { kind: 'ADD_COMMENT', parameter: 'body' } },
    { level: 'BROKEN', path: '/actions/2/kind', code: 'automation.finding.action_unknown', params: { kind: 'ADD_ATTACHMENT_FROM_URL' } },
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
    if (path.endsWith('/api/v1/quotas')) return route.fulfill({ json: [{ quota: 'automation_runs_per_hour', limit: 500, used: 34, ratio: 0.068 }] });
    return route.fulfill({ json: PAGE });
  };
}

const served = await serve(DIST);
test.after(() => served.close());

/** The panel's Blocks tab, opened: where every block is dragged or clicked from (decision 17). */
async function openBlocks(page) {
  await page.locator('aside.inspector').getByRole('tab', { name: 'Blocks' }).click();
  const blocks = page.locator('aside.inspector [data-blocks]');
  await blocks.waitFor();
  return blocks;
}

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
  assert.deepEqual(patches[0].actions[0].params.then.map((action) => action.kind), ['ADD_COMMENT']);
  // And the check right after it: the write leaves the rule unchecked, and the writer is told at
  // the card whether the repair held rather than at the next opening of the list.
  assert.ok(written.some((body) => body.check), 'the editor checked after the save');
});

test('chromium: every building block carries its icon, and a kind outside the groups is found by typing', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const page = await open(browser, [], { width: 1400, height: 900 });

  // The Blocks tab of the panel (decision 17): an icon in every item; the curated groups on the
  // first sub-tab, every served kind on the second - nothing hidden.
  const blocks = await openBlocks(page);
  const items = blocks.locator('.item');
  assert.equal(await items.count(), await items.locator('svg').count(), 'every block has an icon');
  assert.equal(await blocks.locator('.item', { hasText: 'Create access token' }).count(), 0, 'the rest is not among the blocks');
  await blocks.getByRole('tab', { name: /^All/ }).click();
  assert.equal(await blocks.locator('.item', { hasText: 'Create access token' }).count(), 1, 'and is in All, by eye');
  await blocks.locator('.item', { hasText: 'Create access token' }).click();
  assert.equal(await page.locator('[data-canvas] [data-card="3"] .title').textContent(), 'Create access token', 'a click appends to the chain');
  await openBlocks(page);
  await blocks.getByRole('tab', { name: 'Blocks', exact: true }).click();

  // The + menu: the same list, and typing reaches what the blocks leave out.
  await page.locator('.gap[data-list=""][data-index="0"] [data-slot]').click();
  const menu = page.locator('.menu');
  assert.equal(await menu.locator('.item').count(), await menu.locator('.item svg').count(), 'every menu item has an icon');
  assert.equal(await menu.locator('.item', { hasText: 'Create access token' }).count(), 0);
  await menu.locator('input[type="search"]').fill('access');
  await menu.locator('.item', { hasText: 'Create access token' }).click();
  assert.equal(await page.locator('[data-canvas] [data-card="0"] .title').textContent(), 'Create access token');
});

test('chromium: End the run belongs at the end of an arm, a branch ending on every path ends the chain, and + Else if adds a rung', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const page = await open(browser, [], { width: 1400, height: 1200 });
  const titles = async () => page.locator('[data-canvas] [data-card="0"] .title, [data-canvas] [data-card="1"] .title, [data-canvas] [data-card="2"] .title, [data-canvas] [data-card="3"] .title').allTextContents();

  // Into the chain: refused with its own sentence in the hint line, the chain unchanged.
  // Frequent and Flow both list it; the block under Flow is the one dragged.
  const blocks = await openBlocks(page);
  const endTheRun = blocks.locator('[data-block="STOP"]').last();
  await endTheRun.dragTo(page.locator('.gap[data-list=""][data-index="1"]'));
  await page.locator('.dragline span', { hasText: 'goes at the end of an arm' }).waitFor();
  assert.deepEqual(await titles(), ['Add a label', 'Branch', 'Deliver to a webhook']);
  await endTheRun.dragTo(page.locator('.gap[data-list=""][data-index="3"]'));
  assert.deepEqual(await titles(), ['Add a label', 'Branch', 'Deliver to a webhook'], 'the chain ends the run anyway');

  // The + menu of the chain's last gap does not offer it; the last gap of an arm does. The else
  // arm already ends; once the then arm ends too, the branch ends on every path: no gap after it,
  // the end mark says so, and the step stored after it is never reached.
  await page.locator('.gap[data-list=""][data-index="3"] [data-slot]').click();
  assert.equal(await page.locator('.menu .item', { hasText: 'End the run' }).count(), 0);
  await page.keyboard.press('Escape');
  await page.locator('.gap[data-list="1/then"][data-index="1"] [data-slot]').click();
  await page.locator('.menu .item', { hasText: 'End the run' }).last().click();
  await page.locator('[data-card="1/then/1"]').waitFor();
  assert.equal(await page.locator('[data-branch="1"] .join.none').count(), 1, 'no join under a fork whose arms both end');
  assert.equal(await page.locator('.gap[data-list=""][data-index="2"]').count(), 0, 'no gap after a branch that ends every path');
  assert.equal(await page.locator('[data-end="1"]').count(), 1, 'the end mark stands after the branch');
  assert.equal(await page.locator('.never').count(), 1);
  assert.equal(await page.locator('[data-card="2"].unreachable').count(), 1, 'what was stored after it is never reached');
  assert.equal(await page.locator('[data-card="1/then/1"] button[aria-label="Move up"]').isDisabled(), true, 'the end does not move up');
  assert.equal(await page.locator('[data-card="1/then/0"] button[aria-label="Move down"]').isDisabled(), true, 'nothing moves below it');

  // + Else if: a rung under the branch, the former else arm as the last resort, drawn as a ladder.
  await page.locator('[data-add-rung="1"]').click();
  await page.locator('[data-ladder="1"]').waitFor();
  assert.equal(await page.locator('[data-rung="1"]').count(), 1);
  assert.equal(await page.locator('[data-rung="1/else/0"]').count(), 1, 'the new rung');
  assert.equal(await page.locator('[data-rung="1/else/0/else"]').count(), 1, 'the else after it');
  assert.equal(await page.locator('[data-card="1/else/0/else/0"] .title').textContent(), 'Wait', 'what the else held travelled down');
  assert.equal(await page.locator('[data-rung="1/else/0"] .rcond.selected').count(), 1, 'the rung is selected for its condition');

  // A stored rule with a step after an end: drawn faded, with the word once.
  await page.goto(`${served.origin}/administration/rules/${STALE.id}`);
  await page.locator('[data-card="2"]').waitFor();
  assert.equal(await page.locator('.never').count(), 1);
  assert.equal(await page.locator('[data-card="2"].unreachable').count(), 1);
  assert.equal(await page.locator('[data-card="1"].unreachable').count(), 0);
  assert.equal(await page.locator('[data-end=""]').count(), 0, 'the chain drew no second end mark');
});

test('chromium: a condition is composed as a tree in the gate and in a branch, and the write carries the CEL', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const written = [];
  const page = await open(browser, written, { width: 1400, height: 1200 });
  const inspector = page.locator('aside.inspector');

  // The gate's condition: the sentence it holds becomes "any of" it and one more, with a group under them.
  await page.locator('[data-card="conditions/0"]').click();
  await inspector.getByRole('button', { name: 'Add another sentence' }).click();
  await inspector.locator('select.mode').first().selectOption('any');
  await inspector.locator('[data-sentence="1"] select').nth(0).selectOption('completed');
  await inspector.getByRole('button', { name: 'Add a group' }).click();
  await inspector.locator('[data-group="2"] select.mode').selectOption('none');
  await inspector.locator('[data-sentence="2/0"] select').nth(0).selectOption('archived');
  assert.equal(await inspector.locator('code.compiled').textContent(), "item.type == 'TASK' || item.completed == true || (!(item.archived == true))");
  // On the canvas: the sentences in words, the modes as chips, the group marked (F8-18).
  const shown = page.locator('[data-card="conditions/0"] .words');
  assert.equal(await shown.locator('.w').allTextContents().then((texts) => texts.join(' | ')), "the entry's type is TASK | completion yes | archived yes");
  assert.equal(await shown.locator('.chip').allTextContents().then((chips) => chips.join(',')), 'or,or,none of');
  assert.equal(await shown.locator('.group').count(), 1);
  assert.equal(await shown.locator('code').count(), 0, 'no raw expression on the canvas');

  // The branch card says its condition the same way, and the branch is a flow card.
  assert.equal(await page.locator('[data-card="1"] .cond .w').textContent(), 'a due date is set');
  assert.equal(await page.locator('[data-card="1"] .mark.flow').count(), 1);

  // A branch's condition takes the same composer.
  await page.locator('[data-card="1"]').click();
  await inspector.getByRole('button', { name: 'Add another sentence' }).click();
  await inspector.locator('[data-sentence="1"] select').nth(0).selectOption('title');
  await inspector.locator('[data-sentence="1"] input').fill('urgent');
  assert.equal(await inspector.locator('code.compiled').textContent(), "has(item.due_at) && item.title.matches('(?i)urgent')");

  await page.getByRole('button', { name: 'Save the rule' }).click();
  await page.waitForTimeout(500);
  const patch = written.find((body) => body.actions);
  assert.equal(patch.conditions[0].expr, "item.type == 'TASK' || item.completed == true || (!(item.archived == true))");
  assert.equal(patch.actions[1].params.condition, "has(item.due_at) && item.title.matches('(?i)urgent')");
});

test('chromium: a trigger let go on a gap is refused with its sentence, and the trigger stays', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const page = await open(browser, [], { width: 1400, height: 900 });

  // While the piece is lifted: the hint on its own line, whole; the trigger card's strip inside
  // the card; the gap's pill readable on one line (F8-12).
  await openBlocks(page);
  const dt = await page.evaluateHandle(() => new DataTransfer());
  await page.dispatchEvent('[data-blocks] [data-block="T:SCHEDULE"]', 'dragstart', { dataTransfer: dt });
  assert.equal(await page.locator('.dragline').textContent(), 'A trigger goes at the top: let it go on the trigger card, and it replaces the one there.');
  const strip = page.locator('[data-card="trigger"] .dropword');
  assert.equal(await strip.textContent(), 'Replace the trigger');
  const [card, badge] = await Promise.all([page.locator('[data-card="trigger"]').boundingBox(), strip.boundingBox()]);
  assert.ok(badge.y >= card.y && badge.y + badge.height <= card.y + card.height, 'the strip sits inside the card');
  await page.dispatchEvent('[data-blocks] [data-block="T:SCHEDULE"]', 'dragend', { dataTransfer: dt });
  await page.dispatchEvent('[data-blocks] [data-block="ADD_LABEL"]', 'dragstart', { dataTransfer: dt });
  const pill = page.locator('.gap[data-list=""][data-index="1"] [data-slot]');
  const pillBox = await pill.boundingBox();
  assert.ok(pillBox.width > pillBox.height * 2, 'the pill is wide, not a circle with two lines in it');
  assert.equal(await pill.evaluate((el) => el.scrollWidth <= el.clientWidth), true, 'nothing clipped');
  await page.dispatchEvent('[data-blocks] [data-block="ADD_LABEL"]', 'dragend', { dataTransfer: dt });
  assert.equal(await page.locator('.dragline').textContent(), '', 'the line stays and empties');

  await page.getByRole('button', { name: 'A schedule' }).dragTo(page.locator('.gap[data-list=""][data-index="1"]'));
  await page.getByText('A trigger can only be at the top.').waitFor();
  assert.equal(await page.locator('[data-card="trigger"] .title').textContent(), 'Something happens');
  // The event in words composed from its own name, not the wire name; the select groups the
  // manifest's types by what they are about and says them the same way.
  assert.equal(await page.locator('[data-card="trigger"] .meta').textContent(), 'An entry becomes overdue');
  await page.locator('[data-card="trigger"]').click();
  const eventSelect = page.locator('aside.inspector select').nth(1);
  assert.deepEqual(await eventSelect.locator('optgroup').evaluateAll((all) => all.map((group) => group.label)), ['Entries']);
  assert.deepEqual(await eventSelect.locator('option').allTextContents().then((texts) => texts.map((text) => text.trim())), ['Choose an event', 'An entry is created', 'An entry becomes overdue']);
  await page.getByText('On the wire: de.hubtask.work.item.overdue.v1').waitFor();

  // And on the trigger card it lands: the kind changes. Clicking the card opened Details; the
  // blocks are a tab away.
  await openBlocks(page);
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
  await page.getByText('The account it runs as: The account the rule runs as holds no role at the rule\'s scope; every step on an entry would find nothing.').waitFor();
  await page.getByText('Works', { exact: true }).waitFor();
  // The last run under the word (F8-21): the rule that ran says when and how, the other says never.
  assert.match(await page.locator('article', { hasText: RULE.name }).textContent(), /Last run.*Succeeded/);
  assert.match(await page.locator('article', { hasText: STALE.name }).textContent(), /Last run\s*never/);

  // On the shell (issue 880): one heading, from the PageHeader; the check's banner among its
  // notices; *Write a rule* the primary action, which opens the editor at its address.
  assert.equal(await page.getByRole('heading', { level: 1 }).count(), 1);
  assert.equal((await page.getByRole('heading', { level: 1 }).textContent()).trim(), 'Automation');
  await page.locator('[data-opener="new-rule"]').click();
  await page.waitForURL(/\/administration\/rules\/new$/, { timeout: 5_000 });
  await page.goBack();
  await page.getByText('Works', { exact: true }).waitFor();

  // The rule itself: the findings at their cards, and the switch refused with the reason.
  await page.getByRole('link', { name: 'Flag blocked work' }).click();
  await page.locator('[data-card="2"] .flag').waitFor();
  assert.match(await page.locator('[data-card="2"] .flag').textContent(), /no action ADD_ATTACHMENT_FROM_URL/);
  assert.match(await page.locator('[data-card="0"] .flag').textContent(), /no longer exists/);
  // The two findings of F8-19: the missing parameter at its step, the roleless runner at the pill.
  assert.match(await page.locator('[data-card="3"] .flag').textContent(), /needs body/);
  assert.match(await page.locator('.chip-flag').textContent(), /holds no role/);
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
  // On the shell (issue 880): one heading, from the PageHeader.
  assert.equal(await page.getByRole('heading', { level: 1 }).count(), 1);
  assert.equal((await page.getByRole('heading', { level: 1 }).textContent()).trim(), 'What the rules did');

  await page.getByLabel('From').fill('2026-09-20T00:00');
  await page.getByLabel('To').fill('2026-09-21T00:00');
  await page.waitForFunction(() => performance.now() > 0);
  await page.waitForTimeout(500);
  const windowed = written.find((body) => body.runs?.from && body.runs?.to);
  assert.ok(windowed, 'the listing was asked for the window');
  assert.ok(windowed.runs.to > windowed.runs.from, `to ${windowed.runs.to} after from ${windowed.runs.from}`);
  assert.equal(await page.locator('[data-strip] .stat').count(), 5);
  // The hour's standing and the link to Limits (F8-21).
  await page.locator('[data-hourly]').waitFor();
  assert.match(await page.locator('[data-hourly]').textContent(), /This hour: 34 of 500 runs/);
  assert.equal(await page.locator('[data-hourly] a').getAttribute('href'), '/administration/quotas');
});
