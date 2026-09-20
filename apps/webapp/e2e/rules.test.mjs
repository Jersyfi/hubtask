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
  enabled: false,
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

/** The API at the network edge, and a place the last write's body is kept for the assertions. */
function stubFor(written) {
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
    if (path.endsWith('/api/v1/automation/rules')) return route.fulfill({ json: { ...PAGE, data: [RULE] } });
    return route.fulfill({ json: PAGE });
  };
}

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, written, viewport) {
  const context = await browser.newContext({ viewport });
  await context.route('**/api/v1/**', stubFor(written));
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
  assert.equal(written.length, 1, 'one PATCH left');
  assert.deepEqual(written[0].actions.map((action) => action.kind), ['BRANCH', 'SEND_WEBHOOK', 'ADD_LABEL']);
  assert.deepEqual(written[0].actions[0].then.map((action) => action.kind), ['ADD_COMMENT']);
});

test('chromium: a trigger let go on a gap is refused with its sentence, and the trigger stays', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const page = await open(browser, [], { width: 1400, height: 900 });

  await page.getByRole('button', { name: 'A schedule' }).dragTo(page.locator('.gap[data-list=""][data-index="1"]'));
  await page.getByText('A trigger can only be at the top.').waitFor();
  assert.equal(await page.locator('[data-card="trigger"] .title').textContent(), 'Something happens');

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
