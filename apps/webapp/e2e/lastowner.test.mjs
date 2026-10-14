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

async function open(browser) {
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
  const me = rowOf(page, ACCOUNT.display_name);
  await me.getByText('The workspace needs an owner.').waitFor();
  assert.equal(await me.getByRole('button', { name: 'Revoke' }).count(), 0, 'the last owner is offered Revoke');
});

test('taking somebody out of a group that holds Owner asks for the proof and sends it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser);
  t.after(close);

  const failures = [];
  page.on('pageerror', (error) => failures.push(error.message));
  await page.goto(`${served.origin}/administration/groups`);
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
