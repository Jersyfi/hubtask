// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *Installation → Plans* (UC-INS-13 checks 7 and 8, UC-INS-10 check 2): the list with the workspaces
// on each plan and what each leaves out, the workspaces on none, creating a plan, editing one - the
// same groups as *Defaults*, a confirmation naming how many workspaces it reaches, and the notice
// where somebody saved first - and in single mode nothing of it; on *Workspaces*, each one's plan,
// moving it through the dry run's confirmation, the plan a new one starts on, and an exception's
// placeholder under a plan. Against the stubbed API; Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { MANIFEST, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const MULTI = { ...MANIFEST, tenancy_mode: 'multi' };

const tighten = { workspace_may: 'TIGHTEN' };
const raise = { workspace_may: 'RAISE' };

/** A plan's document as the server answers it: every key declared, decided or not. */
function planDocument(name, displayName, { workspaces = 0, version = 1, leftOut = [], quotas = {}, signIn = {} } = {}) {
  return {
    name, display_name: displayName, version, workspaces, left_out: leftOut,
    created_at: '2026-10-01T08:00:00Z', updated_at: '2026-10-01T08:00:00Z',
    sign_in: {
      min_length: { set: false, locked: false, kind: 'COUNT', minimum: 8, maximum: 128, position: 0, ...tighten },
      mfa_required_for: { set: false, locked: false, kind: 'CHOICE', options: ['NONE', 'ADMINS', 'EVERYONE'], position: 1, ...tighten },
      ...signIn,
    },
    legal: {
      imprint_url: { set: false, locked: false, kind: 'LINK', position: 0, workspace_may: 'REPLACE' },
      account_url: { set: false, locked: false, kind: 'LINK', position: 1 },
    },
    quotas: {
      accounts: { set: false, locked: false, kind: 'COUNT', minimum: 0, position: 0, ...raise },
      items: { set: false, locked: false, kind: 'COUNT', minimum: 0, position: 1, ...raise },
      ...quotas,
    },
    ai: {},
  };
}

const PLANS = [
  planDocument('private', 'Private', { workspaces: 1284, leftOut: ['AUTOMATION_RULES', 'WEBHOOKS', 'OWN_SIGN_IN_PROVIDERS'] }),
  planDocument('family', 'Family', {
    workspaces: 412, leftOut: ['WEBHOOKS', 'OWN_SIGN_IN_PROVIDERS'],
    quotas: {
      accounts: { set: true, value: 6, locked: true, kind: 'COUNT', minimum: 0, position: 0, ...raise },
      // Stored before the installation locked the request rate: not in force, and refused on a save.
      api_requests_per_minute: { set: true, value: 1200, locked: false, kind: 'COUNT', minimum: 0, position: 2, ...raise },
    },
  }),
  planDocument('business', 'Business', { workspaces: 0 }),
];

const TENANTS = [
  { id: '01936f2a-7c1e-7000-8000-0000000035a1', slug: 'familie-berger', display_name: 'Familie Berger', status: 'ACTIVE', created_at: '2026-09-01T00:00:00Z', plan: 'family', version: 7 },
  { id: '01936f2a-7c1e-7000-8000-0000000035a2', slug: 'kanzlei-roth', display_name: 'Kanzlei Roth', status: 'ACTIVE', created_at: '2026-09-01T00:00:00Z', version: 3 },
];

/** The installation's own level, as `/admin/settings` answers it. */
const LEVEL = {
  sign_in: {
    min_length: { set: true, value: 12, locked: false, kind: 'COUNT', minimum: 8, maximum: 128, position: 0, ...tighten },
    mfa_required_for: { set: false, locked: false, kind: 'CHOICE', options: ['NONE', 'ADMINS', 'EVERYONE'], position: 1, ...tighten },
  },
  legal: {},
  localisation: {},
  quotas: {
    accounts: { set: false, locked: false, kind: 'COUNT', minimum: 0, position: 0, ...raise },
    items: { set: true, value: 20000, locked: false, kind: 'COUNT', minimum: 0, position: 1, ...raise },
    api_requests_per_minute: { set: true, value: 600, locked: true, kind: 'COUNT', minimum: 0, position: 2, ...raise },
  },
  ai: {},
  source: 'database',
  is_enforced_from_file: false,
};

