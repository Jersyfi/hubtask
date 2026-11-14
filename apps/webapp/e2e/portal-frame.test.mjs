// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The portal family of the work surface (scenes `portal`, `portaldir`, `portalext`): an outsider -
// REQUEST somewhere, READ nowhere - gets the portal's frame alone with its four tabs, approvals on
// top and counted, shared spaces in *knowledge* and the portals chosen at the top (UC-SVC-05 check
// 11, UC-SVC-09 check 5, UC-DOC-13 check 5); requests read by their status names (check 12); a
// member finds every department in *Anfragen & Hilfe* through *Erstellen*, keeps the sidebar and
// sees their requests everywhere (UC-SVC-14); one link opens the right view (check 13). Walked
// signed in as a requester-only account and as a member. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, MANIFEST, PAGE, forgetUnstubbed, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const IT = '01a0e2e0-0000-7000-8000-0000000051d1';
const HR = '01a0e2e0-0000-7000-8000-0000000051d2';
const MINE = '01a0e2e0-0000-7000-8000-0000000051f1';
const THEIRS = '01a0e2e0-0000-7000-8000-0000000051f2';
const SPACE = '01a0e2e0-0000-7000-8000-0000000051a1';
const UNDER = '01a0e2e0-0000-7000-8000-0000000051a2';
const kind = (key, name, description) => ({ key, name, description, icon: 'inbox', form: { fields: [] }, questions: [] });
const KINDS = { [IT]: [kind('laptop', 'Order a laptop', 'Laptop and accessories.'), kind('incident', 'Something is broken', 'A device does not work.')], [HR]: [kind('certificate', 'A certificate', 'For parental leave.')] };
const NAMES = { [IT]: 'IT help', [HR]: 'People' };
const request = (id, collection, title, extra) => ({
  id, number: 8, collection_id: collection, entry_type: 'incident', title, notes: null, status: 'in_progress', status_name: 'Being worked on',
  status_category: 'IN_PROGRESS', is_completed: false, completed_at: null, created_at: '2026-11-10T09:00:00Z', updated_at: '2026-11-12T09:40:00Z', is_own: true, ...extra,
});
// What the team wrote is Markdown; a requester reads it formatted, never its syntax, and never
// markup a text could carry in.
const NOTES = '**Since Monday** the VPN drops every hour.';
const REPLY = { id: '01a0e2e0-0000-7000-8000-0000000051c1', body: 'Please _restart_ the client:\n\n- quit it\n- open it again', created_at: '2026-11-12T09:40:00Z', mine: false, by_requester: false };
const ARTICLE = '## If it is lost\n\nRevoke it under **Account**.\n\n<img src="x" onerror="window.injected=1">';
const APPROVAL = {
  id: '01a0e2e0-0000-7000-8000-0000000051b1', item_id: THEIRS, item_title: 'Access for Max to the supplier portal', item_number: 3, on_behalf_of: null,
  workflow: 'requests', step: 'approve', item_version: 1, state: 'PENDING', current_level: 1,
  levels: [{ rule: 'ANY', count: 1, approvers: [ACCOUNT.id], roles: [], needed: 1 }],
  decisions: [], requested_by: '01a0e2e0-0000-7000-8000-000000000002', requested_at: '2026-11-13T09:00:00Z', decided_at: null, due_at: null, may_decide: true, version: 1,
};

