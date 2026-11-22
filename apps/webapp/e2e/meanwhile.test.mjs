// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// An entry's description or title changed by somebody else while the reader was writing it
// (UC-SYN-02 check 10): leaving the field writes nothing and says that it was changed meanwhile;
// the reader compares both versions and keeps the other - their own text is filed as a comment -
// or writes theirs - the other text is filed first, then the PATCH; a foreign title that arrived
// while only the description was typed is not sent back. The foreign change arrives as a change
// record on the stream, which is answered and closed with a short `retry`, as `livechanges` does.
// The kept version is shown as what it is and has an address: *Show the comment* lands on it in the
// conversation, marked, focused and headed as a kept version, the text as filed; an address of a
// comment that is not there says so; the offline conflict's link to the displaced version lands
// there too and decides nothing. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { serve } from './serve.mjs';
import { ACCOUNT, COLLECTION, ITEMS, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const ENTRY = { ...ITEMS[0], notes: 'Tiles from the shop on the corner.' };
/** The displaced version a push filed, as the server names it in the conflict. */
const DISPLACED = '01a0e2e0-0000-7000-8000-0000000c0fd1';

const served = await serve(DIST);
test.after(() => served.close());

/** A refusal as the server words it. */
const problem = (status, code, detail = code) => ({
  status, contentType: 'application/problem+json', json: { type: 'about:blank', title: code, status, code, detail_code: detail },
});

/**
 * The entry as the backend holds it, the requests the reader's client sent, and the change log.
 * `refuseRead`, `refuseFiling` and `refusePatches` make it refuse the entry's read, a filing or
 * that many writes; a filing is answered only once `filingHeld` settles.
 */
async function openEntry(browser, width) {
  const { page, context, failures } = await signedIn(browser, width, 1000);
  let named;
  /** The reader's name was read: a name asked for offline is not asked again until something else wants it. */
  const isNamed = new Promise((resolve) => (named = resolve));
  const state = {
    entry: ENTRY, sent: [], log: [], filed: 0, comments: [], pushed: [], isOffline: false, theirsAway: undefined, isNamed,
    refuseRead: false, refuseFiling: false, refusePatches: 0, filingHeld: undefined,
  };
  let delivered = 0;
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    if (state.isOffline) return route.abort('internetdisconnected');
    if (path === `/accounts/${ACCOUNT.id}`) named();
    // A device's queued notes met notes somebody else wrote while it was away (offline-sync.md §5):
    // theirs stays, the pushed text is filed as the displaced version and answered as the conflict.
    if (path === '/sync:push' && method === 'POST') {
      const sent = request.postDataJSON();
      state.pushed.push(sent);
      return route.fulfill({
        json: {
          results: (sent.mutations ?? []).map((mutation) => {
            const mine = mutation.fields?.notes?.value;
            if (typeof mine !== 'string' || !state.theirsAway) return { op_id: mutation.op_id, result: 'APPLIED' };
            state.entry = { ...state.entry, notes: state.theirsAway, version: state.entry.version + 1 };
            const displaced = {
              id: DISPLACED, item_id: ENTRY.id, author_id: ACCOUNT.id, parent_comment_id: null, body: mine,
              created_at: '2026-11-22T09:00:00Z', edited_at: null, deleted_at: null, version: 1, kind: 'SYSTEM',
              system_code: 'sync.displaced_version', system_params: { field: 'notes', device: sent.device_id, at: '2026-11-22T09:00:00Z' }, internal: false,
            };
            state.comments.push(displaced);
            return {
              op_id: mutation.op_id, result: 'CONFLICT', entity_id: ENTRY.id, server_state: state.entry,
              conflict: { field: 'notes', mine, theirs: state.theirsAway, preserved_comment_id: DISPLACED },
            };
          }),
        },
      });
    }
    if (path === '/stream') {
      const news = state.log.slice(delivered);
      delivered = state.log.length;
      const frames = news.map((each, i) => `id: c-${delivered - news.length + i + 1}\ndata: ${JSON.stringify(each)}\n\n`).join('');
      return route.fulfill({ status: 200, contentType: 'text/event-stream', body: `retry: 250\n\n${frames}` });
    }
    if (method !== 'GET') state.sent.push({ method, path, body: request.postDataJSON(), ifMatch: request.headers()['if-match'] });
    if (path === `/items/${ENTRY.id}` && method === 'GET') {
      state.sent.push({ method, path, version: state.entry.version });
      if (state.refuseRead) return route.fulfill(problem(429, 'rate_limited'));
      return route.fulfill({ json: state.entry, headers: { ETag: `"${state.entry.version}"` } });
    }
    if (path === `/items/${ENTRY.id}` && method === 'PATCH') {
      if (state.refusePatches > 0) {
        state.refusePatches -= 1;
        return route.fulfill(problem(403, 'forbidden'));
      }
      state.entry = { ...state.entry, ...request.postDataJSON(), version: state.entry.version + 1 };
      return route.fulfill({ json: state.entry, headers: { ETag: `"${state.entry.version}"` } });
    }
    if (path === `/items/${ENTRY.id}/comments:file-version` && method === 'POST') {
      if (state.refuseFiling) return route.fulfill(problem(403, 'forbidden'));
      if (state.filingHeld) await state.filingHeld;
      const { field, side, body } = request.postDataJSON();
      state.filed += 1;
      const filed = {
        id: `01a0e2e0-0000-7000-8000-0000000c0f${String(state.filed).padStart(2, '0')}`, item_id: ENTRY.id, author_id: ACCOUNT.id,
        parent_comment_id: null, body, created_at: '2026-11-22T09:00:00Z', edited_at: null, deleted_at: null, version: 1,
        kind: 'SYSTEM', system_code: side === 'MINE' ? 'sync.set_aside_version' : 'sync.replaced_version',
        system_params: { field, at: '2026-11-22T09:00:00Z' }, internal: false,
      };
      state.comments.push(filed);
      return route.fulfill({ status: 201, json: filed });
    }
    // The conversation, newest first, as the server answers it.
    if (path === `/items/${ENTRY.id}/comments` && method === 'GET') {
      return route.fulfill({ json: { data: [...state.comments].reverse(), page: { has_more: false, next_cursor: null } } });
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

/**
 * The kept version at its address: marked, holding the focus, headed as a kept version that names
 * whose version it is - never drawn as a post by its author - and the text exactly as filed.
 */
async function landsOnKept(page, id, heading, text) {
  const kept = page.getByRole('article', { name: /^Kept version/ });
  await kept.waitFor({ timeout: 10_000 });
  await page.waitForFunction((named) => document.activeElement?.closest('article')?.getAttribute('data-comment-id') === named, id, { timeout: 5_000 })
    .catch(async () => assert.fail(`the focus is on ${await page.evaluate(() => document.activeElement?.outerHTML.slice(0, 120))}`));
  assert.equal(await kept.getAttribute('data-marked'), '', 'the addressed comment is not marked');
  // The author's name arrives with the names' read, which a reconnect may still be retrying.
  await kept.filter({ hasText: heading }).waitFor({ timeout: 10_000 })
    .catch(async () => assert.fail(`the heading reads ${await kept.textContent()}`));
  assert.equal(await kept.getByRole('blockquote').textContent(), text, 'the kept text is not shown as filed');
  assert.equal(await kept.getByText(ACCOUNT.display_name, { exact: true }).count(), 0, 'the kept version reads as a post by its author');
  assert.equal(await kept.getByRole('img', { name: ACCOUNT.display_name }).count(), 0, 'the kept version carries its author’s avatar');
  assert.ok(await kept.isVisible(), 'the addressed comment is not in view');
}

for (const width of [1280, 375]) {
  test(`chromium: ${width} px — write mine, then show the comment: it is marked and focused, headed as the version replaced (UC-SYN-02 check 10)`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, context, failures, state, changeElsewhere } = await openEntry(browser, width);
    t.after(() => context.close());

    await typeWhileChanged(page, changeElsewhere, 'Tiles from the big market, ask for Ana.', 'Tiles ordered online already.');
    await page.locator('[data-meanwhile="notes"]').getByRole('button', { name: 'Compare both versions' }).click();
    await page.getByRole('dialog', { name: 'Two versions of the description' }).getByRole('button', { name: 'Write mine' }).click();
    const notice = page.locator('[data-meanwhile-kept="REPLACED"]');
    await notice.waitFor({ timeout: 10_000 });
    await notice.getByRole('button', { name: 'Show the comment' }).click();

    const [kept] = state.comments;
    await page.waitForURL((url) => url.pathname === `/items/${ENTRY.id}` && url.searchParams.get('comment') === kept.id, { timeout: 5_000 });
    await landsOnKept(page, kept.id, /The text as it stood before Jérôme Winkel replaced it at .+, after somebody else had changed it meanwhile\./, 'Tiles ordered online already.');
    // It is the other person's text, and it can be removed but not rewritten.
    await page.getByRole('article', { name: /^Kept version/ }).getByRole('button', { name: 'More for this comment' }).click();
    assert.equal(await page.getByRole('menuitem', { name: 'Edit' }).count(), 0, 'a kept version offers to be rewritten');
    await page.getByRole('menuitem', { name: 'Remove' }).waitFor({ timeout: 5_000 });
    await page.keyboard.press('Escape');
    assert.deepEqual(failures, []);
  });
}

test('chromium: 1280 px — an address names a comment: opened at it the entry lands there, and one that is not there says so (UC-SYN-02 check 10)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures, state } = await openEntry(browser, 1280);
  t.after(() => context.close());
  const KEPT = {
    id: '01a0e2e0-0000-7000-8000-0000000c0f31', item_id: ENTRY.id, author_id: ACCOUNT.id, parent_comment_id: null,
    body: 'Tiles from the big market,\n  ask for Ana.', created_at: '2026-11-22T09:00:00Z', edited_at: null, deleted_at: null, version: 1,
    kind: 'SYSTEM', system_code: 'sync.set_aside_version', system_params: { field: 'notes', at: '2026-11-22T09:00:00Z' }, internal: false,
  };
  state.comments.push(KEPT);

  await page.goto(`${served.origin}/items/${ENTRY.id}?comment=${KEPT.id}`);
  await landsOnKept(page, KEPT.id, /Version Jérôme Winkel set aside at .+, because somebody else changed the text meanwhile\. The entry carries the other one\./, KEPT.body);

  await page.goto(`${served.origin}/items/${ENTRY.id}?comment=01a0e2e0-0000-7000-8000-0000000c0f99`);
  await page.getByRole('paragraph').filter({ hasText: 'This comment could not be found, or you may not see it.' }).waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('[data-marked]').count(), 0, 'another comment was marked in its place');
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — offline, the conflict’s link to the displaced version lands on it and decides nothing (UC-SYN-02 check 10)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures, state } = await openEntry(browser, 1280);
  t.after(() => context.close());

  await state.isNamed;
  state.isOffline = true;
  await context.setOffline(true);
  await page.waitForFunction(() => document.querySelector('.trigger')?.getAttribute('data-connection') === 'offline', null, { timeout: 5_000 });
  const notes = page.getByRole('textbox', { name: 'Notes' });
  await notes.fill('Tiles from the big market, ask for Ana.');
  await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Edit title' }).focus();
  await page.waitForFunction(() => document.querySelector('.trigger')?.getAttribute('aria-label')?.includes('1 change is waiting to be sent'), null, { timeout: 5_000 })
    .catch(() => assert.fail('the notes were not kept as a waiting change'));

  // Somebody else wrote the notes meanwhile; back online, the push meets theirs.
  state.theirsAway = 'Tiles ordered online already.';
  state.isOffline = false;
  await context.setOffline(false);
  const strip = page.getByText('The notes were changed by somebody else while you were away.');
  await strip.waitFor({ timeout: 15_000 });
  await page.getByRole('button', { name: 'Show both versions' }).click();
  const dialog = page.getByRole('dialog', { name: 'Two versions of the notes' });
  await dialog.getByRole('link', { name: 'Your version, as a comment on the entry' }).click();

  await page.waitForURL((url) => url.pathname === `/items/${ENTRY.id}` && url.searchParams.get('comment') === DISPLACED, { timeout: 5_000 });
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
  await landsOnKept(page, DISPLACED, /Diverging version from Jérôme Winkel, written on this device at .+\. This is the text that lost the merge; the entry carries the other one\./, 'Tiles from the big market, ask for Ana.');
  // Looking is not choosing: the conflict waits, and *Write mine again* is still offered.
  assert.equal(await strip.count(), 1, 'following the link decided the conflict');
  await page.getByRole('button', { name: 'Show both versions' }).click();
  await dialog.getByRole('button', { name: 'Write mine again' }).waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
});

