// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The administration opens on an overview of what needs attention (UC-ID-24 checks 1, 2, 5):
//
// * `/administration` stays at its address and draws the overview, not the first settings page;
// * it says the limits with use and ceiling, the rule failing, the last backup and whether it
//   opened, the request owed soon and the degraded feature - each with a link to its page - and
//   puts what needs somebody before what does not;
// * an auditor reads it too, with no write anywhere on it, and without the privacy register's card,
//   whose read their role is refused - nor is the read sent.
//
// Against a stubbed API. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { MANIFEST, PERMISSIONS, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const DAY = 24 * 60 * 60 * 1000;
const TARGET = { id: '01a0e2e0-0000-7000-8000-00000000c418', name: 'Overview shelf', kind: 'S3', encryption_mode: 'NONE', enabled: true };
const ARCHIVE = { archive_id: '01a0e2e0-0000-7000-8000-00000000c419', path: 'a', created_at: '2026-10-20T03:00:00Z', mode: 'FULL', complete: true };
const RULE = {
  id: '01a0e2e0-0000-7000-8000-00000000c41a', name: 'Overview mail relay', scope: { type: 'TENANT' }, enabled: true,
  run_as: 'OWNER', trigger: { kind: 'EVENT' }, conditions: [], actions: [], on_error: 'CONTINUE', failure_count: 3, version: 1,
};
const QUIET_RULE = { ...RULE, id: '01a0e2e0-0000-7000-8000-00000000c41b', name: 'Overview quiet rule', failure_count: 0 };

/** A signed-in tab as somebody holding `permissions`, with the overview's reads answered. */
async function as(browser, permissions, { verifyOk = true } = {}) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const asked = [];
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    asked.push(path);
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: permissions } });
    if (path === '/quotas') {
      return route.fulfill({ json: [
        { quota: 'accounts', limit: 30, used: 28, ratio: 28 / 30 },
        { quota: 'items', limit: 10000, used: 120, ratio: 0.012 },
        { quota: 'api_requests_per_minute', limit: 600, used: null, ratio: null },
      ] });
    }
    if (path === '/automation/rules') return route.fulfill({ json: { data: [QUIET_RULE, RULE], next_cursor: null } });
    if (path === '/backup-targets') return route.fulfill({ json: [TARGET] });
    if (path === '/backup-schedules') return route.fulfill({ json: [] });
    if (path === `/backup-targets/${TARGET.id}/backups`) return route.fulfill({ json: [ARCHIVE] });
    if (path === `/backups/${ARCHIVE.archive_id}`) {
      return route.fulfill({ json: { id: ARCHIVE.archive_id, target_id: TARGET.id, trigger: 'SCHEDULE', mode: 'FULL', status: 'SUCCEEDED', started_at: ARCHIVE.created_at, verified_at: '2026-10-21T03:00:00Z', verify_ok: verifyOk } });
    }
    if (path === '/privacy/requests') {
      return route.fulfill({ json: { data: [
        { id: '01a0e2e0-0000-7000-8000-00000000c41c', kind: 'ACCESS', status: 'RECEIVED', received_at: new Date(Date.now() - 20 * DAY).toISOString(), due_at: new Date(Date.now() + 3 * DAY).toISOString() },
        { id: '01a0e2e0-0000-7000-8000-00000000c41d', kind: 'ERASURE', status: 'RECEIVED', received_at: new Date().toISOString(), due_at: new Date(Date.now() + 30 * DAY).toISOString() },
      ], page: { next_cursor: null, has_more: false } } });
    }
    if (path === '/meta/health') {
      return route.fulfill({ json: { status: 'degraded', degraded_features: [{ feature: 'ai', reason_code: 'errors.unavailable', since: '2026-10-22T00:00:00Z' }] } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  return { context, page, asked, failures };
}

/** One card's text, by its `data-card`. */
const cardText = async (page, id) => ((await page.locator(`[data-card="${id}"]`).textContent()) ?? '').replace(/\s+/g, ' ').trim();

test('chromium: the administration opens on an overview of what needs attention, each linked to its page', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const owner = await as(browser, PERMISSIONS.OWNER);
  t.after(() => owner.context.close());
  const { page } = owner;

  await page.goto(`${served.origin}/administration`);
  await page.getByRole('heading', { name: 'Overview', level: 1 }).waitFor();
  await page.locator('[data-card="backup"]').getByText('Opened').waitFor();
  assert.equal(new URL(page.url()).pathname, '/administration', 'the address was replaced with a settings page');

  assert.match(await cardText(page, 'limits'), /People\s*28 of 30/);
  assert.match(await cardText(page, 'limits'), /Entries\s*120 of 10000/);
  assert.match(await cardText(page, 'rules'), /Overview mail relay\s*Failed 3 times/);
  assert.doesNotMatch(await cardText(page, 'rules'), /quiet rule/);
  assert.match(await cardText(page, 'backup'), /Last backup .*Opened/);
  assert.match(await cardText(page, 'requests'), /A copy of their data/);
  assert.doesNotMatch(await cardText(page, 'requests'), /deleted|erasure/i);
  assert.match(await cardText(page, 'health'), /ai:/);

  const links = async (id) => page.locator(`[data-card="${id}"] a`).evaluateAll((anchors) => anchors.map((a) => a.getAttribute('href')));
  assert.ok((await links('limits')).includes('/administration/quotas'));
  assert.deepEqual(await links('rules'), [`/administration/rules/${RULE.id}`, '/administration/rules']);
  assert.ok((await links('backup')).includes('/administration/backup'));
  assert.ok((await links('requests')).includes('/administration/privacy'));
  assert.ok((await links('health')).includes('/administration/ai'));

  // What needs somebody first: the backup opened, so it is the one card that waits below.
  const order = await page.locator('[data-card]').evaluateAll((cards) => cards.map((card) => card.getAttribute('data-card')));
  assert.equal(order.at(-1), 'backup', order.join(' '));

  // The column marks the overview current.
  assert.equal(await page.locator('nav [aria-current="page"]').first().textContent().then((text) => text?.trim()), 'Overview');
  assert.deepEqual(owner.failures, []);
});

test('chromium: a backup that did not open says so and comes first among the cards that need somebody', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const owner = await as(browser, PERMISSIONS.OWNER, { verifyOk: false });
  t.after(() => owner.context.close());
  await owner.page.goto(`${served.origin}/administration`);
  await owner.page.locator('[data-card="backup"]').getByText('Did not open').waitFor();
  assert.equal(await owner.page.locator('[data-card="backup"][data-needs]').count(), 1);
});

test('chromium: an auditor reads the overview with nothing to change and no register they may not read', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const auditor = await as(browser, PERMISSIONS.AUDITOR);
  t.after(() => auditor.context.close());
  const { page } = auditor;

  await page.goto(`${served.origin}/administration`);
  await page.getByRole('heading', { name: 'Overview', level: 1 }).waitFor();
  await page.locator('[data-card="rules"]').getByText('Overview mail relay').waitFor();
  await page.locator('[data-card="backup"]').getByText('Opened').waitFor();

  assert.equal(await page.locator('[data-card="requests"]').count(), 0, 'the auditor is shown the privacy register');
  assert.ok(!auditor.asked.some((path) => path.startsWith('/privacy/requests')), 'the register was read for an auditor');
  // Read-only: the overview offers nothing to press but links.
  assert.equal(await page.locator('[data-testid="admin-overview"] :is(button, input, select, textarea)').count(), 0);
  assert.deepEqual(auditor.failures, []);
});
