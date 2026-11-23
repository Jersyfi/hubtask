// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Private hubs in the web app (UC-ID-16): *Private* offered when a hub is made where the workspace
// has more than one person and not where it has one (check 8); a hub made private from its own
// menu, with a confirmation that names who loses it, and shared again; the lock on a private hub.
// The emergency access as a member meets it (check 11): told once at entry in a dialog, then the
// bar's mark and the hub's row, and no banner in any view.
// Chromium only, as the shell walk.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { serve } from './serve.mjs';
import { ACCOUNT, EMERGENCY_ACCESS, HUB, MANIFEST, OTHER, PRIVATE_HUBS, WORKSPACE, chooseNew, signedIn } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

/** Opens the page *Create hub* (ADR-0111) from the navigation. */
async function openCreate(page) {
  await page.goto(`${served.origin}/`);
  await chooseNew(page, 'Create hub');
  const form = page.locator('#main [data-create-hub]');
  await form.waitFor({ timeout: 10_000 });
  return form;
}

test('chromium: *Private* is offered where the workspace has more than one person, and sent', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  let created;
  await context.route('**/api/v1/containers', (route) => {
    if (route.request().method() !== 'POST') return route.fallback();
    created = route.request().postDataJSON();
    return route.fulfill({ status: 201, json: { ...HUB, id: '01a0e2e0-0000-7000-8000-0000000005c1', name: created.name, private: created.private === true } });
  });

  const dialog = await openCreate(page);
  const toggle = dialog.getByRole('switch', { name: 'Private' });
  await toggle.waitFor({ timeout: 10_000 });
  await dialog.getByRole('textbox', { name: 'Hub name' }).fill('Diary');
  await toggle.check();
  await page.locator('#main').getByRole('button', { name: 'Create hub', exact: true }).click();
  await dialog.waitFor({ state: 'detached', timeout: 10_000 });
  assert.equal(created?.private, true, `the body was ${JSON.stringify(created)}`);
  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed().filter((each) => !each.includes('/containers/01a0e2e0-0000-7000-8000-0000000005c1')), []);
});