/** A signed-in operator on a multi-mode installation; `answer` takes a route first. */
async function openInstance(browser, { manifest = MULTI, level = LEVEL, answer = () => undefined } = {}) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const sent = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    const handled = answer(route, request, path, sent);
    if (handled !== undefined) return handled;
    if (path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    if (path === '/meta/capabilities') return route.fulfill({ json: manifest });
    if (path === '/admin/plans' && request.method() === 'GET') return route.fulfill({ json: { data: PLANS } });
    if (path === '/admin/tenants' && request.method() === 'GET') return route.fulfill({ json: TENANTS });
    if (path === '/admin/settings' && request.method() === 'GET') return route.fulfill({ json: level });
    if (path === '/admin/backup-targets' && request.method() === 'GET') return route.fulfill({ json: [] });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  return { page, sent, close: () => context.close() };
}

test('chromium: the plans list says how many workspaces each holds, what each leaves out, and who is on none', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await openInstance(browser);
  t.after(close);

  await page.goto(`${served.origin}/instance/plans`);
  const family = page.getByRole('row').filter({ hasText: 'Family' });
  await family.waitFor({ timeout: 10_000 });
  assert.match(await family.textContent() ?? '', /family.*412.*webhooks, own providers/);
  assert.match(await page.getByRole('row').filter({ hasText: 'Business' }).textContent() ?? '', /business.*0.*–/);
  assert.equal(await page.getByTestId('without-plan').innerText(), 'Without a plan: 1 workspace. They have the installation\'s values and every feature.');
  assert.equal(await page.locator('nav').getByText('Plans', { exact: true }).count(), 1, 'the navigation has no Plans row');
});

test('chromium: an operator creates a plan with a short name that stays, and lands in its editor', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await openInstance(browser, {
    answer: (route, request, path, sent) => {
      if (path === '/admin/plans' && request.method() === 'POST') {
        const body = request.postDataJSON();
        sent.push(body);
        if (body.name === 'family') {
          return route.fulfill({ status: 409, json: { code: 'conflict', detail_code: 'admin.plan_name_taken', status: 409, request_id: 'r1', field_errors: [{ path: '/name', code: 'admin.plan_name_taken' }] } });
        }
        return route.fulfill({ status: 201, json: planDocument(body.name, body.display_name) });
      }
      if (path === '/admin/plans/premium') return route.fulfill({ json: planDocument('premium', 'Premium') });
      return undefined;
    },
  });
  t.after(close);

  await page.goto(`${served.origin}/instance/plans`);
  await page.getByRole('button', { name: 'New plan' }).click();
  await page.getByLabel(/^Name/).fill('Premium');
  await page.getByLabel(/^Short name/).fill('family');
  await page.getByRole('button', { name: 'Create the plan' }).click();
  await page.getByText('A plan already has that short name, or had it.', { exact: false }).waitFor({ timeout: 5_000 });

  await page.getByLabel(/^Short name/).fill('premium');
  await page.getByRole('button', { name: 'Create the plan' }).click();
  await page.waitForURL(/\/instance\/plans\/premium$/, { timeout: 5_000 });
  assert.deepEqual(sent.at(-1), { name: 'premium', display_name: 'Premium' });
});

test('chromium: a single installation offers no plans', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await openInstance(browser, { manifest: MANIFEST });
  t.after(close);

  await page.goto(`${served.origin}/instance/plans`);
  await page.getByText('Plans exist where an installation runs several workspaces', { exact: false }).waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('nav').getByText('Plans', { exact: true }).count(), 0);
  assert.equal(await page.getByRole('button', { name: 'New plan' }).count(), 0);
});

test('chromium: the plan editor draws the installation\'s levels, and a save names how many workspaces it reaches', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await openInstance(browser, {
    answer: (route, request, path, sent) => {
      if (path === '/admin/plans/family' && request.method() === 'PUT') {
        sent.push({ body: request.postDataJSON(), ifMatch: request.headers()['if-match'] });
        return route.fulfill({ json: { ...PLANS[1], version: 2 } });
      }
      return undefined;
    },
  });
  t.after(close);

  await page.goto(`${served.origin}/instance/plans/family`);
  await page.getByRole('heading', { name: 'Family', level: 1 }).waitFor({ timeout: 10_000 });
  assert.match(await page.locator('main').innerText(), /412 workspaces/);

  // The installation locks the request rate: its value, no control - and the plan's own, overtaken.
  const locked = page.locator('[data-value="quotas.api_requests_per_minute"]');
  assert.match(await locked.innerText(), /Set by the installation: 600/);
  assert.match(await locked.innerText(), /1200, is not in force/);
  await locked.getByRole('button', { name: 'Remove the plan\'s value' }).click();

  // An undecided key says what it falls back to.
  assert.ok(await page.getByText('Not decided by the plan: the installation\'s applies, 20000.').isVisible());

  // The features: webhooks and own providers left out; automation switched off now.
  assert.equal(await page.getByLabel('Webhooks', { exact: true }).isChecked(), false);
  await page.getByLabel('Automation', { exact: true }).uncheck();

  await page.getByRole('button', { name: 'Save the plan' }).click();
  const dialog = page.getByRole('dialog', { name: 'Save Family?' });
  await dialog.waitFor();
  assert.match(await dialog.innerText(), /It changes at once for all 412 workspaces on this plan\./);
  await dialog.getByRole('button', { name: 'Save', exact: true }).click();
  await page.getByText('Saved.').waitFor({ timeout: 5_000 });

  const put = sent.at(-1);
  assert.equal(put.ifMatch, '"1"');
  assert.deepEqual(put.body.left_out, ['AUTOMATION_RULES', 'WEBHOOKS', 'OWN_SIGN_IN_PROVIDERS']);
  assert.deepEqual(Object.keys(put.body.quotas), ['accounts'], 'the overtaken value was sent again');
  assert.equal(await page.getByRole('button', { name: 'Remove the plan', exact: true }).count(), 0, 'a plan in use offers its removal');
});

