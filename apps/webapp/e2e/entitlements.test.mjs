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

const SHOW_DETAIL = 'What happened';
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
    const answered = extra(route, request, path);
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
  // A failed run, opened: what it did is readable, and nothing finishes it.
  await page.getByRole('button', { name: SHOW_DETAIL }).first().click();
  await page.getByRole('button', { name: 'Hide the detail' }).waitFor({ timeout: 5_000 });
  await page.waitForTimeout(300);
  assert.equal(await page.getByRole('button', { name: 'Finish this run' }).count(), 0, 'a replay is offered');

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

  // The same failed run, opened where the rules are included, can be finished.
  await page.goto(`${served.origin}/administration/runs`);
  await page.getByRole('button', { name: SHOW_DETAIL }).first().click();
  await page.getByRole('button', { name: 'Finish this run' }).waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
});

const subscription = (id, state) => ({
  id, target_url: `https://shop.example/hooks/${id.slice(-2)}`, event_types: ['de.hubtask.work.item.created.v1'],
  state, failure_count: 0, created_at: '2026-10-01T08:00:00Z', version: 1,
});
const DEAD = {
  id: '01a0e2e0-0000-7000-8000-000000001933', subscription_id: '01a0e2e0-0000-7000-8000-000000001931',
  event_id: '01a0e2e0-0000-7000-8000-0000000019e1', attempt: 8, status: 'DEAD_LETTER', response_status: 503,
  error_code: 'webhooks.target_refused', created_at: '2026-10-02T08:00:00Z',
};
const SUBSCRIPTIONS = [
  subscription('01a0e2e0-0000-7000-8000-000000001931', 'ACTIVE'),
  subscription('01a0e2e0-0000-7000-8000-000000001932', 'PAUSED'),
];

test('chromium: webhooks a plan leaves out rest with their secrets; pausing and unsubscribing stay', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await onPlan(browser, ['WEBHOOKS'], (route, _request, path) => {
    if (path === '/integrations/webhooks') return route.fulfill({ json: SUBSCRIPTIONS });
    if (path === `/integrations/webhooks/${SUBSCRIPTIONS[0].id}/deliveries`) {
      return route.fulfill({ json: { data: [DEAD], page: { next_cursor: null, has_more: false } } });
    }
    return undefined;
  });
  t.after(() => context.close());

  await page.goto(`${served.origin}/administration/webhooks`);
  await page.getByText('2 subscriptions rest with their secrets; nothing is delivered.', { exact: false }).waitFor({ timeout: 5_000 });
  assert.ok(await page.getByText('Not included in your plan', { exact: true }).isVisible());
  assert.equal(await page.getByText('Subscribe a system').count(), 0, 'a new subscription is offered');
  assert.equal(await page.getByRole('button', { name: 'Switch it back on' }).count(), 0, 'resuming is offered');
  assert.equal(await page.getByRole('button', { name: 'Pause it' }).count(), 1);
  assert.equal(await page.getByRole('button', { name: 'Unsubscribe' }).count(), 2);
  // A dead letter is readable, and not sent again.
  await page.getByRole('button', { name: 'What was delivered' }).first().click();
  await page.getByText('503', { exact: false }).first().waitFor({ timeout: 5_000 });
  assert.equal(await page.getByRole('button', { name: 'Send it again' }).count(), 0, 'a replay is offered');
  assert.deepEqual(failures, []);
});

const OWN = {
  id: '01a0e2e0-0000-7000-8000-000000001941', scope: 'workspace', issuer: 'https://login.firma.example', client_id: 'hubtask',
  display_name: 'Company login', kind: 'GENERIC', provisioning: 'INVITED_ONLY', position: 0, enabled: true, offered_here: false,
  allowed_email_domains: [], allowed_directories: [], created_at: '2026-09-01T08:00:00Z', version: 1,
};

const OWN_OFF = { ...OWN, id: '01a0e2e0-0000-7000-8000-000000001942', display_name: 'Lab login', enabled: false };

const setting = (value) => ({ value, installation: value, lock: null, source: 'DEFAULT', installation_source: 'DEFAULT' });
const POLICY = {
  password: Object.fromEntries(['min_length', 'min_lowercase', 'min_uppercase', 'min_digits', 'min_symbols', 'min_classes', 'max_repeat', 'max_age_days', 'history_count', 'min_age_hours']
    .map((name) => [name, setting(name === 'min_length' ? 12 : 0)])
    .concat([['common_passwords', setting(true)], ['context_words', setting(true)], ['breach_check', setting(false)]])),
  mfa_required_for: setting('NOBODY'),
  methods: setting(['PASSWORD', 'OIDC']),
  session: { max_days: setting(30), idle_minutes: setting(0) },
  legal: { imprint_url: setting(''), privacy_url: setting(''), terms_url: setting(''), accessibility_url: setting(''), terms_version: setting('') },
  rotation_from: null,
};
const TENANT = {
  id: '01a0e2e0-0000-7000-8000-0000000005a1', slug: 'house', display_name: 'House', status: 'ACTIVE', default_locale: 'en',
  default_time_zone: 'UTC', require_admin_totp: false, created_at: '2026-08-01T08:00:00Z', version: 1, sign_in_policy: POLICY,
};

test('chromium: the workspace\'s own providers a plan leaves out sign nobody in, and nothing switches them on', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await onPlan(browser, ['OWN_SIGN_IN_PROVIDERS'], (route, _request, path) => {
    if (path === '/identity-providers') return route.fulfill({ json: [OWN, OWN_OFF] });
    if (path === '/tenant') return route.fulfill({ json: TENANT });
    return undefined;
  });
  t.after(() => context.close());

  // Configuring: the notice, the row resting, no new provider and no change; removing stays.
  await page.goto(`${served.origin}/administration/identity-provider`);
  await page.getByText('2 of this workspace\'s own providers sign nobody in; their connections stay.', { exact: false }).waitFor({ timeout: 5_000 });
  assert.ok(await page.getByText('Not included in your plan', { exact: true }).isVisible());
  assert.equal(await page.getByRole('button', { name: 'Add a provider' }).count(), 0, 'a new provider is offered');
  assert.equal(await page.getByRole('button', { name: 'Change' }).count(), 0, 'changing a resting provider is offered');
  assert.equal(await page.getByRole('button', { name: 'Remove' }).count(), 2);

  // The ways in: a resting provider that is on keeps only the way to switch it off; one that is
  // off says so in place of its switch.
  await page.goto(`${served.origin}/administration/sign-in`);
  await page.getByText('Lab login', { exact: true }).waitFor({ timeout: 5_000 });
  assert.equal(await page.getByLabel('Lab login').count(), 0, 'a resting provider that is off has a switch');
  assert.ok(await page.getByText('Not included in your plan', { exact: true }).isVisible());
  const on = page.getByLabel('Company login');
  assert.equal(await on.isChecked(), true);
  assert.ok(await page.getByText('Switched off here, it stays off when your plan includes it again.', { exact: false }).isVisible());
  assert.deepEqual(failures, []);
});
