// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A webhook subscription narrowed by a condition and a hub or collection (UC-INT-01 check 9): the
// form sends both as the contract spells them, a stored subscription says where it listens and
// what it waits for, and an event the condition could not judge reads as held back. Against the
// stubbed API; Chromium only, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { COLLECTION, HUB, MANIFEST, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const CREATED = 'de.hubtask.work.item.created.v1';

const SUBSCRIPTION = {
  id: '01a0e2e0-0000-7000-8000-000000001801',
  target_url: 'https://example.org/hooks/kitchen',
  event_types: [CREATED],
  state: 'ACTIVE',
  failure_count: 0,
  filter: 'data.title.startsWith("Ship")',
  scope: { type: 'HUB', id: HUB.id },
  created_at: '2026-10-01T08:00:00Z',
  version: 1,
};

const HELD = {
  id: '01a0e2e0-0000-7000-8000-000000001811',
  subscription_id: SUBSCRIPTION.id,
  event_id: '01a0e2e0-0000-7000-8000-0000000018e1',
  attempt: 0,
  status: 'FILTER_FAILED',
  error_code: 'expression.evaluation_failed',
  created_at: '2026-10-02T08:00:00Z',
};

test('chromium: a subscription takes a condition and a scope, and says both back', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 1280, 1400);
  t.after(() => context.close());

  let sent;
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.pathname.endsWith('/meta/capabilities')) {
      return route.fulfill({ json: { ...MANIFEST, event_types: [CREATED] } });
    }
    if (url.pathname.endsWith('/integrations/webhooks') && request.method() === 'POST') {
      sent = request.postDataJSON();
      return route.fulfill({ status: 201, json: { ...SUBSCRIPTION, ...sent, id: '01a0e2e0-0000-7000-8000-000000001802', secret: 'whsec_e2e' } });
    }
    if (url.pathname.endsWith('/integrations/webhooks')) return route.fulfill({ json: [SUBSCRIPTION] });
    if (url.pathname.endsWith(`/integrations/webhooks/${SUBSCRIPTION.id}/deliveries`)) {
      return route.fulfill({ json: { data: [HELD], page: { next_cursor: null, has_more: false } } });
    }
    return stub(route);
  });

  await page.goto(`${served.origin}/administration/webhooks`);

  // What a stored subscription listens to, and what it waits for: its record page (page-system §9.3).
  await page.getByRole('link', { name: SUBSCRIPTION.target_url }).first().click({ timeout: 10_000 });
  await page.getByText(`Listens to: The hub ${HUB.name}, and everything in it`).waitFor({ timeout: 5_000 });
  assert.ok(await page.getByText(SUBSCRIPTION.filter, { exact: true }).isVisible(), 'the condition is not shown');

  // An event held back reads as such, with no attempt counted.
  const deliveries = page.locator('main tbody');
  await deliveries.getByText('Held back: the condition could not be judged').waitFor({ timeout: 5_000 });
  assert.equal(await deliveries.getByText(/^Attempt/).count(), 0);

  // The form sends both, the scope with its level; the scope and the condition are its Advanced.
  await page.goto(`${served.origin}/administration/webhooks`);
  await page.getByRole('button', { name: 'Add a webhook' }).first().click();
  const dialog = page.locator('dialog[open]');
  await dialog.getByLabel('Where to deliver').fill('https://example.org/hooks/narrow');
  await dialog.getByRole('button', { name: /^Advanced/ }).click();
  await dialog.getByLabel('An entry is created', { exact: true }).check();
  await dialog.getByLabel('Where it listens').selectOption({ label: `The collection ${HUB.name} · ${COLLECTION.name}` });
  await dialog.getByLabel('Only when (optional)').fill('data.title.startsWith("Ship")');
  const posted = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/integrations/webhooks'), { timeout: 5_000 });
  await dialog.getByRole('button', { name: 'Subscribe', exact: true }).last().click();
  await posted;

  assert.deepEqual(sent, {
    target_url: 'https://example.org/hooks/narrow',
    event_types: [CREATED],
    filter: 'data.title.startsWith("Ship")',
    scope: { type: 'COLLECTION', id: COLLECTION.id },
  });
  assert.deepEqual(failures, []);
});

// The column counts a connection switched off after failures on the *Webhooks* row, on every
// screen of the administration, before the webhooks are opened (user-experience.md §7.10).
test('chromium: the column counts a webhook switched off after failures', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  const OFF = { ...SUBSCRIPTION, id: '01a0e2e0-0000-7000-8000-000000001803', state: 'DISABLED', failure_count: 12 };
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => (new URL(route.request().url()).pathname.endsWith('/integrations/webhooks')
    ? route.fulfill({ json: [SUBSCRIPTION, OFF] })
    : stub(route)));

  await page.goto(`${served.origin}/administration/people`);
  const column = page.getByRole('navigation', { name: 'Administration', exact: true });
  await column.getByText('1 switched off').waitFor({ timeout: 10_000 });
  assert.equal(await column.getByRole('treeitem', { name: /Webhooks.*1 switched off/ }).count(), 1, 'the figure is not heard with the row');
  assert.deepEqual(failures, []);
});
