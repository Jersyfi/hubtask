// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A project file brought into an empty plan in the import's four steps (user-experience.md §7.11,
// UC-PLN-09): the empty plan offers it, the source is a card, the check is a dry run said in words
// before anything is written, the file's own format wins over the card, and the import reports in
// the same words. Chromium only, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { COLLECTION, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const EMPTY = { zone: 'UTC', finish: null, entries: [], dependencies: [], conflicts: [], unscheduled: [], undated: [], critical_path: [] };
const counts = { tasks: 2, work_packages: 0, activities: 0, milestones: 1, dependencies: 1, flattened: 0 };

test('chromium: an empty plan brings a project file in through a check said in words', async () => {
  const browser = await chromium.launch();
  try {
    const sent = [];
    const { page, failures, context } = await signedIn(browser, 1280, 900, { timezoneId: 'UTC' });
    await context.unroute('**/api/v1/**');
    await context.route('**/api/v1/**', async (route) => {
      const request = route.request();
      const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
      if (path === `/plans/${COLLECTION.id}/schedule`) return route.fulfill({ json: EMPTY });
      if (path === `/plans/${COLLECTION.id}/baselines`) return route.fulfill({ json: { data: [] } });
      if (path === `/plans/${COLLECTION.id}/tolerance`) {
        return route.fulfill({ json: { time_tolerance_days: null, source: 'DEFAULT', locked_by: null, baseline_id: null, exception: null } });
      }
      if (path === `/collections/${COLLECTION.id}/plan:import`) {
        const body = request.postDataJSON();
        sent.push(body);
        return route.fulfill({ json: { dry_run: body.dry_run, ...counts, created: body.dry_run ? [] : [{ key: '1', item_id: COLLECTION.id }] } });
      }
      return stub(route);
    });
    await page.goto(`${served.origin}/collections/${COLLECTION.id}/plan`);
    await page.getByText('Nothing to plan here yet').waitFor({ timeout: 15_000 });
    await page.getByRole('button', { name: 'Bring in a project file' }).first().click();

    const dialog = page.getByRole('dialog', { name: 'Bring in a project file' });
    await dialog.getByText('Step 1 of 4: Where from').waitFor({ timeout: 5_000 });
    // The card says CSV, the file is a project tool's XML: what the file is wins.
    await dialog.getByRole('button', { name: 'Choose A table (CSV)' }).click();
    await dialog.getByText('At most 524.3 kB', { exact: false }).waitFor({ timeout: 5_000 });
    await dialog.locator('input[type="file"]').setInputFiles({ name: 'launch.xml', mimeType: 'application/xml', buffer: Buffer.from('<?xml version="1.0"?><Project></Project>') });
    await dialog.getByRole('button', { name: 'Next', exact: true }).click();

    await dialog.getByText('Read as Project XML.').waitFor({ timeout: 5_000 });
    const plan = dialog.getByRole('region', { name: 'What the import would do' });
    await plan.getByText('Creates 2 tasks, 1 milestone, and 1 dependency.').waitFor({ timeout: 5_000 });
    assert.equal(sent.length, 1, 'the check is one dry run');
    assert.equal(sent[0].dry_run, true);
    assert.equal(sent[0].format, 'MSPROJECT_XML', 'the card won over the file');

    await dialog.getByRole('button', { name: 'Bring them in' }).click();
    await dialog.getByRole('region', { name: 'What the import did' }).getByText('Created 2 tasks, 1 milestone, and 1 dependency.').waitFor({ timeout: 5_000 });
    assert.equal(sent[1]?.dry_run, false, 'the import was asked as a dry run');
    await dialog.getByRole('button', { name: 'See the plan' }).click();
    await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
    assert.deepEqual(failures, []);
    await context.close();
  } finally {
    await browser.close();
  }
});
