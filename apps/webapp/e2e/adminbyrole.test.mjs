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

import { chromium } from 'playwright';

import { MANIFEST, PERMISSIONS, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

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
  { path: '/administration/workspace', buttons: ['Save'], gates: ['STRUCTURE'] },
  { path: '/administration/sign-in', buttons: ['Save', 'Require a new password from everyone'], gates: ['STRUCTURE'] },
  { path: '/administration/identity-provider', buttons: ['Add a provider'], gates: ['MANAGE_MEMBERS'] },
  { path: '/administration/apps', buttons: ['Register it'], gates: ['MANAGE_MEMBERS'] },
  { path: '/administration/groups', buttons: ['Create the group'], gates: ['MANAGE_MEMBERS'] },
  { path: '/administration/service-accounts', buttons: ['Create it'], gates: ['MANAGE_MEMBERS'] },
  { path: '/administration/privacy', buttons: ['Record it'], gates: ['MANAGE_MEMBERS'] },
  { path: '/administration/rules', buttons: ['Write a rule'], gates: ['AUTOMATION'] },
  { path: '/administration/rules/new', buttons: ['Save the rule'], gates: ['AUTOMATION'] },
  { path: '/administration/runs', buttons: [{ role: 'combobox', name: 'Which rule' }], gates: ['AUTOMATION'] },
  { path: '/administration/webhooks', buttons: ['Subscribe'], gates: ['AUTOMATION'] },
  { path: '/administration/ai', buttons: ['Save'], gates: ['STRUCTURE'] },
  { path: '/administration/backup', buttons: ['Add'], gates: ['STRUCTURE', 'DELETE_CONTAINER'] },
  { path: '/administration/retention', buttons: ['Write it', 'Place it'], gates: ['DELETE_CONTAINER'] },
  { path: '/administration/restore', buttons: ['See what it would do'], gates: ['STRUCTURE'] },
  { path: '/administration/audit', buttons: ['Anchor here'], gates: ['STRUCTURE'] },
];

/**
 * What an administrator - every column but the owner's `DELETE_CONTAINER` - is not offered: the
 * backup targets and schedules, the retention rules and the holds are set behind it on the server.
 */
const OWNERS_ONLY = [
  { path: '/administration/backup', buttons: ['Add'] },
  { path: '/administration/retention', buttons: ['Write it', 'Place it'] },
];

/** A control as the table names it: a button by its words, or another role by its name. */
const controlOf = (entry) => (typeof entry === 'string' ? { role: 'button', name: entry } : entry);

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
    const browser = await chromium.launch();
    t.after(() => browser.close());

    const owner = await as(browser, PERMISSIONS.OWNER);
    t.after(() => owner.context.close());
    await owner.page.goto(`${served.origin}${screen.path}`);
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
    for (const permission of screen.gates) {
      await auditor.page.getByText(refusedFor(permission)).first().waitFor({ timeout: 15_000 })
        .catch(() => assert.fail(`the auditor is not told why on ${screen.path} (${permission})`));
    }
    for (const control of screen.buttons.map(controlOf)) {
      assert.equal(await reachable(auditor.page, control), 0, `the auditor is offered "${control.name}" on ${screen.path}`);
    }
    assert.deepEqual(auditor.failures, []);
  });
}

for (const screen of OWNERS_ONLY) {
  test(`chromium: ${screen.path} — an administrator is not offered what only the owner may set`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const admin = await as(browser, PERMISSIONS.ADMIN);
    t.after(() => admin.context.close());
    await admin.page.goto(`${served.origin}${screen.path}`);
    await admin.page.getByText(refusedFor('DELETE_CONTAINER')).first().waitFor({ timeout: 15_000 })
      .catch(() => assert.fail(`the administrator is not told why on ${screen.path}`));
    assert.equal(await admin.page.getByText(refusedFor('STRUCTURE')).count(), 0, 'the administrator is told STRUCTURE is missing');
    for (const control of screen.buttons.map(controlOf)) {
      assert.equal(await reachable(admin.page, control), 0, `the administrator is offered "${control.name}" on ${screen.path}`);
    }
    assert.deepEqual(admin.failures, []);
  });
}
