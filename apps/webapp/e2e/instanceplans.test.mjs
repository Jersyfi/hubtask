// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *Installation → Plans* (UC-INS-13 checks 7 and 8): the list with the workspaces on each plan and
// what each leaves out, the workspaces on none, creating a plan, editing one - the same groups as
// *Defaults*, a confirmation naming how many workspaces it reaches, and the notice where somebody
// saved first - and in single mode nothing of it. Against the stubbed API; Chromium only.

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
      mfa_required_for: { set: false, locked: false, kind: 'CHOICE', options: ['NOBODY', 'ADMINS', 'EVERYONE'], position: 1, ...tighten },
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
    mfa_required_for: { set: false, locked: false, kind: 'CHOICE', options: ['NOBODY', 'ADMINS', 'EVERYONE'], position: 1, ...tighten },
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
