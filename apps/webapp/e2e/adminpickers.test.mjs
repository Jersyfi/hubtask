// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A person is chosen by name, not typed as an identifier (UC-ID-24 check 4): in the trail, in
// people's requests, in restricting and in withdrawing consent. The picker lists only whom the
// reader may see - the workspace's people where the reader reads the workspace, and the actors of
// the trail entries the reader was already answered.
//
// Against a stubbed API. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, MANIFEST, OTHER, PERMISSIONS, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

/** Somebody who appears only in the trail - left the workspace, or a service account. */
const FORMER = { id: '01a0e2e0-0000-7000-8000-00000000c420', kind: 'USER', display_name: 'Pickers Former Member', status: 'ACTIVE', locale: 'en' };

const ENTRY = {
  id: '01a0e2e0-0000-7000-8000-00000000c421', seq: 1, occurred_at: '2026-10-20T08:00:00Z', action: 'container.created', outcome: 'SUCCESS',
  actor: { type: 'USER', id: FORMER.id, label: FORMER.display_name }, target: { type: 'container', id: null },
};

async function as(browser, permissions) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const sent = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    sent.push({ method: request.method(), path, search: url.search, body: request.method() === 'GET' ? undefined : request.postDataJSON() });
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: permissions } });
    if (path === '/audit' && request.method() === 'GET') return route.fulfill({ json: { items: [ENTRY], next_cursor: null } });
    if (path === `/accounts/${FORMER.id}`) return route.fulfill({ json: FORMER });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  return { context, page, sent, failures };
}

/** Opens a picker by its label and chooses somebody by name. */
async function pick(page, label, name) {
  const picker = page.locator('[data-person-picker]').filter({ has: page.getByText(label, { exact: true }) });
  await picker.getByRole('button').first().click();
  const panel = page.getByRole('dialog', { name: label });
  await panel.getByText(name, { exact: true }).click();
  return picker;
}

test('chromium: the trail\'s actor is picked from the workspace\'s people and those in the trail', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const owner = await as(browser, PERMISSIONS.OWNER);
  t.after(() => owner.context.close());
  const { page, sent } = owner;

  await page.goto(`${served.origin}/administration/audit`);
  await page.getByText('Pickers Former Member').first().waitFor();
  assert.equal(await page.getByRole('textbox', { name: /Who did it/ }).count(), 0, 'the actor is still typed as an identifier');

  const picker = await pick(page, 'Who did it', OTHER.display_name);
  assert.match(await picker.textContent() ?? '', new RegExp(OTHER.display_name));
  await page.getByRole('button', { name: 'Look', exact: true }).click();
  await page.waitForFunction(() => true);
  await new Promise((resolve) => setTimeout(resolve, 300));
  assert.ok(sent.some((each) => each.path === '/audit' && new URLSearchParams(each.search).get('actor_id') === OTHER.id), 'the query did not carry the chosen actor');

  // Somebody only the trail names is offered too: the trail's own read answered them.
  await pick(page, 'Who did it', FORMER.display_name);
  assert.deepEqual(owner.failures, []);
});

test('chromium: a request\'s subject is picked by name and sent as the account', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const owner = await as(browser, PERMISSIONS.OWNER);
  t.after(() => owner.context.close());
  const { page, sent } = owner;

  await page.goto(`${served.origin}/administration/privacy`);
  await page.getByText('Their account', { exact: true }).first().waitFor();
  for (const label of ['Their account', 'Whose account', 'Whose consent']) {
    assert.equal(await page.getByRole('textbox', { name: new RegExp(`^${label}`) }).count(), 0, `"${label}" is still typed as an identifier`);
  }
  await pick(page, 'Their account', OTHER.display_name);
  await page.getByRole('button', { name: 'Record it' }).click();
  await page.waitForFunction(() => true);
  await new Promise((resolve) => setTimeout(resolve, 300));
  const recorded = sent.find((each) => each.method === 'POST' && each.path === '/privacy/requests');
  assert.equal(recorded?.body?.subject_account_id, OTHER.id, JSON.stringify(recorded?.body));
  assert.deepEqual(owner.failures, []);
});

test('chromium: an auditor\'s trail picker offers only whom the trail named, and reads no one else', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const auditor = await as(browser, PERMISSIONS.AUDITOR);
  t.after(() => auditor.context.close());
  const { page, sent } = auditor;

  await page.goto(`${served.origin}/administration/audit`);
  await page.getByText('Pickers Former Member').first().waitFor();
  const picker = page.locator('[data-person-picker]').filter({ has: page.getByText('Who did it', { exact: true }) });
  await picker.getByRole('button').first().click();
  const panel = page.getByRole('dialog', { name: 'Who did it' });
  await panel.getByText(FORMER.display_name, { exact: true }).waitFor();
  assert.equal(await panel.getByText(OTHER.display_name, { exact: true }).count(), 0, 'the auditor is offered somebody the trail did not name');
  assert.equal(await panel.getByText(ACCOUNT.display_name, { exact: true }).count(), 0);
  assert.ok(!sent.some((each) => each.path.startsWith('/memberships')), 'the memberships were read for an auditor');
  assert.deepEqual(auditor.failures, []);
});