test('chromium: a workspace of one is not offered *Private*', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  await context.route('**/api/v1/tenant', (route) => route.fulfill({ json: { ...WORKSPACE, private_hubs_offered: false } }));

  const dialog = await openCreate(page);
  await dialog.getByRole('textbox', { name: 'Hub name' }).waitFor({ timeout: 10_000 });
  // The read has answered when the page has: give it the moment the workspace takes to arrive.
  await page.waitForTimeout(500);
  assert.equal(await dialog.getByRole('switch', { name: 'Private' }).count(), 0);
  await page.goto(`${served.origin}/hubs/${HUB.id}`);
  await page.getByRole('heading', { name: HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
  await page.getByRole('button', { name: `Actions for ${HUB.name}` }).click();
  assert.equal(await page.getByRole('menuitem', { name: /Make private/ }).count(), 0, 'nothing to keep it from');
  assert.deepEqual(failures, []);
});

test('chromium: the owner of the hub makes it private, told whom it shuts out, and shares it again', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  let isPrivate = false;
  const sent = [];
  // The hub's level and the hub itself answer what was last written, as the server would.
  await context.route((url) => url.pathname.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB', (route) =>
    route.fulfill({ json: { data: [{ ...HUB, private: isPrivate, version: HUB.version + sent.length }], items: [], page: { next_cursor: null, has_more: false } } }));
  await context.route(`**/api/v1/containers/${HUB.id}`, (route) => route.fulfill({ json: { ...HUB, private: isPrivate, version: HUB.version + sent.length } }));
  await context.route(`**/api/v1/containers/${HUB.id}/privacy`, (route) => {
    const body = route.request().postDataJSON();
    sent.push(body.private);
    isPrivate = body.private;
    return route.fulfill({ json: { ...HUB, private: isPrivate, version: HUB.version + sent.length } });
  });

  await page.goto(`${served.origin}/hubs/${HUB.id}`);
  await page.getByRole('heading', { name: HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
  await page.getByRole('button', { name: `Actions for ${HUB.name}` }).click();
  await page.getByRole('menuitem', { name: 'Make private' }).click();
  const dialog = page.locator('dialog[open]');
  await dialog.waitFor({ timeout: 5_000 });
  assert.ok((await dialog.getByRole('heading').first().textContent()).includes(`Make ${HUB.name} private?`));
  // Mara holds a role on the workspace and nothing on the hub: she is the one who loses it.
  await dialog.getByText(`Who loses it: ${OTHER.display_name}.`).waitFor({ timeout: 5_000 });
  await dialog.getByRole('button', { name: 'Make private' }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
  await page.getByText('Private — only the people who hold a role in it can open it.').waitFor({ timeout: 10_000 });

  await page.getByRole('button', { name: `Actions for ${HUB.name}` }).click();
  await page.getByRole('menuitem', { name: 'Share with the workspace' }).click();
  await page.locator('dialog[open]').getByRole('button', { name: 'Share with the workspace' }).click();
  await page.locator('dialog[open]').waitFor({ state: 'hidden', timeout: 5_000 });
  assert.deepEqual(sent, [true, false]);
  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

test('chromium: an owner of the hub without the right to create hubs is offered sharing only (P-05)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  let isPrivate = false;
  // A member of the workspace, the hub's owner: MEMBER carries no STRUCTURE in the fixture's matrix.
  await context.route((url) => url.pathname.endsWith('/api/v1/memberships') && url.searchParams.get('scope_type') === 'TENANT', (route) =>
    route.fulfill({ json: { data: [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'MEMBER' }], items: [], page: { next_cursor: null, has_more: false } } }));
  await context.route((url) => url.pathname.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB', (route) =>
    route.fulfill({ json: { data: [{ ...HUB, private: isPrivate }], items: [], page: { next_cursor: null, has_more: false } } }));
  await context.route(`**/api/v1/containers/${HUB.id}`, (route) => route.fulfill({ json: { ...HUB, private: isPrivate } }));

  const menuOf = async () => {
    await page.goto(`${served.origin}/hubs/${HUB.id}`);
    await page.getByRole('heading', { name: HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
    await page.waitForTimeout(500);
    await page.getByRole('button', { name: `Actions for ${HUB.name}` }).click();
    await page.getByRole('menu').waitFor({ timeout: 5_000 });
    return page.getByRole('menuitem').allTextContents();
  };
  const shared = await menuOf();
  assert.ok(!shared.some((each) => each.includes('Make private')), `offered ${shared}`);
  isPrivate = true;
  const owned = await menuOf();
  assert.ok(owned.some((each) => each.includes('Share with the workspace')), `offered ${owned}`);
  assert.deepEqual(failures, []);
});

test('chromium: the administration lists the private hubs - whose and how big, never a name', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());

  await page.goto(`${served.origin}/administration/workspace`);
  const nav = page.getByRole('navigation', { name: 'Administration', exact: true });
  await nav.getByRole('link', { name: 'Private hubs', exact: true }).click();
  const table = page.getByRole('table', { name: 'Private hubs' });
  await table.waitFor({ timeout: 10_000 });
  const rows = table.getByRole('row');
  // The head and the two hubs of the fixture: one of Mara's, one nobody is left in.
  assert.equal(await rows.count(), 3);
  // A row's title is its first cell (page-system §7.1: a DataTable), the owners.
  await table.getByRole('cell', { name: OTHER.display_name, exact: true }).waitFor({ timeout: 5_000 });
  await table.getByRole('cell', { name: 'Nobody — its last member left', exact: true }).waitFor();
  assert.ok((await table.textContent()).includes('In the trash, removed for good on'));
  assert.equal(await page.getByRole('link', { name: /House|Diary/ }).count(), 0, 'no hub is named or linked');
  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

test('chromium: no private hub in a workspace of one, or no right to the list, and the row is absent', async (t) => {
  const browser = await sharedChromium(t);
  for (const answer of [{ json: [] }, { status: 403, json: { code: 'access.not_permitted' } }]) {
    const { page, failures, context } = await signedIn(browser, 1280, 900);
    await context.route('**/api/v1/private-hubs', (route) => route.fulfill(answer));
    await context.route('**/api/v1/tenant', (route) => route.fulfill({ json: { ...WORKSPACE, several_people: false, private_hubs_offered: false } }));
    await page.goto(`${served.origin}/administration/workspace`);
    const nav = page.getByRole('navigation', { name: 'Administration', exact: true });
    await nav.getByRole('link', { name: 'Limits', exact: true }).waitFor({ timeout: 10_000 });
    await page.waitForTimeout(300);
    assert.equal(await nav.getByRole('link', { name: 'Private hubs', exact: true }).count(), 0, `answered ${answer.status ?? 200}`);
    assert.deepEqual(failures, []);
    await context.close();
  }
});

/** The rule as `/tenant` answers it, with *Private hubs allowed* as a level above left it. */
function withSwitch(setting) {
  return { ...WORKSPACE, sign_in_policy: { private_hubs: { value: true, installation: true, lock: null, source: 'DEFAULT', installation_source: 'DEFAULT', ...setting } } };
}

test('chromium: with several people and no private hub the page stays, with *Private hubs allowed* on it', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  await context.route('**/api/v1/private-hubs', (route) => route.fulfill({ json: [] }));
  let patched;
  await context.route('**/api/v1/tenant', (route) => {
    // The server answers what it stored, on the write and on every read after it.
    if (route.request().method() === 'PATCH') patched = route.request().postDataJSON();
    return route.fulfill({ json: withSwitch(patched ? { value: false, source: 'WORKSPACE' } : {}) });
  });

  await page.goto(`${served.origin}/administration/workspace`);
  const nav = page.getByRole('navigation', { name: 'Administration', exact: true });
  await nav.getByRole('link', { name: 'Private hubs', exact: true }).click();
  const toggle = page.getByRole('switch', { name: 'People here may make private hubs' });
  await toggle.waitFor({ timeout: 10_000 });
  assert.equal(await toggle.isChecked(), true);
  // The chip beside the switch says whose value it is (UC-WRK-32 check 5), and that it is this
  // workspace's own once it was switched here; the save confirms as a passing message. The chip's
  // words are the quiet *Default* (lib/data/source.ts).
  await page.locator('[data-private-hubs-allowed]').getByRole('button', { name: 'Default', exact: true }).waitFor({ timeout: 10_000 });
  await toggle.uncheck();
  await page.getByRole('button', { name: 'Set here' }).waitFor({ timeout: 10_000 });
  await page.getByText('Saved: who may make private hubs.').first().waitFor();
  assert.deepEqual(patched, { sign_in_policy: { private_hubs: false } });
  assert.deepEqual(failures, []);
});

test('chromium: an installation lock shows the switch with who set it, and offers nothing', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  await context.route('**/api/v1/tenant', (route) => route.fulfill({ json: withSwitch({ value: false, installation: false, lock: 'INSTANCE', source: 'INSTANCE', installation_source: 'INSTANCE' }) }));

  await page.goto(`${served.origin}/administration/private-hubs`);
  const toggle = page.getByRole('switch', { name: 'People here may make private hubs' });
  await toggle.waitFor({ timeout: 10_000 });
  assert.equal(await toggle.isChecked(), false);
  assert.equal(await toggle.isDisabled(), true);
  assert.ok((await page.textContent('[data-private-hubs-allowed]')).includes('Set by the installation for every workspace.'));
  assert.equal(await page.locator('[data-private-hubs-allowed] [data-source]').getAttribute('data-source'), 'INSTANCE-locked');
  assert.deepEqual(failures, []);
});

// UC-ID-16 check 7: a plan that switches private hubs off says it is the plan, not the installation.
test('chromium: a plan lock shows the switch with your plan as who set it', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  await context.route('**/api/v1/tenant', (route) => route.fulfill({ json: withSwitch({ value: false, installation: false, lock: 'PLAN', source: 'PLAN', installation_source: 'PLAN' }) }));

  await page.goto(`${served.origin}/administration/private-hubs`);
  const toggle = page.getByRole('switch', { name: 'People here may make private hubs' });
  await toggle.waitFor({ timeout: 10_000 });
  assert.equal(await toggle.isChecked(), false);
  assert.equal(await toggle.isDisabled(), true);
  const said = await page.textContent('[data-private-hubs-allowed]');
  assert.ok(said.includes('Set by your plan.'), said);
  assert.ok(!said.includes('installation'), said);
  assert.equal(await page.locator('[data-private-hubs-allowed] [data-source]').getAttribute('data-source'), 'PLAN-locked');
  assert.deepEqual(failures, []);
});

test('chromium: the owner opens a private hub in an emergency - the reason, then the hub', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  let sent;
  await context.route(`**/api/v1/private-hubs/${PRIVATE_HUBS[0].id}/accesses`, (route) => {
    if (route.request().method() === 'POST') {
      sent = route.request().postDataJSON();
      return route.fulfill({ status: 201, json: { ...EMERGENCY_ACCESS, reason: sent.reason } });
    }
    return route.fulfill({ json: [EMERGENCY_ACCESS] });
  });

  await page.goto(`${served.origin}/administration/private-hubs`);
  const table = page.getByRole('table', { name: 'Private hubs' });
  await table.waitFor({ timeout: 10_000 });
  await table.getByRole('button', { name: 'Open in an emergency' }).first().click();
  const dialog = page.locator('dialog[open]');
  await dialog.waitFor({ timeout: 10_000 });
  assert.ok((await dialog.textContent()).includes('Every member of the hub is told at once'));
  await dialog.getByRole('textbox', { name: 'Reason' }).fill(EMERGENCY_ACCESS.reason);
  await dialog.getByRole('button', { name: 'Open for one hour' }).click();
  await page.waitForURL(`**/hubs/${PRIVATE_HUBS[0].id}`, { timeout: 10_000 });
  assert.deepEqual(sent, { reason: EMERGENCY_ACCESS.reason });
  assert.deepEqual(failures, []);
});

/*
 * UC-ID-16 check 11: a member is told once when the app opens, then the bar's mark and the hub's row
 * carry it for thirty days, and no view draws a banner for it. The server half - the confirmation
 * stored, twice is once, not found for anybody not told - is WP-17's integration and contract tests.
 */

const MINUTE = 60_000;
/** Two more private hubs of the reader's: one without a chosen symbol, one with. */
const FINANCES = { ...HUB, id: '01a0e2e0-0000-7000-8000-0000000005e1', name: 'Finances', private: true, color_token: 'violet', order_key: 'a1' };
const HEALTH = { ...HUB, id: '01a0e2e0-0000-7000-8000-0000000005e2', name: 'Health', private: true, color_token: 'teal', icon: 'heart', order_key: 'a2' };
const HUBS = [{ ...HUB, private: true }, FINANCES, HEALTH];
const LONG_REASON = 'The bank called this morning: for the follow-up financing they need the contract number and the last annual statement, and the deadline ends at noon today. '.repeat(3).trim();

/** A notice of an access to `hub`, opened `ago` before now, as the reader's own list answers it. */
const told = (hub, ago, { id = hub.id.replace(/^01a0e2e0/, '01a0e2e1'), reason = EMERGENCY_ACCESS.reason, acknowledged = null } = {}) => ({
  ...EMERGENCY_ACCESS,
  id,
  hub_id: hub.id,
  opened_by: OTHER.id,
  reason,
  opened_at: new Date(Date.now() - ago).toISOString(),
  ends_at: new Date(Date.now() - ago + 60 * MINUTE).toISOString(),
  acknowledged_at: acknowledged,
});

/**
 * The reader's notices and their hubs. Each `:acknowledge` is recorded and, unless refused, marks the
 * notice confirmed, so the list read again answers it. `held` keeps the first read of the list
 * waiting until the walk lets it go.
 */
async function notices(context, list, { refuse = false, held } = {}) {
  const sent = [];
  let state = list.map((each) => ({ ...each }));
  await context.route('**/api/v1/containers?*', (route) => {
    const url = new URL(route.request().url());
    if (url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { data: HUBS, page: { next_cursor: null, has_more: false } } });
    // The two hubs of this suite hold nothing; the fixture's collection is the first hub's.
    if ([FINANCES.id, HEALTH.id].includes(url.searchParams.get('parent_id'))) return route.fulfill({ json: { data: [], page: { next_cursor: null, has_more: false } } });
    return route.fallback();
  });
  for (const hub of HUBS) await context.route(`**/api/v1/containers/${hub.id}`, (route) => route.fulfill({ json: hub }));
  await context.route('**/api/v1/private-hubs/*/accesses', (route) => route.fulfill({ json: state.map(({ acknowledged_at: _, ...access }) => access) }));
  await context.route('**/api/v1/accounts/me/emergency-notices', async (route) => {
    if (held) {
      await held;
      held = undefined;
    }
    return route.fulfill({ json: state });
  });
  await context.route('**/api/v1/accounts/me/emergency-notices/*', (route) => {
    const id = new URL(route.request().url()).pathname.split('/').pop().replace(/:acknowledge$/, '');
    sent.push({ method: route.request().method(), id });
    if (refuse) {
      return route.fulfill({ status: 404, contentType: 'application/problem+json', json: { type: 'about:blank', title: 'not found', status: 404, code: 'not_found', detail_code: 'containers.emergency_access_not_found' } });
    }
    state = state.map((each) => (each.id === id ? { ...each, acknowledged_at: new Date().toISOString() } : each));
    return route.fulfill({ status: 204 });
  });
  return { sent };
}

