// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The workspace's own logo and header choice, on *Administration → Workspace → Name and look*
// (UC-INS-22 checks 1, 2, 3, 4, 10 and 11, the screen's half; the server's half is proven by the
// integration and contract tests the use case names), and where every member then sees it
// (UC-INS-22 checks 5, 8, 9 and 12): the header in light and dark, the tab, the about page, the
// sign-in card at the workspace's own address. Whoever may rename the workspace uploads a
// logo for light, one for dark and a square symbol through the three media steps and sees them in
// the preview of both surfaces; chooses what the header shows and saves it with the section; a
// refused file is said at its picture; removing the last picture under "the logo alone" returns the
// header to the product's name; a lock from the installation or a plan leaves no control and names
// who set it; switched off by the workspace itself, it points to *Sign-in → Own logo and name*,
// whose switch turns it on again; somebody who may not rename the workspace is not shown the group.
// Against a stubbed API, Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';
import { deflateSync } from 'node:zlib';

import { sharedChromium } from './browser.mjs';

import { ACCOUNT, MANIFEST, PERMISSIONS, SIGN_IN_POLICY, WORKSPACE, lookOf, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

// The policy the binary sends on every answer (security.md §9): a picture is drawn under it.
const POLICY =
  "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; font-src 'self'; " +
  "connect-src 'self'; manifest-src 'self'; worker-src 'self' blob:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'";

const served = await serve(DIST, { headers: { 'content-security-policy': POLICY } });
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
async function open(browser, {
  width = 1280, height = 900, held = NONE, own = OPEN, permissions = PERMISSIONS.OWNER, refuse = {}, locale,
  path: at = '/administration/workspace/name', colorScheme = 'light', name = WORKSPACE.display_name, bytes = {}, broken = [],
  blocked = [], slow = [], slowConfirm = false, isTouch = false,
} = {}) {
  const context = await browser.newContext({ viewport: { width, height }, colorScheme, ...(isTouch ? { hasTouch: true, isMobile: true } : {}) });
  const state = { branding: { ...held }, bytes: { ...bytes }, next: 0, contents: [] };
  const sent = [];
  // The bytes of a `slow` picture wait until the walk lets them go.
  let release = () => {};
  const gate = new Promise((resolve) => (release = resolve));
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
      return route.fulfill({ json: { ...WORKSPACE, display_name: name, branding: state.branding, sign_in_policy: { ...SIGN_IN_POLICY, own_branding: own } } });
    }
    if (path === '/tenant/look' && method === 'GET') return route.fulfill({ json: lookOf({ display_name: name, branding: state.branding }) });
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
    if (bytes) {
      state.contents.push(bytes[1]);
      // A file storage on another address without a rule letting this page read it: no answer.
      if (blocked.includes(bytes[1])) return route.abort('failed');
      if (slow.includes(bytes[1])) await gate;
      // The object store refusing a picture: the mark falls back past it.
      if (broken.includes(bytes[1])) {
        return route.fulfill({ status: 404, contentType: 'application/problem+json', body: JSON.stringify({ status: 404, code: 'media.not_found' }) });
      }
      return route.fulfill({ status: 200, contentType: 'image/png', body: state.bytes[bytes[1]] ?? Buffer.alloc(0) });
    }
    const confirm = /^\/media\/([^/:]+):confirm$/.exec(path);
    if (confirm) {
      sent.push(['confirm']);
      if (slowConfirm) await gate;
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
    if (path === '/tenant' && method === 'PATCH') {
      const body = request.postDataJSON();
      sent.push(['PATCH /tenant', body]);
      // The workspace's own level of the switch: what the server answers is the rule in force.
      const switched = body.sign_in_policy?.own_branding;
      if (typeof switched === 'boolean') {
        own = { ...own, value: switched, source: 'WORKSPACE' };
        state.branding = { ...state.branding, allowed: switched };
        return route.fulfill({ json: { ...WORKSPACE, display_name: name, branding: state.branding, sign_in_policy: { ...SIGN_IN_POLICY, own_branding: own } } });
      }
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
  await page.goto(`${served.origin}${at}`);
  return { page, sent, state, failures, release, close: () => context.close() };
}

/** The bar's home link by its one name: the product's or the workspace's. */
const homeLink = (page, name) => page.getByRole('banner').getByRole('link', { name, exact: true });
/** The header's home link, for what it holds. */
const home = (page) => page.locator('header a.home');
/** What of the mark is drawn now under the document's theme. */
const drawnImages = (page) => home(page).locator('img:visible');

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

  await group(page).getByText('Logo and app bar').waitFor({ timeout: 15_000 });
  // Nothing uploaded: both bars show the product, and the logo alone is not offered, with why.
  assert.equal(await preview(page, 'light').getAttribute('aria-label'), 'Hubtask');
  assert.equal(await preview(page, 'dark').getAttribute('aria-label'), 'Hubtask');
  const choice = page.getByRole('group', { name: 'What the app bar shows' });
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
  await group(page).getByText('Logo and app bar').waitFor({ timeout: 15_000 });

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
  const { page, sent, failures, close } = await open(browser, { held: { ...NONE, header: 'LOGO', symbol_media_id: IDS[3] }, bytes: { [IDS[3]]: TEAL } });
  t.after(close);
  await group(page).getByText('Logo and app bar').waitFor({ timeout: 15_000 });
  const choice = page.getByRole('group', { name: 'What the app bar shows' });
  assert.equal(await choice.getByRole('radio', { name: 'Logo only' }).isChecked(), true);
  // The symbol alone stands in for the logo on both bars, and in the header itself, named by the workspace.
  await page.locator('[data-preview="dark"] img').waitFor({ timeout: 10_000 });
  await drawnImages(page).first().waitFor({ timeout: 10_000 });
  await homeLink(page, WORKSPACE.display_name).waitFor();

  await row(page, 'SYMBOL').getByRole('button', { name: 'Remove' }).click();
  await page.getByText('Removed. Without a picture, the app bar shows the product\'s name again.').first().waitFor({ timeout: 10_000 });
  assert.deepEqual(sent, [['DELETE /tenant/branding/SYMBOL', undefined]]);
  await choice.getByRole('radio', { name: 'Product name' }).and(page.locator(':checked')).waitFor({ timeout: 10_000 });
  assert.equal(await choice.getByRole('radio', { name: 'Logo only' }).isDisabled(), true);
  assert.equal(await preview(page, 'light').getAttribute('aria-label'), 'Hubtask');
  assert.equal(await page.evaluate(() => document.activeElement?.textContent?.trim()), 'Choose symbol');
  // UC-INS-22 checks 4 and 5: the header itself shows the product's mark again, once the workspace is read again.
  await homeLink(page, 'Hubtask').waitFor({ timeout: 10_000 });
  assert.equal(await home(page).locator('img').count(), 0, 'a picture is still in the header');
  assert.deepEqual(failures, []);
});

