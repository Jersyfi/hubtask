// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The administration offers each write only to a reader who holds the permission it asks (P-05),
// walked against the manifest's `workspace_permissions`:
//
// * an auditor (`READ_CONFIGURATION`, `AUDIT_READ`) reads every screen it reaches and is offered
//   nothing it may not change - the form stays readable, inert, with the reason beneath it;
// * an owner is offered every control, as before.
//
// A control behind a closed gate is inside an inert subtree - out of the tab order and the
// accessibility tree - so "reachable" counts the controls of a name outside one. Each case also
// asserts the gate's sentence, so a screen that drew nothing at all cannot pass.
// Against a stubbed API; that the server refuses the same writes is its own test. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { ACCOUNT, MANIFEST, PERMISSIONS, ROLE_CATALOGUE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';
import { chipsOf, describedBy, focused, pick, valueOf } from './several.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

/** The gate's sentence for one permission, as `app.gate.workspace_permission_missing` words it. */
const NAMES = { STRUCTURE: 'Structure', MANAGE_MEMBERS: 'People', AUTOMATION: 'Automation', DELETE_CONTAINER: 'Delete containers' };
const refusedFor = (permission) =>
  `You can read this, but your role in this workspace cannot change it. That needs the permission “${NAMES[permission]}”.`;

/** One rule of the sign-in policy as the contract answers it, unlocked and from the defaults. */
const rule = (value) => ({ value, installation: value, lock: null, source: 'DEFAULT', installation_source: 'DEFAULT' });

/** The workspace with a sign-in policy, which the sign-in screen needs to draw anything at all. */
const TENANT = {
  id: 'w1', slug: 'acme', display_name: 'Acme', status: 'ACTIVE', default_locale: 'en',
  default_time_zone: 'Europe/Berlin', created_at: '2026-09-01T00:00:00Z', version: 3,
  sign_in_policy: {
    password: {
      min_length: rule(12), min_lowercase: rule(0), min_uppercase: rule(0), min_digits: rule(0), min_symbols: rule(0),
      min_classes: rule(0), max_repeat: rule(0), common_passwords: rule(true), context_words: rule(true),
      breach_check: rule(false), max_age_days: rule(0), history_count: rule(0), min_age_hours: rule(0),
    },
    mfa_required_for: rule('ADMINS'),
    methods: rule(['PASSWORD', 'OIDC']),
    session: { max_days: rule(30), idle_minutes: rule(30) },
    legal: { imprint_url: rule(''), privacy_url: rule(''), terms_url: rule(''), accessibility_url: rule(''), terms_version: rule('') },
    rotation_from: null,
  },
};

/**
 * Each screen, the controls its writes are, and the permissions whose gate must say why. `buttons`
 * are names a reader would press; `exact` where a name is a prefix of another.
 */
const SCREENS = [
  { path: '/administration/workspace', buttons: [{ role: 'textbox', name: /^Name/ }], gates: ['STRUCTURE'] },
  // *Require a new password from everyone* is the head's ⋯ on Sign-in (page-system §7.3), the
  // menu's one item: the menu is offered exactly where the rotation is.
  { path: '/administration/sign-in', buttons: ['More for Sign-in'], gates: ['STRUCTURE'] },
  { path: '/administration/identity-provider', buttons: ['Add a provider'], gates: ['MANAGE_MEMBERS'] },
  { path: '/administration/machines?tab=apps', buttons: ['Register an app'], gates: ['MANAGE_MEMBERS'] },
  { path: '/administration/people?tab=groups', buttons: ['New group'], gates: ['MANAGE_MEMBERS'] },
  { path: '/administration/machines', buttons: ['Add a service account'], gates: ['MANAGE_MEMBERS'] },
  { path: '/administration/privacy', buttons: ['Record a request'], gates: ['MANAGE_MEMBERS'] },
  { path: '/administration/rules', buttons: ['Write a rule'], gates: ['AUTOMATION'] },
  { path: '/administration/rules/new', buttons: ['Save the rule'], gates: ['AUTOMATION'] },
  { path: '/administration/webhooks', buttons: ['Add a webhook'], gates: ['AUTOMATION'] },
  // The own model's form is AI's first section here; its save bar shows only once something changed.
  { path: '/administration/ai', buttons: [{ role: 'textbox', name: /^Model for proposals/ }], gates: ['STRUCTURE'] },
  // Backup and Retention are one section per address (page-system §9.3): each section's own add.
  { path: '/administration/backup', buttons: ['Back up now'], gates: ['STRUCTURE', 'DELETE_CONTAINER'] },
  { path: '/administration/backup/targets', buttons: ['Add a target'], gates: ['STRUCTURE', 'DELETE_CONTAINER'] },
  { path: '/administration/retention', buttons: ['Write a rule'], gates: ['DELETE_CONTAINER'] },
  { path: '/administration/retention/holds', buttons: ['Place a hold'], gates: ['DELETE_CONTAINER'] },
  { path: '/administration/restore', buttons: ['See what it would do'], gates: ['STRUCTURE'] },
  // Where the chain is anchored is a Dialog M from the head's ⋯ (page-system §9.3).
  { path: '/administration/audit', open: [{ role: 'button', name: 'More for Audit log' }, { role: 'menuitem', name: 'Anchoring…' }], buttons: ['Anchor here'], gates: ['STRUCTURE'] },
];

