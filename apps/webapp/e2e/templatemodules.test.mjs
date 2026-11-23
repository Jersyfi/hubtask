// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A template a fresh workspace cannot take because its modules are off (UC-CFG-01): the dialog
// names the module in words, offers its owner to switch it on, plans again and applies - and
// shows neither a message code nor a key path. A template taken for an existing hub - or for a
// collection, in its hub - switches its products on in that hub (UC-INS-21 check 13): the
// workspace's part names the hub with its switch, a product the hub still lacks is switched on
// there, not at the workspace again, and the banner says which of the two the press does. A
// hub's owner who may not shape the workspace may press the hub's own switch, and only that one.
//
// Chromium only; what the server decides is its own tests' (config/ModuleSwitch_test.go names
// the hub in the apply's switch).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { ACCOUNT, COLLECTION, DEFAULT_STRUCTURE, HUB, MANIFEST, PAGE, PERMISSIONS, PRESETS, WORKSPACE, forgetUnstubbed, stub } from './fixture.mjs';
import { productWalk } from './productwalk.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const SCRUM = {
  key: 'scrum_team', version: 1, name: 'Scrum team', origin: 'HUBTASK', provenance: 'scrum_team@1', document: {},
  created_at: '2026-09-01T00:00:00Z', update_available: false, taken_version: null,
};
const counts = { added: 0, changed: 0, removed: 0 };
const flags = { hides_data: false, changes_permissions: false };
const refused = {
  template: SCRUM.key, upgrade: [], document: {}, mode: 'MERGE', base_hash: 'b0', plan_hash: null, changes: [], operations: [], flags, counts,
  refusals: [
    { path: '/types', code: 'module.off', params: { module: 'WORKFLOWS' } },
    { path: '/workflows', code: 'module.off', params: { module: 'WORKFLOWS' } },
  ],
};
const planned = {
  ...refused, base_hash: 'b1', plan_hash: 'p1', refusals: [], counts: { ...counts, added: 1 },
  changes: [{ path: 'workflows.scrum', section: 'workflows', key: 'scrum', op: 'ADD', fields: [] }],
};

async function as(browser, role) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  forgetUnstubbed();
  const written = [];
  let workflowsOn = false;
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    if (method !== 'GET') written.push({ method, path, body: request.postDataJSON?.() ?? null });
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: PERMISSIONS[role] } });
    if (path === '/memberships') {
      const rows = url.searchParams.get('scope_type') === 'TENANT' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role }] : [];
      return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: [] } });
    }
    if (path === '/tenant' && method === 'GET') return route.fulfill({ json: { ...WORKSPACE, structure: { ...DEFAULT_STRUCTURE, default_preset: null, default_preset_id: null } } });
    if (path === '/presets' && method === 'GET') return route.fulfill({ json: PRESETS });
    if (path === '/config/templates' && method === 'GET') return route.fulfill({ json: { data: [SCRUM] } });
    if (path === `/config/templates/${SCRUM.key}:plan`) return route.fulfill({ json: workflowsOn ? planned : refused });
    if (path === '/tenant/modules/WORKFLOWS' && method === 'PUT') {
      workflowsOn = true;
      return route.fulfill({ json: { data: [] } });
    }
    if (path === '/config:apply') return route.fulfill({ json: { version: 2, applied: true } });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  return { context, page, failures, written };
}

async function openScrum(page) {
  await page.goto(`${served.origin}/administration/templates`);
  const card = page.getByRole('button', { name: 'Scrum team', exact: true });
  await card.waitFor({ timeout: 15_000 });
  await card.click();
  const dialog = page.getByRole('dialog');
  await dialog.waitFor({ timeout: 5_000 });
  return dialog;
}