// UC-INS-22 check 1: a stored picture is the workspace's whatever became of its bytes. One the file
// storage refuses and one it gives the browser no answer for keep their box, *Replace* and *Remove*,
// and say why they cannot be shown; the preview falls back past them by G8 and says so.
test('chromium: 1280 px — a stored picture that cannot be read keeps Replace and Remove, says why, and the preview falls back', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, close } = await open(browser, {
    held: { ...NONE, header: 'LOGO', logo_light_media_id: IDS[0], logo_dark_media_id: IDS[1], symbol_media_id: IDS[2] },
    bytes: { [IDS[0]]: NAVY, [IDS[1]]: PALE, [IDS[2]]: TEAL }, broken: [IDS[0]], blocked: [IDS[1]],
  });
  t.after(close);
  await row(page, 'LOGO_LIGHT').getByText('the file storage does not hand it out').waitFor({ timeout: 15_000 });
  await row(page, 'LOGO_DARK').getByText('the file storage does not let the browser read it').waitFor({ timeout: 15_000 });
  for (const picture of ['LOGO_LIGHT', 'LOGO_DARK']) {
    const said = await row(page, picture).innerText();
    assert.match(said, /This picture is stored, but it cannot be shown here/);
    assert.equal(await row(page, picture).getByRole('button', { name: 'Replace' }).count(), 1, `${picture} offers no Replace`);
    assert.equal(await row(page, picture).getByRole('button', { name: 'Remove' }).count(), 1, `${picture} offers no Remove`);
    assert.doesNotMatch(said, /Choose logo|drop it here/, `${picture} reads as empty`);
  }
  const choice = page.getByRole('group', { name: 'What the app bar shows' });
  assert.equal(await choice.getByRole('radio', { name: 'Logo only' }).isChecked(), true);
  // Both bars fall back past the two logos to the symbol, the one that can be drawn, and say so.
  for (const surface of ['light', 'dark']) {
    const drawn = page.locator(`[data-preview="${surface}"] img`);
    await drawn.waitFor({ timeout: 10_000 });
    assert.equal(await drawn.evaluate((image) => image.naturalWidth), 16, `the ${surface} preview draws no symbol`);
  }
  await page.getByText('A stored picture that cannot be shown here is left out of the preview').waitFor();
  assert.deepEqual(failures, []);
});