const noticeDialog = (page) => page.getByRole('alertdialog');
const focused = (page) => page.evaluate(() => {
  const at = document.activeElement;
  return { id: at?.id ?? '', text: (at?.textContent ?? '').trim(), inDialog: !!at?.closest('dialog[open]') };
});
const said = (page) => page.locator('.announcement[role="status"]').textContent();
/** Two frames on: the notice is put up a task after the moment it may show, so a check that it did not waits this long. */
const settled = (page) => page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));

test('chromium: 1280 px - a member is told once at entry who opened which hub, when, until when and why; Acknowledged by the keyboard confirms it, and a reload asks no more (UC-ID-16 check 11)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 1280, 860);
  t.after(() => context.close());
  const notice = told(HUB, 2 * 60 * MINUTE);
  const { sent } = await notices(context, [notice]);

  await page.goto(`${served.origin}/`);
  const dialog = noticeDialog(page);
  await dialog.waitFor({ timeout: 10_000 });
  assert.equal((await dialog.locator('h2').textContent()).trim(), 'Emergency access to your private hub');
  const text = await dialog.textContent();
  for (const fact of [OTHER.display_name, 'House', notice.reason, 'ended at']) assert.ok(text.includes(fact), `the dialog names ${fact}`);
  assert.deepEqual(await focused(page), { id: '', text: 'Open the hub', inDialog: true }, 'the focus starts on the first answer, never on the confirmation');
  // No banner for it, in the zone or anywhere.
  assert.equal(await page.locator('[data-banners] [data-emergency-notice], [data-emergency-notice="frame"]').count(), 0);

  await page.keyboard.press('Tab');
  assert.equal((await focused(page)).text, 'Acknowledged');
  await page.keyboard.press('Enter');
  await dialog.waitFor({ state: 'hidden', timeout: 10_000 });
  assert.deepEqual(sent, [{ method: 'POST', id: notice.id }]);
  await page.waitForFunction(() => document.activeElement?.id === 'main', null, { timeout: 5_000 });
  assert.ok((await said(page)).startsWith('Acknowledged.'), 'the confirmation is said once, without the reason');
  assert.ok(!(await said(page)).includes(notice.reason));

  // Quiet afterwards: the shield, no dot, the name says what it holds.
  const mark = page.getByRole('button', { name: 'Emergency access to your private hubs: notices of the last 30 days' });
  await mark.waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('.notice-mark .dot').count(), 0);

  await page.reload();
  await mark.waitFor({ timeout: 10_000 });
  await settled(page);
  assert.equal(await noticeDialog(page).count(), 0, 'confirmed, it never shows again');
  assert.deepEqual(sent.length, 1);
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px - closed without confirming, nothing is sent, the mark keeps its dot, and the notice asks again at the next opening (UC-ID-16 check 11)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 1280, 860);
  t.after(() => context.close());
  const { sent } = await notices(context, [told(HUB, 10 * MINUTE)]);

  await page.goto(`${served.origin}/`);
  const dialog = noticeDialog(page);
  await dialog.waitFor({ timeout: 10_000 });
  assert.ok((await dialog.textContent()).includes('open until'), 'the access is still running');
  // A press beside it does not close it: an alert dialog is answered, or closed by ✕ or Escape.
  await page.mouse.click(8, 430);
  assert.equal(await dialog.isVisible(), true, 'a click on the backdrop keeps it open');
  assert.equal((await focused(page)).inDialog, true);
  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'hidden' });
  assert.deepEqual(sent, []);
  await page.waitForFunction(() => document.activeElement?.id === 'main', null, { timeout: 5_000 });
  const mark = page.getByRole('button', { name: 'Emergency access to your private hubs: one notice not yet acknowledged' });
  await mark.waitFor();
  assert.equal(await page.locator('.notice-mark .dot').count(), 1);
  // The hub's row: the shield in the lock's place, named with both facts, in the warning accent
  // while the hour runs.
  const row = page.locator('aside.sidenav').getByRole('treeitem', { name: /^House Private · opened through the emergency access, until \d/ });
  await row.waitFor();
  assert.equal(await row.locator('.badge[data-tone="warning"]').count(), 1);

  await page.reload();
  await noticeDialog(page).waitFor({ timeout: 10_000 });
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px - a refused confirmation keeps the dialog open and says so, the focus stays in it, and the mark keeps its dot (UC-ID-16 check 11)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 1280, 860);
  t.after(() => context.close());
  const { sent } = await notices(context, [told(HUB, 2 * 60 * MINUTE)], { refuse: true });

  await page.goto(`${served.origin}/`);
  const dialog = noticeDialog(page);
  await dialog.waitFor({ timeout: 10_000 });
  await dialog.getByRole('button', { name: 'Acknowledged' }).click();
  const refusal = dialog.getByRole('alert');
  await refusal.waitFor({ timeout: 10_000 });
  assert.ok((await refusal.textContent()).startsWith('The acknowledgement was not saved.'));
  assert.equal(sent.length, 1);
  assert.equal((await focused(page)).inDialog, true, 'the focus does not fall out of the dialog');
  await dialog.getByRole('button', { name: 'Close' }).click();
  await dialog.waitFor({ state: 'hidden' });
  await page.getByRole('button', { name: 'Emergency access to your private hubs: one notice not yet acknowledged' }).waitFor();
  assert.equal(await page.locator('.notice-mark .dot').count(), 1);
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px - three notices are one dialog; All acknowledged confirms each; the mark lists them with their hubs, and each hub row carries the shield (UC-ID-16 check 11)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 1280, 860);
  t.after(() => context.close());
  const three = [told(HUB, 10 * MINUTE), told(FINANCES, 2 * 24 * 60 * MINUTE, { reason: LONG_REASON }), told(HEALTH, 6 * 24 * 60 * MINUTE)];
  const { sent } = await notices(context, three);

  await page.goto(`${served.origin}/`);
  const dialog = noticeDialog(page);
  await dialog.waitFor({ timeout: 10_000 });
  assert.equal((await dialog.locator('h2').textContent()).trim(), 'Emergency access to 3 of your private hubs');
  assert.equal(await dialog.getByRole('listitem').count(), 3);
  assert.equal(await dialog.getByRole('link', { name: 'Open the hub' }).count(), 3, 'each row opens its hub');
  assert.equal((await focused(page)).text, 'All acknowledged', 'the one answer of the foot');
  // A long reason is folded and read whole on a press; the focus stays on the press.
  const more = dialog.getByRole('button', { name: 'Read the whole reason' });
  await more.click();
  assert.equal(await dialog.getByRole('button', { name: 'Less' }).getAttribute('aria-expanded'), 'true');
  assert.equal((await focused(page)).text, 'Less');

  await dialog.getByRole('button', { name: 'All acknowledged' }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 10_000 });
  assert.deepEqual(sent.map((each) => each.id).sort(), three.map((each) => each.id).sort());

  const mark = page.getByRole('button', { name: 'Emergency access to your private hubs: notices of the last 30 days' });
  await mark.waitFor({ timeout: 10_000 });
  assert.equal(await mark.getAttribute('aria-expanded'), 'false');
  await mark.click();
  assert.equal(await mark.getAttribute('aria-expanded'), 'true');
  const section = page.getByRole('region', { name: 'Emergency access to your private hubs' });
  await section.waitFor();
  for (const hub of HUBS) await section.getByRole('link', { name: hub.name }).waitFor();
  for (const hub of HUBS) {
    await page.locator('aside.sidenav').getByRole('treeitem', { name: new RegExp(`^${hub.name} Private · opened through the emergency access`) }).waitFor();
  }
  await section.getByRole('link', { name: 'Finances' }).click();
  await page.waitForURL(`**/hubs/${FINANCES.id}`, { timeout: 10_000 });
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px - Open the hub confirms and goes there, and the hub says it itself (UC-ID-16 checks 4 and 11)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 1280, 860);
  t.after(() => context.close());
  const notice = told(HUB, 2 * 60 * MINUTE);
  const { sent } = await notices(context, [notice]);

  await page.goto(`${served.origin}/`);
  await noticeDialog(page).waitFor({ timeout: 10_000 });
  await page.keyboard.press('Enter');
  await page.waitForURL(`**/hubs/${HUB.id}`, { timeout: 10_000 });
  assert.deepEqual(sent, [{ method: 'POST', id: notice.id }]);
  const onHub = page.locator('[data-emergency-notice="member"]');
  await onHub.waitFor({ timeout: 10_000 });
  assert.ok((await onHub.textContent()).includes(OTHER.display_name));
  assert.equal(await noticeDialog(page).count(), 0);
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px - the notice waits while the person has a dialog open, and comes once it is closed (UC-ID-16 check 11)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 1280, 860);
  t.after(() => context.close());
  let release;
  const held = new Promise((resolve) => (release = resolve));
  await notices(context, [told(HUB, 2 * 60 * MINUTE)], { held });

  await page.goto(`${served.origin}/`);
  await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 10_000 });
  // A dialog the person opened: the notice does not cover it.
  await page.keyboard.press('?');
  const help = page.getByRole('dialog').first();
  await help.waitFor();
  release();
  await page.getByRole('button', { name: 'Emergency access to your private hubs: one notice not yet acknowledged' }).waitFor({ timeout: 10_000 });
  await settled(page);
  assert.equal(await noticeDialog(page).count(), 0, 'not over the open dialog');
  await page.keyboard.press('Escape');
  await help.waitFor({ state: 'hidden' });
  await noticeDialog(page).waitFor({ timeout: 10_000 });
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px - the notice waits while the person types, and comes once the focus leaves the field (UC-ID-16 check 11)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 1280, 860);
  t.after(() => context.close());
  let release;
  const held = new Promise((resolve) => (release = resolve));
  await notices(context, [told(HUB, 2 * 60 * MINUTE)], { held });

  await page.goto(`${served.origin}/`);
  const heading = page.getByRole('heading', { level: 1 }).first();
  await heading.waitFor({ timeout: 10_000 });
  await page.getByRole('combobox', { name: 'Search everything' }).click();
  await page.keyboard.type('insur');
  release();
  // The notice has arrived once the hub's row carries the shield; the bar's marks give way to the
  // field while it is worked in.
  await page.locator('aside.sidenav').getByRole('treeitem', { name: /^House Private · opened through the emergency access/ }).waitFor({ timeout: 10_000 });
  await settled(page);
  assert.equal(await noticeDialog(page).count(), 0, 'not in the middle of typing');
  await page.keyboard.press('Escape');
  await heading.click();
  await noticeDialog(page).waitFor({ timeout: 10_000 });
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px - a newcomer whose tour would start is told first, and the tour starts once the notice is closed (UC-ID-16 check 11)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 1280, 860);
  t.after(() => context.close());
  await notices(context, [told(HUB, 2 * 60 * MINUTE)]);
  await context.route('**/api/v1/accounts/me', (route) => (route.request().method() === 'GET' ? route.fulfill({ json: { ...ACCOUNT, onboarding_completed_at: null } }) : route.fallback()));

  await page.goto(`${served.origin}/`);
  const dialog = noticeDialog(page);
  await dialog.waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('.spotlight').count(), 0, 'no tour under the notice');
  // One Escape closes the notice: no other layer stands above it.
  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'hidden' });
  await page.locator('.spotlight').waitFor({ state: 'attached', timeout: 15_000 });
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px - a member who has left the hub still reads the notice, as a private hub without a way there (UC-ID-16 check 11)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 1280, 860);
  t.after(() => context.close());
  const gone = { ...HUB, id: '01a0e2e0-0000-7000-8000-0000000005e3' };
  await notices(context, [told(gone, 2 * 60 * MINUTE)]);

  await page.goto(`${served.origin}/`);
  const dialog = noticeDialog(page);
  await dialog.waitFor({ timeout: 10_000 });
  assert.ok((await dialog.textContent()).includes('A private hub'));
  assert.equal(await dialog.getByRole('button', { name: 'Open the hub' }).count(), 0);
  assert.equal((await focused(page)).text, 'Acknowledged', 'the first answer drawn');
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px - no view moves for a notice: the start view and a whiteboard start where they start without one (UC-ID-16 check 11)', async (t) => {
  const browser = await sharedChromium(t);
  const BOARD = '01a0e2e0-0000-7000-8000-0000000005f1';
  const board = { id: BOARD, hub_id: HUB.id, title: 'Retro', element_count: 0, elements: [], writable: true, trashed_at: null, created_by: ACCOUNT.id, created_at: '2026-10-30T09:00:00Z', updated_by: ACCOUNT.id, updated_at: '2026-10-30T09:00:00Z', version: 1 };
  const modulesOn = [...(MANIFEST.modules_on ?? []), 'WHITEBOARD'];
  const tops = {};
  for (const withNotice of [false, true]) {
    const { page, failures, context } = await signedIn(browser, 1280, 860);
    await notices(context, withNotice ? [told(HUB, 2 * 60 * MINUTE, { acknowledged: new Date().toISOString() })] : []);
    await context.route('**/api/v1/meta/capabilities', (route) => route.fulfill({ json: { ...MANIFEST, modules_on: modulesOn } }));
    await context.route(/\/api\/v1\/containers\/[^/]+\/modules$/, (route) => route.fulfill({ json: { modules_on: modulesOn } }));
    await context.route(`**/api/v1/whiteboards/${BOARD}`, (route) => route.fulfill({ json: board }));
    await context.route(`**/api/v1/whiteboards/${BOARD}/comments`, (route) => route.fulfill({ json: { data: [] } }));
    await context.routeWebSocket(/\/realtime\/documents\/whiteboard\//, (ws) => {
      ws.onMessage(() => {});
      ws.send(Buffer.from([0, 1, 2, 0, 0]));
    });
    await page.goto(`${served.origin}/`);
    const heading = page.getByRole('heading', { level: 1 }).first();
    await heading.waitFor({ timeout: 10_000 });
    if (withNotice) await page.getByRole('button', { name: /^Emergency access to your private hubs/ }).waitFor({ timeout: 10_000 });
    const start = { heading: (await heading.boundingBox()).y, banners: (await page.locator('[data-banners]').boundingBox()).height };
    await page.goto(`${served.origin}/whiteboards/${BOARD}`);
    const surface = page.getByRole('application');
    await surface.waitFor({ timeout: 10_000 });
    if (withNotice) await page.getByRole('button', { name: /^Emergency access to your private hubs/ }).waitFor({ timeout: 10_000 });
    tops[withNotice] = { ...start, whiteboard: (await surface.boundingBox()).y };
    assert.equal(await noticeDialog(page).count(), 0);
    assert.deepEqual(failures, []);
    await context.close();
  }
  assert.deepEqual(tops[true], tops[false], 'the start view and the whiteboard keep their place');
  assert.equal(tops[true].banners, 0, 'the banner zone stays empty');
});