/**
 * What an administrator - every column but the owner's `DELETE_CONTAINER` - is not offered: the
 * backup targets and schedules, the retention rules and the holds are set behind it on the server.
 */
const OWNERS_ONLY = [
  { path: '/administration/backup/targets', buttons: ['Add a target'] },
  { path: '/administration/retention', buttons: ['Write a rule'] },
  { path: '/administration/retention/holds', buttons: ['Place a hold'] },
];

/** A control as the table names it: a button by its words, or another role by its name. */
const controlOf = (entry) => (typeof entry === 'string' ? { role: 'button', name: entry } : entry);

/** Opens what a screen keeps in a dialog: the controls `open` names, pressed in turn. */
async function opened(page, screen) {
  for (const { role, name } of screen.open ?? []) await page.getByRole(role, { name, exact: true }).first().click({ timeout: 15_000 });
}

/** A signed-in tab as somebody holding `permissions` on the workspace. */
async function as(browser, permissions) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: permissions } });
    if (path === '/tenant' && route.request().method() === 'GET') return route.fulfill({ json: TENANT });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  return { context, page, failures };
}

/**
 * How many controls of this name a reader can reach on the page: not inside an inert subtree, which
 * takes a control out of the tab order and the accessibility tree (Playwright's role query still
 * lists it, so the walk asks the DOM).
 */
const reachable = (page, { role, name }) =>
  page.getByRole(role, { name, exact: true }).evaluateAll((controls) => controls.filter((control) => !control.closest('[inert]')).length);

for (const screen of SCREENS) {
  test(`chromium: ${screen.path} — an auditor reads it and is offered no write; an owner is offered each`, async (t) => {
    const browser = await sharedChromium(t);

    const owner = await as(browser, PERMISSIONS.OWNER);
    t.after(() => owner.context.close());
    await owner.page.goto(`${served.origin}${screen.path}`);
    await opened(owner.page, screen);
    for (const { role, name } of screen.buttons.map(controlOf)) {
      await owner.page.getByRole(role, { name, exact: true }).first().waitFor({ timeout: 15_000 })
        .catch(() => assert.fail(`the owner is not offered "${name}" on ${screen.path}`));
    }
    for (const permission of screen.gates) {
      assert.equal(await owner.page.getByText(refusedFor(permission)).count(), 0, `the owner is told ${permission} is missing`);
    }
    assert.deepEqual(owner.failures, []);

    const auditor = await as(browser, PERMISSIONS.AUDITOR);
    t.after(() => auditor.context.close());
    await auditor.page.goto(`${served.origin}${screen.path}`);
    await opened(auditor.page, screen);
    for (const permission of screen.gates) {
      await auditor.page.getByText(refusedFor(permission)).first().waitFor({ timeout: 15_000 })
        .catch(() => assert.fail(`the auditor is not told why on ${screen.path} (${permission})`));
      // Once per permission per page, however many areas it gates (UC-WRK-32 check 4).
      assert.equal(await auditor.page.getByText(refusedFor(permission)).count(), 1, `the auditor is told twice on ${screen.path} (${permission})`);
    }
    // Nothing the auditor reads is dimmed into illegibility: no gated area is drawn faded.
    assert.equal(await auditor.page.locator('[data-status="refused"]').count(), 0, `a dimmed gate on ${screen.path}`);
    for (const control of screen.buttons.map(controlOf)) {
      assert.equal(await reachable(auditor.page, control), 0, `the auditor is offered "${control.name}" on ${screen.path}`);
    }
    assert.deepEqual(auditor.failures, []);
  });
}