// UC-INS-22 check 1: while a stored picture's bytes are on their way, its row already holds its box,
// *Replace* and *Remove*, and says it is loading; the picture appears when they arrive.
test('chromium: 1280 px — a stored picture being read keeps its box, Replace and Remove until it appears', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, release, close } = await open(browser, {
    held: { ...NONE, header: 'WORKSPACE_NAME', symbol_media_id: IDS[2] }, bytes: { [IDS[2]]: TEAL }, slow: [IDS[2]],
  });
  t.after(close);
  const symbol = row(page, 'SYMBOL');
  await symbol.getByText('Loading the stored picture…').waitFor({ timeout: 15_000 });
  assert.equal(await symbol.getByRole('button', { name: 'Replace' }).count(), 1);
  assert.equal(await symbol.getByRole('button', { name: 'Remove' }).count(), 1);
  assert.doesNotMatch(await symbol.innerText(), /Choose symbol|drop it here/);
  assert.equal(await symbol.locator('img').count(), 0);

  release();
  await symbol.locator('img').first().waitFor({ timeout: 10_000 });
  assert.equal(await symbol.getByText('Loading the stored picture…').count(), 0);
  assert.equal(await page.locator('[data-branding-fallback]').count(), 0, 'a picture that arrived is called unreadable');
  assert.deepEqual(failures, []);
});

// UC-INS-22 check 1: once the bytes are sent, only the confirmation and the setting remain, which
// an abort no longer reaches; *Cancel* is no longer offered then, and the row says why.
test('chromium: 1280 px — once the bytes are sent, Cancel gives way to what is still happening', async (t) => {
  const browser = await sharedChromium(t);
  const { page, sent, failures, release, close } = await open(browser, { slowConfirm: true });
  t.after(close);
  await group(page).getByText('Logo and app bar').waitFor({ timeout: 15_000 });
  await upload(page, 'SYMBOL', { name: 'mark.png', mimeType: 'image/png', buffer: TEAL });
  await row(page, 'SYMBOL').getByText('Uploaded; setting the picture. This can no longer be cancelled.').waitFor({ timeout: 10_000 });
  assert.deepEqual(sent, [['POST /media', 'WORKSPACE_LOGO'], ['PUT bytes'], ['confirm']]);
  assert.equal(await row(page, 'SYMBOL').getByRole('button', { name: 'Cancel' }).count(), 0, 'Cancel is offered after the bytes are sent');

  release();
  await page.getByText('The symbol is set.').first().waitFor({ timeout: 10_000 });
  assert.deepEqual(sent.at(-1), ['PUT /tenant/branding/SYMBOL', { media_id: IDS[0] }]);
  assert.deepEqual(failures, []);
});

