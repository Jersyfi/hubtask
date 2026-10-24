// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *Import a file*, a mode of capture opened in a collection, lands the file there (design-system.md §11.8):
// the dialog says where the entries go, the request names the collection and its hub, the report
// arrives, no collection is offered to open, and the collection's entries are read again once the
// job is done - the job writes without a change log entry, so the stream would not say.
// Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, COLLECTION, HUB, ITEMS, OWNER_GRANTS, PAGE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

// Identifiers no other fixture record uses.
const MEDIA_ID = '01a0e2e0-0000-7000-8000-0000000017a1';
const JOB_ID = '01a0e2e0-0000-7000-8000-0000000017a2';
const IMPORT_ID = '01a0e2e0-0000-7000-8000-0000000017a3';

const now = new Date().toISOString();
const object = (status) => ({
  id: MEDIA_ID, file_name: 'errands.csv', content_type: 'text/csv', size: 22, usage: 'IMPORT', status,
  created_at: now, ...(status === 'PENDING' ? { upload: { url: '/e2e-upload/errands', method: 'PUT' } } : {}),
});

test('chromium: Import a file, from capture in a collection, lands the file there and reads its entries again', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  const sent = [];
  let queriesAfterReport = 0;
  let reportRead = false;
  await context.route('**/e2e-upload/**', (route) => route.fulfill({ status: 200, body: '' }));
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/memberships') {
      const scope = new URL(request.url()).searchParams.get('scope_type');
      const rows = scope === 'TENANT' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'OWNER' }] : [];
      return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: OWNER_GRANTS } });
    }
    if (path === '/media' && request.method() === 'POST') return route.fulfill({ status: 201, json: object('PENDING') });
    if (path === `/media/${MEDIA_ID}:confirm`) return route.fulfill({ json: object('READY') });
    if (path === '/imports' && request.method() === 'POST') {
      sent.push(request.postDataJSON());
      return route.fulfill({ status: 202, json: { job_id: JOB_ID, status: 'QUEUED', progress: null, result_url: `/imports/${IMPORT_ID}` } });
    }
    if (path === `/jobs/${JOB_ID}`) {
      return route.fulfill({ json: { job_id: JOB_ID, status: 'SUCCEEDED', progress: 1, result_url: `/imports/${IMPORT_ID}`, error_code: null, created_at: now, finished_at: now } });
    }
    if (path === `/imports/${IMPORT_ID}`) {
      // The entries are read again only once the report says the job landed something.
      await new Promise((resolve) => setTimeout(resolve, 300));
      reportRead = true;
      return route.fulfill({
        json: {
          id: IMPORT_ID, kind: 'CSV', hub_id: HUB.id, collection_id: COLLECTION.id, media_id: MEDIA_ID, status: 'SUCCEEDED',
          report: { new: 2, overwritten: 0, skipped: 0, duplicated: 0, conflicts: 0, deleted: 0, entities: { work_items: 2 } },
          refused: [], error_code: null, created_at: now, finished_at: now,
        },
      });
    }
    if (path === '/items:query' && reportRead) queriesAfterReport++;
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));

  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.getByText(ITEMS[2].title).first().waitFor();
  await page.waitForTimeout(600);
  await page.locator('header.head').getByRole('button', { name: 'Add an entry', exact: true }).click();
  await page.getByRole('dialog', { name: 'Capture' }).getByRole('button', { name: 'Import a file' }).click();
  await page.getByRole('dialog', { name: 'Capture' }).waitFor({ state: 'hidden', timeout: 5_000 });

  const dialog = page.locator('dialog[open]');
  await dialog.waitFor({ timeout: 10_000 });
  assert.equal(await dialog.getByText(`The entries land in ${COLLECTION.name}.`, { exact: false }).count(), 1, 'the dialog does not say where the entries land');
  await dialog.getByRole('button', { name: 'Next' }).click();
  await dialog.locator('input[type="file"]').setInputFiles({ name: 'errands.csv', mimeType: 'text/csv', buffer: Buffer.from('title\nMilk\nBread\n') });
  await dialog.getByText('errands.csv is uploaded and ready.').waitFor({ timeout: 10_000 });
  await dialog.getByRole('button', { name: 'Next' }).click();
  await dialog.getByRole('button', { name: 'Import', exact: true }).click();
  await dialog.getByText('The import finished.', { exact: false }).first().waitFor({ timeout: 10_000 });

  assert.equal(sent.length, 1, 'no import was asked');
  assert.equal(sent[0].collection_id, COLLECTION.id, 'the import does not name the collection');
  assert.equal(sent[0].hub_id, COLLECTION.parent_id, 'the import does not name the collection\'s hub');
  assert.equal(sent[0].hub_id, HUB.id);
  assert.equal(await dialog.getByRole('link', { name: /^Open / }).count(), 0, 'a collection is offered to open');
  await page.waitForTimeout(300);
  assert.ok(queriesAfterReport > 0, 'the collection\'s entries were not read again after the job');
  assert.deepEqual(failures, []);
});