test('chromium: a save after somebody else saved says so and overwrites nothing', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await openInstance(browser, {
    answer: (route, request, path) => {
      if (path === '/admin/plans/business' && request.method() === 'PUT') {
        return route.fulfill({ status: 409, json: { code: 'version_conflict', detail_code: 'admin.plan_version_moved', status: 409, request_id: 'r2' } });
      }
      return undefined;
    },
  });
  t.after(close);

  await page.goto(`${served.origin}/instance/plans/business`);
  await page.getByRole('button', { name: 'Save the plan' }).click();
  await page.getByRole('dialog', { name: 'Save Business?' }).getByRole('button', { name: 'Save', exact: true }).click();
  await page.getByText('Somebody saved the plan in the meantime.').waitFor({ timeout: 5_000 });
  assert.ok(await page.getByRole('button', { name: 'Read it again' }).isVisible());
});

test('chromium: a plan nobody is on can be removed, and its short name is said to stay taken', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await openInstance(browser, {
    answer: (route, request, path, sent) => {
      if (path === '/admin/plans/business' && request.method() === 'DELETE') {
        sent.push('removed');
        return route.fulfill({ status: 204, body: '' });
      }
      return undefined;
    },
  });
  t.after(close);

  await page.goto(`${served.origin}/instance/plans/business`);
  await page.getByRole('button', { name: 'Remove the plan', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Remove Business?' });
  assert.match(await dialog.innerText(), /Its short name business stays taken and is never used again\./);
  await dialog.getByRole('button', { name: 'Remove it' }).click();
  await page.waitForURL(/\/instance\/plans$/, { timeout: 5_000 });
  assert.deepEqual(sent, ['removed']);
});

test('chromium: where a file enforces the level, a plan shows its values and the file, and no control', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await openInstance(browser, { level: { ...LEVEL, is_enforced_from_file: true, source: '/etc/hubtask/instance.json' } });
  t.after(close);

  await page.goto(`${served.origin}/instance/plans/family`);
  await page.getByText('/etc/hubtask/instance.json').waitFor({ timeout: 10_000 });
  assert.equal(await page.getByRole('button', { name: 'Save the plan' }).count(), 0);
  assert.equal(await page.getByRole('button', { name: 'Remove the plan', exact: true }).count(), 0);
  await page.goto(`${served.origin}/instance/plans`);
  await page.getByText('/etc/hubtask/instance.json').waitFor({ timeout: 10_000 });
  assert.equal(await page.getByRole('button', { name: 'New plan' }).count(), 0);
});