for (const screen of OWNERS_ONLY) {
  test(`chromium: ${screen.path} — an administrator is not offered what only the owner may set`, async (t) => {
    const browser = await sharedChromium(t);
    const admin = await as(browser, PERMISSIONS.ADMIN);
    t.after(() => admin.context.close());
    await admin.page.goto(`${served.origin}${screen.path}`);
    await opened(admin.page, screen);
    await admin.page.getByText(refusedFor('DELETE_CONTAINER')).first().waitFor({ timeout: 15_000 })
      .catch(() => assert.fail(`the administrator is not told why on ${screen.path}`));
    assert.equal(await admin.page.getByText(refusedFor('STRUCTURE')).count(), 0, 'the administrator is told STRUCTURE is missing');
    for (const control of screen.buttons.map(controlOf)) {
      assert.equal(await reachable(admin.page, control), 0, `the administrator is offered "${control.name}" on ${screen.path}`);
    }
    assert.deepEqual(admin.failures, []);
  });
}

// A new role's permissions, area by area (design-system.md §4): an area of six is one field named
// by the area, an area of two stays ticks. Opened from the keyboard on its first permission, Space
// ticks, Escape closes the list and leaves the dialog open with focus on the field; the role is
// defined with what was ticked in both areas. In English and in German.
const AREAS = { READ: 'content', WRITE_ITEMS: 'content', STRUCTURE: 'content', AUTOMATION: 'content', DELETE_CONTAINER: 'content', READ_CONFIGURATION: 'content', MANAGE_MEMBERS: 'people', AUDIT_READ: 'people' };
for (const locale of ['en', 'de']) {
  test(`chromium: a new role's permissions are a field for an area of six and ticks for two (${locale})`, async (t) => {
    const browser = await sharedChromium(t);
    const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    t.after(() => context.close());
    const created = [];
    await context.route('**/api/v1/**', async (route) => {
      const request = route.request();
      const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/u, '');
      if (path === '/accounts/me') return route.fulfill({ json: { ...ACCOUNT, locale } });
      if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: PERMISSIONS.OWNER } });
      if (path === '/roles' && request.method() === 'GET') {
        return route.fulfill({ json: { ...ROLE_CATALOGUE, catalogue: ROLE_CATALOGUE.catalogue.map((entry) => ({ ...entry, group: AREAS[entry.permission] ?? 'content' })) } });
      }
      if (path === '/roles' && request.method() === 'POST') {
        const body = request.postDataJSON();
        created.push(body);
        return route.fulfill({ status: 201, json: { id: '01a0e2e0-0000-7000-8000-0000000029c2', key: body.key, name: body.name, permissions: body.permissions, version: 1, created_at: '2026-11-23T09:00:00Z', updated_at: '2026-11-23T09:00:00Z' } });
      }
      if (path === '/access/explain') {
        return route.fulfill({ json: { account_id: ACCOUNT.id, scope_type: 'TENANT', roles: ['OWNER'], permissions: PERMISSIONS.OWNER, sources: [] } });
      }
      return stub(route);
    });
    await context.addInitScript(() => {
      sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
      sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    });
    const page = await context.newPage();
    const words = locale === 'de'
      ? { new: 'Neue Rolle', content: 'Inhalte', people: 'Personen', read: 'Lesen', structure: 'Struktur', write: 'Einträge schreiben', members: 'Leute', empty: 'Nichts gewählt', name: 'Name', create: 'Rolle festlegen' }
      : { new: 'New role', content: 'Content', people: 'People', read: 'Read', structure: 'Structure', write: 'Write entries', members: 'People', empty: 'Nothing chosen', name: 'Name', create: 'Define the role' };
    await page.goto(`${served.origin}/administration/people?tab=roles`);
    if (locale === 'de') await page.waitForFunction(() => document.documentElement.lang === 'de', null, { timeout: 15_000 });
    await page.getByRole('button', { name: words.new, exact: true }).first().click({ timeout: 15_000 });
    const dialog = page.getByRole('dialog', { name: words.new });
    await dialog.waitFor({ timeout: 5_000 });

    const content = dialog.getByRole('button', { name: new RegExp(`^${words.content}`, 'u') });
    await content.waitFor({ timeout: 5_000 }).catch(() => {});
    assert.equal(await content.count(), 1, 'an area of six permissions is not one field');
    assert.equal(await content.getAttribute('aria-haspopup'), 'listbox');
    assert.equal(await valueOf(content), words.empty);
    const people = dialog.getByRole('group', { name: words.people, exact: true });
    assert.equal(await people.getByRole('checkbox').count(), 2, 'an area of two is not ticks');

    await content.focus();
    await page.keyboard.press('Enter');
    const list = page.getByRole('listbox', { name: words.content });
    await list.waitFor({ timeout: 5_000 });
    assert.equal(await focused(page), `option "${words.read}"`, 'Enter did not land on the first permission');
    await page.keyboard.press('Space');
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('Space');
    await page.keyboard.press('Escape');
    await list.waitFor({ state: 'detached', timeout: 5_000 });
    assert.equal(await dialog.isVisible(), true, 'Escape closed the dialog, not only the list');
    assert.match(await focused(page), new RegExp(`^button "${words.content} `, 'u'), 'closing the list left focus elsewhere');
    assert.deepEqual(await chipsOf(content), [words.read, words.write]);
    assert.deepEqual(await pick(page, content, [words.structure]), [words.read, words.write, words.structure]);
    await people.getByRole('checkbox', { name: words.members, exact: true }).check();

    await dialog.getByLabel(new RegExp(`^${words.name}`, 'u')).first().fill('Planner');
    await dialog.getByRole('button', { name: words.create }).click();
    await dialog.waitFor({ state: 'detached', timeout: 5_000 });
    assert.equal(created.length, 1, 'no role was defined');
    assert.deepEqual([...created[0].permissions].sort(), ['MANAGE_MEMBERS', 'READ', 'STRUCTURE', 'WRITE_ITEMS']);
    assert.equal(created[0].name, 'Planner');
  });
}