/** Notes written offline meet notes somebody else wrote meanwhile; the strip offers both versions. */
async function conflictWhileAway(page, context, state) {
  await state.isNamed;
  state.isOffline = true;
  await context.setOffline(true);
  await page.waitForFunction(() => document.querySelector('.trigger')?.getAttribute('data-connection') === 'offline', null, { timeout: 5_000 });
  await page.getByRole('textbox', { name: 'Notes' }).fill('Tiles from the big market, ask for Ana.');
  await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Edit title' }).focus();
  await page.waitForFunction(() => document.querySelector('.trigger')?.getAttribute('aria-label')?.includes('1 change is waiting to be sent'), null, { timeout: 5_000 })
    .catch(() => assert.fail('the notes were not kept as a waiting change'));
  state.theirsAway = 'Tiles ordered online already.';
  state.isOffline = false;
  await context.setOffline(false);
  const strip = page.getByText('The notes were changed by somebody else while you were away.');
  await strip.waitFor({ timeout: 15_000 });
  return strip;
}

test('chromium: 1280 px — offline, write mine again whose filing is refused keeps the conflict and says why (UC-SYN-02 check 10)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, state } = await openEntry(browser, 1280);
  t.after(() => context.close());

  const strip = await conflictWhileAway(page, context, state);
  state.refuseFiling = true;
  await page.getByRole('button', { name: 'Show both versions' }).click();
  await page.getByRole('dialog', { name: 'Two versions of the notes' }).getByRole('button', { name: 'Write mine again' }).click();
  await page.getByText('You do not have permission for this action.').waitFor({ timeout: 10_000 })
    .catch(async () => assert.fail(`the refusal is not said: ${await page.locator('main').innerText()}`));
  assert.equal(await strip.count(), 1, 'the refused choice dropped the conflict');
  assert.equal(writes(state).filter((each) => each.method === 'PATCH').length, 0, 'mine was written over theirs unkept');
});