test('chromium: moving a workspace names what rests, what is over the new limit and which exceptions stay, then moves it on the version read', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const familie = TENANTS[0];
  const { page, sent, close } = await openInstance(browser, {
    answer: (route, request, path, sent) => {
      if (path === `/admin/tenants/${familie.id}:move-plan`) {
        const body = request.postDataJSON();
        sent.push({ body, key: request.headers()['idempotency-key'] });
        return route.fulfill({
          json: {
            tenant: { ...familie, plan: body.plan, version: body.dry_run ? 7 : 8 },
            features_resting: ['WEBHOOKS', 'OWN_SIGN_IN_PROVIDERS'],
            limits_exceeded: [{ quota: 'accounts', used: 9, limit: 3 }, { quota: 'items', used: 2400, limit: 1000 }],
            exceptions_kept: ['items'],
          },
        });
      }
      return undefined;
    },
  });
  t.after(close);

  await page.goto(`${served.origin}/instance/workspaces`);
  const row = page.getByRole('row').filter({ hasText: 'Familie Berger' });
  await row.waitFor({ timeout: 10_000 });
  assert.match(await row.textContent() ?? '', /Family/);
  assert.match(await page.getByRole('row').filter({ hasText: 'Kanzlei Roth' }).textContent() ?? '', /Without a plan/);

  await row.getByRole('button', { name: 'Change plan' }).click();
  const choosing = page.getByRole('dialog', { name: 'Change Familie Berger\'s plan' });
  const offered = await choosing.getByRole('combobox', { name: 'Plan' }).locator('option:not([disabled])').allInnerTexts();
  assert.deepEqual(offered, ['Private', 'Business', 'Without a plan'], 'the plan it is on is offered');
  await choosing.getByRole('combobox', { name: 'Plan' }).selectOption('private');

  const confirming = page.getByRole('dialog', { name: 'Put Familie Berger on Private?' });
  const changes = confirming.getByTestId('move-changes');
  await changes.waitFor({ timeout: 5_000 });
  assert.deepEqual(await changes.locator('li').allInnerTexts(), [
    'Rest from now: Webhooks, Own sign-in providers. Nothing is deleted.',
    'Over the new limit: People 9 of 3, Entries 2400 of 1000. What exists stays; only something new is refused.',
    'Stays as an exception: Entries, set for this workspace.',
  ]);
  assert.equal(sent.length, 1);
  assert.deepEqual(sent[0].body, { plan: 'private', expected_version: 7, dry_run: true });

  await confirming.getByRole('button', { name: 'Put it on Private' }).click();
  await confirming.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.equal(sent.length, 2);
  assert.deepEqual(sent[1].body, { plan: 'private', expected_version: 7 }, 'the move is not a dry run and carries the version read');
  assert.notEqual(sent[0].key, sent[1].key, 'the dry run and the move share a key');
});

test('chromium: a move somebody overtook is refused and says so', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const familie = TENANTS[0];
  const { page, close } = await openInstance(browser, {
    answer: (route, request, path) => {
      if (path === `/admin/tenants/${familie.id}:move-plan`) {
        return route.fulfill({ status: 409, json: { code: 'version_conflict', detail_code: 'admin.tenant_version_moved', status: 409, request_id: 'r9' } });
      }
      return undefined;
    },
  });
  t.after(close);

  await page.goto(`${served.origin}/instance/workspaces`);
  const row = page.getByRole('row').filter({ hasText: 'Familie Berger' });
  await row.waitFor({ timeout: 10_000 });
  await row.getByRole('button', { name: 'Change plan' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('combobox', { name: 'Plan' }).selectOption('-');
  await dialog.getByText('The workspace changed while you were editing; read it again.').waitFor({ timeout: 5_000 });
  assert.equal(await dialog.getByRole('button', { name: 'Take it off its plan' }).count(), 0, 'a refused dry run offers the move');
});

test('chromium: a new workspace starts without a plan among several, and on the one chosen', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await openInstance(browser, {
    answer: (route, request, path, sent) => {
      if (path === '/admin/tenants' && request.method() === 'POST') {
        const body = request.postDataJSON();
        sent.push(body);
        return route.fulfill({
          status: 201,
          json: {
            id: '01936f2a-7c1e-7000-8000-0000000035a9', slug: body.slug, display_name: body.display_name, status: 'ACTIVE',
            created_at: '2026-10-18T00:00:00Z', plan: body.plan ?? null, version: 1,
            owner_account_id: '01936f2a-7c1e-7000-8000-0000000035b9', owner_redemption_token: 'e2e-token',
            default_hub_id: '01936f2a-7c1e-7000-8000-0000000035c9', example_collection_id: '01936f2a-7c1e-7000-8000-0000000035d9',
          },
        });
      }
      return undefined;
    },
  });
  t.after(close);

  await page.goto(`${served.origin}/instance/workspaces`);
  await page.getByRole('main').getByRole('button', { name: 'New workspace' }).click();
  const dialog = page.getByRole('dialog', { name: 'New workspace' });
  const plan = dialog.getByRole('combobox', { name: 'Plan' });
  assert.equal(await plan.inputValue(), '', 'among three plans, one was chosen for the operator');
  assert.deepEqual(await plan.locator('option').allInnerTexts(), ['Without a plan', 'Private', 'Family', 'Business']);
  await dialog.getByRole('textbox', { name: 'Address', exact: true }).fill('familie-weber');
  await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill('Familie Weber');
  await dialog.getByRole('textbox', { name: 'The owner\'s email address' }).fill('weber@example.org');
  await plan.selectOption('family');
  await dialog.getByRole('button', { name: 'Create it' }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.equal(sent.length, 1);
  assert.equal(sent[0].plan, 'family');
});

test('chromium: where the installation has one plan, a new workspace starts on it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await openInstance(browser, {
    answer: (route, request, path) =>
      path === '/admin/plans' && request.method() === 'GET' ? route.fulfill({ json: { data: [PLANS[1]] } }) : undefined,
  });
  t.after(close);

  await page.goto(`${served.origin}/instance/workspaces`);
  await page.getByRole('columnheader', { name: 'Plan' }).waitFor({ timeout: 10_000 });
  await page.getByRole('main').getByRole('button', { name: 'New workspace' }).click();
  assert.equal(await page.getByRole('dialog', { name: 'New workspace' }).getByRole('combobox', { name: 'Plan' }).inputValue(), 'family');
});