// UC-INS-22 checks 10 and 11: a lock from above leaves no control and names who set it - once, by
// the chip, as *Private hubs allowed* says it.
for (const [lock, words, level] of [['INSTANCE', 'Locked by the installation', 'installation'], ['PLAN', 'Locked by your plan', 'plan']]) {
  test(`chromium: 1280 px — switched off by the ${level}, the group names it once and offers nothing`, async (t) => {
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
    assert.equal(said.split(level).length - 1, 1, `the level is said more than once: ${said}`);
    assert.match(said, /Not allowed/);
    assert.equal(await locked.getByRole('link').count(), 0, 'a lock from above points to a switch');
    assert.equal(await locked.locator('[data-source]').getAttribute('data-source'), `${lock}-locked`);
    assert.equal(await group(page).locator('input').count(), 0, 'a control is offered under a lock');
    assert.equal(await group(page).getByRole('button', { name: /Choose|Replace|Remove/ }).count(), 0);
    assert.deepEqual(failures, []);
  });
}

// Switched off at the workspace's own level (through the API, hubctl or MCP): the group names this
// workspace and points to the one row that switches it on again, *Sign-in → Own logo and name*, a
// switch drawn as *Private hubs allowed* is (ADR-0112). Pressed there, it is on at once.
for (const [width, height] of [[1280, 900], [390, 844]]) {
  test(`chromium: ${width} px — switched off in this workspace, the group points to Sign-in, where the switch turns it on again`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, sent, failures, close } = await open(browser, {
      width, height,
      held: { ...NONE, allowed: false },
      own: { value: false, installation: true, lock: null, source: 'WORKSPACE', installation_source: 'DEFAULT' },
    });
    t.after(close);
    const locked = page.locator('[data-branding-locked="WORKSPACE"]');
    await locked.waitFor({ timeout: 15_000 });
    const said = await locked.innerText();
    assert.match(said, /Set here/, said);
    assert.equal(await locked.locator('[data-source]').getAttribute('data-source'), 'WORKSPACE');
    await locked.getByRole('link', { name: 'Switch it on under Sign-in → Own logo and name' }).click();
    await page.waitForURL('**/administration/sign-in/own-branding', { timeout: 10_000 });

    const toggle = page.getByRole('switch', { name: 'This workspace may show its own logo and name' });
    await toggle.waitFor({ timeout: 10_000 });
    assert.equal(await toggle.isChecked(), false);
    assert.equal(await toggle.isDisabled(), false, 'the workspace\'s own off cannot be switched on');
    await toggle.click();
    await page.getByText('Saved: whether this workspace shows its own logo and name.').first().waitFor({ timeout: 10_000 });
    assert.deepEqual(sent.filter(([what]) => what === 'PATCH /tenant'), [['PATCH /tenant', { sign_in_policy: { own_branding: true } }]]);
    assert.equal(await toggle.isChecked(), true);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    assert.equal(overflow, 0, 'the page scrolls sideways');
    assert.deepEqual(failures, []);
  });
}

// The same switch, locked by the installation, stays on the screen off and switched off, as
// *Private hubs allowed* does under a lock.
test('chromium: 1280 px — Sign-in → Own logo and name under the installation\'s lock: off, not offered, the chip names it', async (t) => {
  const browser = await sharedChromium(t);
  const { page, sent, failures, close } = await open(browser, {
    path: '/administration/sign-in/own-branding',
    held: { ...NONE, allowed: false },
    own: { value: false, installation: false, lock: 'INSTANCE', source: 'INSTANCE', installation_source: 'INSTANCE' },
  });
  t.after(close);
  const row = page.locator('[data-policy-switch="own_branding"]');
  await row.waitFor({ timeout: 15_000 });
  const toggle = row.getByRole('switch', { name: 'This workspace may show its own logo and name' });
  assert.equal(await toggle.isDisabled(), true, 'a lock from above is offered');
  assert.equal(await row.locator('[data-source]').getAttribute('data-source'), 'INSTANCE-locked');
  assert.equal(sent.filter(([what]) => what === 'PATCH /tenant').length, 0);
  assert.deepEqual(failures, []);
});

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