test('chromium: 390 px - on a phone the notice is the centred card; the mark opens a drawer that confirms it; the phone\'s hub list carries the shield (UC-ID-16 check 11)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 390, 844, { hasTouch: true });
  t.after(() => context.close());
  const notice = told(HUB, 2 * 60 * MINUTE);
  const { sent } = await notices(context, [notice]);

  await page.goto(`${served.origin}/`);
  const dialog = noticeDialog(page);
  await dialog.waitFor({ timeout: 10_000 });
  const card = await dialog.boundingBox();
  assert.ok(card.x > 0 && card.width < 390, `a card in the middle, not the whole screen: ${JSON.stringify(card)}`);
  assert.equal((await focused(page)).text, 'Open the hub');
  assert.ok((await dialog.textContent()).includes(notice.reason));
  await dialog.getByRole('button', { name: 'Close' }).click();
  await dialog.waitFor({ state: 'hidden' });
  assert.deepEqual(sent, []);

  const mark = page.getByRole('button', { name: 'Emergency access to your private hubs: one notice not yet acknowledged' });
  await mark.click();
  const drawer = page.getByRole('dialog', { name: 'Notices for you' });
  await drawer.waitFor();
  const section = drawer.getByRole('region', { name: 'Emergency access to your private hubs' });
  await section.getByRole('link', { name: 'House' }).waitFor();
  await section.getByRole('button', { name: 'Acknowledged' }).click();
  await section.getByRole('button', { name: 'Acknowledged' }).waitFor({ state: 'detached', timeout: 10_000 });
  assert.deepEqual(sent, [{ method: 'POST', id: notice.id }]);
  await page.keyboard.press('Escape');
  await drawer.waitFor({ state: 'hidden' });
  await page.getByRole('button', { name: 'Emergency access to your private hubs: notices of the last 30 days' }).waitFor();

  await page.getByRole('navigation', { name: 'Sections' }).getByRole('link', { name: 'Browse' }).click();
  await page.getByRole('link', { name: /^House Private · opened through the emergency access on / }).waitFor({ timeout: 10_000 });
  assert.deepEqual(failures, []);
});

