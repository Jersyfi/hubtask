// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The operators screen (UC-INS-05 checks 1, 3, 5 and 6; UC-INS-06 check 1): names and the reader's
// own row, the note before the first addition, a service account chosen from a list, and removal
// asked once - twice for oneself. At desktop width and at 375 px.
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

const ME = {
  tenant_id: '01a0e2e0-0000-7000-8000-0000000005a1', account_id: ACCOUNT.id, added_at: '2026-10-01T08:00:00Z',
  display_name: ACCOUNT.display_name, email: ACCOUNT.email, kind: 'USER',
  workspace_slug: 'house', workspace_name: 'House',
};
const ADA = {
  tenant_id: '01a0e2e0-0000-7000-8000-0000000005a1', account_id: '01a0e2e0-0000-7000-8000-0000000000ad',
  added_at: '2026-10-02T08:00:00Z', added_by: ACCOUNT.id, added_by_name: ACCOUNT.display_name,
  display_name: 'Ada Lovelace', email: 'ada@example.invalid', kind: 'USER',
  workspace_slug: 'house', workspace_name: 'House',
};
const SHOP = { id: '01a0e2e0-0000-7000-8000-0000000000aa', kind: 'SERVICE_ACCOUNT', display_name: 'The shop', status: 'ACTIVE' };

async function operatorsScreen(browser, register, viewport = { width: 1280, height: 900 }) {
  const sent = { added: [], removed: [] };
  const context = await browser.newContext({ viewport });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/admin/operators' && request.method() === 'GET') return route.fulfill({ json: register });
    if (path === '/admin/operators' && request.method() === 'POST') {
      const body = request.postDataJSON();
      sent.added.push(body);
      if (body.email === 'nobody@example.invalid') {
        return route.fulfill({ status: 422, json: { code: 'errors.validation', detail_code: 'admin.operator_unknown_account', status: 422, request_id: 'req_o' } });
      }
      return route.fulfill({ status: 204 });
    }
    if (path.startsWith('/admin/operators/') && request.method() === 'DELETE') {
      sent.removed.push(path.split('/').pop());
      return route.fulfill({ status: 204 });
    }
    if (path === '/auth/service-accounts') return route.fulfill({ json: [SHOP] });
    if (path === '/admin/journal') return route.fulfill({ json: { data: [], page: { next_cursor: null, has_more: false } } });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/instance/operators`);
  return { context, page, sent };
}

test('chromium: the register names its operators and marks the reader', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  for (const viewport of [{ width: 1280, height: 900 }, { width: 375, height: 812 }]) {
    const { context, page } = await operatorsScreen(browser, [ME, ADA], viewport);
    const table = page.getByRole('table');
    await table.getByText('Ada Lovelace').waitFor();
    const text = await table.textContent() ?? '';
    assert.match(text, /Jérôme Winkel\s*you/, 'the reader\'s own row is not marked');
    assert.match(text, /ada@example\.invalid/);
    assert.match(text, /House \(house\)/);
    assert.match(text, /by Jérôme Winkel/);
    assert.doesNotMatch(text, /01a0e2e0-/, 'an identifier is drawn');
    // The service account of the reader's own workspace, by name.
    assert.equal(await page.getByRole('option', { name: 'The shop' }).count(), 1);
    await context.close();
  }
});

test('chromium: the first addition says it adds the reader too, and a service account is chosen by name', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, sent } = await operatorsScreen(browser, []);

  await page.getByText('Adding somebody adds you as well').first().waitFor();
  await page.getByLabel('Service account').selectOption({ label: 'The shop' });
  await page.getByRole('button', { name: 'Make it an operator' }).click();
  await assertEventually(() => sent.added.length === 1);
  assert.deepEqual(sent.added[0], { account_id: SHOP.id });
  await context.close();
});

test('chromium: removing somebody asks once; removing oneself asks twice; the last has no Remove', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, sent } = await operatorsScreen(browser, [ME, ADA]);
  await page.getByRole('table').getByText('Ada Lovelace').waitFor();

  const rows = page.getByRole('table').getByRole('row');
  await rows.filter({ hasText: 'Ada Lovelace' }).getByRole('button', { name: 'Remove' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByText('Remove Ada Lovelace as an operator?').waitFor();
  assert.match(await dialog.textContent() ?? '', /Ada Lovelace of House \(house\)/);
  await dialog.getByRole('button', { name: 'Remove' }).click();
  await assertEventually(() => sent.removed.length === 1);
  assert.equal(sent.removed[0], ADA.account_id);

  await rows.filter({ has: page.getByRole('rowheader', { name: /Jérôme Winkel/ }) })
    .getByRole('button', { name: 'Remove' }).click();
  await dialog.getByText('Remove yourself as an operator?').waitFor();
  await dialog.getByRole('button', { name: 'Remove' }).click();
  await dialog.getByText('The installation closes to you at once').waitFor();
  assert.equal(sent.removed.length, 1, 'removed oneself after one question');
  await dialog.getByRole('button', { name: 'Remove me' }).click();
  await assertEventually(() => sent.removed.length === 2);
  assert.equal(sent.removed[1], ACCOUNT.id);
  await context.close();

  const alone = await operatorsScreen(browser, [ME]);
  await alone.page.getByRole('table').getByText('Jérôme Winkel').waitFor();
  assert.equal(await alone.page.getByRole('button', { name: 'Remove' }).count(), 0, 'the last operator offers Remove');
  await alone.page.getByText('The only operator cannot be removed').waitFor();
  await alone.context.close();
});

async function assertEventually(condition, timeoutMs = 5_000) {
  const started = Date.now();
  while (!condition()) {
    if (Date.now() - started > timeoutMs) assert.fail('the condition did not hold in time');
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
}

// UC-INS-05 check 2: adding by workspace and address, and an address that matches no account there
// refused with one sentence - the screen says it and adds nobody.
test('chromium: an operator is added by workspace and address, and an unknown address is one sentence', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, sent } = await operatorsScreen(browser, [ME]);

  const workspace = page.getByLabel(/^Their workspace/);
  await workspace.waitFor();
  const choice = (await workspace.locator('option').allTextContents()).find((label) => !/Pick a workspace/.test(label));
  assert.ok(choice, 'no workspace to pick');
  await workspace.selectOption({ label: choice });
  await page.getByLabel(/^Their email address/).fill('nobody@example.invalid');
  await page.getByRole('button', { name: 'Add them' }).click();
  await page.getByText('There is no such account on this installation.').first().waitFor();
  assert.equal(await page.getByText('There is no such account on this installation.').count(), 1, 'more than one sentence');
  assert.equal(sent.added.length, 1);
  assert.equal(sent.added[0].email, 'nobody@example.invalid');
  assert.ok(sent.added[0].workspace ?? sent.added[0].tenant_id ?? sent.added[0].workspace_slug, `no workspace was sent: ${JSON.stringify(sent.added[0])}`);

  await page.getByLabel(/^Their email address/).fill('ada@example.invalid');
  await page.getByRole('button', { name: 'Add them' }).click();
  await assertEventually(() => sent.added.length === 2);
  assert.equal(sent.added[1].email, 'ada@example.invalid');
  await context.close();
});