/** The role dialog opened by a reader who holds `role` on the workspace. */
async function roleDialog(browser, role) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/u, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: PERMISSIONS[role] } });
    if (path === '/roles' && request.method() === 'GET') {
      return route.fulfill({ json: { ...ROLE_CATALOGUE, catalogue: ROLE_CATALOGUE.catalogue.map((entry) => ({ ...entry, group: AREAS[entry.permission] ?? 'content' })) } });
    }
    if (path === '/access/explain') {
      return route.fulfill({ json: { account_id: ACCOUNT.id, scope_type: 'TENANT', roles: [role], permissions: PERMISSIONS[role], sources: [] } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/administration/people?tab=roles`);
  await page.getByRole('button', { name: 'New role', exact: true }).first().click({ timeout: 15_000 });
  const dialog = page.getByRole('dialog', { name: 'New role' });
  await dialog.waitFor({ timeout: 5_000 });
  await dialog.getByRole('button', { name: /^Content/u }).or(dialog.getByRole('group', { name: 'Content', exact: true })).first().waitFor({ timeout: 5_000 });
  return { page, dialog, close: () => context.close() };
}

// One look per area, the same for every reader (design-system.md §4): *Content* goes by its six
// in the catalogue, so an administrator, who holds five of them, sees the field an owner sees.
for (const role of ['OWNER', 'ADMIN']) {
  test(`chromium: a role's areas look the same for an ${role === 'OWNER' ? 'owner' : 'administrator'}`, async (t) => {
    const browser = await sharedChromium(t);
    const { dialog, close } = await roleDialog(browser, role);
    t.after(close);
    assert.equal(await dialog.getByRole('button', { name: /^Content/u }).getAttribute('aria-haspopup'), 'listbox', 'Content is not one field');
    assert.equal(await dialog.getByRole('group', { name: 'People', exact: true }).getByRole('checkbox').count(), 2, 'People are not ticks');
  });
}

// A role defined without a permission (design-system.md §4, case 3): the error stands on the first
// area's control, which is marked and described by it, and focus moves there, as in a token's form.
test('chromium: a role without a permission marks the first area and takes focus there', async (t) => {
  const browser = await sharedChromium(t);
  const { page, dialog, close } = await roleDialog(browser, 'OWNER');
  t.after(close);
  await dialog.getByLabel(/^Name/u).first().fill('Planner');
  await dialog.getByRole('button', { name: 'Define the role' }).click();
  const content = dialog.getByRole('button', { name: /^Content/u });
  await page.waitForFunction(() => document.querySelector('dialog[open] .multiselect[data-invalid]') !== null, null, { timeout: 5_000 }).catch(() => {});
  assert.equal(await content.evaluate((node) => node.closest('.multiselect')?.hasAttribute('data-invalid')), true, 'the area is not marked');
  assert.match(await describedBy(content), /Choose at least one permission\./u);
  assert.match(await focused(page), /^button "Content /u, 'focus did not move to the area');
});
