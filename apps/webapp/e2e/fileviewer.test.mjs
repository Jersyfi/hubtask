// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The one file viewer (F-1) as a person reaches it: from an entry's files (UC-MED-05 checks 2, 3),
// with an office file shown as its document server's PDF or as a card and no *Edit* while the
// launch waits (UC-MED-06 check 2, ADR-0104); from a page's file card (UC-DOC-14 check 1); from a
// file on a whiteboard (UC-CNV-08 checks 1, 3; UC-MED-05 check 4); and the workspace's *Files*
// section that chooses and tests the document server (UC-MED-06 check 1). The server is stubbed
// at the network edge.
// Chromium: no engine-specific part.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, HUB, ITEMS, MANIFEST, OTHER, signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const ENTRY = ITEMS[0];
const AT = '2026-10-30T09:00:00Z';
const LATER = '2099-01-01T00:00:00Z';
const TEXT_ID = '01a0e2e0-0000-7000-8000-000000003101';
const DOCX_ID = '01a0e2e0-0000-7000-8000-000000003102';
const PNG_ID = '01a0e2e0-0000-7000-8000-000000003103';
const BIN_ID = '01a0e2e0-0000-7000-8000-000000003104';
const PAGE_ID = '01a0e2e0-0000-7000-8000-000000003111';
const BOARD_ID = '01a0e2e0-0000-7000-8000-000000003121';

/** Where a file's bytes are served: the same origin, so the page's policy admits it. */
const contentOf = (id) => `/api/v1/e2e-content/${id}`;
const record = (id, file_name, content_type, size, extra = {}) => ({
  id, file_name, content_type, size, usage: 'ATTACHMENT', status: 'READY', ref_count: 1, created_by: OTHER.id, created_at: AT,
  inline: false, office: false, editable: false, ...extra,
});
const TEXT = record(TEXT_ID, 'notes.txt', 'text/plain', 26);
// The record says it may be edited; the launch is not decided, so *Edit* stays undrawn all the same.
const DOCX = record(DOCX_ID, 'offer.docx', 'application/vnd.openxmlformats-officedocument.wordprocessingml.document', 4096, { office: true });
const PNG = record(PNG_ID, 'site.png', 'image/png', 68, { inline: true });
const BIN = record(BIN_ID, 'survey.dwg', 'application/octet-stream', 2048);
const withDownload = (object) => ({ ...object, download: { url: `${served.origin}${contentOf(object.id)}`, method: 'GET', expires_at: LATER } });

// A 1×1 PNG and the smallest PDF pdf.js opens: one empty page.
const ONE_PIXEL = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==', 'base64');
function tinyPdf() {
  const objects = ['<</Type/Catalog/Pages 2 0 R>>', '<</Type/Pages/Kids[3 0 R]/Count 1>>', '<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 200]>>'];
  let out = '%PDF-1.4\n';
  const offsets = objects.map((body, i) => {
    const at = out.length;
    out += `${i + 1} 0 obj${body}endobj\n`;
    return at;
  });
  const xref = out.length;
  out += `xref\n0 ${objects.length + 1}\n0000000000 65535 f \n${offsets.map((at) => `${String(at).padStart(10, '0')} 00000 n \n`).join('')}`;
  out += `trailer<</Size ${objects.length + 1}/Root 1 0 R>>\nstartxref\n${xref}\n%%EOF\n`;
  return Buffer.from(out, 'latin1');
}

const problem = (route, status, code, detail_code) => route.fulfill({
  status, contentType: 'application/problem+json', body: JSON.stringify({ type: 'about:blank', title: code, status, code, detail_code }),
});

/**
 * The media edge every walk shares: a record per id with a download target, its bytes, the
 * workspace's document server and the conversion. `pdf` is what `:pdf` answers.
 */