test('chromium: 1280 px — offline, write mine again while the entry cannot be read files nothing and writes nothing (UC-SYN-02 check 10)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, state } = await openEntry(browser, 1280);
  t.after(() => context.close());

  const strip = await conflictWhileAway(page, context, state);
  state.refuseRead = true;
  const before = writes(state).length;
  await page.getByRole('button', { name: 'Show both versions' }).click();
  await page.getByRole('dialog', { name: 'Two versions of the notes' }).getByRole('button', { name: 'Write mine again' }).click();
  await page.getByText('Too many requests. Please try again in a moment.').first().waitFor({ timeout: 10_000 })
    .catch(async () => assert.fail(`the failure is not said: ${await page.locator('main').innerText()}`));
  assert.deepEqual(writes(state).slice(before), [], 'a text was filed or written without the entry’s current version');
  assert.equal(await strip.count(), 1, 'the conflict was dropped');
});

test('chromium: 1280 px — keep theirs: what the reader types while their version is being filed stays (UC-SYN-02 check 10)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures, state, changeElsewhere } = await openEntry(browser, 1280);
  t.after(() => context.close());

  const notes = await typeWhileChanged(page, changeElsewhere, 'Tiles from the big market, ask for Ana.', 'Tiles ordered online already.');
  let release;
  state.filingHeld = new Promise((resolve) => (release = resolve));
  const asked = page.waitForRequest((request) => request.url().endsWith('/comments:file-version'), { timeout: 10_000 });
  await page.locator('[data-meanwhile="notes"]').getByRole('button', { name: 'Compare both versions' }).click();
  await page.getByRole('dialog', { name: 'Two versions of the description' }).getByRole('button', { name: 'Keep theirs' }).click();
  await asked;
  await notes.fill('Tiles ordered online already. Ask Ana about the grout.');
  const patched = page.waitForRequest((request) => request.method() === 'PATCH' && new URL(request.url()).pathname.endsWith(`/api/v1/items/${ENTRY.id}`), { timeout: 10_000 });
  release();
  await page.getByText('Your version is kept as a comment on the entry').waitFor({ timeout: 10_000 });
  await notes.filter({ hasText: 'Ask Ana about the grout.' }).waitFor({ timeout: 5_000 })
    .catch(async () => assert.fail(`the text typed meanwhile is gone: ${await notes.textContent()}`));
  // It is the reader's change of the entry's text, written as any other once the field is left.
  await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Edit title' }).focus();
  await patched.catch(() => assert.fail(`the text typed meanwhile was not written: ${JSON.stringify(writes(state))}`));
  assert.deepEqual(writes(state).slice(1), [{ method: 'PATCH', path: `/items/${ENTRY.id}`, body: { notes: 'Tiles ordered online already. Ask Ana about the grout.' } }]);
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — write mine refused, then theirs changes again: the new text is filed before mine is written (UC-SYN-02 check 10)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, state, changeElsewhere } = await openEntry(browser, 1280);
  t.after(() => context.close());

  await typeWhileChanged(page, changeElsewhere, 'Tiles from the big market, ask for Ana.', 'Tiles ordered online already.');
  const notice = page.locator('[data-meanwhile="notes"]');
  state.refusePatches = 1;
  await notice.getByRole('button', { name: 'Compare both versions' }).click();
  await page.getByRole('dialog', { name: 'Two versions of the description' }).getByRole('button', { name: 'Write mine' }).click();
  await notice.locator('[data-meanwhile-failure]').waitFor({ timeout: 10_000 });

  await changeElsewhere({ notes: 'Tiles ordered online, delivery on Friday.' });
  await notice.getByRole('button', { name: 'Compare both versions' }).click();
  await page.getByRole('dialog', { name: 'Two versions of the description' }).getByRole('button', { name: 'Write mine' }).click();
  await page.getByText('The version you replaced is kept as a comment on the entry').waitFor({ timeout: 10_000 });

  assert.deepEqual(writes(state), [
    { method: 'POST', path: `/items/${ENTRY.id}/comments:file-version`, body: { field: 'notes', side: 'REPLACED', body: 'Tiles ordered online already.' } },
    { method: 'PATCH', path: `/items/${ENTRY.id}`, body: { notes: 'Tiles from the big market, ask for Ana.' } },
    { method: 'POST', path: `/items/${ENTRY.id}/comments:file-version`, body: { field: 'notes', side: 'REPLACED', body: 'Tiles ordered online, delivery on Friday.' } },
    { method: 'PATCH', path: `/items/${ENTRY.id}`, body: { notes: 'Tiles from the big market, ask for Ana.' } },
  ]);
});