test('chromium: under a plan, an empty exception shows the plan\'s value where the installation does not lock it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await openInstance(browser);
  t.after(close);

  await page.goto(`${served.origin}/instance/workspaces`);
  const row = page.getByRole('row').filter({ hasText: 'Familie Berger' });
  await page.getByRole('columnheader', { name: 'Plan' }).waitFor({ timeout: 10_000 });
  await row.getByRole('button', { name: 'Limits', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Limits for Familie Berger' });
  const people = dialog.getByRole('spinbutton', { name: 'People' });
  assert.equal(await people.getAttribute('placeholder'), '6', 'the plan sets people');
  assert.match(await dialog.getByText('Empty uses the limit of the plan Family: 6.').innerText(), /Family/);
  assert.equal(await dialog.getByRole('spinbutton', { name: 'Requests per minute' }).getAttribute('placeholder'), '600', 'the installation locks the rate; the plan\'s 1200 is not in force');
  assert.equal(await dialog.getByRole('spinbutton', { name: 'Entries', exact: true }).getAttribute('placeholder'), '20000', 'the plan sets no entries');

  await dialog.getByRole('button', { name: 'Cancel' }).last().click();
  await page.getByRole('row').filter({ hasText: 'Kanzlei Roth' }).getByRole('button', { name: 'Limits', exact: true }).click();
  assert.equal(await page.getByRole('dialog', { name: 'Limits for Kanzlei Roth' }).getByRole('spinbutton', { name: 'People' }).getAttribute('placeholder'), null, 'without a plan, the installation sets no people');
});

/** Presses Tab until `locator` has the focus, as a keyboard user reaches it; fails after `limit`. */
async function tabTo(page, locator, limit = 80) {
  for (let i = 0; i < limit; i += 1) {
    if (await locator.evaluate((element) => element === document.activeElement)) return;
    await page.keyboard.press('Tab');
  }
  assert.fail(`Tab never reached ${locator}`);
}

/**
 * Whether the focus stays out of the page behind `dialog`. A modal `<dialog>` cycles Tab through its
 * own controls and the browser's, so between the last and the first the page's focus is on no
 * element (`body`) - never on one behind the dialog.
 */
const focusIsIn = (dialog) =>
  dialog.evaluate((element) => element.contains(document.activeElement) || document.activeElement === document.body);

test('chromium: the plans list, the editor and the move dialog work from the keyboard', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const familie = TENANTS[0];
  const { page, sent, close } = await openInstance(browser, {
    answer: (route, request, path, sent) => {
      if (path === `/admin/tenants/${familie.id}:move-plan`) {
        const body = request.postDataJSON();
        sent.push(body);
        return route.fulfill({ json: { tenant: { ...familie, plan: body.plan }, features_resting: [], exceptions_kept: [] } });
      }
      if (path === '/admin/backup-targets') return route.fulfill({ json: [] });
      return undefined;
    },
  });
  t.after(close);

  await page.goto(`${served.origin}/instance/plans`);
  const family = page.getByRole('link', { name: 'Family' });
  await family.waitFor({ timeout: 10_000 });
  await tabTo(page, family);
  await page.keyboard.press('Enter');
  await page.waitForURL(/\/instance\/plans\/family$/, { timeout: 5_000 });

  const save = page.getByRole('button', { name: 'Save the plan' });
  await save.waitFor({ timeout: 10_000 });
  await tabTo(page, save, 120);
  await page.keyboard.press('Enter');
  const confirming = page.getByRole('dialog', { name: 'Save Family?' });
  await confirming.waitFor({ timeout: 5_000 });
  for (let i = 0; i < 6; i += 1) {
    await page.keyboard.press('Tab');
    assert.ok(await focusIsIn(confirming), 'the focus left the open confirmation');
  }
  await page.keyboard.press('Escape');
  await confirming.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.ok(await save.evaluate((element) => element === document.activeElement), 'the focus did not return to Save the plan');

  await page.goto(`${served.origin}/instance/workspaces`);
  const change = page.getByRole('row').filter({ hasText: 'Familie Berger' }).getByRole('button', { name: 'Change plan' });
  await change.waitFor({ timeout: 10_000 });
  await tabTo(page, change);
  await page.keyboard.press('Enter');
  const dialog = page.getByRole('dialog', { name: 'Change Familie Berger\'s plan' });
  await dialog.waitFor({ timeout: 5_000 });
  for (let i = 0; i < 6; i += 1) {
    await page.keyboard.press('Tab');
    assert.ok(await focusIsIn(dialog), 'the focus left the open move dialog');
  }
  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.ok(await change.evaluate((element) => element === document.activeElement), 'the focus did not return to Change plan');

  await page.keyboard.press('Enter');
  await dialog.waitFor({ timeout: 5_000 });
  const plan = dialog.getByRole('combobox', { name: 'Plan' });
  // The choice itself is the native select's, whose keyboard is the platform's picker (a popup
  // headless Chromium does not draw): what is walked here is reaching it and leaving it.
  await tabTo(page, plan, 10);
  await plan.selectOption('private');
  const confirm = page.getByRole('button', { name: 'Put it on Private' });
  await confirm.waitFor({ timeout: 5_000 });
  assert.match(await page.getByTestId('move-changes').innerText(), /Nothing rests/);
  await tabTo(page, confirm, 10);
  await page.keyboard.press('Enter');
  await page.getByRole('dialog', { name: 'Put Familie Berger on Private?' }).waitFor({ state: 'hidden', timeout: 5_000 });
  assert.deepEqual(sent.map((body) => body.dry_run === true), [true, false]);
});