function mediaEdge({ server = { kind: 'NONE' }, pdf = 'pdf' } = {}) {
  const asked = { pdf: [] };
  const records = { [TEXT_ID]: TEXT, [DOCX_ID]: DOCX, [PNG_ID]: PNG, [BIN_ID]: BIN };
  const handle = (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path === '/api/v1/media:document-server' && request.method() === 'GET') return route.fulfill({ json: server });
    const converting = path.match(/^\/api\/v1\/media\/([^/]+):pdf$/);
    if (converting && request.method() === 'POST') {
      asked.pdf.push(converting[1]);
      if (pdf === 'pdf') return route.fulfill({ status: 200, contentType: 'application/pdf', body: tinyPdf() });
      return problem(route, 409, 'conflict', 'media.conversion_unavailable');
    }
    const read = path.match(/^\/api\/v1\/media\/([^/:]+)$/);
    if (read && records[read[1]] && request.method() === 'GET') return route.fulfill({ json: withDownload(records[read[1]]) });
    const content = path.match(/^\/api\/v1\/e2e-content\/([^/]+)$/);
    if (content) {
      if (content[1] === TEXT_ID) return route.fulfill({ status: 200, contentType: 'text/plain', body: 'Bring the tiles on Monday.' });
      if (content[1] === PNG_ID) return route.fulfill({ status: 200, contentType: 'image/png', body: ONE_PIXEL });
      return route.fulfill({ status: 404, body: '' });
    }
    return undefined;
  };
  return { asked, handle };
}

const viewerOf = (page) => page.getByRole('dialog', { name: 'File viewer' });
const positionOf = async (page) => ((await viewerOf(page).locator('[data-viewer-position]').textContent()) ?? '').replace(/\s+/g, ' ').trim();

/** The entry with its two attachments, the viewer not yet opened. */
async function openEntry(browser, options) {
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 1000);
  const edge = mediaEdge(options);
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith(`/items/${ENTRY.id}/attachments`) && request.method() === 'GET') {
      return route.fulfill({ json: { data: [TEXT, { ...DOCX, editable: options.editable === true }], page: { next_cursor: null, has_more: false } } });
    }
    if (path.endsWith(`/media/${DOCX_ID}:edit`) && request.method() === 'POST') {
      edge.asked.edit = [...(edge.asked.edit ?? []), DOCX_ID];
      return route.fulfill({ json: {
        access_token: 'e2e-wopi-token', access_token_ttl: 1_794_650_000_000, wopi_src: `https://hub.example/api/v1/wopi/files/${DOCX_ID}`,
        action_url: `https://office.example/browser/e2e/cool.html?WOPISrc=https%3A%2F%2Fhub.example%2Fapi%2Fv1%2Fwopi%2Ffiles%2F${DOCX_ID}`,
        server: { kind: 'COLLABORA_ONLINE', url: 'https://office.example' },
      } });
    }
    return edge.handle(route) ?? stub(route);
  });
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });
  const files = page.locator('[data-entry-attachments]');
  await files.getByRole('button', { name: 'notes.txt', exact: true }).waitFor({ timeout: 10_000 });
  return { page, failures, context, unstubbed, files, asked: edge.asked };
}

test('chromium: an entry\'s file opens the viewer with its place among the files; ← → page, Esc closes, an office file without a server is a card, and no Edit is drawn (UC-MED-05 checks 2, 3; UC-MED-06 check 2)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, files, asked } = await openEntry(browser, { server: { kind: 'NONE' } });
  t.after(() => context.close());

  await files.getByRole('button', { name: 'notes.txt', exact: true }).click();
  const viewer = viewerOf(page);
  await viewer.waitFor({ timeout: 5_000 });
  assert.equal((await viewer.getByRole('heading', { level: 2 }).textContent()).trim(), 'notes.txt');
  assert.match(await positionOf(page), /^· 1 of 2 · on \S/);
  // The details name the entry by its key and its title, apart.
  assert.match((await viewer.locator('[data-viewer-details] p:has(.key)').textContent()).trim(), /^[A-Z]+-\d+ \S/);
  // A text file is drawn from its bytes, in the viewer itself.
  await viewer.locator('pre.text').getByText('Bring the tiles on Monday.').waitFor({ timeout: 10_000 });

  await page.keyboard.press('ArrowRight');
  await viewer.getByRole('heading', { level: 2, name: 'offer.docx' }).waitFor({ timeout: 5_000 });
  assert.match(await positionOf(page), /^· 2 of 2 · on \S/);
  // No document server: the office file is the card that says so, and nothing is converted.
  const card = viewer.locator('[data-viewer-card]');
  await card.waitFor({ timeout: 5_000 });
  assert.ok(await card.getByText('Office files are shown by a document server, and this workspace has none.').isVisible());
  assert.ok(await card.getByRole('button', { name: 'Download' }).isVisible());
  assert.deepEqual(asked.pdf, []);
  // Without a server the record is not editable, and no *Edit* is drawn.
  assert.equal(await viewer.locator('[data-viewer-edit]').count(), 0);
  assert.equal(await viewer.getByRole('button', { name: 'Edit in the document server' }).count(), 0);

  await page.keyboard.press('ArrowRight');
  await viewer.getByRole('heading', { level: 2, name: 'notes.txt' }).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');
  await viewer.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('chromium: with a document server the office file is asked for as a PDF and drawn as its pages (UC-MED-05 check 3)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, files, asked } = await openEntry(browser, { server: { kind: 'COLLABORA_ONLINE', url: 'https://office.example' }, pdf: 'pdf' });
  t.after(() => context.close());

  await files.getByRole('button', { name: 'offer.docx', exact: true }).click();
  const viewer = viewerOf(page);
  await viewer.waitFor({ timeout: 5_000 });
  await viewer.locator('[data-viewer-pdf] canvas').first().waitFor({ timeout: 15_000 });
  assert.deepEqual(asked.pdf, [DOCX_ID]);
  assert.equal(await viewer.locator('[data-viewer-card]').count(), 0);
  // A reader who may not change the entry: the record is not editable, and no *Edit* is drawn.
  assert.equal(await viewer.getByRole('button', { name: 'Edit in the document server' }).count(), 0);
  assert.deepEqual(failures, []);
});

