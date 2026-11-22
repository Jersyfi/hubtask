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

async function opened(browser, rows, width = 1280, { body = BODY, unpublished = false, library = false } = {}) {
  const written = [];
  const kept = new Map();
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
    if (path === `/hubs/${HUB.id}/page-drafts`) return route.fulfill({ json: { hub_id: HUB.id, drafts: unpublished } });
    const one = rows.find((each) => path === `/pages/${each.id}`) ?? (path === `/pages/${id(50)}` ? row(50, 'Knowledge base') : undefined);
    if (one && request.method() === 'PATCH') {
      kept.set(one.id, { ...kept.get(one.id), ...request.postDataJSON() });
      return route.fulfill({ json: { ...one, body, ...kept.get(one.id), version: one.version + 1, ancestors: [], writable: true, shared: false, drafts: unpublished, unpublished, draft: null } });
    }
    if (one) return route.fulfill({ json: { ...one, body, ...kept.get(one.id), ancestors: [], writable: true, shared: false, drafts: unpublished, unpublished, draft: null } });
    if (/^\/pages\/[^/]+\/(comments|versions|backlinks)$/.test(path)) return route.fulfill({ json: empty });
    if (/^\/pages\/[^/]+\/cover-images$/.test(path)) {
      const pictures = library && query.get('query') ? [{ id: '11111111-1111-1111-1111-111111111111', creator: 'Ada Lovelace', preview: 'data:image/png;base64,iVBORw0KGgo=' }] : [];
      return route.fulfill({ json: { offered: library, pictures } });
    }
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

test('a full space opens on its home page: the path on top, the title once, the trust line, comments beside it (S-1, S-2, S-5)', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await opened(browser, FULL);
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.goto(`${served.origin}/hubs/${HUB.id}/pages`);
    await page.waitForURL(`**/pages/${HOME.id}`, { timeout: 15_000 });
    const head = page.locator('[data-page-head]');
    const reading = page.locator('[data-page-reading]');
    await reading.getByLabel('Title').waitFor({ timeout: 10_000 });
    assert.equal(await head.getByLabel('Title').count(), 0, 'the top row holds only the path and the people');
    assert.equal(await page.getByRole('button', { name: 'Put in the trash' }).count(), 0, 'the trash is not a button in the head');
    await reading.locator('[data-trust-line]').waitFor();
    // From 1240 px the comments stand beside the reading column, writing at their foot.
    const margin = page.locator('[data-page-margin]');
    await margin.waitFor();
    assert.equal(await page.locator('[data-page-side]').count(), 0, 'the side sheet is closed at first');
    await head.locator('[data-page-comments]').click();
    assert.equal(await margin.locator('[data-page-comment-new] textarea').evaluate((field) => field === document.activeElement), true, 'the count leads to the writing field');
    // The reading column keeps its measure; a writer's column has the handle's gutter before it.
    const measure = await reading.locator('article').evaluate((column) => column.clientWidth - parseFloat(getComputedStyle(column).paddingInlineStart) - parseFloat(getComputedStyle(column).paddingInlineEnd));
    assert.ok(measure <= 720, `the reading column is at most 720 px wide, not ${measure}`);
    // The filters wait behind one control.
    assert.equal(await page.getByLabel('Only pages I wrote').count(), 0);
    await head.getByRole('button', { name: 'More for this page' }).click();
    await page.getByRole('menuitem', { name: 'Put in the trash' }).waitFor();
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

// A page put in the trash from its menu leaves for the hub's pages, and nothing asks for it again:
// a read or a save - of a title or of words still waiting - after the delete would find a page that is gone and leave the reader on it.
test('a page put in the trash from its menu leaves for the space and is not asked for again', async () => {
  const browser = await chromium.launch();
  try {
    const ONLY = row(3, 'How we deploy');
    const rows = [HOME, ONLY];
    const { page, failures, close } = await opened(browser, rows, 1440);
    const asked = [];
    page.on('request', (request) => {
      const path = new URL(request.url()).pathname.replace('/api/v1', '');
      if (path.startsWith(`/pages/${ONLY.id}`)) asked.push({ method: request.method(), path });
    });
    await page.context().route(`**/api/v1/pages/${ONLY.id}`, (route) => {
      if (route.request().method() !== 'DELETE') return route.fallback();
      rows.splice(rows.indexOf(ONLY), 1);
      return route.fulfill({ json: { trashed: 1 } });
    });
    await page.context().routeWebSocket(/\/realtime\/documents\/page\//, (ws) => {
      ws.onMessage(() => {});
      ws.send(Buffer.from([0, 1, 2, 0, 0]));
      ws.send(Buffer.from([0, 0, 1, 0]));
    });
    await page.goto(`${served.origin}/pages/${ONLY.id}`);
    // A title saved, and words typed whose save still waits when the page goes.
    const title = page.getByLabel('Title', { exact: true });
    await title.fill('How we deploy now', { timeout: 15_000 });
    const renamed = page.waitForResponse((response) => response.request().method() === 'PATCH');
    await title.press('Enter');
    await renamed;
    await page.locator('[contenteditable="true"]').first().click();
    await page.keyboard.type('Words still waiting to be saved');
    const head = page.locator('[data-page-head]');
    await head.getByRole('button', { name: 'More for this page' }).click({ timeout: 15_000 });
    await page.getByRole('menuitem', { name: 'Put in the trash' }).click();
    await page.waitForURL(`**/hubs/${HUB.id}/pages`, { timeout: 10_000 });
    await page.getByText('1 page went into the trash.').first().waitFor({ timeout: 10_000 });
    const deleted = asked.findIndex((each) => each.method === 'DELETE');
    assert.ok(deleted >= 0, 'the delete left the browser');
    assert.deepEqual(asked.slice(deleted + 1), [], 'the trashed page was asked for after its delete');
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
    // The switch joins *General*'s one save (PS-10).
    await page.getByRole('region', { name: 'Unsaved changes' }).getByRole('button', { name: 'Save', exact: true }).click();
    assert.deepEqual((await sent).postDataJSON(), { drafts: true });
    assert.equal(written.filter((write) => write.method === 'PUT').length, 1);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

// S-6 (scene pnew): a new page is the page at once - the drafts band over it, the templates as chips
// under its empty title, one previewed beside it before it is used, "/" and "@" explained in the
// empty text.
test('a new page offers its templates as chips with a preview beside, under the drafts band (S-6)', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, written, close } = await opened(browser, FULL, 1440, { body: '', unpublished: true });
    await page.setViewportSize({ width: 1440, height: 900 });
    // The live document answers empty, as a new page's does: the editor writes then.
    await page.context().routeWebSocket(/\/realtime\/documents\/page\//, (ws) => {
      ws.onMessage(() => {});
      ws.send(Buffer.from([0, 1, 2, 0, 0]));
      ws.send(Buffer.from([0, 0, 1, 0]));
    });
    await page.goto(`${served.origin}/pages/${HOME.id}`);
    const band = page.locator('[data-page-draft-band]');
    await band.waitFor({ timeout: 15_000 });
    assert.match(await band.innerText(), /^Draft\. Only you see this page until you publish it/);
    const starts = page.locator('[data-page-starts]');
    await starts.waitFor({ timeout: 10_000 });
    const chips = await starts.locator('.chips > [data-page-start]').allInnerTexts();
    assert.deepEqual(chips, ['🗓️ Meeting notes', '⚖️ Decision record', '🔄 Retrospective', '📘 How-to guide', '🎯 Project brief', 'Our template']);
    assert.match(await starts.innerText(), /Whole space: Team space · Project space · Knowledge base/);
    const empty = page.locator('.page-empty');
    await empty.waitFor({ timeout: 10_000 });
    assert.match(await empty.getAttribute('data-placeholder') ?? '', /type \/ for blocks, @ for people, pages and entries/);
    await starts.locator('[data-page-start="shipped:decision"]').click();
    const preview = page.locator('[data-page-start-preview]');
    await preview.waitFor({ timeout: 10_000 });
    assert.match(await preview.innerText(), /Decision record[\s\S]*Context[\s\S]*Consequences/);
    const column = await page.locator('article.column').boundingBox();
    const beside = await preview.boundingBox();
    assert.ok(column && beside && beside.x >= column.x + column.width - 1, 'the preview stands beside the page, not in it');
    await preview.getByRole('button', { name: 'Use this template' }).click();
    await preview.waitFor({ state: 'detached', timeout: 10_000 });
    const change = written.find((each) => each.method === 'PATCH' && each.path === `/pages/${HOME.id}`);
    assert.ok(change && change.body.body.includes('## Context'), 'the template\'s words were not written');
    assert.equal(change.body.icon, '⚖️');
    assert.equal(change.body.title, undefined, 'a page with a title of its own keeps it');
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

// UC-DOC-16 check 5: where the workspace offers an image library, the picker has a fourth tab; a
// search goes to the server with its words, and each picture names who made it.
test('a cover takes a picture from the image library the workspace offers (UC-DOC-16)', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await opened(browser, FULL, 1440, { library: true });
    const asked = [];
    page.on('request', (request) => { if (request.url().includes('/cover-images')) asked.push(new URL(request.url()).searchParams.get('query')); });
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.goto(`${served.origin}/pages/${HOME.id}`);
    await page.locator('[data-page-cover-add]').click({ timeout: 15_000 });
    const picker = page.locator('[data-page-cover-picker]');
    await picker.getByRole('tab', { name: 'Image library' }).waitFor({ timeout: 10_000 });
    assert.deepEqual(await picker.getByRole('tab').allInnerTexts(), ['Colours', 'Gallery', 'Upload', 'Image library']);
    await picker.getByRole('tab', { name: 'Image library' }).click();
    await picker.getByLabel('Search pictures').fill('fjord');
    await picker.getByLabel('Search pictures').press('Enter');
    await picker.getByRole('button', { name: 'Picture by Ada Lovelace' }).waitFor({ timeout: 10_000 });
    assert.ok(asked.includes('fjord'), 'the search went to the server with its words');
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

// Scene pread: while reading, nothing to make stands above the page's first words - *Add a cover*
// and the cover's own acts show where the pointer is over them or the keyboard reaches them.
test('a page read with a pointer shows its cover acts only when they are pointed at', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await opened(browser, FULL, 1440);
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.goto(`${served.origin}/pages/${HOME.id}`);
    const add = page.locator('[data-page-cover-add]');
    await add.waitFor({ timeout: 15_000 });
    const shown = (locator) => locator.evaluate((element) => getComputedStyle(element.closest('.page-adds, .acts')).opacity);
    await page.mouse.move(2, 880);
    assert.equal(await shown(add), '0', 'Add a cover stands over the page while reading');
    await page.locator('article.column').hover({ position: { x: 40, y: 40 } });
    assert.equal(await shown(add), '1', 'Add a cover is not shown when the head is pointed at');
    await add.click();
    const change = page.locator('[data-page-cover-change]');
    await change.waitFor({ timeout: 10_000 });
    // Closed and left: the reader moves on, the keyboard elsewhere.
    if (await page.locator('[data-page-cover-picker]').count() > 0) await change.click();
    await page.locator('[data-page-cover-picker]').waitFor({ state: 'detached', timeout: 5_000 });
    await page.evaluate(() => (document.activeElement instanceof HTMLElement ? document.activeElement.blur() : undefined));
    await page.mouse.move(2, 880);
    assert.equal(await shown(change), '0', 'the cover acts stand on the picture while reading');
    await page.locator('[data-page-cover] .picture').hover();
    assert.equal(await shown(change), '1', 'the cover acts are not shown when the cover is pointed at');
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

// E-18, UC-DOC-16 checks 1 and 2 (scene pcover): a cover over the content's width, the icon and title under the
// picture; picked from the design system's gradients, the gallery or an upload, and taken away.
test('a page takes a cover over its width, picked in tabs, the title under it (UC-DOC-16)', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, written, close } = await opened(browser, FULL, 1440);
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.goto(`${served.origin}/pages/${HOME.id}`);
    await page.locator('[data-page-cover-add]').click({ timeout: 15_000 });
    const cover = page.locator('[data-page-cover]');
    await cover.waitFor({ timeout: 10_000 });
    const covers = () => written.filter((each) => each.method === 'PATCH' && 'cover' in each.body).map((each) => each.body.cover);
    assert.deepEqual(covers()[0], { source: 'GRADIENT', key: 'indigo-teal', position: 50 });
    const picker = page.locator('[data-page-cover-picker]');
    await picker.waitFor({ timeout: 5_000 });
    assert.deepEqual(await picker.getByRole('tab').allInnerTexts(), ['Colours', 'Gallery', 'Upload'], 'no image provider unless the administration switches one on');
    assert.equal(await picker.locator('[data-cover-key]').count(), 12);
    await picker.getByRole('tab', { name: 'Gallery' }).click();
    await picker.locator('[data-cover-key="hills"]').click();
    await page.locator('[data-page-cover="GALLERY:hills"]').waitFor({ timeout: 10_000 });
    assert.deepEqual(covers().at(-1), { source: 'GALLERY', key: 'hills', position: 50 });
    const band = await page.locator('[data-page-cover] .picture').boundingBox();
    const column = await page.locator('article.column').boundingBox();
    const title = await page.getByLabel('Title').boundingBox();
    assert.ok(band && column && title, 'the cover, the column and the title are drawn');
    assert.ok(band.width > column.width, 'the cover spans the content, not the reading column');
    assert.ok(title.y >= band.y + band.height, 'the title stands under the picture, never on it');
    await picker.getByRole('tab', { name: 'Upload' }).click();
    await picker.getByText('Choose a picture').waitFor({ timeout: 5_000 });
    await page.locator('[data-page-cover-remove]').click();
    await page.locator('[data-page-cover-add]').waitFor({ timeout: 10_000 });
    assert.equal(covers().at(-1), null);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});