test('chromium: the move dialog closed and opened again at once stays open', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await openInstance(browser);
  t.after(close);

  await page.goto(`${served.origin}/instance/workspaces`);
  const change = page.getByRole('row').filter({ hasText: 'Familie Berger' }).getByRole('button', { name: 'Change plan' });
  await change.waitFor({ timeout: 10_000 });
  // Held before it opens: behind a modal the opener is inert, and no role query reaches it.
  const opener = await change.elementHandle();
  await change.focus();
  await page.keyboard.press('Enter');
  const dialog = page.getByRole('dialog', { name: 'Change Familie Berger\'s plan' });
  await dialog.waitFor({ timeout: 5_000 });
  // Escape and the opener within one task, so the native close event of the first closing is
  // certainly still queued when the dialog opens again: it must not close the dialog it finds open.
  // Svelte applies a change in a microtask, which still runs within the task.
  const closed = await opener.evaluate(async (element) => {
    const open = document.querySelector('dialog[open]');
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }));
    for (let i = 0; i < 50 && open?.open; i += 1) await Promise.resolve();
    const wasClosed = open !== null && !open.open;
    element.click();
    return wasClosed;
  });
  assert.ok(closed, 'Escape did not close the dialog within the task');
  await page.evaluate(() => new Promise((resolve) => setTimeout(resolve, 200)));
  assert.ok(await dialog.isVisible(), 'the reopened move dialog closed itself');
});