test('chromium: Edit posts the token into a frame at the document server\'s editor, never in a URL (UC-MED-06 checks 2, 3)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, files, asked } = await openEntry(browser, { server: { kind: 'COLLABORA_ONLINE', url: 'https://office.example' }, pdf: 'pdf', editable: true });
  t.after(() => context.close());
  const posted = [];
  await context.route('https://office.example/**', (route) => {
    const request = route.request();
    posted.push({ method: request.method(), url: request.url(), body: request.postData() ?? '' });
    return route.fulfill({ contentType: 'text/html', body: '<!DOCTYPE html><title>Editor</title><p>The document server\'s editor</p>' });
  });

  await files.getByRole('button', { name: 'offer.docx', exact: true }).click();
  const viewer = viewerOf(page);
  await viewer.locator('[data-viewer-pdf] canvas').first().waitFor({ timeout: 15_000 });
  await viewer.getByRole('button', { name: 'Edit in the document server' }).first().click();
  const frame = viewer.locator('iframe[data-viewer-editor]');
  await frame.waitFor({ timeout: 5_000 });
  await page.frameLocator('iframe[data-viewer-editor]').getByText('The document server\'s editor').waitFor({ timeout: 10_000 });
  assert.deepEqual(asked.edit, [DOCX_ID]);
  assert.equal(posted.length, 1, `posted ${JSON.stringify(posted)}`);
  assert.equal(posted[0].method, 'POST');
  assert.ok(posted[0].url.startsWith('https://office.example/browser/e2e/cool.html?WOPISrc='), posted[0].url);
  assert.ok(!posted[0].url.includes('e2e-wopi-token'), 'the token travelled in the URL');
  const form = new URLSearchParams(posted[0].body);
  assert.equal(form.get('access_token'), 'e2e-wopi-token');
  assert.equal(form.get('access_token_ttl'), '1794650000000');
  // The editor closes back to the preview.
  await viewer.getByRole('button', { name: 'Close the editor' }).click();
  await viewer.locator('[data-viewer-pdf] canvas').first().waitFor({ timeout: 15_000 });
  assert.equal(await frame.count(), 0);
  assert.deepEqual(failures, []);
});

test('chromium: a document server that converts nothing leaves the office file a card (UC-MED-05 check 3)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, files, asked } = await openEntry(browser, { server: { kind: 'COLLABORA_ONLINE', url: 'https://office.example' }, pdf: 'unavailable' });
  t.after(() => context.close());

  await files.getByRole('button', { name: 'offer.docx', exact: true }).click();
  const viewer = viewerOf(page);
  await viewer.locator('[data-viewer-card]').waitFor({ timeout: 10_000 });
  assert.deepEqual(asked.pdf, [DOCX_ID]);
  assert.equal(await viewer.getByRole('alert').count(), 0, 'the refusal was drawn as a failure, not as the card');
  assert.deepEqual(failures, []);
});