async function open(browser, { width, outsider, path, approvals = [APPROVAL] }) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  forgetUnstubbed();
  const asked = [];
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    const p = url.pathname.replace(/^.*\/api\/v1/, '');
    const collection = url.searchParams.get('collection_id');
    asked.push(`${route.request().method()} ${p}`);
    if (p === '/accounts/me') return route.fulfill({ json: { ...ACCOUNT, display_name: 'Lena Berg', has_access: !outsider, portals: [{ collection_id: IT, name: 'IT help' }, { collection_id: HR, name: 'People' }] } });
    if (p === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: outsider ? [] : ['READ', 'WRITE_ITEMS'] } });
    if (p === '/request-catalogue') return route.fulfill({ json: { desk: { name: NAMES[collection], description: '', icon: null, color_token: null, logo_media_id: null }, data: KINDS[collection] ?? [], may_raise: true } });
    if (p === '/requests') return route.fulfill({ json: { data: collection === IT ? [request(MINE, IT, 'The VPN drops', { last_reply_at: '2026-11-12T09:40:00Z', next_due_at: '2026-11-12T14:00:00Z' })] : [request('01a0e2e0-0000-7000-8000-0000000051f3', HR, 'Certificate for leave', { status_name: null, status_category: 'TODO', created_at: '2026-11-11T09:00:00Z' })], page_info: { next_cursor: null }, organisation: null } });
    if (p === `/requests/${THEIRS}` || p === `/requests/${MINE}`) {
      if (!outsider && p === `/requests/${THEIRS}`) return route.fulfill({ status: 404, json: { status: 404, code: 'errors.not_found' } });
      return route.fulfill({ json: { ...request(p.split('/').pop(), IT, 'The VPN drops', { notes: NOTES }), comments: [REPLY], sla: [], satisfaction: null, merged_into: null } });
    }
    // A requester reads no entry through any door (UC-SVC-05): their REST answers are as before.
    if (p.startsWith('/items/') && outsider) return route.fulfill({ status: 404, json: { status: 404, code: 'errors.not_found' } });
    if (p === '/approvals') return route.fulfill({ json: { data: approvals, page: { next_cursor: null, has_more: false } } });
    if (p === '/portal-knowledge') return route.fulfill({ json: { data: [{ page_id: SPACE, title: 'Customer handbook', icon: '', with_subpages: true, pages: [{ page_id: UNDER, parent_id: SPACE, title: 'Lost device', icon: '' }] }] } });
    if (p === '/articles') return route.fulfill({ json: { data: [] } });
    if (p === `/articles/${UNDER}`) return route.fulfill({ json: { page_id: UNDER, collection_id: IT, variant_page_id: null, title: 'Lost device', icon: '', body: ARTICLE, content_language: null, updated_at: '2026-11-01T09:00:00Z' } });
    if (p === '/memberships') return route.fulfill({ json: { ...PAGE, data: [], grantable_roles: [] } });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}${path}`);
  return { page, failures, asked, close: () => context.close() };
}

// UC-SVC-05 checks 11 and 12, UC-SVC-09 check 5, UC-DOC-13 check 5.
test('chromium 1280 px: an outsider gets the portal frame alone, approvals on top and counted, spaces in knowledge', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, asked, close } = await open(browser, { width: 1280, outsider: true, path: '/' });
  t.after(close);
  // Today is no outsider's place: they land in a portal.
  await page.waitForURL(`**/portal/${IT}`, { timeout: 15_000 });
  const frame = page.locator('[data-portal-frame]');
  await frame.waitFor({ timeout: 10_000 });
  await page.getByRole('heading', { name: 'Hello Lena, how can we help?' }).waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('aside.sidenav').count(), 0, 'no sidebar');
  assert.equal(await page.locator('[data-tour="capture"]').count(), 0, 'no Erstellen');
  assert.equal(await page.locator('[data-tour="palette"]').count(), 0, 'no search over Hubtask');
  assert.equal(await page.locator('[data-portal-all-departments]').count(), 0, 'the directory is the inside\'s');
  await page.locator('[data-portal-tab="approvals"]', { hasText: 'Approvals · 1' }).waitFor({ timeout: 10_000 });
  await page.locator('[data-portal-tab="requests"]', { hasText: 'My requests · 1' }).waitFor({ timeout: 10_000 });
  // What waits for their yes stands on top of the start, the entry's title without a link to it.
  const waiting = page.locator('[data-portal-waiting]');
  await waiting.getByText('Access for Max to the supplier portal').first().waitFor({ timeout: 10_000 });
  assert.equal(await waiting.locator(`a[href="/items/${THEIRS}"]`).count(), 0, 'the team\'s entry is not opened');
  // The state in the desk's own words, never its key.
  const mine = page.locator('[data-portal-mine]');
  await mine.getByText('Being worked on').waitFor({ timeout: 10_000 });
  assert.equal(await mine.getByText('in_progress').count(), 0);
  // What comes next says when the team replied (P-12).
  await mine.getByText(/The team replied/).waitFor({ timeout: 10_000 });
  // The outsiders' start in two columns (scene `portalext`): what to ask and the knowledge on the
  // left, what waits and their requests beside it.
  const box = async (selector) => page.locator(selector).first().boundingBox();
  const [ask, waits, own] = [await box('[data-portal-kind]'), await box('[data-portal-waiting]'), await box('[data-portal-mine]')];
  assert.ok(waits.x > ask.x + ask.width, 'what waits stands beside the kinds');
  assert.ok(Math.abs(own.x - waits.x) < 2 && own.y > waits.y, 'the requests stand under what waits');

  await page.locator('[data-portal-tab="approvals"]').click();
  await page.waitForURL(`**/portal/${IT}/approvals`);
  await page.locator('[data-portal-approvals]').getByText('Access for Max to the supplier portal').first().waitFor({ timeout: 10_000 });

  await page.locator('[data-portal-tab="knowledge"]').click();
  await page.waitForURL(`**/portal/${IT}/knowledge`);
  const space = page.locator(`[data-portal-space="${SPACE}"]`);
  await space.getByText('2 articles · space').waitFor({ timeout: 10_000 });
  await space.getByRole('link', { name: 'Lost device' }).click();
  await page.waitForURL(`**/portal/${IT}/articles/${UNDER}`);
  await page.locator('[data-portal-outline]').getByRole('link', { name: 'Lost device' }).waitFor({ timeout: 10_000 });

  // Several portals are chosen at the top.
  await page.locator('[data-portal-switch]').click();
  await page.getByRole('menuitem', { name: 'People' }).click();
  await page.waitForURL(`**/portal/${HR}/knowledge`);

  // Any other address of the app leads back to the portal.
  await page.goto(`${served.origin}/search`);
  await page.waitForURL(`**/portal/${HR}`, { timeout: 15_000 });
  assert.equal(asked.includes('GET /items'), false);
  assert.deepEqual(failures, []);
});

test('chromium 375 px: on a phone the outsider has four portal tabs at the foot', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 375, outsider: true, path: `/portal/${IT}` });
  t.after(close);
  const bar = page.getByRole('navigation', { name: 'Portal' });
  await bar.waitFor({ timeout: 15_000 });
  assert.deepEqual(await bar.getByRole('link').allInnerTexts().then((texts) => texts.map((each) => each.split('\n').map((line) => line.trim()).find((line) => /[A-Za-z]/.test(line)))), ['Start', 'My requests', 'Approvals', 'Knowledge']);
  const width = await page.evaluate(() => ({ scroll: document.documentElement.scrollWidth, view: window.innerWidth }));
  assert.equal(width.scroll, width.view, 'no sideways scroll');
  // The language is chosen behind the bar's menu mark, as the scene draws it.
  assert.equal(await page.locator('header button[data-portal-language]').count(), 0, 'no language menu in the bar');
  await page.locator('[data-portal-frame] button[aria-haspopup="dialog"]').click();
  const languages = page.getByRole('dialog').locator('[data-portal-language]');
  await languages.getByText('Deutsch').waitFor({ timeout: 5_000 });
  // On a phone what waits comes first, before the kinds.
  const [waits, ask] = [await page.locator('[data-portal-waiting]').boundingBox(), await page.locator('[data-portal-kind]').first().boundingBox()];
  assert.ok(waits.y < ask.y, 'what waits stands above the kinds');
  assert.deepEqual(failures, []);
});

// UC-SVC-05 check 13 (PO-5): one link, the right view for each.
test('chromium: a request\'s link opens the portal request for its requester and the entry for the team', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const outsider = await open(browser, { width: 1280, outsider: true, path: `/items/${MINE}` });
  t.after(outsider.close);
  await outsider.page.waitForURL(`**/portal/requests/${MINE}`, { timeout: 15_000 });
  await outsider.page.getByText('Being worked on').first().waitFor({ timeout: 10_000 });
  assert.deepEqual(outsider.failures, []);

  const member = await open(browser, { width: 1280, outsider: false, path: `/portal/requests/${THEIRS}` });
  t.after(member.close);
  await member.page.waitForURL(`**/items/${THEIRS}`, { timeout: 15_000 });
  assert.deepEqual(member.failures, []);
});

// UC-SVC-14/1, UC-SVC-14/2, UC-SVC-14/3, UC-SVC-14/4.
test('chromium 1280 px: a member reaches every department through Erstellen, searches them all and sees their requests everywhere', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, outsider: false, path: `/portal/${IT}` });
  t.after(close);
  await page.locator('[data-portal-all-departments]').waitFor({ timeout: 15_000 });
  assert.equal(await page.locator('[data-portal-frame]').count(), 0, 'a member keeps the frame of the work');
  await page.locator('[data-tour="capture"]').first().click();
  await page.locator('[data-create-ask]').first().click();
  await page.waitForURL('**/portals', { timeout: 10_000 });
  const directory = page.locator('[data-portal-directory]');
  await directory.locator(`[data-portal-department="${IT}"]`).waitFor({ timeout: 10_000 });
  await directory.locator(`[data-portal-department="${HR}"]`).waitFor({ timeout: 10_000 });
  // My requests everywhere: newest first, each with its department, in the desk's words.
  const everywhere = directory.locator('[data-portal-dir-mine] [data-portal-request]');
  await everywhere.first().waitFor({ timeout: 10_000 });
  assert.deepEqual(await everywhere.evaluateAll((rows) => rows.map((row) => row.querySelector('.title')?.textContent)), ['Certificate for leave', 'The VPN drops']);
  await directory.locator('[data-portal-dir-mine] [data-portal-state="todo"]', { hasText: 'Received' }).waitFor();
  // One search over every department's kinds.
  await page.locator('[data-portal-dir-search] input, input[data-portal-dir-search]').first().fill('laptop');
  await page.locator('[data-portal-dir-kind="laptop"]').click();
  await page.getByRole('dialog', { name: 'Order a laptop' }).getByLabel('In a few words').waitFor({ timeout: 5_000 });
  assert.equal(await page.getByRole('dialog', { name: 'Order a laptop' }).getByLabel('In a few words').inputValue(), 'laptop');
  assert.deepEqual(failures, []);
});

// P-12: a request's notes, the team's replies and an article read as formatted text - no `**`, no
// `##` - and an HTML tag in the text is neither drawn nor run.
test('chromium 1280 px: the portal reads notes, replies and articles formatted, never as Markdown', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, outsider: true, path: `/portal/requests/${MINE}` });
  t.after(close);
  const notes = page.locator('[data-portal-notes]');
  await notes.locator('strong', { hasText: 'Since Monday' }).waitFor({ timeout: 15_000 });
  const replies = page.locator('[data-portal-reply]');
  await replies.locator('em', { hasText: 'restart' }).waitFor({ timeout: 10_000 });
  assert.equal(await replies.locator('li', { hasText: 'open it again' }).count(), 1, 'the reply\'s list is a list');
  for (const text of [await notes.innerText(), await replies.innerText()]) assert.doesNotMatch(text, /\*\*|_restart_|^- /m);

  await page.goto(`${served.origin}/portal/${IT}/articles/${UNDER}`);
  const body = page.locator('[data-portal-article-body]');
  await body.getByRole('heading', { name: 'If it is lost' }).waitFor({ timeout: 15_000 });
  await body.locator('strong', { hasText: 'Account' }).waitFor();
  assert.doesNotMatch(await body.innerText(), /##|\*\*/);
  assert.equal(await body.locator('img').count(), 0, 'an HTML tag in the text was drawn');
  assert.equal(await page.evaluate(() => window.injected), undefined, 'an HTML tag in the text ran');
  assert.deepEqual(failures, []);
});