test('chromium: the editor says each refusal where it belongs', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  let creations = 0;
  const { page, close } = await openInstance(browser, {
    answer: (route, request, path) => {
      if (path === '/admin/plans/family' && request.method() === 'PUT') {
        return route.fulfill({ status: 422, json: { code: 'validation_failed', detail_code: 'admin.plan_looser_than_installation', status: 422, request_id: 'r3',
          field_errors: [{ path: '/sign_in/min_length', code: 'admin.plan_looser_than_installation', params: { setting: 'the minimum length' } }] } });
      }
      if (path === '/admin/plans/business' && request.method() === 'DELETE') {
        return route.fulfill({ status: 409, json: { code: 'conflict', detail_code: 'admin.plan_in_use', status: 409, request_id: 'r4' } });
      }
      if (path === '/admin/plans/business' && request.method() === 'PUT') {
        return route.fulfill({ status: 404, json: { code: 'not_found', detail_code: 'admin.plan_not_found', status: 404, request_id: 'r5' } });
      }
      if (path === '/admin/plans' && request.method() === 'POST') {
        creations += 1;
        const [code, field] = creations === 1 ? ['admin.plan_name_invalid', '/name'] : ['admin.plan_display_name_invalid', '/display_name'];
        return route.fulfill({ status: 422, json: { code: 'validation_failed', detail_code: code, status: 422, request_id: 'r6',
          field_errors: [{ path: field, code }] } });
      }
      return undefined;
    },
  });
  t.after(close);

  // The account link is the plan's value alone: a plan cannot lock it.
  await page.goto(`${served.origin}/instance/plans/family`);
  const link = page.locator('[data-value="legal.account_url"]');
  await link.waitFor({ timeout: 10_000 });
  assert.equal(await link.getByRole('combobox', { name: 'Workspaces may' }).count(), 0, 'the account link offers a lock');

  // A value looser than the installation's: at its field, not as a banner alone.
  await page.getByRole('button', { name: 'Save the plan' }).click();
  await page.getByRole('dialog', { name: 'Save Family?' }).getByRole('button', { name: 'Save', exact: true }).click();
  const field = page.locator('[data-value="sign_in.min_length"]');
  await field.getByText('A plan may only make the minimum length stricter than the installation does.').waitFor({ timeout: 5_000 });

  // Removing a plan somebody put a workspace on meanwhile: the server's refusal, said.
  await page.goto(`${served.origin}/instance/plans/business`);
  await page.getByRole('button', { name: 'Remove the plan', exact: true }).click();
  await page.getByRole('dialog', { name: 'Remove Business?' }).getByRole('button', { name: 'Remove it' }).click();
  await page.getByText('Workspaces are on this plan. Move them to another plan before removing it.').first().waitFor({ timeout: 5_000 });

  // A plan removed meanwhile: said gone, with the way back to the list.
  await page.goto(`${served.origin}/instance/plans/business`);
  await page.getByRole('button', { name: 'Save the plan' }).click();
  await page.getByRole('dialog', { name: 'Save Business?' }).getByRole('button', { name: 'Save', exact: true }).click();
  await page.getByText('This plan no longer exists.').waitFor({ timeout: 5_000 });

  // A short name the server will not take: at the field.
  await page.goto(`${served.origin}/instance/plans`);
  await page.getByRole('button', { name: 'New plan' }).click();
  const create = page.getByRole('dialog', { name: 'New plan' });
  await create.getByRole('textbox', { name: 'Name', exact: true }).fill('Premium');
  await create.getByRole('textbox', { name: 'Short name' }).fill('-x');
  await create.getByRole('button', { name: 'Create the plan' }).click();
  await create.getByText('A plan\'s short name is 2 to 40 lowercase letters, digits and hyphens, starting with a letter or digit.').waitFor({ timeout: 5_000 });
  await create.getByRole('textbox', { name: 'Short name' }).fill('premium');
  await create.getByRole('button', { name: 'Create the plan' }).click();
  const named = create.getByRole('textbox', { name: 'Name', exact: true });
  await page.waitForFunction(() => document.querySelector('dialog[open] [aria-invalid="true"]') !== null, undefined, { timeout: 5_000 });
  assert.equal(await named.getAttribute('aria-invalid'), 'true', 'the name\'s refusal is not at the name');
});

test('chromium: an unknown plan is said at the plan field when provisioning, and in the move dialog', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const familie = TENANTS[0];
  const unknown = { status: 422, json: { code: 'validation_failed', detail_code: 'admin.plan_unknown', status: 422, request_id: 'r7',
    field_errors: [{ path: '/plan', code: 'admin.plan_unknown' }] } };
  const { page, close } = await openInstance(browser, {
    answer: (route, request, path) => {
      if (path === '/admin/tenants' && request.method() === 'POST') return route.fulfill(unknown);
      if (path === `/admin/tenants/${familie.id}:move-plan`) return route.fulfill(unknown);
      if (path === '/admin/backup-targets') return route.fulfill({ json: [] });
      return undefined;
    },
  });
  t.after(close);

  await page.goto(`${served.origin}/instance/workspaces`);
  await page.getByRole('columnheader', { name: 'Plan' }).waitFor({ timeout: 10_000 });
  await page.getByRole('main').getByRole('button', { name: 'New workspace' }).click();
  const dialog = page.getByRole('dialog', { name: 'New workspace' });
  await dialog.getByRole('textbox', { name: 'Address', exact: true }).fill('familie-weber');
  await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill('Familie Weber');
  await dialog.getByRole('textbox', { name: 'The owner\'s email address' }).fill('weber@example.org');
  await dialog.getByRole('combobox', { name: 'Plan' }).selectOption('family');
  await dialog.getByRole('button', { name: 'Create it' }).click();
  const plan = dialog.getByRole('combobox', { name: 'Plan' });
  await page.waitForFunction(() => document.querySelector('[aria-invalid="true"]') !== null, undefined, { timeout: 5_000 });
  assert.equal(await plan.getAttribute('aria-invalid'), 'true', 'the refusal is not at the plan field');
  await dialog.getByRole('button', { name: 'Cancel' }).last().click();

  await page.getByRole('row').filter({ hasText: 'Familie Berger' }).getByRole('button', { name: 'Change plan' }).click();
  const moving = page.getByRole('dialog');
  await moving.getByRole('combobox', { name: 'Plan' }).selectOption('business');
  await moving.getByText('There is no plan with that name to put the workspace on.').waitFor({ timeout: 5_000 });
});

