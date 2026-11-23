// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The workflow designer opens a workflow a template brought (UC-FLW-02): the store holds it as
// reactive state, which a structured clone refuses, and the screen once stopped working on every
// opening. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { ACCOUNT, MANIFEST, PAGE, PERMISSIONS, forgetUnstubbed, stub } from './fixture.mjs';
import { serve } from './serve.mjs';
import { chipsOf, focused } from './several.mjs';

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
  const browser = await sharedChromium(t);
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

const GROUPS = [
  { id: '01a0e2e0-0000-7000-8000-0000000012b1', name: 'Front desk', description: null, version: 1 },
  { id: '01a0e2e0-0000-7000-8000-0000000012b2', name: 'Night shift', description: null, version: 1 },
];

// A step's several-of-many (design-system.md §4): where it leaves from is ticks named by their
// question; who may take it - five roles and the workspace's groups - is one field that opens a
// grouped list. Escape closes the list and the dialog stays with what was chosen; the save sends
// the roles in their order and the group, and leaves `from` as the ticks say.
for (const locale of ['en', 'de']) {
  test(`chromium: 1280 px - a step's statuses, roles and groups are chosen several of many, by the keyboard (${locale})`, async (t) => {
    const browser = await sharedChromium(t);
    const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    t.after(() => context.close());
    forgetUnstubbed();
    await context.route('**/api/v1/**', async (route) => {
      const request = route.request();
      const url = new URL(request.url());
      const path = url.pathname.replace(/^.*\/api\/v1/, '');
      if (path === '/accounts/me') return route.fulfill({ json: { ...ACCOUNT, locale } });
      if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: PERMISSIONS.OWNER } });
      if (path === '/memberships') {
        const rows = url.searchParams.get('scope_type') === 'TENANT' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'OWNER' }] : [];
        return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: [] } });
      }
      if (path === '/groups') return route.fulfill({ json: { ...PAGE, data: GROUPS } });
      if (path === `/workflows/${FROM_TEMPLATE.key}` && request.method() === 'PATCH') {
        return route.fulfill({ json: { ...FROM_TEMPLATE, draft: request.postDataJSON().shape, version: 2 } });
      }
      if (path === '/workflows') return route.fulfill({ json: { data: [SHIPPED, FROM_TEMPLATE], scheme: [] } });
      return stub(route);
    });
    await context.addInitScript(() => {
      sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
      sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    });
    const page = await context.newPage();
    const words = locale === 'de'
      ? { from: 'Geht aus von', who: 'Wer ihn gehen darf', roles: 'Rollen', groups: 'Gruppen', member: 'Mitglied', owner: 'Eigentümer', done: 'Fertig', anyone: 'Alle, die den Eintrag ändern dürfen' }
      : { from: 'Leaves from', who: 'Who may take it', roles: 'Roles', groups: 'Groups', member: 'Member', owner: 'Owner', done: 'Done', anyone: 'Anyone who may change the entry' };

    await page.goto(`${served.origin}/administration/workflows/${FROM_TEMPLATE.key}`);
    await page.getByText('In progress').first().waitFor({ timeout: 15_000 });
    if (locale === 'de') await page.waitForFunction(() => document.documentElement.lang === 'de', null, { timeout: 15_000 });
    await page.locator('button.press').filter({ hasText: 'In progress' }).last().click();
    const dialog = page.locator('dialog[open]');
    await dialog.locator('[data-step-dialog="start"]').waitFor({ timeout: 5_000 });

    // Where it leaves from: two statuses, the design system's ticks, named by their question.
    const from = dialog.getByRole('group', { name: words.from, exact: true });
    assert.equal(await from.getByRole('checkbox').count(), 2, 'the statuses are not ticks of their own group');
    await from.getByRole('checkbox', { name: 'Resolved' }).check();

    // Who may take it: one field, its list in two named groups.
    const who = dialog.getByRole('button', { name: new RegExp(`^${words.who}`, 'u') });
    assert.equal(await who.getAttribute('aria-haspopup'), 'listbox', 'who may take it is not one field');
    // Nothing chosen limits nobody: the empty field says so, not "Nothing chosen".
    assert.equal((await who.locator('.value').textContent()).trim(), words.anyone, 'the empty field does not say what it means');
    await who.focus();
    await page.keyboard.press('Enter');
    const list = page.getByRole('listbox', { name: words.who });
    await list.waitFor({ timeout: 5_000 });
    assert.deepEqual(
      await list.getByRole('group').evaluateAll((groups) => groups.map((group) => document.getElementById(group.getAttribute('aria-labelledby'))?.textContent)),
      [words.roles, words.groups],
    );
    await list.getByRole('option', { name: words.member, exact: true }).click();
    await list.getByRole('option', { name: words.owner, exact: true }).click();
    await list.getByRole('option', { name: 'Night shift', exact: true }).click();
    await page.keyboard.press('Escape');
    await list.waitFor({ state: 'detached', timeout: 5_000 });
    assert.ok(await dialog.isVisible(), 'Escape on the list closed the dialog');
    assert.match(await focused(page), new RegExp(`^button "${words.who} `, 'u'), 'closing the list left focus elsewhere');
    assert.deepEqual(await chipsOf(who), [words.owner, words.member, 'Night shift']);

    await dialog.getByRole('button', { name: words.done, exact: true }).click();
    await dialog.waitFor({ state: 'detached', timeout: 5_000 });
    const sent = page.waitForRequest((request) => request.method() === 'PATCH' && request.url().endsWith(`/workflows/${FROM_TEMPLATE.key}`));
    await page.locator('[data-save-draft]').click();
    const step = (await sent).postDataJSON().shape.steps.find((each) => each.key === 'start');
    assert.deepEqual(step?.from, ['new', 'resolved']);
    assert.deepEqual(step?.who, { roles: ['OWNER', 'MEMBER'], groups: [GROUPS[1].id] });
  });
}