// Nothing can be dropped where the pointer is a finger: `UploadField` offers the drop only where the
// pointer is fine, and the button, the one way that always exists, everywhere.
for (const isTouch of [false, true]) {
  test(`chromium: ${isTouch ? '390 px, touch' : '1280 px, mouse'} — an empty picture row ${isTouch ? 'offers no drop' : 'offers the drop beside its button'}`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, failures, close } = await open(browser, isTouch ? { width: 390, height: 844, isTouch } : {});
    t.after(close);
    const empty = row(page, 'SYMBOL');
    await empty.getByRole('button', { name: /Choose/ }).waitFor({ timeout: 15_000 });
    assert.equal(await empty.getByText('…or drop it here').isVisible(), !isTouch);
    assert.deepEqual(failures, []);
  });
}

test('chromium: 390 px — the preview stacks its two surfaces, and every picture keeps its row', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, close, state } = await open(browser, { width: 390, height: 844, held: { ...NONE, logo_light_media_id: IDS[3] } });
  state.bytes[IDS[3]] = NAVY;
  t.after(close);
  await group(page).getByText('Logo and app bar').waitFor({ timeout: 15_000 });
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
  await group(page).getByText('Logo and app bar').waitFor({ timeout: 15_000 });
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
  await group(page).getByText('Logo und Anwendungsleiste').waitFor({ timeout: 15_000 });
  const said = await group(page).innerText();
  for (const words of ['Vorschau der Anwendungsleiste', 'Was die Anwendungsleiste zeigt', 'Nur das Logo', 'Lade zuerst ein Logo oder ein Symbol hoch.', 'Logo für helle Oberflächen', 'PNG, JPEG, GIF oder WebP, höchstens 5 MB', 'Logo wählen', 'Quadratisches Symbol', 'Symbol wählen']) {
    assert.ok(said.includes(words), `${words} is missing`);
  }
  assert.deepEqual(failures, []);
});

// ---- Where every member sees it: the header, the tab, the about page, the sign-in card --------
// UC-INS-22 checks 5, 8, 9 and 12 (the web halves; the server's are the tests the use case names).

const LONG = 'Freiwillige Feuerwehr Unterlindenbach e. V.';
const LOGOS = { ...NONE, logo_light_media_id: IDS[0], logo_dark_media_id: IDS[1], symbol_media_id: IDS[2] };
const PICTURE_BYTES = { [IDS[0]]: NAVY, [IDS[1]]: PALE, [IDS[2]]: TEAL };

/** Where the controls beside the mark stand: the search's way in and the account. */
async function barPlaces(page) {
  const box = async (locator) => {
    const at = await locator.boundingBox();
    return at ? [Math.round(at.x), Math.round(at.y), Math.round(at.width)] : undefined;
  };
  return {
    search: await box(page.getByRole('banner').getByRole('combobox').first()),
    account: await box(page.getByRole('banner').getByRole('button', { name: ACCOUNT.display_name })),
  };
}

for (const colorScheme of ['light', 'dark']) {
  test(`chromium: 1280 px, ${colorScheme} — with the workspace's name chosen the header shows its symbol and name, and nothing beside it moves`, async (t) => {
    const browser = await sharedChromium(t);
    const before = await open(browser, { path: '/', colorScheme });
    t.after(before.close);
    await homeLink(before.page, 'Hubtask').waitFor({ timeout: 15_000 });
    await before.page.getByRole('banner').getByRole('button', { name: ACCOUNT.display_name }).waitFor();
    const product = await barPlaces(before.page);

    const { page, failures, close } = await open(browser, {
      path: '/', colorScheme, held: { ...LOGOS, header: 'WORKSPACE_NAME' }, bytes: PICTURE_BYTES,
    });
    t.after(close);
    const link = homeLink(page, WORKSPACE.display_name);
    await link.waitFor({ timeout: 15_000 });
    assert.equal(await page.evaluate(() => document.documentElement.dataset.theme), colorScheme);
    // The symbol beside the name, 24 px, drawn from the picture the member read as an avatar is.
    const symbol = drawnImages(page);
    await symbol.waitFor({ timeout: 10_000 });
    assert.match((await symbol.getAttribute('src')) ?? '', new RegExp(`/media/${IDS[2]}:content`));
    const size = await symbol.boundingBox();
    assert.ok(size && Math.round(size.width) === 24 && Math.round(size.height) === 24, JSON.stringify(size));
    assert.equal((await home(page).innerText()).trim(), WORKSPACE.display_name);
    assert.equal(await homeLink(page, 'Hubtask').count(), 0, 'the product is named beside the workspace');
    // The controls beside it stand where they stood with the product's mark.
    await page.getByRole('banner').getByRole('button', { name: ACCOUNT.display_name }).waitFor();
    assert.deepEqual(await barPlaces(page), product, 'the mark moved the bar');
    assert.deepEqual([...before.failures, ...failures], []);
  });
}