// design-system.md §10 (2.5.8): each card's two presses are a target of their own - `layout.target`
// on a phone with touch, and never under 24 px.
for (const [width, height, hasTouch, least] of [[390, 844, true, 44], [1280, 860, false, 24]]) {
  for (const colorScheme of ['light', 'dark']) {
    test(`chromium: ${width} px, ${colorScheme}${hasTouch ? ', touch' : ''} - several notices: each card's Open the hub and Read the whole reason are at least ${least} px high (UC-ID-16 check 11)`, async (t) => {
      const browser = await sharedChromium(t);
      const { page, failures, context } = await signedIn(browser, width, height, { hasTouch });
      t.after(() => context.close());
      await page.emulateMedia({ colorScheme });
      await notices(context, [told(HUB, 10 * MINUTE), told(FINANCES, 2 * 24 * 60 * MINUTE, { reason: LONG_REASON }), told(HEALTH, 6 * 24 * 60 * MINUTE)]);

      await page.goto(`${served.origin}/`);
      const dialog = noticeDialog(page);
      await dialog.waitFor({ timeout: 10_000 });
      // Measured once the dialog has finished opening: its entrance scales it.
      await dialog.evaluate((element) => Promise.all(element.getAnimations({ subtree: true }).map((each) => each.finished)));
      const presses = [...await dialog.getByRole('link', { name: 'Open the hub' }).all(), dialog.getByRole('button', { name: 'Read the whole reason' })];
      assert.equal(presses.length, 4);
      for (const press of presses) {
        const box = await press.boundingBox();
        assert.ok(box && box.height >= least, `${await press.textContent()}: ${JSON.stringify(box)}`);
      }
      assert.deepEqual(failures, []);
    });
  }
}

