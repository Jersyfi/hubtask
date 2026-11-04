// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *Import a file*, a mode of capture opened in a collection, lands the file there (user-experience.md
// §7.11): the dialog says where the entries go, the source is a card, the check is a dry run said
// in words before anything is written, the request names the collection and its hub, the report
// arrives in the same words, no collection is offered to open, and the collection's entries are
// read again once the job is done - the job writes without a change log entry, so the stream would
// not say. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, COLLECTION, HUB, ITEMS, MANIFEST, OWNER_GRANTS, PAGE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

// Identifiers no other fixture record uses.
const MEDIA_ID = '01a0e2e0-0000-7000-8000-0000000017a1';
const JOB_ID = '01a0e2e0-0000-7000-8000-0000000017a2';
const IMPORT_ID = '01a0e2e0-0000-7000-8000-0000000017a3';
const CHECK_JOB_ID = '01a0e2e0-0000-7000-8000-0000000017a4';
const CHECK_ID = '01a0e2e0-0000-7000-8000-0000000017a5';

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
    // The installation's upload limit, said before a file is chosen.
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, limits: { ...MANIFEST.limits, max_upload_bytes: 25 * 1024 * 1024 } } });
    if (path === '/media' && request.method() === 'POST') return route.fulfill({ status: 201, json: object('PENDING') });
    if (path === `/media/${MEDIA_ID}:confirm`) return route.fulfill({ json: object('READY') });
    if (path === '/imports' && request.method() === 'POST') {
      const body = request.postDataJSON();
      sent.push(body);
      const [job, run] = body.dry_run ? [CHECK_JOB_ID, CHECK_ID] : [JOB_ID, IMPORT_ID];
      return route.fulfill({ status: 202, json: { job_id: job, status: 'QUEUED', progress: null, result_url: `/imports/${run}` } });
    }
    for (const [job, run] of [[JOB_ID, IMPORT_ID], [CHECK_JOB_ID, CHECK_ID]]) {
      if (path === `/jobs/${job}`) {
        return route.fulfill({ json: { job_id: job, status: 'SUCCEEDED', progress: 1, result_url: `/imports/${run}`, error_code: null, created_at: now, finished_at: now } });
      }
    }
    if (path === `/imports/${CHECK_ID}`) {
      return route.fulfill({
        json: {
          id: CHECK_ID, kind: 'CSV', hub_id: HUB.id, collection_id: COLLECTION.id, media_id: MEDIA_ID, dry_run: true, status: 'SUCCEEDED',
          report: { new: 2, entities: { work_items: 2 } }, refused: [{ row: 3, code: 'imports.row_title_missing' }],
          error_code: null, created_at: now, finished_at: now,
        },
      });
    }
    if (path === `/imports/${IMPORT_ID}`) {
      // The entries are read again only once the report says the job landed something.
      await new Promise((resolve) => setTimeout(resolve, 300));
      reportRead = true;
      return route.fulfill({
        json: {
          id: IMPORT_ID, kind: 'CSV', hub_id: HUB.id, collection_id: COLLECTION.id, media_id: MEDIA_ID, dry_run: false, status: 'SUCCEEDED',
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
  // Importing is one of capture's ways of adding: a tab, the import in its panel, the dialog the same.
  await page.getByRole('main').getByRole('button', { name: 'Add an entry', exact: true }).first().click();
  await page.getByRole('dialog', { name: 'Capture' }).getByRole('tab', { name: 'Import a file' }).click();

  const dialog = page.getByRole('dialog', { name: 'Capture' }).getByRole('tabpanel');
  await dialog.waitFor({ timeout: 10_000 });
  assert.equal(await dialog.getByText(`The entries land in ${COLLECTION.name}.`, { exact: false }).count(), 1, 'the dialog does not say where the entries land');
  // Where from: the sources as cards; one press chooses and moves on.
  await dialog.getByRole('button', { name: 'Choose A spreadsheet (CSV)' }).click();
  await dialog.getByText('Files up to', { exact: false }).first().waitFor({ timeout: 5_000 });
  await dialog.locator('input[type="file"]').setInputFiles({ name: 'errands.csv', mimeType: 'text/csv', buffer: Buffer.from('title\nMilk\nBread\n') });
  await dialog.getByText('errands.csv is uploaded and ready.').waitFor({ timeout: 10_000 });
  await dialog.getByRole('button', { name: 'Next' }).click();
  // The check: the header names the title, so the dry run starts at once and says its plan in words.
  const plan = dialog.getByRole('region', { name: 'What the import would do' });
  await plan.getByText('Creates 2 entries.').waitFor({ timeout: 10_000 });
  assert.equal(await plan.getByText('1 row would not land', { exact: false }).count(), 1, 'the plan does not say the row that would not land');
  assert.equal(await dialog.getByText('Row 3:', { exact: false }).count(), 1, 'the row that would not land is not listed');
  assert.equal(await dialog.getByText('imports.', { exact: false }).count(), 0, 'a key is shown');
  assert.equal(sent.length, 1, 'the check is one dry run');
  assert.equal(sent[0].dry_run, true, 'the check is not a dry run');
  await dialog.getByRole('button', { name: 'Import', exact: true }).click();
  await dialog.getByText('The import finished.', { exact: false }).first().waitFor({ timeout: 10_000 });
  await dialog.getByRole('region', { name: 'What the import did' }).getByText('Created 2 entries.').waitFor({ timeout: 5_000 });

  assert.equal(sent.length, 2, 'no import was asked');
  assert.equal(sent[1].dry_run, false, 'the import is asked as a dry run');
  assert.equal(sent[1].media_id, sent[0].media_id, 'the import names another file than the check');
  assert.equal(sent[1].collection_id, COLLECTION.id, 'the import does not name the collection');
  assert.equal(sent[1].hub_id, COLLECTION.parent_id, 'the import does not name the collection\'s hub');
  assert.equal(sent[1].hub_id, HUB.id);
  assert.equal(await dialog.getByRole('link', { name: /^Open / }).count(), 0, 'a collection is offered to open');
  await page.waitForTimeout(300);
  assert.ok(queriesAfterReport > 0, 'the collection\'s entries were not read again after the job');
  assert.deepEqual(failures, []);
});
