// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The role ceiling on the screens, walked as an administrator (domain-model.md §3.2, P-05): on
// *People* an owner's row offers no *Revoke* while a member's does; on *Groups* a group the server
// says the reader may not change shows no member controls and no delete, says why, and keeps
// *Edit*, while a group within the reader's roles keeps them all.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { ACCOUNT, OTHER, PAGE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';
import { chipsOf, focused, pick, valueOf } from './several.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const ADMIN_GRANTS = ['ADMIN', 'MEMBER', 'CONTRIBUTOR', 'VIEWER', 'GUEST', 'AUDITOR'];
const OWNERS = { id: '01a0e2e0-0000-7000-8000-0000000000f1', name: 'Owners', version: 1 };
const HELPERS = { id: '01a0e2e0-0000-7000-8000-0000000000f2', name: 'Helpers', version: 1 };

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/memberships' && url.searchParams.get('scope_type') === 'TENANT') {
      return route.fulfill({
        json: {
          ...PAGE,
          data: [
            { id: 'm-owner', scope_type: 'TENANT', account_id: OTHER.id, role: 'OWNER' },
            { id: 'm-me', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'ADMIN' },
          ],
          grantable_roles: ADMIN_GRANTS,
        },
      });
    }
    if (path === `/accounts/${OTHER.id}`) return route.fulfill({ json: OTHER });
    if (path === '/groups') return route.fulfill({ json: { ...PAGE, data: [HELPERS, OWNERS] } });
    if (path === `/groups/${OWNERS.id}`) {
      return route.fulfill({ json: { ...OWNERS, members: [OTHER.id], members_changeable: false } });
    }
    if (path === `/groups/${HELPERS.id}`) {
      return route.fulfill({ json: { ...HELPERS, members: [OTHER.id], members_changeable: true } });
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

const rowOf = (page, name) => page.getByRole('row').filter({ has: page.getByRole('link', { name, exact: true }) });

/** What a member's row on a group's page offers in its ⋯. */
async function memberMenu(page, name) {
  const row = page.getByRole('row').filter({ has: page.getByRole('link', { name, exact: true }) });
  await row.getByRole('button', { name: `More for ${name}` }).click();
  await page.getByRole('menu').waitFor();
  const items = (await page.getByRole('menu').getByRole('menuitem').allTextContents()).map((each) => each.trim());
  await page.keyboard.press('Escape');
  await page.getByRole('menu').waitFor({ state: 'hidden' });
  return items;
}

/** What a person's row offers in its ⋯ (page-system PS-6). */
async function menuOf(page, row, name) {
  await row.getByRole('button', { name: `More for ${name}` }).click();
  await page.getByRole('menu').waitFor();
  const items = (await page.getByRole('menu').getByRole('menuitem').allTextContents()).map((each) => each.trim());
  await page.keyboard.press('Escape');
  await page.getByRole('menu').waitFor({ state: 'hidden' });
  return items;
}

test('an administrator is not offered to revoke an owner', async (t) => {
  const browser = await sharedChromium(t);
  const { page, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/administration/people`);
  const me = rowOf(page, ACCOUNT.display_name);
  await me.getByRole('button', { name: `More for ${ACCOUNT.display_name}` }).waitFor({ timeout: 15_000 });
  assert.ok((await menuOf(page, me, ACCOUNT.display_name)).includes('Revoke access…'), 'the reader\'s own row offers no Revoke');
  const owner = rowOf(page, OTHER.display_name);
  await owner.waitFor();
  assert.equal((await menuOf(page, owner, OTHER.display_name)).includes('Revoke access…'), false, 'an owner row offers Revoke');
});

test('a group above the reader keeps its name editable and nothing else, and says why', async (t) => {
  const browser = await sharedChromium(t);
  const { page, close } = await open(browser);
  t.after(close);

  // A failure while drawing stops the whole screen; it is caught here rather than read as a timeout.
  const failures = [];
  page.on('pageerror', (error) => failures.push(error.message));
  // Each group's own page holds its members and its delete (page-system §7.2).
  await page.goto(`${served.origin}/administration/groups/${HELPERS.id}`);
  await page.getByRole('button', { name: 'Delete group…' }).first().waitFor({ timeout: 15_000 });
  assert.equal(await page.getByRole('button', { name: 'Add people' }).count(), 1);
  assert.equal(await page.getByRole('button', { name: 'Edit', exact: true }).count(), 1);
  assert.ok((await memberMenu(page, OTHER.display_name)).includes('Take out'), 'a changeable group offers no Take out');

  await page.goto(`${served.origin}/administration/groups/${OWNERS.id}`);
  await page.getByText('gives a role you do not hold yourself').waitFor({ timeout: 15_000 });
  await page.waitForLoadState('networkidle');
  assert.equal(await page.getByRole('button', { name: 'Add people' }).count(), 0);
  assert.equal(await page.getByRole('button', { name: 'Delete group…' }).count(), 0);
  assert.equal(await page.getByRole('button', { name: `More for Owners` }).count(), 0, 'the head offers the delete in its menu');
  assert.equal(await page.getByRole('button', { name: 'Edit', exact: true }).count(), 1);
  assert.equal((await memberMenu(page, OTHER.display_name)).includes('Take out'), false, 'a group above the reader offers Take out');
  assert.deepEqual(failures, [], 'the screen failed while drawing');
});

// Adding people to a group (design-system.md §4): the workspace's people are one field named by its
// question, whose own filter narrows them - no second search beside it. Opened by the keyboard,
// narrowed by typing, a name ticked, Escape closes the list and leaves the dialog open with focus
// on the field, and the add writes the members with the people chosen. At 1280 in English and at
// 390, where the list is a sheet over the dialog, in German.
const NAMES = ['Ada Kern', 'Ben Ott', 'Clara Ruiz', 'Dana Wolf', 'Emil Haas', 'Frida Lang', 'Greta Sommer', 'Hugo Brandt'];
const PEOPLE = NAMES.map((name, at) => ({ ...OTHER, id: `01a0e2e0-0000-7000-8000-0000000001${String(at).padStart(2, '0')}`, display_name: name }));
for (const [width, locale] of [[1280, 'en'], [390, 'de']]) {
  test(`chromium: the people to add to a group are one field with its own filter, at ${width} (${locale})`, async (t) => {
    const browser = await sharedChromium(t);
    const context = await browser.newContext({ viewport: { width, height: 900 } });
    t.after(() => context.close());
    const patches = [];
    let members = [OTHER.id];
    await context.route('**/api/v1/**', async (route) => {
      const request = route.request();
      const url = new URL(request.url());
      const path = url.pathname.replace(/^.*\/api\/v1/u, '');
      if (path === '/accounts/me') return route.fulfill({ json: { ...ACCOUNT, locale } });
      if (path === '/memberships' && url.searchParams.get('scope_type') === 'TENANT') {
        return route.fulfill({ json: { ...PAGE, data: [
          { id: 'm-owner', scope_type: 'TENANT', account_id: OTHER.id, role: 'OWNER' },
          { id: 'm-me', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'ADMIN' },
          ...PEOPLE.map((person) => ({ id: `m-${person.id}`, scope_type: 'TENANT', account_id: person.id, role: 'MEMBER' })),
        ], grantable_roles: ADMIN_GRANTS } });
      }
      const person = PEOPLE.find((each) => path === `/accounts/${each.id}`);
      if (person) return route.fulfill({ json: person });
      if (path === `/accounts/${OTHER.id}`) return route.fulfill({ json: OTHER });
      if (path === '/groups') return route.fulfill({ json: { ...PAGE, data: [HELPERS] } });
      if (path === `/groups/${HELPERS.id}` && request.method() === 'PATCH') {
        patches.push(request.postDataJSON());
        members = request.postDataJSON().members;
        return route.fulfill({ json: { ...HELPERS, members, members_changeable: true, version: 2 } });
      }
      if (path === `/groups/${HELPERS.id}`) return route.fulfill({ json: { ...HELPERS, members, members_changeable: true } });
      return stub(route);
    });
    await context.addInitScript(() => {
      sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
      sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    });
    const page = await context.newPage();
    const words = locale === 'de'
      ? { add: 'Personen hinzufügen', title: 'Personen zu Helpers hinzufügen', field: 'Jemanden hinzufügen', empty: 'Nichts gewählt', go: '2 Personen hinzufügen', filter: 'Liste filtern', done: 'Fertig' }
      : { add: 'Add people', title: 'Add people to Helpers', field: 'Add somebody', empty: 'Nothing chosen', go: 'Add 2 people', filter: 'Filter the list', done: 'Done' };
    await page.goto(`${served.origin}/administration/groups/${HELPERS.id}`);
    if (locale === 'de') await page.waitForFunction(() => document.documentElement.lang === 'de', null, { timeout: 15_000 });
    await page.getByRole('button', { name: words.add }).first().click({ timeout: 15_000 });
    const dialog = page.getByRole('dialog', { name: words.title });
    await dialog.waitFor({ timeout: 5_000 });

    const field = dialog.getByRole('button', { name: new RegExp(`^${words.field}`, 'u') });
    await field.waitFor({ timeout: 5_000 }).catch(() => {});
    assert.equal(await field.count(), 1, 'the people to add are not one field');
    assert.equal(await field.getAttribute('aria-haspopup'), 'listbox');
    assert.equal(await dialog.getByRole('searchbox').count(), 0, 'a second search stands beside the field');
    assert.equal(await valueOf(field), words.empty);

    await field.focus();
    await page.keyboard.press('Enter');
    const list = page.getByRole('listbox', { name: words.field });
    await list.waitFor({ timeout: 5_000 });
    assert.equal(await list.getByRole('option').count(), NAMES.length + 1, 'everybody not yet a member is offered');
    await page.getByRole('searchbox', { name: words.filter }).fill('wolf');
    await list.getByRole('option').first().waitFor({ timeout: 5_000 });
    assert.deepEqual(await list.getByRole('option').allTextContents().then((all) => all.map((each) => each.trim())), ['Dana Wolf']);
    await page.keyboard.press('ArrowDown');
    assert.equal(await focused(page), 'option "Dana Wolf"', 'ArrowDown from the filter did not reach the option');
    await page.keyboard.press('Space');
    await page.keyboard.press('Escape');
    await list.waitFor({ state: 'detached', timeout: 5_000 });
    assert.equal(await dialog.isVisible(), true, 'Escape closed the dialog, not only the list');
    assert.match(await focused(page), new RegExp(`^button "${words.field} `, 'u'), 'closing the list left focus elsewhere');
    assert.deepEqual(await pick(page, field, ['Ada Kern']), ['Ada Kern', 'Dana Wolf']);
    assert.deepEqual(await chipsOf(field), ['Ada Kern', 'Dana Wolf']);

    await dialog.getByRole('button', { name: words.go }).click();
    await dialog.waitFor({ state: 'detached', timeout: 5_000 });
    assert.deepEqual(patches, [{ members: [OTHER.id, PEOPLE[3].id, PEOPLE[0].id] }], 'the members are not the group’s and then those chosen, in the order chosen');
    await page.getByRole('main').getByRole('link', { name: 'Dana Wolf', exact: true }).waitFor({ timeout: 5_000 });
  });
}