test('chromium: an empty workspace switches Workflows on and takes Scrum team', async (t) => {
  const browser = await sharedChromium(t);
  const { context, page, failures, written } = await as(browser, 'OWNER');
  t.after(() => context.close());

  const dialog = await openScrum(page);
  await dialog.locator('[data-template-needs-modules]').waitFor({ timeout: 5_000 });
  const said = await dialog.textContent();
  assert.match(said, /Needs Workflows, which is switched off here/);
  for (const raw of ['module.off', 'config.section_withheld', '/workflows', '/types']) {
    assert.ok(!said.includes(raw), `the dialog shows ${raw}`);
  }
  await dialog.getByRole('button', { name: 'Switch on Workflows and use this template' }).click();
  await dialog.getByText('Adds the workflow “scrum”').waitFor({ timeout: 5_000 });
  assert.ok(written.some((write) => write.method === 'PUT' && write.path === '/tenant/modules/WORKFLOWS' && write.body?.switch === 'ON'), 'the module was not switched on');
  await dialog.getByRole('button', { name: 'Take template' }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.ok(written.some((write) => write.path === '/config:apply'), 'the template was not applied');
  assert.deepEqual(failures, []);
});

// §3.2: a GROUP question in an empty workspace offers *Create a group* in place, and the group made
// there is the answer - the plan is asked again with it - so no part is left out for want of one.
test('chromium: a group question makes its group in place and answers with it', async (t) => {
  const browser = await sharedChromium(t);
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  forgetUnstubbed();
  const ITIL = { ...SCRUM, key: 'itil_service_desk', name: 'ITIL service desk', provenance: 'itil_service_desk@1',
    questions: [{ key: 'first_group', kind: 'GROUP', required: false, options: [] }] };
  const GROUP = { id: '01a0e2e0-0000-7000-8000-0000000007a1', name: 'First line', description: null, version: 1, member_count: 0 };
  const groups = [];
  const plans = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    if (path === '/meta/capabilities') return route.fulfill({ json: MANIFEST });
    if (path === '/tenant' && method === 'GET') return route.fulfill({ json: { ...WORKSPACE, structure: { ...DEFAULT_STRUCTURE, default_preset: null, default_preset_id: null } } });
    if (path === '/presets' && method === 'GET') return route.fulfill({ json: PRESETS });
    if (path === '/config/templates' && method === 'GET') return route.fulfill({ json: { data: [ITIL] } });
    if (path === `/config/templates/${ITIL.key}:plan`) {
      plans.push(request.postDataJSON());
      return route.fulfill({ json: { ...planned, template: ITIL.key } });
    }
    if (path === '/groups' && method === 'GET') return route.fulfill({ json: { ...PAGE, data: groups } });
    if (path === '/groups' && method === 'POST') {
      groups.push({ ...GROUP, name: request.postDataJSON().name });
      return route.fulfill({ status: 201, json: groups[0] });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/administration/templates`);
  const card = page.getByRole('button', { name: 'ITIL service desk', exact: true });
  await card.waitFor({ timeout: 15_000 });
  await card.click();
  const dialog = page.getByRole('dialog');
  const make = dialog.locator('[data-create-group="first_group"]');
  await make.waitFor({ timeout: 5_000 });
  await make.getByLabel('Or a new group').fill('First line');
  await make.getByRole('button', { name: 'Create a group' }).click();
  await page.waitForFunction(() => document.querySelector('[data-question="first_group"]')?.value?.length > 0, null, { timeout: 5_000 });
  assert.equal(await dialog.locator('[data-question="first_group"] option:checked').textContent(), 'First line');
  assert.equal(plans.at(-1)?.answers?.first_group, GROUP.id, 'the plan was not asked again with the new group');
  assert.deepEqual(failures, []);
});

const PROJECT = { ...SCRUM, key: 'classic_project', name: 'Classic project', provenance: 'classic_project@1' };
const GARDEN = { ...HUB, id: '01a0e2e0-0000-7000-8000-00000014b0a1', name: 'Garden', order_key: 'a1' };

/**
 * Plans at the workspace and in its two hubs, kept as the server keeps them (ADR-0109): on at the
 * workspace changes no existing hub but those its switch names - `hub_ids`, or an apply's
 * `module_hub_ids` - and a hub's plan is refused while that hub has Plans off.
 */
function hubTemplate({ workspaceOn, hubOff, workspaceTook }) {
  const on = workspaceOn ? ['PLANNING'] : [];
  const own = new Map(hubOff ? [[HUB.id, false]] : []);
  const hubs = [HUB, GARDEN];
  let took = workspaceTook;
  const hubHas = (id) => on.includes('PLANNING') && (own.get(id) ?? true);
  const switchWorkspace = (named = []) => {
    if (!on.includes('PLANNING')) {
      for (const hub of hubs) if (!own.has(hub.id)) own.set(hub.id, false);
      on.push('PLANNING');
    }
    for (const id of named) own.set(id, true);
  };
  const answer = (method, path, query, body) => {
    if (path === '/containers' && method === 'GET') return { json: { ...PAGE, data: query.get('type') === 'HUB' ? hubs : [] } };
    if (path === '/config/templates' && method === 'GET') return { json: { data: [{ ...PROJECT, workspace_first: !took }] } };
    if (path === `/hubs/${HUB.id}/sample-entries` && method === 'GET') return { json: { samples: [] } };
    if (path === `/config/templates/${PROJECT.key}:plan` && method === 'POST') {
      const hubId = body?.hub_id ?? null;
      const collectionId = body?.collection_id ?? null;
      // A collection's part needs its products in the collection's hub.
      const placeHub = hubId ?? (collectionId ? COLLECTION.parent_id : null);
      const refusals = placeHub && !hubHas(placeHub) ? [{ path: '/modules', code: 'module.off', params: { module: 'PLANNING' } }] : [];
      const place = { template: PROJECT.key, hub_id: hubId, ...(collectionId ? { collection_id: collectionId } : {}) };
      return { json: refusals.length > 0 ? { ...refused, ...place, refusals } : { ...planned, ...place } };
    }
    if (path === '/config:apply' && method === 'POST') {
      if (!body.hub_id && !body.collection_id) {
        took = true;
        switchWorkspace(body.module_hub_ids);
      }
      return { json: { version: 2, applied: true } };
    }
    if (path === '/tenant/modules/PLANNING' && method === 'PUT') {
      if (body.hub_id) {
        if (!on.includes('PLANNING')) return { status: 403, json: { code: 'forbidden', detail_code: 'setting.not_allowed', status: 403, request_id: 'req_e2e' } };
        own.set(body.hub_id, true);
      } else {
        switchWorkspace(body.hub_ids);
      }
      return { json: { modules: [{ module: 'PLANNING', on: true, source: body.hub_id ? 'HUB' : 'WORKSPACE' }] } };
    }
    return undefined;
  };
  return { on, answer, hubHas };
}

/**
 * Opens the template's dialog on a hub's settings - or a collection's, `isCollection` - in
 * `locale`; `isHubOnly` is a reader who shapes the hub (its owner) but not the workspace (a member).
 */
async function openForHub(browser, t, state, { isCollection = false, locale = 'en', isHubOnly = false } = {}) {
  const shared = hubTemplate(state);
  const answer = (method, path, query, body) => {
    if (path === '/accounts/me' && method === 'GET') return { json: { ...ACCOUNT, locale } };
    if (isHubOnly && path === '/memberships' && query.get('scope_type') === 'TENANT') {
      return { json: { ...PAGE, data: [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'MEMBER' }], grantable_roles: [] } };
    }
    return shared.answer(method, path, query, body);
  };
  const walk = await productWalk(browser, { modules: shared.on, answer, manifest: isHubOnly ? { workspace_permissions: PERMISSIONS.MEMBER } : {} });
  t.after(walk.close);
  const name = locale === 'de' ? 'Klassisches Projekt' : 'Classic project';
  if (isCollection) {
    await walk.page.goto(`${served.origin}/collections/${COLLECTION.id}/settings/template`);
    const use = walk.page.getByRole('row', { name: new RegExp(`^${name}`, 'u') }).getByRole('button', { name: 'Use', exact: true });
    await use.waitFor({ timeout: 15_000 });
    await use.click();
  } else {
    await walk.page.goto(`${served.origin}/hubs/${HUB.id}/settings/template?template=${PROJECT.key}`);
  }
  const dialog = walk.page.getByRole('dialog', { name });
  await dialog.waitFor({ timeout: 15_000 });
  return { ...walk, dialog, hubHas: shared.hubHas, switches: () => walk.written.filter((each) => each.method === 'PUT').map((each) => ({ path: each.path, body: each.body })) };
}

test('chromium: a template taken for a hub names that hub in the workspace part\'s switch, so the hub takes its part (UC-INS-21 check 13)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, dialog, written, hubHas, switches, errors } = await openForHub(browser, t, { workspaceOn: false, hubOff: false, workspaceTook: false });

  await dialog.locator('[data-template-workspace-first]').waitFor({ timeout: 10_000 });
  await dialog.getByRole('button', { name: 'Take template' }).click();
  await dialog.locator('[data-template-workspace-first]').waitFor({ state: 'detached', timeout: 10_000 });
  await dialog.getByText('Adds the workflow “scrum”').waitFor({ timeout: 10_000 });
  const applies = () => written.filter((each) => each.path === '/config:apply').map((each) => each.body);
  assert.deepEqual(applies()[0].module_hub_ids, [HUB.id], 'the workspace part left without the hub it was taken for');
  assert.equal(applies()[0].hub_id, undefined);
  assert.equal(await dialog.locator('[data-template-needs-modules]').count(), 0, 'the hub was left without the product its template switches on');
  await dialog.getByRole('button', { name: 'Take template' }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 10_000 });
  assert.deepEqual(applies().map((each) => each.hub_id ?? ''), ['', HUB.id]);
  assert.equal(applies()[1].module_hub_ids, undefined, 'the hub\'s own part names hubs');
  assert.deepEqual(switches(), [], 'a switch was sent beside the apply');
  assert.equal(hubHas(HUB.id), true);
  assert.equal(hubHas(GARDEN.id), false, 'a hub nobody chose was switched on');
  await page.getByText('Classic project is in use.').first().waitFor({ timeout: 5_000 });
  assert.deepEqual(errors(), []);
});

const SAID = {
  hub: 'Switching it on turns it on in this hub; no other hub changes, and nothing else does until you apply the template.',
  hubWorkspace: 'Switching it on turns it on for the whole workspace as well: it shows its screens to everyone in the workspace, and hubs made from now on have it. Of the hubs that exist, only this one gets it; nothing else changes until you apply the template.',
};

for (const [workspaceOn, expected, words, said] of [
  [true, { hub_id: HUB.id }, 'on at the workspace and off in the hub: the hub\'s own switch', SAID.hub],
  [false, { hub_ids: [HUB.id] }, 'off at the workspace: the workspace\'s switch naming the hub', SAID.hubWorkspace],
]) {
  test(`chromium: a hub's template that needs a product the hub lacks switches it on in that hub - ${words} (UC-INS-21 check 13)`, async (t) => {
    const browser = await sharedChromium(t);
    const { dialog, hubHas, switches, errors } = await openForHub(browser, t, { workspaceOn, hubOff: workspaceOn, workspaceTook: true });
    const garden = hubHas(GARDEN.id);

    const banner = dialog.locator('[data-template-needs-modules]');
    await banner.waitFor({ timeout: 10_000 });
    assert.equal((await banner.textContent()).replace(/\s+/gu, ' ').trim(), `Needs Plans, which is switched off here. ${said}`, 'the banner does not say what the press does');
    await dialog.getByRole('button', { name: 'Switch on Plans and use this template' }).click();
    await dialog.getByText('Adds the workflow “scrum”').waitFor({ timeout: 10_000 });
    assert.deepEqual(switches(), [{ path: '/tenant/modules/PLANNING', body: { switch: 'ON', ...expected } }]);
    assert.equal(hubHas(HUB.id), true, 'the hub still lacks the product');
    assert.equal(hubHas(GARDEN.id), garden, 'a hub nobody chose changed');
    await dialog.getByRole('button', { name: 'Take template' }).click();
    await dialog.waitFor({ state: 'hidden', timeout: 10_000 });
    assert.deepEqual(errors(), []);
  });
}

// The banner in German, for both cases: the hub's own switch, and the workspace's naming the hub.
for (const [workspaceOn, said] of [
  [true, 'Braucht Pläne, das hier ausgeschaltet ist. Einschalten schaltet ihn in diesem Hub ein; kein anderer Hub ändert sich, und sonst nichts, bis du die Vorlage anwendest.'],
  [false, 'Braucht Pläne, das hier ausgeschaltet ist. Einschalten schaltet ihn auch für den ganzen Arbeitsbereich ein: Er zeigt allen im Arbeitsbereich seine Ansichten, und Hubs, die ab jetzt entstehen, haben ihn. Von den bestehenden Hubs bekommt ihn nur dieser; sonst ändert sich nichts, bis du die Vorlage anwendest.'],
]) {
  test(`chromium: in German, the hub's banner says what switching on does - ${workspaceOn ? 'in this hub only' : 'at the workspace as well'}`, async (t) => {
    const browser = await sharedChromium(t);
    const { dialog, errors } = await openForHub(browser, t, { workspaceOn, hubOff: workspaceOn, workspaceTook: true }, { locale: 'de' });
    const banner = dialog.locator('[data-template-needs-modules]');
    await banner.waitFor({ timeout: 10_000 });
    assert.equal((await banner.textContent()).replace(/\s+/gu, ' ').trim(), said);
    assert.deepEqual(errors(), []);
  });
}

// A hub's owner who may not shape the workspace may press the switch where only the hub's switch
// is needed: the server asks STRUCTURE on the hub's path for it (UC-INS-21 check 13).
test('chromium: a hub owner who may not shape the workspace switches the product on in the hub and takes the template', async (t) => {
  const browser = await sharedChromium(t);
  const { dialog, hubHas, switches, errors } = await openForHub(browser, t, { workspaceOn: true, hubOff: true, workspaceTook: true }, { isHubOnly: true });

  const banner = dialog.locator('[data-template-needs-modules]');
  await banner.waitFor({ timeout: 10_000 });
  assert.match(await banner.textContent(), /turns it on in this hub; no other hub changes/u);
  await dialog.getByRole('button', { name: 'Switch on Plans and use this template' }).click();
  await dialog.getByText('Adds the workflow “scrum”').waitFor({ timeout: 10_000 });
  assert.deepEqual(switches(), [{ path: '/tenant/modules/PLANNING', body: { switch: 'ON', hub_id: HUB.id } }]);
  assert.equal(hubHas(HUB.id), true);
  await dialog.getByRole('button', { name: 'Take template' }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 10_000 });
  assert.deepEqual(errors(), []);
});