test('chromium: 1280 px — a long name is cut at its box, its whole name the tooltip', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, close } = await open(browser, { path: '/', name: LONG, held: { ...NONE, header: 'WORKSPACE_NAME' } });
  t.after(close);
  await homeLink(page, LONG).waitFor({ timeout: 15_000 });
  const word = home(page).getByText(LONG, { exact: true });
  const cut = await word.evaluate((element) => ({ shown: element.clientWidth, whole: element.scrollWidth, title: element.getAttribute('title') }));
  assert.ok(cut.shown <= 240 && cut.whole > cut.shown, JSON.stringify(cut));
  assert.equal(cut.title, LONG);
  // Two letters where there is no symbol: the first word's and the last's.
  assert.equal((await home(page).locator('[aria-hidden="true"]').first().innerText()).trim(), 'FV');
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — the logo alone: each theme its own logo, a switch of theme shows the other at once', async (t) => {
  const browser = await sharedChromium(t);
  const { page, state, failures, close } = await open(browser, { path: '/', held: { ...LOGOS, header: 'LOGO' }, bytes: PICTURE_BYTES });
  t.after(close);
  // The logo stands alone, so the link is named by the workspace and the picture is named nowhere else.
  await homeLink(page, WORKSPACE.display_name).waitFor({ timeout: 15_000 });
  const logo = drawnImages(page);
  await logo.waitFor({ timeout: 10_000 });
  assert.match((await logo.getAttribute('src')) ?? '', new RegExp(IDS[0]));
  assert.equal(await logo.getAttribute('alt'), '');
  await page.waitForFunction(() => [...document.querySelectorAll('header a.home img')].every((each) => each.complete && each.naturalWidth > 0));
  // Contained in its box: 32 px high and no wider than 192, never stretched (60 × 12 drawn 160 × 32).
  const box = await logo.boundingBox();
  assert.ok(box && Math.round(box.height) === 32 && box.width <= 192 && Math.round(box.width) === 160, JSON.stringify(box));
  const asked = state.contents.length;

  await page.emulateMedia({ colorScheme: 'dark' });
  await page.waitForFunction(() => document.documentElement.dataset.theme === 'dark');
  assert.match((await drawnImages(page).getAttribute('src')) ?? '', new RegExp(IDS[1]));
  assert.equal(state.contents.length, asked, 'the theme switch had to load the other logo');
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — a picture that fails to load falls back by the rule, never to a broken image', async (t) => {
  const browser = await sharedChromium(t);
  const held = { ...NONE, header: 'LOGO', logo_light_media_id: IDS[0], symbol_media_id: IDS[2] };
  // The light logo is refused by the store: the symbol stands in for it.
  const first = await open(browser, { path: '/', held, bytes: PICTURE_BYTES, broken: [IDS[0]] });
  t.after(first.close);
  await first.page.waitForFunction((id) => {
    const shown = [...document.querySelectorAll('header a.home img')].filter((each) => each.checkVisibility());
    return shown.length === 1 && shown[0].src.includes(id) && shown[0].naturalWidth > 0;
  }, IDS[2], { timeout: 15_000 });
  await homeLink(first.page, WORKSPACE.display_name).waitFor();

  // Both refused: the product's mark, and no image left in the header.
  const second = await open(browser, { path: '/', held, bytes: PICTURE_BYTES, broken: [IDS[0], IDS[2]] });
  t.after(second.close);
  await homeLink(second.page, 'Hubtask').waitFor({ timeout: 15_000 });
  await second.page.waitForFunction(() => document.querySelectorAll('header a.home img').length === 0);
  assert.deepEqual([...first.failures, ...second.failures], []);
});