test('chromium: an earlier choice\'s late dry run is not shown under a later choice', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const familie = TENANTS[0];
  const { page, close } = await openInstance(browser, {
    answer: (route, request, path) => {
      if (path !== `/admin/tenants/${familie.id}:move-plan`) return undefined;
      const body = request.postDataJSON();
      const delay = body.plan === 'private' ? 800 : 0;
      return new Promise((resolve) => setTimeout(resolve, delay)).then(() => route.fulfill({ json: {
        tenant: { ...familie, plan: body.plan },
        features_resting: body.plan === 'private' ? ['WEBHOOKS'] : [],
        exceptions_kept: [],
      } }));
    },
  });
  t.after(close);

  await page.goto(`${served.origin}/instance/workspaces`);
  await page.getByRole('row').filter({ hasText: 'Familie Berger' }).getByRole('button', { name: 'Change plan' }).click();
  const plan = page.getByRole('dialog').getByRole('combobox', { name: 'Plan' });
  await plan.selectOption('private');
  await plan.selectOption('business');
  await page.getByRole('dialog', { name: 'Put Familie Berger on Business?' }).getByTestId('move-changes').waitFor({ timeout: 5_000 });
  await page.waitForTimeout(1_200);
  assert.match(await page.getByTestId('move-changes').innerText(), /Nothing rests/, 'Private\'s answer replaced Business\'s');
});

test('chromium: a workspace read without a version moves without guessing one', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { version: _, ...unversioned } = TENANTS[1];
  const { page, sent, close } = await openInstance(browser, {
    answer: (route, request, path, sent) => {
      if (path === '/admin/tenants' && request.method() === 'GET') return route.fulfill({ json: [TENANTS[0], unversioned] });
      if (path === `/admin/tenants/${unversioned.id}:move-plan`) {
        sent.push(request.postDataJSON());
        return route.fulfill({ json: { tenant: { ...unversioned, plan: 'business' }, features_resting: [], exceptions_kept: [] } });
      }
      return undefined;
    },
  });
  t.after(close);

  await page.goto(`${served.origin}/instance/workspaces`);
  await page.getByRole('row').filter({ hasText: 'Kanzlei Roth' }).getByRole('button', { name: 'Change plan' }).click();
  await page.getByRole('dialog').getByRole('combobox', { name: 'Plan' }).selectOption('business');
  await page.getByRole('button', { name: 'Put it on Business' }).click();
  await page.getByRole('dialog').waitFor({ state: 'hidden', timeout: 5_000 });
  assert.deepEqual(sent, [{ plan: 'business', dry_run: true }, { plan: 'business' }]);
});

test('chromium: a single installation shows no plan on Workspaces; an enforced one still moves and provisions on plans', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const single = await openInstance(browser, { manifest: MANIFEST });
  t.after(single.close);
  await single.page.goto(`${served.origin}/instance/workspaces`);
  await single.page.getByRole('row').filter({ hasText: 'Familie Berger' }).waitFor({ timeout: 10_000 });
  assert.equal(await single.page.getByRole('columnheader', { name: 'Plan' }).count(), 0);
  assert.equal(await single.page.getByRole('button', { name: 'Change plan' }).count(), 0);

  // The file enforces the level's values, not which plan a workspace is on.
  const enforced = await openInstance(browser, {
    level: { ...LEVEL, is_enforced_from_file: true, source: '/etc/hubtask/instance.json' },
    answer: (route, request, path) => (path === '/admin/backup-targets' ? route.fulfill({ json: [] }) : undefined),
  });
  t.after(enforced.close);
  await enforced.page.goto(`${served.origin}/instance/workspaces`);
  await enforced.page.getByRole('columnheader', { name: 'Plan' }).waitFor({ timeout: 10_000 });
  assert.equal(await enforced.page.getByRole('button', { name: 'Change plan' }).count(), 2);
  await enforced.page.getByRole('main').getByRole('button', { name: 'New workspace' }).click();
  assert.equal(await enforced.page.getByRole('dialog', { name: 'New workspace' }).getByRole('combobox', { name: 'Plan' }).count(), 1);
});