test('chromium: 1280 px - in German the notice, its answers, the mark and the hub row speak German (UC-ID-16 check 11)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context } = await signedIn(browser, 1280, 860);
  t.after(() => context.close());
  await notices(context, [told(HUB, 10 * MINUTE)]);
  await context.route('**/api/v1/accounts/me', (route) => (route.request().method() === 'GET' ? route.fulfill({ json: { ...ACCOUNT, locale: 'de' } }) : route.fallback()));

  await page.goto(`${served.origin}/`);
  await page.waitForFunction(() => document.documentElement.lang === 'de', null, { timeout: 15_000 });
  const dialog = noticeDialog(page);
  await dialog.waitFor({ timeout: 10_000 });
  assert.equal((await dialog.locator('h2').textContent()).trim(), 'Notfallzugang auf deinen privaten Hub');
  const text = await dialog.textContent();
  for (const words of ['hat einen privaten Hub geöffnet, in dem du Mitglied bist', 'Geöffnet', 'Zugriff', 'Grund', 'läuft bis', 'Uhr']) assert.ok(text.includes(words), words);
  await dialog.getByRole('button', { name: 'Hub öffnen' }).waitFor();
  await dialog.getByRole('button', { name: 'Zur Kenntnis genommen' }).waitFor();
  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'hidden' });
  await page.getByRole('button', { name: 'Notfallzugang auf deine privaten Hubs: eine Meldung noch nicht bestätigt' }).waitFor();
  await page.locator('aside.sidenav').getByRole('treeitem', { name: /^House Privat · über den Notfallzugang geöffnet, bis .* Uhr/ }).waitFor();
  assert.deepEqual(failures, []);
});
