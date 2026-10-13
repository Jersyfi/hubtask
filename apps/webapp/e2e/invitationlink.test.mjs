// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Invitations where no mail goes out, walked (UC-ID-14 checks 5 and 6): *People* says before the
// press that a link follows, shows the link once and keeps it nowhere, renews an invitation still
// waiting from its row, and names an address already invited instead of inviting it twice. Where
// mail goes out, the screen is what it was.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, ADMIN_GRANTS, INVITED, MANIFEST, OWNER_GRANTS, PAGE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const LINK = 'https://home.example/redeem#token=hbt_inv_e2e-link';
const RENEWED = 'https://home.example/redeem#token=hbt_inv_e2e-renewed';
const NEWCOMER = '01a0e2e0-0000-7000-8000-0000000000d1';

const served = await serve(DIST);
test.after(() => served.close());

const roles = [
  { role: 'OWNER', permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS', 'DELETE_CONTAINER'] },
  { role: 'ADMIN', permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS'] },
  { role: 'MEMBER', permissions: ['READ', 'WRITE_ITEMS'] },
];

async function open(browser, { mail, invite, grants = OWNER_GRANTS }, sent) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, roles, features: { ...MANIFEST.features, mail } } });
    }
    if (path === '/memberships' && request.method() === 'GET') {
      const scope = new URL(request.url()).searchParams.get('scope_type');
      const rows = scope === 'TENANT'
        ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'OWNER' },
          { id: 'm-bert', scope_type: 'TENANT', account_id: INVITED.id, role: 'MEMBER' }]
        : [];
      return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: grants } });
    }
    if (path === '/memberships' && request.method() === 'POST') {
      sent.grant = request.postDataJSON();
      return route.fulfill({ status: 201, json: { id: 'm-new', scope_type: 'TENANT', account_id: NEWCOMER, role: 'MEMBER' } });
    }
    if (path === '/accounts:invite') {
      sent.invite = request.postDataJSON();
      return route.fulfill(invite);
    }
    if (path === `/accounts/${INVITED.id}:renew-invitation`) {
      sent.renewed = true;
      const body = { ...INVITED, ...(mail ? {} : { invitation_link: RENEWED }) };
      return route.fulfill({ status: 200, headers: { 'cache-control': 'no-store' }, json: body });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  return { page, close: () => context.close() };
}

const linked = {
  status: 201, headers: { 'cache-control': 'no-store' },
  json: { id: NEWCOMER, kind: 'USER', display_name: 'anna', email: 'anna@example.invalid', status: 'INVITED', invitation_link: LINK },
};

async function invite(page, address) {
  const form = page.locator('form').filter({ has: page.getByLabel('Email address') });
  await form.getByLabel('Email address').fill(address);
  await form.getByLabel('Role').selectOption('MEMBER');
  return form;
}

test('without mail the form promises a link, shows it once, and keeps it nowhere after', async () => {
  const browser = await chromium.launch();
  const sent = {};
  const { page, close } = await open(browser, { mail: false, invite: linked }, sent);
  try {
    await page.goto(`${served.origin}/administration/people`);
    await page.getByText('No mail goes out from here.', { exact: false }).waitFor();
    const form = await invite(page, 'anna@example.invalid');
    await form.getByRole('button', { name: 'Create the invitation link' }).click();

    await page.getByText('Invitation link for anna@example.invalid').waitFor();
    assert.deepEqual(sent.invite, { email: 'anna@example.invalid' });
    assert.equal(sent.grant?.account_id, NEWCOMER, 'the role was not given to the invited account');
    // It does not pretend to send.
    assert.equal(await page.getByText('Invitation sent', { exact: false }).count(), 0, 'a mail is claimed');
    // Masked until asked for; then the link itself.
    assert.equal(await page.locator('body').innerText().then((text) => text.includes('hbt_inv_e2e-link')), false,
      'the link is drawn before it was asked for');
    await page.getByRole('button', { name: 'Show the link' }).click();
    await page.getByText(LINK).waitFor();

    // Kept nowhere once dismissed: not in the document, not in the address, not in storage.
    await page.getByLabel('I have copied the link').check();
    await page.getByRole('button', { name: 'Done' }).click();
    await page.locator('[data-invitation-link]').waitFor({ state: 'detached' });
    const traces = await page.evaluate(() => [
      document.documentElement.outerHTML, location.href,
      JSON.stringify({ ...localStorage }), JSON.stringify({ ...sessionStorage }),
    ].join('\n'));
    assert.equal(traces.includes('hbt_inv_e2e-link'), false, 'the link outlived its one showing');
  } finally {
    await close();
    await browser.close();
  }
});

test('a row still invited is given a new link, drawn where the reader is taken to', async () => {
  const browser = await chromium.launch();
  const sent = {};
  const { page, close } = await open(browser, { mail: false, invite: linked }, sent);
  try {
    await page.goto(`${served.origin}/administration/people`);
    const row = page.getByRole('row').filter({ hasText: 'Bert' });
    await row.getByRole('button', { name: 'New invitation link' }).click();

    await page.getByText('Invitation link for Bert').waitFor();
    assert.equal(sent.renewed, true);
    const focused = await page.evaluate(() => document.activeElement?.hasAttribute('data-invitation-link'));
    assert.equal(focused, true, 'the reader is not taken to the new link');
    // A person who arrived has nothing to renew.
    const arrived = page.getByRole('row').filter({ hasText: ACCOUNT.display_name });
    assert.equal(await arrived.getByRole('button', { name: 'New invitation link' }).count(), 0);
  } finally {
    await close();
    await browser.close();
  }
});

