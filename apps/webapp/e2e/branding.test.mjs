// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The workspace's own logo and header choice, on *Administration → Workspace → Name and look*
// (UC-INS-22 checks 1, 2, 3, 4, 10 and 11, the screen's half; the server's half is proven by the
// integration and contract tests the use case names). Whoever may rename the workspace uploads a
// logo for light, one for dark and a square symbol through the three media steps and sees them in
// the preview of both surfaces; chooses what the header shows and saves it with the section; a
// refused file is said at its picture; removing the last picture under "the logo alone" returns the
// header to the product's name; a lock from the installation or a plan leaves no control and names
// who set it; somebody who may not rename the workspace is not shown the group.
// Against a stubbed API, Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';
import { deflateSync } from 'node:zlib';

import { sharedChromium } from './browser.mjs';

import { ACCOUNT, MANIFEST, PERMISSIONS, SIGN_IN_POLICY, WORKSPACE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const CRC = Array.from({ length: 256 }, (_, n) => {
  let c = n;
  for (let k = 0; k < 8; k += 1) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
  return c >>> 0;
});
const crc = (bytes) => {
  let c = 0xffffffff;
  for (const byte of bytes) c = CRC[(c ^ byte) & 0xff] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
};
function chunk(type, data) {
  const length = Buffer.alloc(4);
  length.writeUInt32BE(data.length);
  const body = Buffer.concat([Buffer.from(type), data]);
  const sum = Buffer.alloc(4);
  sum.writeUInt32BE(crc(body));
  return Buffer.concat([length, body, sum]);
}
/** A PNG of one colour (RGBA), as an uploaded logo stands in for here. */
function png(width, height, rgba) {
  const head = Buffer.alloc(13);
  head.writeUInt32BE(width, 0);
  head.writeUInt32BE(height, 4);
  head[8] = 8;
  head[9] = 6;
  const row = Buffer.concat([Buffer.from([0]), Buffer.from(Array.from({ length: width }, () => rgba).flat())]);
  const raw = Buffer.concat(Array.from({ length: height }, () => row));
  return Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk('IHDR', head), chunk('IDAT', deflateSync(raw)), chunk('IEND', Buffer.alloc(0))]);
}

/** A dark wordmark: reads on light, hard to make out on dark. */
const NAVY = png(60, 12, [20, 50, 74, 255]);
/** A light wordmark: reads on dark. */
const PALE = png(60, 12, [242, 245, 250, 255]);
/** A mid-tone symbol: reads on both. */
const TEAL = png(16, 16, [11, 143, 125, 255]);

const IDS = ['01a0e2e0-0000-7000-8000-00000000b001', '01a0e2e0-0000-7000-8000-00000000b002', '01a0e2e0-0000-7000-8000-00000000b003', '01a0e2e0-0000-7000-8000-00000000b004'];
const NONE = { header: 'PRODUCT', logo_light_media_id: null, logo_dark_media_id: null, symbol_media_id: null, allowed: true };
const FIELD = { LOGO_LIGHT: 'logo_light_media_id', LOGO_DARK: 'logo_dark_media_id', SYMBOL: 'symbol_media_id' };
const OPEN = { value: true, installation: true, lock: null, source: 'DEFAULT', installation_source: 'DEFAULT' };

/**
 * A signed-in reader of the workspace whose branding the stub keeps: the three media steps, the
 * three branding writes, every request the walk asserts recorded in `sent`.
 */
