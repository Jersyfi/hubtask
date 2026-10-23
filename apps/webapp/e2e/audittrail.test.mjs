// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The trail's screen as an owner reads it (UC-AUD-01): an erased actor is named by the stable
// pseudonym its entries carry, so two people erased are two names rather than one "former user"
// (check 3), and the screen says that reading adds nothing (check 6).
//
// Against a stubbed API. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const KIM = '01a0e2e0-0000-7000-8000-0000000000e1';
const SAM = '01a0e2e0-0000-7000-8000-0000000000e2';

const entry = (n, actorId, label) => ({
  id: `01a0e2e0-0000-7000-8000-00000000a${String(n).padStart(3, '0')}`, seq: n,
  occurred_at: `2026-10-0${n}T08:00:00Z`, action: 'container.created', outcome: 'SUCCESS',
  actor: { type: 'USER', id: actorId, label }, target: { type: 'container', id: null },
});

test('chromium: an erased actor is named by its pseudonym, and reading is said to add nothing', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/audit') {
      return route.fulfill({ json: { items: [
        entry(1, KIM, 'former-user-1a2b3c'), entry(2, KIM, 'former-user-1a2b3c'), entry(3, SAM, 'former-user-9f8e7d'),
        entry(4, ACCOUNT.id, ACCOUNT.display_name),
      ], next_cursor: null } });
    }
    for (const id of [KIM, SAM]) {
      if (path === `/accounts/${id}`) {
        return route.fulfill({ json: { id, kind: 'USER', display_name: 'former user', status: 'ANONYMIZED' } });
      }
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/administration/audit`);
  await page.getByText('former-user-9f8e7d').first().waitFor({ timeout: 15_000 });

  const text = await page.locator('main').innerText();
  assert.equal(text.match(/former-user-1a2b3c/g)?.length, 2, 'the same pseudonym is not on both of the first person\'s entries');
  assert.equal(text.match(/former-user-9f8e7d/g)?.length, 1);
  assert.doesNotMatch(text, /by former user\b/, 'two erased people read as one "former user"');
  assert.match(text, new RegExp(ACCOUNT.display_name), 'a person not erased lost their name');
  assert.match(text, /not recorded|adds no entry|is not itself recorded/i, 'the screen does not say reading adds nothing');
});

// UC-AUD-01 check 2: a sensitive field set for the first time carries only the hash of what it
// became; it reads as changed, never as having become nothing.
test('chromium: a sensitive field set for the first time reads as changed', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/audit') {
      return route.fulfill({ json: { items: [{
        ...entry(1, ACCOUNT.id, ACCOUNT.display_name),
        changes: [
          { field: 'name', changed: true, to_hash: 'hmac-sha256:0f1e2d' },
          { field: 'colour', from: 'blue', to: 'green' },
        ],
      }], next_cursor: null } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/administration/audit`);
  await page.getByText('colour became green').waitFor({ timeout: 15_000 });

  const text = await page.locator('main').innerText();
  assert.match(text, /name changed — the trail keeps that it changed/);
  assert.doesNotMatch(text, /name became/);
});

