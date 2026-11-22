// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// An entry's description or title changed by somebody else while the reader was writing it
// (UC-SYN-02 check 10): leaving the field writes nothing and says that it was changed meanwhile;
// the reader compares both versions and keeps the other - their own text is filed as a comment -
// or writes theirs - the other text is filed first, then the PATCH; a foreign title that arrived
// while only the description was typed is not sent back. The foreign change arrives as a change
// record on the stream, which is answered and closed with a short `retry`, as `livechanges` does.
// Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { serve } from './serve.mjs';
import { ACCOUNT, COLLECTION, ITEMS, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const ENTRY = { ...ITEMS[0], notes: 'Tiles from the shop on the corner.' };

const served = await serve(DIST);
test.after(() => served.close());

/** The entry as the backend holds it, the requests the reader's client sent, and the change log. */
async function openEntry(browser, width) {
  const { page, context, failures } = await signedIn(browser, width, 1000);
  const state = { entry: ENTRY, sent: [], log: [], filed: 0 };
  let delivered = 0;
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    if (path === '/stream') {
      const news = state.log.slice(delivered);
      delivered = state.log.length;
      const frames = news.map((each, i) => `id: c-${delivered - news.length + i + 1}\ndata: ${JSON.stringify(each)}\n\n`).join('');
      return route.fulfill({ status: 200, contentType: 'text/event-stream', body: `retry: 250\n\n${frames}` });
    }
    if (method !== 'GET') state.sent.push({ method, path, body: request.postDataJSON(), ifMatch: request.headers()['if-match'] });
    if (path === `/items/${ENTRY.id}` && method === 'GET') {
      state.sent.push({ method, path, version: state.entry.version });
      return route.fulfill({ json: state.entry, headers: { ETag: `"${state.entry.version}"` } });
    }
    if (path === `/items/${ENTRY.id}` && method === 'PATCH') {
      state.entry = { ...state.entry, ...request.postDataJSON(), version: state.entry.version + 1 };
      return route.fulfill({ json: state.entry, headers: { ETag: `"${state.entry.version}"` } });
    }
    if (path === `/items/${ENTRY.id}/comments:file-version` && method === 'POST') {
      const { field, side, body } = request.postDataJSON();
      state.filed += 1;
      return route.fulfill({
        status: 201,
        json: {
          id: `01a0e2e0-0000-7000-8000-0000000c0f${String(state.filed).padStart(2, '0')}`, item_id: ENTRY.id, author_id: ACCOUNT.id,
          parent_comment_id: null, body, created_at: '2026-11-22T09:00:00Z', edited_at: null, deleted_at: null, version: 1,
          kind: 'SYSTEM', system_code: side === 'MINE' ? 'sync.set_aside_version' : 'sync.replaced_version',
          system_params: { field, at: '2026-11-22T09:00:00Z' }, internal: false,
        },
      });
    }
    return stub(route);
  });
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });

  /** Somebody else's change: the backend holds it, the stream tells the reader, who reads it again. */
  async function changeElsewhere(change) {
    state.entry = { ...state.entry, ...change, version: state.entry.version + 1 };
    state.log.push({ op: 'UPSERT', entity: 'item', entity_id: ENTRY.id, container_id: COLLECTION.id, payload: state.entry });
    const version = state.entry.version;
    await page.waitForResponse((response) => new URL(response.url()).pathname.endsWith(`/api/v1/items/${ENTRY.id}`) && response.request().method() === 'GET'
      && response.headers().etag === `"${version}"`, { timeout: 15_000 });
  }
  return { page, context, failures, state, changeElsewhere };
}

/** The writes of the entry and its comments; reads sent as POST (`:query`, `/search`) are not writes. */
const writes = (state) => state.sent
  .filter((each) => (each.method === 'PATCH' && each.path === `/items/${ENTRY.id}`) || (each.method === 'POST' && each.path.startsWith(`/items/${ENTRY.id}/comments`)))
  .map(({ method, path, body }) => ({ method, path, body }));

/** Types into the description and lets somebody else change it to `theirs` before leaving it. */
async function typeWhileChanged(page, changeElsewhere, mine, theirs) {
  const notes = page.getByRole('textbox', { name: 'Notes' });
  await notes.fill(mine);
  await changeElsewhere({ notes: theirs });
  // The reader leaves the field: to the title's pencil, as a person tabs on.
  await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Edit title' }).focus();
  return notes;
}