async function open(browser, { width = 1280, height = 900, held = NONE, own = OPEN, permissions = PERMISSIONS.OWNER, refuse = {}, locale } = {}) {
  const context = await browser.newContext({ viewport: { width, height } });
  const state = { branding: { ...held }, bytes: {}, next: 0 };
  const sent = [];
  const later = new Date(Date.now() + 5 * 60_000).toISOString();
  const record = (id, size) => ({ id, usage: 'WORKSPACE_LOGO', content_type: 'image/png', size, ref_count: 0, created_by: ACCOUNT.id, created_at: '2026-11-23T10:00:00Z' });
  const content = (id) => `${served.origin}/api/v1/media/${id}:content`;
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const method = request.method();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: permissions } });
    if (path === '/accounts/me' && method === 'GET' && locale) return route.fulfill({ json: { ...ACCOUNT, locale } });
    if (path === '/tenant' && method === 'GET') {
      return route.fulfill({ json: { ...WORKSPACE, branding: state.branding, sign_in_policy: { ...SIGN_IN_POLICY, own_branding: own } } });
    }
    if (path === '/media' && method === 'POST') {
      const id = IDS[state.next];
      state.next += 1;
      const body = request.postDataJSON();
      sent.push(['POST /media', body.usage]);
      return route.fulfill({ status: 201, json: { ...record(id, body.size), status: 'PENDING', upload: { url: `${content(id)}?token=up`, method: 'PUT', expires_at: later } } });
    }
    const bytes = /^\/media\/([^/:]+):content$/.exec(path);
    if (bytes && method === 'PUT') {
      state.bytes[bytes[1]] = request.postDataBuffer();
      sent.push(['PUT bytes']);
      return route.fulfill({ status: 204 });
    }
    if (bytes) return route.fulfill({ status: 200, contentType: 'image/png', body: state.bytes[bytes[1]] ?? Buffer.alloc(0) });
    const confirm = /^\/media\/([^/:]+):confirm$/.exec(path);
    if (confirm) {
      sent.push(['confirm']);
      return route.fulfill({ json: { ...record(confirm[1], state.bytes[confirm[1]]?.length ?? 0), status: 'READY' } });
    }
    const one = /^\/media\/([^/:]+)$/.exec(path);
    if (one) {
      return route.fulfill({ json: { ...record(one[1], state.bytes[one[1]]?.length ?? 0), status: 'READY', download: { url: `${content(one[1])}?token=down`, method: 'GET', expires_at: later } } });
    }
    const picture = /^\/tenant\/branding\/(LOGO_LIGHT|LOGO_DARK|SYMBOL)$/.exec(path);
    if (picture) {
      const body = method === 'PUT' ? request.postDataJSON() : undefined;
      sent.push([`${method} /tenant/branding/${picture[1]}`, body]);
      const refusal = refuse[picture[1]];
      if (refusal) return route.fulfill({ status: 422, contentType: 'application/problem+json', body: JSON.stringify({ status: 422, code: refusal }) });
      state.branding = { ...state.branding, [FIELD[picture[1]]]: body?.media_id ?? null };
      const left = Object.values(FIELD).some((field) => state.branding[field]);
      if (!left && state.branding.header === 'LOGO') state.branding = { ...state.branding, header: 'PRODUCT' };
      return route.fulfill({ json: state.branding });
    }
    if (path === '/tenant/branding' && method === 'PUT') {
      const body = request.postDataJSON();
      sent.push(['PUT /tenant/branding', body]);
      state.branding = { ...state.branding, header: body.header };
      return route.fulfill({ json: state.branding });
    }
    if (path === '/tenant' && method === 'PATCH') sent.push(['PATCH /tenant', request.postDataJSON()]);
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/administration/workspace/name`);
  return { page, sent, state, failures, close: () => context.close() };
}

const group = (page) => page.locator('[data-branding]');
const row = (page, picture) => page.locator(`[data-branding-picture="${picture}"]`);
const preview = (page, surface) => page.locator(`[data-preview="${surface}"]`).getByRole('img');

/** Chooses a file for a picture's row, through its own file input. */
async function upload(page, picture, file) {
  await row(page, picture).locator('input[type="file"]').setInputFiles(file);
}

// UC-INS-22 checks 1, 3 and 4: the three pictures through the media steps, each shown on its
// surface in the preview; the choice saved with the section.
test('chromium: 1280 px — the three pictures are uploaded, previewed on both surfaces, and the choice is saved', async (t) => {
  const browser = await sharedChromium(t);
  const { page, sent, failures, close } = await open(browser);
  t.after(close);

  await group(page).getByText('Logo and header').waitFor({ timeout: 15_000 });
  // Nothing uploaded: both bars show the product, and the logo alone is not offered, with why.
  assert.equal(await preview(page, 'light').getAttribute('aria-label'), 'Hubtask');
  assert.equal(await preview(page, 'dark').getAttribute('aria-label'), 'Hubtask');
  const choice = page.getByRole('group', { name: 'What the header shows' });
  assert.equal(await choice.getByRole('radio', { name: 'Logo only' }).isDisabled(), true);
  assert.match(await choice.innerText(), /Upload a logo or a symbol first\./);
  assert.match(await row(page, 'LOGO_LIGHT').innerText(), /PNG, JPEG, GIF or WebP, at most 5 MB/);

  await upload(page, 'LOGO_LIGHT', { name: 'nordlicht.png', mimeType: 'image/png', buffer: NAVY });
  await page.getByText('The logo for light surfaces is set.').first().waitFor({ timeout: 10_000 });
  assert.deepEqual(sent, [
    ['POST /media', 'WORKSPACE_LOGO'],
    ['PUT bytes'],
    ['confirm'],
    ['PUT /tenant/branding/LOGO_LIGHT', { media_id: IDS[0] }],
  ], 'the logo took another way');
  // Its picture on the light surface, judged; the dark row says the light logo stands in, and that
  // it is hard to make out there - a warning, nothing blocked.
  await row(page, 'LOGO_LIGHT').locator('img').first().waitFor({ timeout: 10_000 });
  await row(page, 'LOGO_LIGHT').getByText('Easy to make out on a light surface.').waitFor({ timeout: 10_000 });
  await row(page, 'LOGO_DARK').getByText('The logo for light surfaces applies.', { exact: true }).waitFor();
  await row(page, 'LOGO_DARK').getByRole('status').getByText('The logo is hard to make out on a dark surface.', { exact: false }).waitFor({ timeout: 10_000 });

  await upload(page, 'LOGO_DARK', { name: 'nordlicht-dark.png', mimeType: 'image/png', buffer: PALE });
  await page.getByText('The logo for dark surfaces is set.').first().waitFor({ timeout: 10_000 });
  await row(page, 'LOGO_DARK').getByText('Easy to make out on a dark surface.').waitFor({ timeout: 10_000 });
  await upload(page, 'SYMBOL', { name: 'symbol.png', mimeType: 'image/png', buffer: TEAL });
  await page.getByText('The symbol is set.').first().waitFor({ timeout: 10_000 });
  await row(page, 'SYMBOL').getByText('Easy to make out on light and on dark.').waitFor({ timeout: 10_000 });
  assert.deepEqual(sent.filter(([what]) => what.startsWith('PUT /tenant')), [
    ['PUT /tenant/branding/LOGO_LIGHT', { media_id: IDS[0] }],
    ['PUT /tenant/branding/LOGO_DARK', { media_id: IDS[1] }],
    ['PUT /tenant/branding/SYMBOL', { media_id: IDS[2] }],
  ]);
  // Two thumbnails for the symbol, one per surface; each row now offers Replace and Remove.
  assert.equal(await row(page, 'SYMBOL').locator('img').count(), 2);
  assert.equal(await row(page, 'LOGO_DARK').getByRole('button', { name: 'Replace' }).count(), 1);

  // The workspace's name: the symbol and the name on both bars, the row marked changed, nothing sent.
  await choice.getByRole('radio', { name: 'Workspace name' }).check();
  assert.equal(await preview(page, 'light').getAttribute('aria-label'), WORKSPACE.display_name);
  await page.locator('[data-preview="dark"] img').waitFor();
  assert.equal(sent.some(([what]) => what === 'PUT /tenant/branding'), false, 'the choice left before Save');
  // The logo alone: each surface its own logo.
  await choice.getByRole('radio', { name: 'Logo only' }).check();
  assert.equal(await preview(page, 'light').getAttribute('aria-label'), `The logo of ${WORKSPACE.display_name}`);
  const lightSource = await page.locator('[data-preview="light"] img').getAttribute('src');
  const darkSource = await page.locator('[data-preview="dark"] img').getAttribute('src');
  assert.notEqual(lightSource, darkSource, 'both surfaces draw the same logo');

  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await page.getByText('Saved.').first().waitFor({ timeout: 10_000 });
  assert.deepEqual(sent.at(-1), ['PUT /tenant/branding', { header: 'LOGO' }]);
  assert.equal(sent.some(([what]) => what === 'PATCH /tenant'), false, 'the name was written though it did not move');
  // Stored now: the chip says it is this workspace's own.
  await page.locator('[data-branding-header] [data-source="WORKSPACE"]').waitFor({ timeout: 10_000 });
  assert.deepEqual(failures, []);
});

// UC-INS-22 check 2: a file the browser names as an SVG, or one past 5 MB, is refused at its row
// before a byte leaves; the server's refusal is said at the picture it concerns, the focus on its upload.
test('chromium: 1280 px — a refused file is said at its picture, and the pictures stay as they were', async (t) => {
  const browser = await sharedChromium(t);
  const { page, sent, failures, close } = await open(browser, { refuse: { SYMBOL: 'media.workspace_logo_not_an_image' } });
  t.after(close);
  await group(page).getByText('Logo and header').waitFor({ timeout: 15_000 });

  await upload(page, 'LOGO_LIGHT', { name: 'nordlicht.svg', mimeType: 'image/svg+xml', buffer: Buffer.from('<svg xmlns="http://www.w3.org/2000/svg"/>') });
  await row(page, 'LOGO_LIGHT').getByText('This is not a PNG, JPEG, GIF or WebP picture. Hubtask does not take SVG files; save the logo as a PNG.').waitFor({ timeout: 5_000 });
  assert.ok(await row(page, 'LOGO_LIGHT').locator('[data-invalid]').count() > 0, 'the field is not marked');
  assert.equal(await page.evaluate(() => document.activeElement?.textContent?.trim()), 'Choose logo');

  await upload(page, 'LOGO_DARK', { name: 'druck.png', mimeType: 'image/png', buffer: Buffer.alloc(5_000_001) });
  await row(page, 'LOGO_DARK').getByText('The file is too large. A logo may be at most 5 MB; make the picture smaller and choose it again.').waitFor({ timeout: 5_000 });
  assert.match(await row(page, 'LOGO_DARK').innerText(), /druck\.png\s+5 MB of 5 MB/);
  assert.deepEqual(sent, [], 'a refused file left the browser');

  // The server judges the bytes: its words, at the symbol's row.
  await upload(page, 'SYMBOL', { name: 'symbol.png', mimeType: 'image/png', buffer: TEAL });
  await row(page, 'SYMBOL').getByText("A workspace's logo or symbol has to be a PNG, JPEG, GIF or WebP image.").waitFor({ timeout: 10_000 });
  assert.equal(await page.evaluate(() => document.activeElement?.textContent?.trim()), 'Choose symbol');
  assert.equal(await page.getByText('The symbol is set.').count(), 0, 'said set before the server answered');
  // Nothing set: the preview still shows the product.
  assert.equal(await preview(page, 'light').getAttribute('aria-label'), 'Hubtask');
  assert.deepEqual(failures, []);
});

// UC-INS-22 check 4: removing the last picture under the logo alone returns the header to the
// product's name, the screen says so, and the focus is back on that row's upload.
test('chromium: 1280 px — removing the last picture returns the header to the product name', async (t) => {
  const browser = await sharedChromium(t);
  const { page, sent, failures, close, state } = await open(browser, { held: { ...NONE, header: 'LOGO', symbol_media_id: IDS[3] } });
  state.bytes[IDS[3]] = TEAL;
  t.after(close);
  await group(page).getByText('Logo and header').waitFor({ timeout: 15_000 });
  const choice = page.getByRole('group', { name: 'What the header shows' });
  assert.equal(await choice.getByRole('radio', { name: 'Logo only' }).isChecked(), true);
  // The symbol alone stands in for the logo on both bars.
  await page.locator('[data-preview="dark"] img').waitFor({ timeout: 10_000 });

  await row(page, 'SYMBOL').getByRole('button', { name: 'Remove' }).click();
  await page.getByText('Removed. Without a picture, the header shows the product\'s name again.').first().waitFor({ timeout: 10_000 });
  assert.deepEqual(sent, [['DELETE /tenant/branding/SYMBOL', undefined]]);
  await choice.getByRole('radio', { name: 'Product name' }).and(page.locator(':checked')).waitFor({ timeout: 10_000 });
  assert.equal(await choice.getByRole('radio', { name: 'Logo only' }).isDisabled(), true);
  assert.equal(await preview(page, 'light').getAttribute('aria-label'), 'Hubtask');
  assert.equal(await page.evaluate(() => document.activeElement?.textContent?.trim()), 'Choose symbol');
  assert.deepEqual(failures, []);
});

// UC-INS-22 checks 10 and 11: a lock from above leaves no control and names who set it.
for (const [lock, words] of [['INSTANCE', 'Set by the installation for every workspace.'], ['PLAN', 'Set by your plan.']]) {
  test(`chromium: 1280 px — switched off by the ${lock === 'INSTANCE' ? 'installation' : 'plan'}, the group names it and offers nothing`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, failures, close } = await open(browser, {
      held: { ...NONE, allowed: false },
      own: { value: false, installation: false, lock, source: lock, installation_source: lock },
    });
    t.after(close);
    const locked = page.locator('[data-branding-locked]');
    await locked.waitFor({ timeout: 15_000 });
    const said = await locked.innerText();
    assert.match(said, /Own logo and name/);
    assert.ok(said.includes(words), said);
    assert.match(said, /Not allowed/);
    assert.equal(await locked.locator('[data-source]').getAttribute('data-source'), `${lock}-locked`);
    assert.equal(await group(page).locator('input').count(), 0, 'a control is offered under a lock');
    assert.equal(await group(page).getByRole('button', { name: /Choose|Replace|Remove/ }).count(), 0);
    assert.deepEqual(failures, []);
  });
}

test('chromium: 1280 px — somebody who may not rename the workspace is not shown the group', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, close } = await open(browser, { permissions: PERMISSIONS.AUDITOR });
  t.after(close);
  await page.getByRole('heading', { name: 'Name and look' }).waitFor({ timeout: 15_000 });
  await page.getByText('Name', { exact: true }).first().waitFor();
  assert.equal(await group(page).count(), 0);
  assert.equal(await page.locator('input[type="file"]').count(), 0);
  assert.deepEqual(failures, []);
});

test('chromium: 390 px — the preview stacks its two surfaces, and every picture keeps its row', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, close, state } = await open(browser, { width: 390, height: 844, held: { ...NONE, logo_light_media_id: IDS[3] } });
  state.bytes[IDS[3]] = NAVY;
  t.after(close);
  await group(page).getByText('Logo and header').waitFor({ timeout: 15_000 });
  const light = await page.locator('[data-preview="light"]').boundingBox();
  const dark = await page.locator('[data-preview="dark"]').boundingBox();
  assert.ok(light && dark && dark.y >= light.y + light.height, 'the surfaces stand side by side');
  for (const picture of ['LOGO_LIGHT', 'LOGO_DARK', 'SYMBOL']) assert.equal(await row(page, picture).count(), 1);
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.equal(overflow, 0, 'the page scrolls sideways');
  await row(page, 'LOGO_LIGHT').getByRole('button', { name: 'Replace' }).waitFor({ timeout: 10_000 });
  assert.deepEqual(failures, []);
});

// The keyboard reaches every row in reading order: the choice, then each picture's upload.
test('chromium: 1280 px — Tab reaches the choice and each picture in reading order', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, close } = await open(browser);
  t.after(close);
  await group(page).getByText('Logo and header').waitFor({ timeout: 15_000 });
  await page.getByRole('textbox', { name: 'Name' }).focus();
  const reached = [];
  for (let step = 0; step < 8; step += 1) {
    await page.keyboard.press('Tab');
    reached.push(await page.evaluate(() => {
      const at = document.activeElement;
      return at?.getAttribute('type') === 'radio' ? `radio:${at.labels?.[0]?.textContent?.trim()}` : (at?.textContent?.trim() ?? '');
    }));
  }
  const order = reached.filter((each) => /^radio:|^Choose/.test(each));
  assert.deepEqual(order.slice(0, 4), ['radio:Product name', 'Choose logo', 'Choose logo', 'Choose symbol'], reached.join(' | '));
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — in German, the group speaks du and names its limits', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, close } = await open(browser, { locale: 'de' });
  t.after(close);
  await group(page).getByText('Logo und Kopfzeile').waitFor({ timeout: 15_000 });
  const said = await group(page).innerText();
  for (const words of ['Vorschau der Kopfzeile', 'Was die Kopfzeile zeigt', 'Nur das Logo', 'Lade zuerst ein Logo oder ein Symbol hoch.', 'Logo für helle Oberflächen', 'PNG, JPEG, GIF oder WebP, höchstens 5 MB', 'Logo wählen', 'Quadratisches Symbol', 'Symbol wählen']) {
    assert.ok(said.includes(words), `${words} is missing`);
  }
  assert.deepEqual(failures, []);
});
