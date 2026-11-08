// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A hub's pages in three regions (user-experience.md §7.1): an empty space starts from a template
// in one press, a full one opens on its home page, the head is one row with the trash in the
// page's menu, comments are a side sheet the head toggles, and a public link is made from a
// preview of what it shows (UC-DOC-08 check 5).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, HUB, signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const id = (n) => `01a0e2e0-0000-7000-8000-0000000009${String(n).padStart(2, '0')}`;
const row = (n, title, extra = {}) => ({
  id: id(n), hub_id: HUB.id, parent_id: null, title, icon: null, position: `a${n}`, template: false, content_language: null, owner_id: null,
  verified_until: null, verified_at: null, verified_by: null, due_for_review: false, archived: false, restricted: false, daily_for: null,
  trashed_at: null, created_by: ACCOUNT.id, created_at: '2026-10-01T09:00:00Z', updated_by: ACCOUNT.id, updated_at: '2026-10-02T09:00:00Z', version: 1,
  ...extra,
});
const HOME = row(2, 'Welcome');
const FULL = [row(1, 'Our template', { template: true }), HOME, row(3, 'How we deploy'), row(4, 'Under welcome', { parent_id: HOME.id })];
const MANY = [HOME, ...Array.from({ length: 60 }, (_, n) => row(10 + n, `A page with a rather long title that has to wrap somewhere ${n}`, n % 3 ? { parent_id: HOME.id } : {}))];
const BODY = `Start with [](hubtask:page/${id(3)}) and [](hubtask:entry/${id(80)}).\n\n![](hubtask:media/${id(90)})\n`;
const empty = { data: [], page: { next_cursor: null, has_more: false } };

async function opened(browser, rows, width = 1280) {
  const written = [];
  const { page, failures, context } = await signedIn(browser, width, 900);
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace('/api/v1', '');
    const query = new URL(request.url()).searchParams;
    if (request.method() !== 'GET') written.push({ method: request.method(), path, body: request.postDataJSON() });
    if (path === `/hubs/${HUB.id}/pages` && request.method() === 'GET') {
      return route.fulfill({ json: { ...empty, data: query.get('templates') ? rows.filter((each) => each.template) : rows } });
    }
    if (path === `/hubs/${HUB.id}/pages` && request.method() === 'POST') {
      return route.fulfill({ status: 201, json: { ...row(50, request.postDataJSON().title), body: request.postDataJSON().body, ancestors: [], writable: true, shared: false } });
    }
    if (path === `/hubs/${HUB.id}/page-labels` || path === `/hubs/${HUB.id}/trashed-pages`) return route.fulfill({ json: empty });
    if (path === `/hubs/${HUB.id}/page-drafts`) return route.fulfill({ json: { hub_id: HUB.id, drafts: false } });
    const one = rows.find((each) => path === `/pages/${each.id}`) ?? (path === `/pages/${id(50)}` ? row(50, 'Knowledge base') : undefined);
    if (one) return route.fulfill({ json: { ...one, body: BODY, ancestors: [], writable: true, shared: false, drafts: false, unpublished: false, draft: null } });
    if (/^\/pages\/[^/]+\/(comments|versions|backlinks)$/.test(path)) return route.fulfill({ json: empty });
    if (/^\/pages\/[^/]+\/language-variants$/.test(path)) return route.fulfill({ json: { page_id: HOME.id, source_page_id: null, language: null, variants: [] } });
    if (/^\/pages\/[^/]+\/labels$/.test(path)) return route.fulfill({ json: { page_id: HOME.id, labels: [] } });
    if (/^\/pages\/[^/]+\/access$/.test(path)) {
      return route.fulfill({ json: {
        page_id: HOME.id, restricted: false, restricted_to: [], restricted_above: false, readers: [], shares: [], may_share: true,
        public_link: null, public_links_allowed: true, articles: [],
      } });
    }
    if (/^\/pages\/[^/]+\/public-link$/.test(path) && request.method() === 'POST') {
      return route.fulfill({ status: 201, json: { token: 'hbt_page_e2e', link: { page_id: HOME.id, created_by: ACCOUNT.id, created_at: '2026-11-04T09:00:00Z' } } });
    }
    return stub(route);
  });
  return { page, failures, written, close: () => context.close() };
}