test('chromium: 1280 px — switched off above, the header shows the product whatever is stored', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, close } = await open(browser, {
    path: '/', held: { ...LOGOS, header: 'WORKSPACE_NAME', allowed: false }, bytes: PICTURE_BYTES,
    own: { value: false, installation: false, lock: 'INSTANCE', source: 'INSTANCE', installation_source: 'INSTANCE' },
  });
  t.after(close);
  await homeLink(page, 'Hubtask').waitFor({ timeout: 15_000 });
  assert.equal(await home(page).locator('img').count(), 0);
  assert.deepEqual(failures, []);
});

// UC-INS-22 checks 9 and 12: the tab names the page and the workspace; the about page keeps the
// product's name and version under every choice, and the account group keeps its *About*.
for (const header of ['PRODUCT', 'WORKSPACE_NAME', 'LOGO']) {
  test(`chromium: 1280 px — with ${header}, the tab names the workspace and the about page says it runs on Hubtask`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, failures, close } = await open(browser, { path: '/installation', held: { ...LOGOS, header }, bytes: PICTURE_BYTES });
    t.after(close);
    const line = page.locator('[data-runs-with]');
    await line.waitFor({ timeout: 15_000 });
    await page.waitForFunction((name) => document.title === `This installation – ${name}`, WORKSPACE.display_name, { timeout: 10_000 });
    await page.waitForFunction((name) => document.querySelector('[data-runs-with]')?.textContent?.includes(name), WORKSPACE.display_name);
    const said = (await line.innerText()).replace(/\s+/g, ' ').trim();
    assert.match(said, new RegExp(`^${WORKSPACE.display_name} runs on hubtask · Version ${MANIFEST.product_version}$`, 'i'));
    // The name is the product's own, not a styling of it: a screen reader meets "Hubtask".
    assert.equal(await line.getByText('Hubtask', { exact: true }).count(), 1);
    await page.getByRole('banner').getByRole('button', { name: ACCOUNT.display_name }).click();
    await page.getByRole('menuitem', { name: 'About Hubtask' }).waitFor();
    assert.deepEqual(failures, []);
  });
}

test('chromium: 390 px — the phone bar keeps the page title and draws no mark', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, close } = await open(browser, { path: '/', width: 390, height: 844, held: { ...LOGOS, header: 'LOGO' }, bytes: PICTURE_BYTES });
  t.after(close);
  await page.waitForFunction((name) => document.title === `Today – ${name}`, WORKSPACE.display_name, { timeout: 15_000 });
  assert.equal(await page.locator('header img').count(), 0, 'the phone bar draws a logo');
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.equal(overflow, 0);
  assert.deepEqual(failures, []);
});

const RULES = {
  workspace_host: 'house.example', workspace_name: WORKSPACE.display_name, methods: ['PASSWORD'], providers: [], legal: {},
  password: {
    min_length: 12, min_lowercase: 0, min_uppercase: 0, min_digits: 0, min_symbols: 0, min_classes: 0, max_repeat: null,
    common_passwords: true, context_words: true, breach_check: false, history_count: 0, not_current: false,
  },
};

