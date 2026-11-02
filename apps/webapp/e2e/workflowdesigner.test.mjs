// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The workflow designer opens a workflow a template brought (UC-FLW-02): the store holds it as
// reactive state, which a structured clone refuses, and the screen once stopped working on every
// opening. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, MANIFEST, PAGE, PERMISSIONS, forgetUnstubbed, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const shape = {
  statuses: [
    { key: 'new', name: 'New', category: 'TODO', color_token: 'slate' },
    { key: 'in_progress', name: 'In progress', category: 'IN_PROGRESS', color_token: 'blue' },
    { key: 'resolved', name: 'Resolved', category: 'DONE', color_token: 'green' },
  ],
  initial: 'new',
  steps: [{ key: 'start', from: ['new'], to: 'in_progress' }, { key: 'resolve', from: [], to: 'resolved' }],
};
const FROM_TEMPLATE = { key: 'itil_incident', system: false, name: 'Incident', published: shape, draft: null, published_at: '2026-10-30T09:00:00Z', version: 1 };
const SHIPPED = { key: 'default', system: true, name: null, published: shape, draft: null, published_at: null, version: 1 };

test('chromium: the designer opens a workflow a template brought', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  forgetUnstubbed();
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: PERMISSIONS.OWNER } });
    if (path === '/memberships') {
      const rows = url.searchParams.get('scope_type') === 'TENANT' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'OWNER' }] : [];
      return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: [] } });
    }
    if (path === '/workflows') return route.fulfill({ json: { data: [SHIPPED, FROM_TEMPLATE], scheme: [] } });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));

  await page.goto(`${served.origin}/administration/workflows`);
  await page.getByText('In progress').first().waitFor({ timeout: 15_000 });
  assert.equal(await page.getByText('This screen stopped working').count(), 0, 'the designer crashed');
  assert.deepEqual(failures.filter((failure) => /DataCloneError|could not be cloned/.test(failure)), []);
});
