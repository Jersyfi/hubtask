// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What a plan leaves out (UC-INS-13 checks 3 and 4, multi-tenancy.md §4.2): where
// `/meta/capabilities` answers a feature in `not_included`, its screens say "Not included in your
// plan" in place of the controls the server would refuse (P-05), what exists stays readable, and
// switching off stays. Against the stubbed API; Chromium only, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { MANIFEST, PAGE, signedIn, stub, unstubbedSoFar } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const SERVICE_ACCOUNT = { id: '01a0e2e0-0000-7000-8000-000000001901', display_name: 'Automation', kind: 'SERVICE' };

const rule = (id, name, enabled) => ({
  id, name, enabled,
  scope: { type: 'TENANT' },
  run_as: SERVICE_ACCOUNT.id,
  trigger: { kind: 'SCHEDULE', rrule: 'FREQ=WEEKLY;BYDAY=SA;BYHOUR=8', timezone: 'Europe/Berlin' },
  conditions: [],
  actions: [{ kind: 'ADD_COMMENT', params: { body: 'Shopping' } }],
  throttle: { max_runs_per_hour: 100 },
  on_error: 'CONTINUE',
  failure_count: 0,
  version: 1,
  created_by: SERVICE_ACCOUNT.id,
  created_at: '2026-09-01T00:00:00Z',
  updated_at: '2026-09-01T00:00:00Z',
  findings: [],
});

const RULES = [
  rule('01a0e2e0-0000-7000-8000-000000001911', 'Shopping every Saturday', true),
  rule('01a0e2e0-0000-7000-8000-000000001912', 'Done into the archive', true),
  rule('01a0e2e0-0000-7000-8000-000000001913', 'Birthdays', false),
];

const FAILED_RUN = {
  id: '01a0e2e0-0000-7000-8000-000000001921', rule_id: RULES[0].id, status: 'FAILED', trigger: 'SCHEDULE',
  started_at: '2026-10-10T06:00:00Z', finished_at: '2026-10-10T06:00:01Z', causation_depth: 1,
  condition_results: [], action_results: [{ index: 0, kind: 'ADD_COMMENT', status: 'FAILED', error_code: 'items.not_found' }],
};

/** A signed-in page whose plan leaves out what `notIncluded` names. */
async function onPlan(browser, notIncluded, extra = () => undefined) {
  const { page, failures, context } = await signedIn(browser, 1280, 1000);
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, not_included: notIncluded } });
    if (path === '/auth/service-accounts') return route.fulfill({ json: [SERVICE_ACCOUNT] });
    if (path === '/automation/rules') return route.fulfill({ json: { ...PAGE, data: RULES } });
    if (path === '/automation/rules:check') return route.fulfill({ json: { data: RULES } });
    if (path.startsWith('/automation/rules/')) return route.fulfill({ json: RULES.find((each) => path.endsWith(each.id)) ?? RULES[0] });
    if (path === '/automation/runs') return route.fulfill({ json: { ...PAGE, data: [FAILED_RUN] } });
    if (path === `/automation/runs/${FAILED_RUN.id}`) return route.fulfill({ json: FAILED_RUN });
    const answered = await extra(route, request, path);
    if (answered !== undefined) return answered;
    return stub(route);
  });
  return { page, failures, context };
}

test('chromium: rules a plan leaves out rest, readable, with nothing to create or switch on', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await onPlan(browser, ['AUTOMATION_RULES']);
  t.after(() => context.close());

  await page.goto(`${served.origin}/administration/rules`);
  await page.getByText('Rules don\'t run in this plan.').waitFor({ timeout: 5_000 });
  assert.ok(await page.getByText('Not included in your plan', { exact: true }).isVisible());
  assert.ok(await page.getByText('Your 3 rules rest and stay. When automation is included again, they run from then on.').isVisible());

  // Every rule is still listed and reads as resting.
  for (const each of RULES) assert.ok(await page.getByText(each.name).isVisible(), `${each.name} is not listed`);
  assert.equal(await page.getByText('Resting', { exact: true }).count(), RULES.length);

  // No new rule and no switching on; switching off stays for the two that are on.
  assert.equal(await page.getByRole('button', { name: 'Write a rule' }).count(), 0, 'a new rule is offered');
  assert.equal(await page.getByRole('button', { name: 'Switch it on' }).count(), 0, 'switching on is offered');
  assert.equal(await page.getByRole('button', { name: 'Switch it off' }).count(), 2);

  // The editor reads only: no save, no switching on, no dry run; switching off and deleting stay.
  await page.goto(`${served.origin}/administration/rules/${RULES[2].id}`);
  await page.getByText('This rule rests and stays as it is, readable.', { exact: false }).waitFor({ timeout: 5_000 });
  assert.equal(await page.getByRole('button', { name: 'Save the rule' }).count(), 0, 'saving is offered');
  assert.equal(await page.getByRole('button', { name: 'Switch it on' }).count(), 0, 'switching on is offered in the editor');

  // The runs: readable, nothing to try, start or replay.
  await page.goto(`${served.origin}/administration/runs`);
  await page.getByText('there is nothing to try or replay', { exact: false }).waitFor({ timeout: 5_000 });
  assert.equal(await page.getByRole('button', { name: 'Try it' }).count(), 0);
  assert.equal(await page.getByRole('button', { name: 'Run it for real now' }).count(), 0);

  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbedSoFar().filter((read) => read.includes('automation')), []);
});

test('chromium: included, the rules screen offers what it always did', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await onPlan(browser, ['WEBHOOKS']);
  t.after(() => context.close());

  await page.goto(`${served.origin}/administration/rules`);
  await page.getByRole('button', { name: 'Write a rule' }).waitFor({ timeout: 5_000 });
  assert.equal(await page.getByText('Not included in your plan', { exact: true }).count(), 0);
  assert.equal(await page.getByRole('button', { name: 'Switch it on' }).count(), 1);
  assert.deepEqual(failures, []);
});