// Where the workspace has the product off the press would switch the workspace: such a reader is
// sent to an administrator instead, and offered no press.
test('chromium: a hub owner who may not shape the workspace is not offered the workspace\'s switch', async (t) => {
  const browser = await sharedChromium(t);
  const { dialog, switches, errors } = await openForHub(browser, t, { workspaceOn: false, hubOff: false, workspaceTook: true }, { isHubOnly: true });

  const banner = dialog.locator('[data-template-needs-modules]');
  await banner.waitFor({ timeout: 10_000 });
  assert.match(await banner.textContent(), /Ask an administrator of this workspace to switch it on/u);
  assert.equal(await dialog.getByRole('button', { name: 'Switch on Plans and use this template' }).count(), 0, 'the reader is offered a switch the server refuses');
  assert.deepEqual(switches(), []);
  assert.deepEqual(errors(), []);
});

// A template taken for a collection: its products are needed in the collection's hub, so the
// workspace part's apply and the switch name that hub, as for a hub (UC-INS-21 check 13).
test('chromium: a template taken for a collection names the collection\'s hub in the workspace part\'s switch (UC-INS-21 check 13)', async (t) => {
  const browser = await sharedChromium(t);
  const { dialog, written, hubHas, switches, errors } = await openForHub(browser, t, { workspaceOn: false, hubOff: false, workspaceTook: false }, { isCollection: true });

  await dialog.locator('[data-template-workspace-first]').waitFor({ timeout: 10_000 });
  await dialog.getByRole('button', { name: 'Take template' }).click();
  await dialog.locator('[data-template-workspace-first]').waitFor({ state: 'detached', timeout: 10_000 });
  await dialog.getByText('Adds the workflow “scrum”').waitFor({ timeout: 10_000 });
  const applies = () => written.filter((each) => each.path === '/config:apply').map((each) => each.body);
  assert.deepEqual(applies()[0].module_hub_ids, [HUB.id], 'the workspace part left without the collection\'s hub');
  assert.equal(await dialog.locator('[data-template-needs-modules]').count(), 0, 'the collection\'s hub was left without the product');
  await dialog.getByRole('button', { name: 'Take template' }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 10_000 });
  assert.equal(applies()[1].collection_id, COLLECTION.id);
  assert.deepEqual(switches(), []);
  assert.equal(hubHas(GARDEN.id), false, 'a hub nobody chose was switched on');
  assert.deepEqual(errors(), []);
});