for (const width of [1280, 375]) {
  test(`chromium: ${width} px — keep theirs: nothing is written, the typed text is filed as a comment (UC-SYN-02 check 10)`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, context, failures, state, changeElsewhere } = await openEntry(browser, width);
    t.after(() => context.close());

    const notes = await typeWhileChanged(page, changeElsewhere, 'Tiles from the big market, ask for Ana.', 'Tiles ordered online already.');
    const notice = page.locator('[data-meanwhile="notes"]');
    await notice.getByText('The description was changed meanwhile').waitFor({ timeout: 10_000 });
    assert.deepEqual(writes(state), [], 'leaving the field wrote something');

    // Both versions side by side; closing without choosing decides nothing.
    await notice.getByRole('button', { name: 'Compare both versions' }).click();
    const dialog = page.getByRole('dialog', { name: 'Two versions of the description' });
    await dialog.waitFor({ timeout: 5_000 });
    assert.match(await dialog.getByRole('region', { name: 'Theirs, in place' }).textContent(), /Tiles ordered online already\./);
    assert.match(await dialog.getByRole('region', { name: 'Yours, not saved yet' }).textContent(), /Tiles from the big market, ask for Ana\./);
    await page.keyboard.press('Escape');
    await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
    assert.equal(await notice.count(), 1, 'cancelling the comparison dropped the choice');
    assert.deepEqual(writes(state), [], 'cancelling the comparison wrote something');

    await notice.getByRole('button', { name: 'Compare both versions' }).click();
    await dialog.getByRole('button', { name: 'Keep theirs' }).click();
    await page.getByText('Your version is kept as a comment on the entry').waitFor({ timeout: 10_000 });
    assert.deepEqual(writes(state), [{
      method: 'POST', path: `/items/${ENTRY.id}/comments:file-version`,
      body: { field: 'notes', side: 'MINE', body: 'Tiles from the big market, ask for Ana.' },
    }]);
    await notes.filter({ hasText: 'Tiles ordered online already.' }).waitFor({ timeout: 5_000 })
      .catch(async () => assert.fail(`the entry shows ${await notes.textContent()}`));
    assert.deepEqual(failures, []);
  });
}

test('chromium: 1280 px — write mine: the other text is filed first, then the typed one is written (UC-SYN-02 check 10)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures, state, changeElsewhere } = await openEntry(browser, 1280);
  t.after(() => context.close());

  await typeWhileChanged(page, changeElsewhere, 'Tiles from the big market, ask for Ana.', 'Tiles ordered online already.');
  const notice = page.locator('[data-meanwhile="notes"]');
  await notice.getByRole('button', { name: 'Compare both versions' }).click();
  await page.getByRole('dialog', { name: 'Two versions of the description' }).getByRole('button', { name: 'Write mine' }).click();
  await page.getByText('The version you replaced is kept as a comment on the entry').waitFor({ timeout: 10_000 });

  assert.deepEqual(writes(state), [
    { method: 'POST', path: `/items/${ENTRY.id}/comments:file-version`, body: { field: 'notes', side: 'REPLACED', body: 'Tiles ordered online already.' } },
    { method: 'PATCH', path: `/items/${ENTRY.id}`, body: { notes: 'Tiles from the big market, ask for Ana.' } },
  ]);
  // Against the version that holds the other text, not the one the reader began from.
  assert.equal(state.sent.find((each) => each.method === 'PATCH').ifMatch, `"${ENTRY.version + 1}"`);
  assert.equal(state.entry.notes, 'Tiles from the big market, ask for Ana.');
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — a title changed elsewhere while the description was typed is not sent back (UC-SYN-02 check 10)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures, state, changeElsewhere } = await openEntry(browser, 1280);
  t.after(() => context.close());

  const notes = page.getByRole('textbox', { name: 'Notes' });
  await notes.fill('Measure the wall twice.');
  await changeElsewhere({ title: 'Order the tiles, renamed elsewhere' });
  await page.getByRole('heading', { level: 1 }).getByText('Order the tiles, renamed elsewhere').waitFor({ timeout: 10_000 });
  const patched = page.waitForRequest((request) => request.method() === 'PATCH' && new URL(request.url()).pathname.endsWith(`/api/v1/items/${ENTRY.id}`), { timeout: 10_000 });
  await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Edit title' }).focus();
  await patched;

  assert.deepEqual(writes(state), [{ method: 'PATCH', path: `/items/${ENTRY.id}`, body: { notes: 'Measure the wall twice.' } }]);
  assert.equal(state.entry.title, 'Order the tiles, renamed elsewhere');
  assert.equal(await page.locator('[data-meanwhile]').count(), 0, 'a change of another field was offered as a conflict');
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — a title renamed elsewhere while the reader renamed it waits for the choice, and write mine files the other first (UC-SYN-02 check 10)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures, state, changeElsewhere } = await openEntry(browser, 1280);
  t.after(() => context.close());

  await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Edit title' }).click();
  const field = page.getByRole('textbox', { name: 'Title', exact: true });
  await field.fill('Order the tiles, kitchen only');
  await changeElsewhere({ title: 'Order the tiles, bathroom first' });
  await field.press('Enter');

  const notice = page.locator('[data-meanwhile="title"]');
  await notice.getByText('The title was changed meanwhile').waitFor({ timeout: 10_000 });
  assert.deepEqual(writes(state), [], 'the rename wrote over the other title');
  await notice.getByRole('button', { name: 'Compare both versions' }).click();
  const dialog = page.getByRole('dialog', { name: 'Two versions of the title' });
  assert.match(await dialog.getByRole('region', { name: 'Theirs, in place' }).textContent(), /Order the tiles, bathroom first/);
  assert.match(await dialog.getByRole('region', { name: 'Yours, not saved yet' }).textContent(), /Order the tiles, kitchen only/);
  await dialog.getByRole('button', { name: 'Write mine' }).click();
  await page.getByText('The version you replaced is kept as a comment on the entry').waitFor({ timeout: 10_000 });

  assert.deepEqual(writes(state), [
    { method: 'POST', path: `/items/${ENTRY.id}/comments:file-version`, body: { field: 'title', side: 'REPLACED', body: 'Order the tiles, bathroom first' } },
    { method: 'PATCH', path: `/items/${ENTRY.id}`, body: { title: 'Order the tiles, kitchen only' } },
  ]);
  await page.getByRole('heading', { level: 1 }).getByText('Order the tiles, kitchen only').waitFor({ timeout: 10_000 });
  assert.deepEqual(failures, []);
});