test('chromium: a page\'s file card opens the viewer named after the page (UC-DOC-14 check 1, UC-MED-05 check 2)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  const PAGE = {
    id: PAGE_ID, hub_id: HUB.id, parent_id: null, title: 'Offers', icon: null, position: 'a1', template: false, content_language: null, owner_id: null,
    verified_until: null, verified_at: null, verified_by: null, due_for_review: false, archived: false, restricted: false, daily_for: null,
    trashed_at: null, created_by: ACCOUNT.id, created_at: AT, updated_by: ACCOUNT.id, updated_at: AT, version: 1,
  };
  const empty = { data: [], page: { next_cursor: null, has_more: false } };
  const edge = mediaEdge();
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const path = new URL(route.request().url()).pathname.replace('/api/v1', '');
    if (path === `/hubs/${HUB.id}/pages`) return route.fulfill({ json: { ...empty, data: [PAGE] } });
    if (path === `/hubs/${HUB.id}/page-labels` || path === `/hubs/${HUB.id}/trashed-pages`) return route.fulfill({ json: empty });
    if (path === `/hubs/${HUB.id}/page-drafts`) return route.fulfill({ json: { hub_id: HUB.id, drafts: false } });
    if (path === `/pages/${PAGE_ID}`) {
      // Read only: the body is drawn without a live document to play.
      const body = `The offer as sent:\n\n![](hubtask:media/${DOCX_ID})\n`;
      return route.fulfill({ json: { ...PAGE, body, ancestors: [], writable: false, shared: false, drafts: false, unpublished: false, draft: null } });
    }
    if (/^\/pages\/[^/]+\/(comments|versions|backlinks)$/.test(path)) return route.fulfill({ json: empty });
    if (/^\/pages\/[^/]+\/language-variants$/.test(path)) return route.fulfill({ json: { page_id: PAGE_ID, source_page_id: null, language: null, variants: [] } });
    if (/^\/pages\/[^/]+\/labels$/.test(path)) return route.fulfill({ json: { page_id: PAGE_ID, labels: [] } });
    return edge.handle(route) ?? stub(route);
  });
  await page.goto(`${served.origin}/pages/${PAGE_ID}`);
  const card = page.getByRole('button', { name: 'Open offer.docx' });
  await card.waitFor({ timeout: 20_000 });
  await card.click();
  const viewer = viewerOf(page);
  await viewer.waitFor({ timeout: 5_000 });
  assert.equal((await viewer.getByRole('heading', { level: 2 }).textContent()).trim(), 'offer.docx');
  assert.equal(await positionOf(page), '· 1 of 1 · on Offers');
  assert.ok(await viewer.locator('[data-viewer-details]').getByText('Offers').isVisible(), 'the details do not name the page');
  assert.deepEqual(failures, []);
});

const varUint = (n) => {
  const out = [];
  while (n > 0x7f) { out.push(0x80 | (n & 0x7f)); n = Math.floor(n / 128); }
  out.push(n);
  return out;
};
/** The server's answer to a new document: sync step 2 with an empty update. */
const EMPTY_DRAWING = Buffer.from([0, 1, ...varUint(2), 0, 0]);

