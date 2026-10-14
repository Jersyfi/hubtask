// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A next page that did not arrive, on every list that reads by cursor: the reason is shown beside
// the button and announced, the rows already shown stay, and the button is the retry. The page
// fails once, in the ways a reader meets - the server failing, the connection gone, a refusal -
// and answers on the second press. Chromium only, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { PRIVACY_REQUESTS, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const SERVER_FAULT = 'Something went wrong. Try again in a moment.';

/** How the second page fails, once. */
const FAILURES = {
  serverFault: (route) => route.fulfill({ status: 503, contentType: 'application/problem+json', body: '' }),
  offline: (route) => route.abort('internetdisconnected'),
  forbidden: (route) => route.fulfill({ status: 403, contentType: 'application/problem+json', body: JSON.stringify({ status: 403, code: 'forbidden' }) }),
};

/**
 * Routes one list: the first page with a cursor, the second page failing once and then answering.
 * `answer(cursor)` builds the body for a page; everything else is the shared fixture.
 */
async function listFailingOnce(context, { endsWith, failure, answer }) {
  let failed = 0;
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    if (route.request().method() === 'GET' && url.pathname.endsWith(endsWith)) {
      const cursor = url.searchParams.get('cursor');
      if (cursor === 'p2' && failed === 0) {
        failed += 1;
        return FAILURES[failure](route);
      }
      return route.fulfill({ json: answer(cursor) });
    }
    return stub(route);
  });
}

const alert = (page) => page.locator('.load-more [role="alert"]');
const status = (page) => page.locator('.load-more [role="status"]');
const said = async (locator) => ((await locator.textContent()) ?? '').trim();
const until = (page, selector) =>
  page.waitForFunction((s) => (document.querySelector(s)?.textContent ?? '').trim() !== '', selector, { timeout: 5_000 }).catch(() => {});

/**
 * The walk every list shares: the alert region mounted empty, the failure said and shown with the
 * rows and the button kept, then the retry clearing it and saying what arrived.
 */
async function failsThenRetries(page, { button, rows, failure, arrived }) {
  const more = page.getByRole('button', { name: button });
  await more.waitFor({ timeout: 10_000 });
  assert.equal(await alert(page).count(), 1, 'there is no alert region beside the button');
  assert.equal(await said(alert(page)), '', 'the alert region was mounted already filled');
  const before = await rows().count();

  await more.click();
  await until(page, '.load-more [role="alert"]');
  assert.equal(await said(alert(page)), failure);
  assert.ok(await alert(page).isVisible(), 'the failure is announced but not shown');
  assert.equal(await more.count(), 1, 'the button went away with the failure, so there is no retry');
  assert.equal(await rows().count(), before, 'the rows already shown were lost');

  await more.click();
  await until(page, '.load-more [role="status"]');
  assert.equal(await said(alert(page)), '', 'the failure outlived the retry');
  assert.equal(await said(status(page)), arrived);
  assert.equal(await rows().count(), before + 1);
}

const RUN = (n) => ({
  id: `01a0e2e0-0000-7000-8000-0000000014${String(n).padStart(2, '0')}`,
  rule_id: '01a0e2e0-0000-7000-8000-000000001400',
  trigger: 'EVENT',
  status: 'SUCCEEDED',
  started_at: `2026-10-0${n}T08:00:00Z`,
  causation_depth: 1,
  condition_results: [],
  action_results: [],
});

test('chromium: a page of runs that did not arrive is said beside the button, the retry brings it, and a new filter clears the line', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 1280, 1000);
  t.after(() => context.close());
  await listFailingOnce(context, {
    endsWith: '/automation/runs',
    failure: 'serverFault',
    answer: (cursor) => (cursor === 'p2' ? { data: [RUN(3)], next_cursor: null } : { data: [RUN(1), RUN(2)], next_cursor: 'p2' }),
  });
  await page.goto(`${served.origin}/administration/runs`);
  await failsThenRetries(page, {
    button: 'Show more',
    rows: () => page.locator('.run .name'),
    failure: SERVER_FAULT,
    arrived: '1 more run arrived.',
  });

  // A failure, then a narrower listing: neither line belongs to the list now shown.
  await listFailingOnce(context, {
    endsWith: '/automation/runs',
    failure: 'serverFault',
    answer: (cursor) => (cursor === 'p2' ? { data: [RUN(3)], next_cursor: null } : { data: [RUN(1), RUN(2)], next_cursor: 'p2' }),
  });
  await page.getByLabel('From').fill('2026-10-01T00:00');
  await page.getByRole('button', { name: 'Show more' }).click();
  await until(page, '.load-more [role="alert"]');
  assert.equal(await said(alert(page)), SERVER_FAULT);
  await page.getByLabel('From').fill('2026-09-01T00:00');
  await page.waitForFunction(() => (document.querySelector('.load-more [role="alert"]')?.textContent ?? '').trim() === '', null, { timeout: 5_000 }).catch(() => {});
  assert.equal(await said(alert(page)), '', 'a new filter kept the old failure');
  assert.equal(await said(status(page)), '', 'a new filter kept the old arrival');
  assert.deepEqual(failures, []);
});

const ARRIVAL = (n) => ({
  id: `01a0e2e0-0000-7000-8000-0000000015${String(n).padStart(2, '0')}`,
  channel: 'QUICK_CAPTURE',
  raw_subject: `Arrival ${n}`,
  raw_body: null,
  attachments: [],
  status: 'NEW',
  received_at: `2026-10-0${n}T08:00:00Z`,
});

test('chromium: a page of the jumble that did not arrive because the connection was gone is said beside the button, and the retry brings it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 1280, 1000);
  t.after(() => context.close());
  await listFailingOnce(context, {
    endsWith: '/jumble/entries',
    failure: 'offline',
    answer: (cursor) => (cursor === 'p2' ? { data: [ARRIVAL(3)], next_cursor: null } : { data: [ARRIVAL(1), ARRIVAL(2)], next_cursor: 'p2' }),
  });
  await page.goto(`${served.origin}/jumble`);
  await failsThenRetries(page, {
    button: 'Show more',
    rows: () => page.locator('article.entry'),
    // The replica is attached and cannot answer this path, so the engine says it needs the server.
    failure: 'This needs a connection. It is not kept on this device, and the server cannot be reached right now.',
    arrived: '1 more arrival is listed.',
  });
  assert.deepEqual(failures, []);
});

test('chromium: a page of privacy cases that was refused is said beside the button, and the retry brings it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 1280, 1000);
  t.after(() => context.close());
  const later = { ...PRIVACY_REQUESTS[0], id: '01a0e2e0-0000-7000-8000-0000000016a1', subject_email: 'later@example.invalid' };
  await listFailingOnce(context, {
    endsWith: '/privacy/requests',
    failure: 'forbidden',
    answer: (cursor) => (cursor === 'p2'
      ? { data: [later], page: { next_cursor: null, has_more: false } }
      : { data: PRIVACY_REQUESTS.slice(0, 2), page: { next_cursor: 'p2', has_more: true } }),
  });
  await page.goto(`${served.origin}/administration/privacy`);
  await failsThenRetries(page, {
    button: 'Show more',
    rows: () => page.locator('section.panel'),
    failure: 'You do not have permission for this action.',
    arrived: '1 more case is listed.',
  });
  assert.equal(await page.getByText('later@example.invalid').count(), 1, 'the retried page is not the one drawn');
  assert.deepEqual(failures, []);
});
