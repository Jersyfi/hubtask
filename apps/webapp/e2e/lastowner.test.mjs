// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The workspace keeps an owner, and Owner held through a group stays behind a proof
// (domain-model.md §3.2, identity.md §16.3), walked as the workspace's only owner: on *People* the
// owner's own row offers no *Revoke* and says why while another row keeps it; on *Groups* taking
// somebody out of a group that holds OWNER asks for the proof and sends the change again with it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, OTHER, OWNER_GRANTS, PAGE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const OWNERS = { id: '01a0e2e0-0000-7000-8000-0000000000f1', name: 'Owners', version: 1 };
const GRANT = 'hbt_sup_e2e';

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, { details } = {}) {
  const sent = { patches: [] };
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/memberships' && url.searchParams.get('scope_type') === 'TENANT') {
      return route.fulfill({
        json: {
          ...PAGE,
          data: [
            { id: 'm-me', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'OWNER' },
            { id: 'm-other', scope_type: 'TENANT', account_id: OTHER.id, role: 'ADMIN' },
          ],
          grantable_roles: OWNER_GRANTS,
        },
      });
    }
    if (path === '/accounts:sign-in-details') {
      sent.detailsOf = request.postDataJSON().ids;
      return details ? route.fulfill({ json: { data: details } }) : stub(route);
    }
    if (path === `/accounts/${OTHER.id}`) return route.fulfill({ json: OTHER });
    if (path === `/accounts/${ACCOUNT.id}`) return route.fulfill({ json: ACCOUNT });
    if (path === '/groups') return route.fulfill({ json: { ...PAGE, data: [OWNERS] } });
    if (path === `/groups/${OWNERS.id}` && request.method() === 'PATCH') {
      const proof = request.headers()['x-hubtask-step-up'] ?? null;
      sent.patches.push({ proof, body: request.postDataJSON() });
      if (proof !== GRANT) {
        return route.fulfill({
          status: 403,
          json: { code: 'forbidden', detail_code: 'auth.step_up_required', status: 403, request_id: 'req_g', params: { methods: 'PASSWORD' } },
        });
      }
      return route.fulfill({ json: { ...OWNERS, version: 2 } });
    }
    if (path === `/groups/${OWNERS.id}`) {
      return route.fulfill({ json: { ...OWNERS, members: [ACCOUNT.id, OTHER.id], members_changeable: true } });
    }
    if (path === '/auth/step-up') {
      return route.fulfill({ status: 201, json: { step_up_token: GRANT, expires_at: '2099-01-01T00:00:00Z', method: 'PASSWORD' } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  return { page, sent, close: () => context.close() };
}

const rowOf = (page, name) => page.getByRole('row').filter({ has: page.getByRole('rowheader', { name, exact: true }) });

test('the only owner is not offered to revoke their own Owner, and is told why', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/administration/people`);
  const other = rowOf(page, OTHER.display_name);
  await other.getByRole('button', { name: 'Revoke' }).waitFor({ timeout: 15_000 });
  // Revoke stays in its place, unavailable, and says why as the control's own description
  // (UC-WRK-32 check 1) - not as a sentence in the row.
  const me = rowOf(page, ACCOUNT.display_name);
  const revoke = me.getByRole('button', { name: 'Revoke' });
  await revoke.waitFor();
  assert.equal(await revoke.getAttribute('aria-disabled'), 'true', 'the last owner is offered Revoke');
  const described = await revoke.evaluate((button) =>
    (button.getAttribute('aria-describedby') ?? '').split(' ').map((id) => document.getElementById(id)?.textContent ?? '').join(' '));
  assert.match(described, /The workspace needs an owner\./);
  await revoke.click({ force: true });
  assert.equal(await page.getByRole('dialog').count(), 0, 'pressing it asked to revoke the last owner');
});

test('taking somebody out of a group that holds Owner asks for the proof and sends it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser);
  t.after(close);

  const failures = [];
  page.on('pageerror', (error) => failures.push(error.message));
  await page.goto(`${served.origin}/administration/people?tab=groups`);
  const owners = page.locator('section').filter({ has: page.getByRole('heading', { name: 'Owners' }) });
  await owners.getByRole('button', { name: 'Take out' }).first().waitFor({ timeout: 15_000 });
  await owners.getByRole('button', { name: 'Take out' }).last().click();

  const proof = page.getByRole('dialog', { name: 'Prove it is you' });
  await proof.waitFor();
  await proof.getByLabel('Your password').fill('the owner password');
  await proof.getByRole('button', { name: 'Prove it' }).click();

  await page.getByRole('dialog', { name: 'Prove it is you' }).waitFor({ state: 'hidden' });
  await new Promise((resolve) => setTimeout(resolve, 300));
  assert.deepEqual(sent.patches.map((patch) => patch.proof), [null, GRANT]);
  assert.equal(sent.patches[1].body.members.length, 1, 'the retried change is the same list');
  assert.deepEqual(failures, [], 'the screen failed while drawing');
});

// UX-4: a person's row shows what is needed now - *View as* and *Revoke* - and folds the rarer
// acts, their screens and a second factor's reset, under the row's *More*.
test('a person\'s row folds the rarer acts under More', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, { details: [{ account_id: OTHER.id, email: 'other@example.com', authenticator_app: true }] });
  t.after(close);

  await page.goto(`${served.origin}/administration/people`);
  const other = rowOf(page, OTHER.display_name);
  await other.getByRole('button', { name: 'Revoke' }).waitFor({ timeout: 15_000 });
  await other.getByRole('cell', { name: 'other@example.com', exact: true }).waitFor({ timeout: 15_000 });
  for (const folded of ['See their screens', 'Reset second factor']) {
    assert.equal(await other.getByRole('button', { name: folded, exact: true }).count(), 0, `${folded} stands in the row`);
  }
  await other.getByRole('button', { name: `More for ${OTHER.display_name}` }).click();
  const offered = (await page.getByRole('menu').getByRole('menuitem').allTextContents()).map((each) => each.trim());
  assert.ok(offered.includes('Reset second factor'), JSON.stringify(offered));
  // On a phone the standing reads as a word, not as a bar the width of the row.
  await page.setViewportSize({ width: 390, height: 844 });
  await page.keyboard.press('Escape');
  const chip = await other.locator('td').nth(2).locator('.badge, [class*="badge"]').first().evaluate((node) => [node.getBoundingClientRect().width, node.closest('td').getBoundingClientRect().width]);
  assert.ok(chip[0] < chip[1] / 2, `the standing stretches across the row: ${chip}`);
});

// An administrator sees each person's address in its own column, and *Reset second factor* only
// where an authenticator app stands; a managed person reads *No address*.
test('the people list shows the address and offers a reset only where an app stands', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser, { details: [
    { account_id: OTHER.id, email: null, authenticator_app: false },
    { account_id: ACCOUNT.id, email: 'me@example.com', authenticator_app: true },
  ] });
  t.after(close);

  await page.goto(`${served.origin}/administration/people`);
  await page.getByRole('columnheader', { name: 'Email' }).waitFor({ timeout: 15_000 });
  const other = rowOf(page, OTHER.display_name);
  await other.getByRole('cell', { name: 'No address', exact: true }).waitFor({ timeout: 15_000 });
  await rowOf(page, ACCOUNT.display_name).getByRole('cell', { name: 'me@example.com', exact: true }).waitFor();
  assert.deepEqual([...sent.detailsOf].sort(), [ACCOUNT.id, OTHER.id].sort());
  const more = other.getByRole('button', { name: `More for ${OTHER.display_name}` });
  await more.click();
  const offered = (await page.getByRole('menu').getByRole('menuitem').allTextContents()).map((each) => each.trim());
  assert.ok(offered.includes('See their screens') && !offered.includes('Reset second factor'), JSON.stringify(offered));
});