test('an address already invited is named as one, and nothing is shown as sent (UC-ID-14 check 6)', async () => {
  const browser = await chromium.launch();
  const refused = {
    status: 409,
    json: { code: 'errors.conflict', detail_code: 'accounts.already_invited', status: 409, params: { account_id: INVITED.id }, request_id: 'req_e2e' },
  };
  const sent = {};
  const { page, close } = await open(browser, { mail: false, invite: refused }, sent);
  try {
    await page.goto(`${served.origin}/administration/people`);
    const form = await invite(page, INVITED.email);
    await form.getByRole('button', { name: 'Create the invitation link' }).click();

    await page.getByText('That address was invited already and has not accepted yet.', { exact: false }).waitFor();
    assert.equal(sent.grant, undefined, 'a role was given after the refusal');
    assert.equal(await page.locator('[data-invitation-link]').count(), 0, 'a link panel was drawn');
  } finally {
    await close();
    await browser.close();
  }
});

test('an address of somebody already here is named as one, and nothing is shown as sent (UC-ID-14 check 6)', async () => {
  const browser = await chromium.launch();
  const refused = {
    status: 409,
    json: { code: 'errors.conflict', detail_code: 'accounts.email_taken', status: 409, params: { account_id: ACCOUNT.id }, request_id: 'req_e2e' },
  };
  const sent = {};
  const { page, close } = await open(browser, { mail: false, invite: refused }, sent);
  try {
    await page.goto(`${served.origin}/administration/people`);
    const form = await invite(page, ACCOUNT.email);
    await form.getByRole('button', { name: 'Create the invitation link' }).click();

    await page.getByText('Somebody with that address is already in this workspace.', { exact: false }).waitFor();
    assert.equal(sent.grant, undefined, 'a role was given after the refusal');
    assert.equal(await page.locator('[data-invitation-link]').count(), 0, 'a link panel was drawn');
    assert.equal(await page.getByText('Invitation sent', { exact: false }).count(), 0, 'an invitation was said to be sent');
  } finally {
    await close();
    await browser.close();
  }
});

test('with mail the screen is what it was: a mail is promised, sent, and no link is drawn', async () => {
  const browser = await chromium.launch();
  const mailed = { status: 201, json: { id: NEWCOMER, kind: 'USER', display_name: 'anna', email: 'anna@example.invalid', status: 'INVITED' } };
  const sent = {};
  const { page, close } = await open(browser, { mail: true, invite: mailed }, sent);
  try {
    await page.goto(`${served.origin}/administration/people`);
    await page.getByText('They get an email with a link', { exact: false }).waitFor();
    const row = page.getByRole('row').filter({ hasText: 'Bert' });
    await row.getByRole('button', { name: 'Send the invitation again' }).waitFor();

    const form = await invite(page, 'anna@example.invalid');
    await form.getByRole('button', { name: 'Send the invitation' }).click();
    await page.getByText('Invitation sent to anna@example.invalid', { exact: false }).waitFor();
    assert.equal(await page.locator('[data-invitation-link]').count(), 0, 'a link was drawn beside a mail');
  } finally {
    await close();
    await browser.close();
  }
});

// UC-ID-14 check 1: an administrator is offered every role but Owner, in the invitation and in the
// grant alike - the server's `grantable_roles`, not the manifest's whole list.
test('an administrator is not offered Owner', async () => {
  const browser = await chromium.launch();
  const { page, close } = await open(browser, { mail: true, invite: linked, grants: ADMIN_GRANTS }, {});
  try {
    await page.goto(`${served.origin}/administration/people`);
    const inviteForm = page.locator('form').filter({ has: page.getByLabel('Email address') });
    await inviteForm.getByLabel('Role').waitFor();
    const offered = async (select) => select.locator('option').evaluateAll((options) => options.map((option) => option.value));
    const invitable = await offered(inviteForm.getByLabel('Role'));
    assert.equal(invitable.includes('OWNER'), false, 'the invitation offers Owner');
    assert.ok(invitable.includes('ADMIN') && invitable.includes('MEMBER'), `the invitation offers ${invitable}`);
    const roleSelects = page.getByLabel('Role');
    for (let index = 0; index < await roleSelects.count(); index += 1) {
      const values = await offered(roleSelects.nth(index));
      assert.equal(values.includes('OWNER'), false, `a role select offers Owner: ${values}`);
    }
  } finally {
    await close();
    await browser.close();
  }
});

// Where the server says nothing may be given here, nothing is offered: the people are listed and
// the invitation and the grant are absent rather than refused (P-05).
test('a reader who may grant nothing sees the people and no form', async () => {
  const browser = await chromium.launch();
  const { page, close } = await open(browser, { mail: true, invite: linked, grants: [] }, {});
  try {
    await page.goto(`${served.origin}/administration/people`);
    await page.getByRole('row').filter({ hasText: 'Bert' }).waitFor();
    assert.equal(await page.getByLabel('Email address').count(), 0, 'the invitation is drawn');
    assert.equal(await page.getByRole('button', { name: 'Grant' }).count(), 0, 'the grant is drawn');
  } finally {
    await close();
    await browser.close();
  }
});