for (const [workspaceOn, expected, words, said] of [
  [true, { hub_id: HUB.id }, 'on at the workspace and off in its hub: the hub\'s own switch', 'Switching it on turns it on in this collection\'s hub; no other hub changes, and nothing else does until you apply the template.'],
  [false, { hub_ids: [HUB.id] }, 'off at the workspace: the workspace\'s switch naming its hub', 'Switching it on turns it on for the whole workspace as well: it shows its screens to everyone in the workspace, and hubs made from now on have it. Of the hubs that exist, only this collection\'s hub gets it; nothing else changes until you apply the template.'],
]) {
  test(`chromium: a collection's template that needs a product its hub lacks switches it on in that hub - ${words} (UC-INS-21 check 13)`, async (t) => {
    const browser = await sharedChromium(t);
    const { dialog, hubHas, switches, errors } = await openForHub(browser, t, { workspaceOn, hubOff: workspaceOn, workspaceTook: true }, { isCollection: true });
    const garden = hubHas(GARDEN.id);

    const banner = dialog.locator('[data-template-needs-modules]');
    await banner.waitFor({ timeout: 10_000 });
    assert.equal((await banner.textContent()).replace(/\s+/gu, ' ').trim(), `Needs Plans, which is switched off here. ${said}`);
    await dialog.getByRole('button', { name: 'Switch on Plans and use this template' }).click();
    await dialog.getByText('Adds the workflow “scrum”').waitFor({ timeout: 10_000 });
    assert.deepEqual(switches(), [{ path: '/tenant/modules/PLANNING', body: { switch: 'ON', ...expected } }]);
    assert.equal(hubHas(HUB.id), true, 'the collection\'s hub still lacks the product');
    assert.equal(hubHas(GARDEN.id), garden, 'a hub nobody chose changed');
    await dialog.getByRole('button', { name: 'Take template' }).click();
    await dialog.waitFor({ state: 'hidden', timeout: 10_000 });
    assert.deepEqual(errors(), []);
  });
}