/** A visitor at the workspace's own address, signed out; the rules carry its look where `header` is given. */
async function visitor(browser, { header, pictures = {}, colorScheme = 'light', width = 1280, height = 860, broken = [] }) {
  const context = await browser.newContext({ viewport: { width, height }, colorScheme });
  const read = [];
  await context.route('**/api/v1/**', (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, features: { ...MANIFEST.features, sign_in_rules: true } } });
    if (path === '/auth/sign-in-rules') return route.fulfill({ json: { ...RULES, ...(header ? { workspace_header: header, workspace_pictures: pictures } : {}) } });
    const picture = /^\/auth\/workspace-pictures\/([^/]+)$/.exec(path);
    if (picture) {
      read.push(picture[1]);
      if (broken.includes(picture[1])) {
        return route.fulfill({ status: 404, contentType: 'application/problem+json', body: JSON.stringify({ type: 'about:blank', title: 'media.not_found', status: 404, code: 'media.not_found' }) });
      }
      return route.fulfill({ status: 200, contentType: 'image/png', headers: { 'cache-control': 'public, max-age=300', etag: `"${picture[1]}"` }, body: PICTURE_BYTES[picture[1]] });
    }
    return stub(route);
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/`);
  await page.getByRole('heading', { name: 'Sign in' }).waitFor({ timeout: 15_000 });
  return { page, read, failures, close: () => context.close() };
}

const card = (page) => page.locator('main');

// UC-INS-22 checks 8 and 9: the card at the workspace's address shows what the header's choice says.
test('chromium: 1280 px — the sign-in card with the name chosen shows the symbol and the name, the host beside it', async (t) => {
  const browser = await sharedChromium(t);
  const { page, read, failures, close } = await visitor(browser, { header: 'WORKSPACE_NAME', pictures: { symbol_media_id: IDS[2] } });
  t.after(close);
  const symbol = card(page).locator('img:visible');
  await symbol.waitFor({ timeout: 10_000 });
  assert.equal(new URL((await symbol.getAttribute('src')) ?? '', served.origin).pathname, `/api/v1/auth/workspace-pictures/${IDS[2]}`);
  await page.waitForFunction(() => [...document.querySelectorAll('main img')].every((each) => each.complete && each.naturalWidth > 0));
  assert.deepEqual(read, [IDS[2]]);
  const said = (await card(page).innerText()).replace(/\s+/g, ' ').trim();
  assert.ok(said.startsWith(`${WORKSPACE.display_name} house.example`), said);
  assert.equal(said.includes(`to ${WORKSPACE.display_name}`), false, 'the name is said twice');
  assert.equal(await page.title(), WORKSPACE.display_name);
  assert.deepEqual(failures, []);
});

for (const colorScheme of ['light', 'dark']) {
  test(`chromium: 1280 px, ${colorScheme} — the sign-in card with the logo alone shows that theme's logo and names the workspace beside it`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, failures, close } = await visitor(browser, { colorScheme, header: 'LOGO', pictures: { logo_light_media_id: IDS[0], logo_dark_media_id: IDS[1] } });
    t.after(close);
    const logo = card(page).locator('img:visible');
    await logo.waitFor({ timeout: 10_000 });
    assert.match((await logo.getAttribute('src')) ?? '', new RegExp(colorScheme === 'dark' ? IDS[1] : IDS[0]));
    await card(page).getByRole('img', { name: WORKSPACE.display_name, exact: true }).waitFor();
    await card(page).getByText(`to ${WORKSPACE.display_name}`).waitFor();
    assert.deepEqual(failures, []);
  });
}

test('chromium: 390 px — the sign-in card keeps the product where the rules carry no look, and where its picture is refused', async (t) => {
  const browser = await sharedChromium(t);
  const plain = await visitor(browser, { width: 390, height: 844 });
  t.after(plain.close);
  await card(plain.page).getByText(`to ${WORKSPACE.display_name}`).waitFor({ timeout: 10_000 });
  assert.equal(await card(plain.page).getByText('Hubtask', { exact: true }).count(), 1);
  assert.equal(await card(plain.page).locator('img').count(), 0);
  assert.deepEqual(plain.read, []);

  const refused = await visitor(browser, { header: 'LOGO', pictures: { logo_light_media_id: IDS[0] }, broken: [IDS[0]], width: 390, height: 844 });
  t.after(refused.close);
  await refused.page.waitForFunction(() => document.querySelectorAll('main img').length === 0 && document.querySelector('main')?.textContent?.includes('Hubtask'), null, { timeout: 10_000 });
  const overflow = await refused.page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.equal(overflow, 0);
  assert.deepEqual([...plain.failures, ...refused.failures], []);
});