// An empty space also offers to bring a wiki in: the import opens on the sources that land pages,
// and *Every source* widens them (user-experience.md §7.11).
test('an empty space offers a wiki import that opens on the wikis and documents', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await opened(browser, []);
    await page.goto(`${served.origin}/hubs/${HUB.id}/pages`);
    const start = page.locator('[data-space-start]');
    await start.getByText("Start this hub's pages").waitFor({ timeout: 15_000 });
    await start.locator('[data-space-import]').getByRole('button', { name: 'Choose a source' }).click();
    const panel = page.getByRole('dialog', { name: 'Capture' }).getByRole('tabpanel');
    await panel.getByText('Wikis and documents').first().waitFor({ timeout: 10_000 });
    assert.equal(await panel.getByText('Spreadsheets', { exact: true }).count(), 0, 'a source that lands no pages is offered first');
    await panel.getByRole('button', { name: 'Every source' }).click();
    await panel.getByText('Spreadsheets', { exact: true }).first().waitFor({ timeout: 5_000 });
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

test('an empty space offers its templates and starts one in a single press', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, written, close } = await opened(browser, []);
    await page.goto(`${served.origin}/hubs/${HUB.id}/pages`);
    const start = page.locator('[data-space-start]');
    await start.getByText("Start this hub's pages").waitFor({ timeout: 15_000 });
    assert.equal(await start.locator('[data-space-template]').count(), 4);
    await start.locator('[data-space-template="knowledge"]').getByRole('button', { name: 'Use' }).click();
    await page.waitForURL(`**/pages/${id(50)}`, { timeout: 10_000 });
    const made = written.find((write) => write.method === 'POST' && write.path === `/hubs/${HUB.id}/pages`);
    assert.equal(made?.body.title, 'Knowledge base');
    assert.equal(made?.body.subpages.length, 2);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

test('a full space opens on its home page, with one head row and comments beside it on demand', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await opened(browser, FULL);
    await page.goto(`${served.origin}/hubs/${HUB.id}/pages`);
    await page.waitForURL(`**/pages/${HOME.id}`, { timeout: 15_000 });
    const head = page.locator('[data-page-head]');
    await head.getByLabel('Title').waitFor({ timeout: 10_000 });
    assert.equal(await page.getByRole('button', { name: 'Put in the trash' }).count(), 0, 'the trash is not a button in the head');
    assert.equal(await page.locator('[data-page-side]').count(), 0, 'the side sheet is closed at first');
    // The filters wait behind one control.
    assert.equal(await page.getByLabel('Only pages I wrote').count(), 0);
    await head.locator('[data-page-comments]').click();
    await page.locator('[data-page-side="comments"]').waitFor();
    await head.locator('[data-page-comments]').click();
    assert.equal(await page.locator('[data-page-side]').count(), 0);
    await head.getByRole('button', { name: 'More for this page' }).click();
    await page.getByRole('menuitem', { name: 'Put in the trash' }).waitFor();
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

// One anatomy (UX-2): the hub's Pages tab carries the head every hub tab carries - the trail to the
// hub and the tab's title above the tab strip, with the new page as its primary - on the space and
// on an open page, and the screen keeps one h1.
// The navigation concept §2.4: a page draws no tree of its own - the sidebar's *Contents* is the
// tree - and its trail names the hub; the screen keeps one h1.
test('a page and an empty space name the hub in their trail and draw no tree of their own', async () => {
  const browser = await chromium.launch();
  try {
    for (const rows of [[], FULL]) {
      const { page, failures, close } = await opened(browser, rows);
      await page.goto(`${served.origin}/hubs/${HUB.id}/pages`);
      if (rows.length > 0) await page.waitForURL(`**/pages/${HOME.id}`, { timeout: 15_000 });
      await (rows.length ? page.locator('[data-page-head]') : page.locator('[data-space-start]')).waitFor({ timeout: 15_000 });
      const main = page.getByRole('main');
      await main.getByRole('link', { name: HUB.name }).first().waitFor({ timeout: 10_000 });
      assert.equal(await main.getByRole('tree').count(), 0, 'the page draws a tree of its own');
      assert.equal(await page.locator('h1').count(), 1, 'the screen has more or less than one h1');
      assert.deepEqual(failures, []);
      await close();
    }
  } finally {
    await browser.close();
  }
});

test('a public link is made from a preview of what it shows and leaves out', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, written, close } = await opened(browser, FULL);
    await page.goto(`${served.origin}/pages/${HOME.id}`);
    await page.locator('[data-page-share]').click();
    await page.locator('[data-public-link-make]').click();
    const preview = page.locator('[data-public-preview]');
    await preview.getByText('One picture', { exact: true }).waitFor({ timeout: 10_000 });
    await preview.getByText(/^One mentioned page/u).waitFor();
    await preview.getByText('One mention of entries, boards, views and other places').waitFor();
    assert.equal(written.filter((write) => write.path.endsWith('/public-link')).length, 0, 'nothing is made before the preview is confirmed');
    await page.getByRole('button', { name: 'Make the link' }).click();
    await page.getByLabel("The page's public link").waitFor({ timeout: 10_000 });
    assert.equal(written.filter((write) => write.method === 'POST' && write.path.endsWith('/public-link')).length, 1);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

test('the space fits a phone without scrolling sideways', async () => {
  const browser = await chromium.launch();
  try {
    for (const rows of [[], FULL, MANY]) {
      const { page, failures, close } = await opened(browser, rows, 375);
      await page.goto(`${served.origin}/hubs/${HUB.id}/pages`);
      await (rows.length ? page.locator('[data-page-head]') : page.locator('[data-space-start]')).waitFor({ timeout: 15_000 });
      const wide = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      assert.ok(wide <= 0, `the page scrolls ${wide}px sideways with ${rows.length} pages`);
      // The tree is the sidebar's, in its drawer on a phone; the page draws none of its own.
      if (rows.length) assert.equal(await page.getByRole('main').getByRole('tree').count(), 0);
      assert.deepEqual(failures, []);
      await close();
    }
  } finally {
    await browser.close();
  }
});

test("the drafts switch is among the hub's settings, not beside the tree", async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, written, close } = await opened(browser, FULL);
    await page.goto(`${served.origin}/hubs/${HUB.id}/pages`);
    await page.locator('[data-page-head]').waitFor({ timeout: 15_000 });
    assert.equal(await page.getByLabel('Drafts and publishing').count(), 0);
    await page.goto(`${served.origin}/hubs/${HUB.id}/settings`);
    const drafts = page.locator('[data-page-drafts]');
    await drafts.getByLabel('Drafts and publishing').waitFor({ timeout: 15_000 });
    const sent = page.waitForRequest((request) => request.method() === 'PUT' && request.url().endsWith(`/hubs/${HUB.id}/page-drafts`), { timeout: 10_000 });
    await drafts.getByLabel('Drafts and publishing').click();
    assert.deepEqual((await sent).postDataJSON(), { drafts: true });
    assert.equal(written.filter((write) => write.method === 'PUT').length, 1);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});