test('chromium: the next page of the trail is announced, and its end is said', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/audit' && url.searchParams.get('cursor') === 'c2') {
      return route.fulfill({ json: { items: [entry(3, ACCOUNT.id, ACCOUNT.display_name), entry(4, ACCOUNT.id, ACCOUNT.display_name)], next_cursor: null } });
    }
    if (path === '/audit') {
      return route.fulfill({ json: { items: [entry(1, ACCOUNT.id, ACCOUNT.display_name), entry(2, ACCOUNT.id, ACCOUNT.display_name)], next_cursor: 'c2' } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/administration/audit`);
  const more = page.getByRole('button', { name: 'Show more' });
  await more.waitFor({ timeout: 15_000 });
  // The live region is there, empty, before anything arrives: one that appears already filled is
  // not announced.
  const status = page.locator('.load-more [role="status"]');
  assert.equal(await status.count(), 1);
  assert.equal((await status.textContent()).trim(), '');
  await more.click();
  await page.getByText('That is every entry for this query.').waitFor();
  assert.equal((await status.textContent()).trim(), '2 more entries arrived.');
  assert.equal(await more.count(), 0, 'the button stayed after the last page');
});

// UC-WRK-32 check 3: checking and exporting are never unavailable for an empty period or target;
// pressed, they mark what is missing, move focus to the first, and send nothing.
test('chromium: checking and exporting with nothing named mark the fields and send nothing', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  const sent = [];
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (route.request().method() === 'POST' && path.startsWith('/audit')) sent.push(path);
    if (path === '/audit') return route.fulfill({ json: { items: [], next_cursor: null } });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/administration/audit`);
  const check = page.getByRole('button', { name: 'Check the chain' });
  await check.waitFor({ timeout: 15_000 });
  assert.equal(await check.getAttribute('aria-disabled'), null, 'the check is unavailable before it is pressed');

  await check.click();
  // The filter above has a From of its own; the check's is the one that is marked.
  const from = page.getByRole('textbox', { name: 'From', description: 'Name a period first.' });
  await page.waitForFunction(() => document.activeElement?.getAttribute('aria-invalid') === 'true');
  assert.equal(await from.evaluate((field) => field === document.activeElement), true, 'focus is not on the first missing field');
  assert.equal(await page.getByText('Name a period first.').count(), 2);

  const exporting = page.getByRole('button', { name: 'Export the period' });
  assert.equal(await exporting.getAttribute('aria-disabled'), null);
  await exporting.click();
  await page.getByText('Choose a target first.').waitFor();
  // The export's own period is what is marked: it borrows nothing from the check above it.
  const exportFrom = page
    .locator('div')
    .filter({ has: page.getByRole('heading', { name: 'Take a copy away' }) })
    .last()
    .getByRole('textbox', { name: 'From', description: 'Name a period first.' });
  assert.equal(await exportFrom.evaluate((field) => field === document.activeElement), true, 'focus is not on the export\'s own period');
  await page.waitForTimeout(200);
  assert.deepEqual(sent, [], 'a press with something missing sent a request');
});

// ux-inventory §E8: the export has a period of its own, sent as named, whatever the check holds.
test('chromium: the export takes away its own period, not the one checked above it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  const sent = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/audit' && request.method() === 'GET') return route.fulfill({ json: { items: [], next_cursor: null } });
    if (path === '/backup-targets' && request.method() === 'GET') {
      return route.fulfill({ json: [{ id: 'bt-export', name: 'Basement disk', kind: 'WEBDAV', created_at: '2026-09-01T00:00:00Z' }] });
    }
    if (path === '/audit:export') {
      sent.push(request.postDataJSON());
      return route.fulfill({ status: 202, json: { job_id: '01a0e2e0-0000-7000-8000-0000000007e1' } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/administration/audit`);
  const section = page.locator('section, div').filter({ has: page.getByRole('heading', { name: 'Take a copy away' }) }).last();
  await section.getByRole('button', { name: 'Export the period' }).waitFor({ timeout: 15_000 });
  // The check's period is left empty: the export no longer reads it.
  await section.getByRole('textbox', { name: 'From' }).fill('2026-09-01T00:00');
  await section.getByRole('textbox', { name: 'To' }).fill('2026-09-30T23:59');
  await section.getByLabel('Write it to').selectOption('bt-export');
  const asked = page.waitForRequest((request) => request.url().endsWith('/audit:export'));
  await section.getByRole('button', { name: 'Export the period' }).click();
  await asked;
  assert.equal(sent.length, 1, 'the export was not sent');
  assert.equal(sent[0].target_id, 'bt-export');
  assert.equal(sent[0].from, new Date('2026-09-01T00:00').toISOString());
  assert.equal(sent[0].to, new Date('2026-09-30T23:59').toISOString());
});