test('chromium: on a board a picture is drawn as itself, a file as a card, and a double click on the card opens the viewer, where it is a card to download (UC-CNV-08 checks 1, 3; UC-MED-05 checks 2, 4)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 1280, 860);
  t.after(() => context.close());
  const elements = [
    { id: 'p1', kind: 'IMAGE', x: 0, y: 0, width: 200, height: 160, media_id: PNG_ID },
    { id: 'd1', kind: 'FILE', x: 260, y: 0, width: 220, height: 180, media_id: BIN_ID },
  ];
  const board = {
    id: BOARD_ID, hub_id: HUB.id, title: 'Site plan', element_count: elements.length, elements, writable: true, trashed_at: null,
    created_by: ACCOUNT.id, created_at: AT, updated_by: ACCOUNT.id, updated_at: AT, version: 1,
  };
  await context.routeWebSocket(/\/realtime\/documents\/whiteboard\//, (ws) => {
    ws.onMessage(() => {});
    ws.send(EMPTY_DRAWING);
  });
  const edge = mediaEdge();
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const at = new URL(route.request().url()).pathname;
    if (at.endsWith(`/whiteboards/${BOARD_ID}`)) return route.fulfill({ json: board });
    if (at.endsWith(`/whiteboards/${BOARD_ID}/comments`)) return route.fulfill({ json: { data: [] } });
    if (at.endsWith(`/hubs/${HUB.id}/whiteboards`)) return route.fulfill({ json: { data: [board] } });
    if (at.endsWith('/meta/capabilities')) return route.fulfill({ json: { ...MANIFEST, modules_on: [...(MANIFEST.modules_on ?? []), 'WHITEBOARD'] } });
    if (/\/containers\/[^/]+\/modules$/.test(at)) return route.fulfill({ json: { modules_on: [...(MANIFEST.modules_on ?? []), 'WHITEBOARD'] } });
    return edge.handle(route) ?? stub(route);
  });
  await page.goto(`${served.origin}/whiteboards/${BOARD_ID}`);
  const canvas = page.getByRole('application');
  await canvas.locator('g.object, g.card').first().waitFor({ timeout: 20_000 });

  // The picture is an <image> of its own bytes; the drawing file is the card with its name.
  const picture = canvas.locator(`image[href="${served.origin}${contentOf(PNG_ID)}"]`);
  await picture.waitFor({ state: 'attached', timeout: 10_000 });
  const file = canvas.locator('g.card.file');
  await file.getByText('survey.dwg').waitFor({ state: 'attached', timeout: 10_000 });
  assert.equal(await file.getAttribute('data-board-file'), 'card');
  assert.equal(await canvas.locator('g.card.file').count(), 1, 'the picture was drawn as a card');

  const [vx, vy, vw, vh] = (await canvas.getAttribute('viewBox')).split(/\s+/).map(Number);
  const area = await canvas.boundingBox();
  const on = { x: area.x + ((370 - vx) / vw) * area.width, y: area.y + ((60 - vy) / vh) * area.height };
  await page.mouse.dblclick(on.x, on.y);
  const viewer = viewerOf(page);
  await viewer.waitFor({ timeout: 5_000 });
  assert.equal((await viewer.getByRole('heading', { level: 2 }).textContent()).trim(), 'survey.dwg');
  assert.equal(await positionOf(page), '· 2 of 2 · on Site plan');
  // What Hubtask cannot show is a card with its kind, its size and the download.
  const card = viewer.locator('[data-viewer-card]');
  await card.waitFor({ timeout: 5_000 });
  assert.ok(await card.getByText('Hubtask cannot show this kind of file here.').isVisible());
  assert.equal((await card.locator('.quiet').textContent()).replace(/\s+/g, ' ').trim(), 'File · 2 kB');
  assert.ok(await card.getByRole('button', { name: 'Download' }).isVisible());
  // Every file of the board stands at the foot, the picture among them.
  assert.deepEqual((await viewer.locator('[data-viewer-strip] .tile-name').allInnerTexts()).map((each) => each.trim()), ['site.png', 'survey.dwg']);
  assert.deepEqual(failures, []);
});

test('chromium: the workspace\'s Files section tests a document server, says a private address\'s reason, and saves the choice (UC-MED-06 check 1)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  const checks = [];
  const saves = [];
  let refuse = false;
  let stored = { kind: 'NONE' };
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path === '/api/v1/media:document-server-check' && request.method() === 'POST') {
      checks.push(request.postDataJSON());
      if (refuse) return problem(route, 422, 'validation', 'media.document_server_private_network');
      return route.fulfill({ json: { reached: true } });
    }
    if (path === '/api/v1/media:document-server') {
      if (request.method() === 'PUT') {
        saves.push(request.postDataJSON());
        stored = request.postDataJSON();
      }
      return route.fulfill({ json: stored });
    }
    return stub(route);
  });
  await page.goto(`${served.origin}/administration/workspace/files`);
  const kind = page.getByLabel('Document server', { exact: true });
  await kind.waitFor({ timeout: 15_000 });
  await kind.selectOption('COLLABORA_ONLINE');
  const address = page.getByLabel('Address', { exact: true });
  await address.fill('https://office.example');
  const reach = page.locator('[data-document-server-test]');
  await reach.getByRole('button', { name: 'Test the connection' }).click();
  await reach.getByText('The server answered.').waitFor({ timeout: 5_000 });
  assert.deepEqual(checks, [{ kind: 'COLLABORA_ONLINE', url: 'https://office.example' }]);
  assert.deepEqual(saves, [], 'testing the connection stored the choice');

  refuse = true;
  await reach.getByRole('button', { name: 'Test the connection' }).click();
  await reach.getByText('This address lies inside a private network, and this installation does not allow a document server there.', { exact: false }).waitFor({ timeout: 5_000 });
  assert.equal(await reach.getByText('The server answered.').count(), 0);

  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await page.getByText('Document server saved.').first().waitFor({ timeout: 5_000 });
  assert.deepEqual(saves, [{ kind: 'COLLABORA_ONLINE', url: 'https://office.example' }]);
  assert.deepEqual(failures, []);
});
