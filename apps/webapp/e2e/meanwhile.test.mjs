// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// An entry's description or title changed by somebody else while the reader was editing it
// (UC-SYN-02 check 10): saving it writes nothing and a warning asks first - who changed it and
// when, both versions, the focus on the safe answer. *Keep the saved version* writes nothing and
// shows the other text; *Overwrite with mine* sends one PATCH of that field against the newer
// version; a refused overwrite keeps the question and says why; closing it leaves the text unsaved
// and the next save asks again; a foreign title that arrived while only the description was typed
// is not sent back. The foreign change arrives as a change record on the stream, which is answered
// and closed with a short `retry`, as `livechanges` does, and the history names who made it.
// The kept version of an offline merge (UC-SYN-02 check 7) is shown as what it is and has an
// address: opened at it the entry lands there, marked, focused and headed as a kept version, the
// text as filed; an address of a comment that is not there says so; the offline conflict's link
// to the displaced version lands there too and decides nothing. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { serve } from './serve.mjs';
import { ACCOUNT, COLLECTION, ITEMS, OTHER, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const ENTRY = { ...ITEMS[0], notes: 'Tiles from the shop on the corner.' };
/** The displaced version a push filed, as the server names it in the conflict. */
const DISPLACED = '01a0e2e0-0000-7000-8000-0000000c0fd1';
const MINE = 'Tiles from the big market, ask for Ana.';
const THEIRS = 'Tiles ordered online already.';

const served = await serve(DIST);
test.after(() => served.close());

/** A refusal as the server words it. */
const problem = (status, code, detail = code) => ({
  status, contentType: 'application/problem+json', json: { type: 'about:blank', title: code, status, code, detail_code: detail },
});

/** A version a push kept as a comment (offline-sync.md §5). */
const keptComment = (id, body, device = 'another-device') => ({
  id, item_id: ENTRY.id, author_id: ACCOUNT.id, parent_comment_id: null, body,
  created_at: '2026-11-22T09:00:00Z', edited_at: null, deleted_at: null, version: 1, kind: 'SYSTEM',
  system_code: 'sync.displaced_version', system_params: { field: 'notes', device, at: '2026-11-22T09:00:00Z' }, internal: false,
});

/**
 * The entry as the backend holds it, its history, the requests the reader's client sent, and the
 * change log. `refusePatches` makes it refuse that many writes, `refuseRead` the entry's read; a
 * write is answered only once `patchHeld` settles. A write against an older version than the one
 * held is refused as the server refuses it (`version_conflict`, `ensureExpectedVersion`).
 */
async function openEntry(browser, width, address = `/items/${ENTRY.id}`) {
  const { page, context, failures } = await signedIn(browser, width, 1000);
  let named;
  /** The reader's name was read: a name asked for offline is not asked again until something else wants it. */
  const isNamed = new Promise((resolve) => (named = resolve));
  const state = {
    entry: ENTRY, sent: [], log: [], history: [], comments: [], pushed: [], isOffline: false, theirsAway: undefined, isNamed,
    refusePatches: 0, refuseRead: false, patchHeld: undefined, olderRefused: false, commentsRefused: false,
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
            state.comments.push(keptComment(DISPLACED, mine, sent.device_id));
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
      if (state.refuseRead) return route.fulfill(problem(429, 'rate_limited'));
      return route.fulfill({ json: state.entry, headers: { ETag: `"${state.entry.version}"` } });
    }
    if (path === `/items/${ENTRY.id}` && method === 'PATCH') {
      if (state.patchHeld) await state.patchHeld;
      const ifMatch = request.headers()['if-match'];
      if (ifMatch && ifMatch !== `"${state.entry.version}"`) {
        return route.fulfill({
          status: 409, contentType: 'application/problem+json',
          json: {
            type: 'https://hubtask.eu/problems/version_conflict', title: 'version_conflict', status: 409, code: 'version_conflict',
            detail_code: 'items.version_conflict', params: { item_id: ENTRY.id, current_version: String(state.entry.version) },
            docs: 'https://hubtask.eu/problems/version_conflict/',
          },
        });
      }
      if (state.refusePatches > 0) {
        state.refusePatches -= 1;
        return route.fulfill(problem(403, 'forbidden'));
      }
      state.entry = { ...state.entry, ...request.postDataJSON(), version: state.entry.version + 1 };
      return route.fulfill({ json: state.entry, headers: { ETag: `"${state.entry.version}"` } });
    }
    // The entry's history, newest first: who changed what.
    if (path === `/items/${ENTRY.id}/activity` && method === 'GET') {
      return route.fulfill({ json: { data: [...state.history].reverse(), page: { has_more: false, next_cursor: null } } });
    }
    // The conversation, newest first, as the server answers it.
    if (path === `/items/${ENTRY.id}/comments` && method === 'GET' && state.commentsRefused) return route.fulfill(problem(403, 'forbidden'));
    if (path === `/items/${ENTRY.id}/comments` && method === 'GET' && state.olderRefused) {
      if (new URL(request.url()).searchParams.get('cursor')) return route.fulfill(problem(403, 'forbidden'));
      return route.fulfill({ json: { data: [...state.comments].reverse(), page: { has_more: true, next_cursor: 'older' } } });
    }
    if (path === `/items/${ENTRY.id}/comments` && method === 'GET') {
      return route.fulfill({ json: { data: [...state.comments].reverse(), page: { has_more: false, next_cursor: null } } });
    }
    return stub(route);
  });
  await page.goto(`${served.origin}${address}`);
  await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });

  /** Mara's change, held by the backend and its history, the stream not yet saying so. */
  function changeSilently(change) {
    state.entry = { ...state.entry, ...change, version: state.entry.version + 1 };
    state.history.push({
      id: `01a0e2e0-0000-7000-8000-0000000a0c${String(state.history.length + 1).padStart(2, '0')}`, item_id: ENTRY.id,
      code: 'activity.item_updated', actor: { type: 'USER', id: OTHER.id }, occurred_at: '2026-11-22T10:02:00Z',
      change_set: Object.fromEntries(Object.entries(change).map(([field, to]) => [field, field === 'notes' ? { changed: true } : { to }])),
    });
  }

  /** Mara's change: the backend holds it and its history step, the stream tells the reader, who reads it again. */
  async function changeElsewhere(change) {
    changeSilently(change);
    state.log.push({ op: 'UPSERT', entity: 'item', entity_id: ENTRY.id, container_id: COLLECTION.id, payload: state.entry });
    const version = state.entry.version;
    await page.waitForResponse((response) => new URL(response.url()).pathname.endsWith(`/api/v1/items/${ENTRY.id}`) && response.request().method() === 'GET'
      && response.headers().etag === `"${version}"`, { timeout: 15_000 });
  }
  return { page, context, failures, state, changeElsewhere, changeSilently };
}

/** The writes of the entry and its comments; reads sent as POST (`:query`, `/search`) are not writes. */
const writes = (state) => state.sent
  .filter((each) => (each.method === 'PATCH' && each.path === `/items/${ENTRY.id}`) || (each.method === 'POST' && each.path.startsWith(`/items/${ENTRY.id}/comments`)))
  .map(({ method, path, body }) => ({ method, path, body }));

/** Types into the description, lets Mara change it to `theirs`, and leaves the field as a person tabs on. */
async function typeWhileChanged(page, changeElsewhere, mine = MINE, theirs = THEIRS) {
  const notes = page.getByRole('textbox', { name: 'Notes' });
  await notes.fill(mine);
  await changeElsewhere({ notes: theirs });
  await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Edit title' }).focus();
  return notes;
}

/** The warning, asked about the description, naming Mara and showing both versions. */
async function warningAbout(page, field, theirs, mine) {
  const dialog = page.getByRole('alertdialog', { name: field === 'title' ? 'The title was changed meanwhile' : 'The description was changed meanwhile' });
  await dialog.waitFor({ timeout: 10_000 });
  await dialog.getByText(`Mara Lind changed the ${field === 'title' ? 'title' : 'description'} at `).waitFor({ timeout: 10_000 })
    .catch(async () => assert.fail(`the warning does not name who changed it: ${await dialog.textContent()}`));
  // A description is drawn by the editor, which loads on first use: waited for, as a person would.
  for (const [name, text] of [['The saved version', theirs], ['Your version, not saved', mine]]) {
    const region = dialog.getByRole('region', { name });
    await region.filter({ hasText: text }).waitFor({ timeout: 5_000 })
      .catch(async () => assert.fail(`${name} reads ${await region.textContent()}`));
  }
  return dialog;
}

/** Where the focus is: the name of the focused button, or the element's tag. */
const focused = (page) => page.evaluate(() => {
  const at = document.activeElement;
  return at?.tagName === 'BUTTON' ? at.textContent?.trim() : at?.tagName;
});

for (const width of [1280, 390]) {
  test(`chromium: ${width} px — a description changed meanwhile: leaving writes nothing, the warning names Mara and shows both, keep the saved version writes nothing (UC-SYN-02 check 10)`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, context, failures, state, changeElsewhere } = await openEntry(browser, width);
    t.after(() => context.close());

    const notes = await typeWhileChanged(page, changeElsewhere);
    const dialog = await warningAbout(page, 'notes', THEIRS, MINE);
    assert.deepEqual(writes(state), [], 'leaving the field wrote something');
    // The focus starts on the safe answer, never on the one that overwrites.
    await page.waitForFunction(() => document.activeElement?.textContent?.trim() === 'Keep the saved version', null, { timeout: 5_000 })
      .catch(async () => assert.fail(`the focus starts on ${await focused(page)}`));
    // Exactly two answers, beside the control that closes it unanswered.
    const answers = (await dialog.getByRole('button').allInnerTexts()).map((each) => each.trim()).filter(Boolean);
    assert.deepEqual(answers, ['Close', 'Keep the saved version', 'Overwrite with mine']);
    assert.equal(await dialog.getByRole('button', { name: 'Close', exact: true }).count(), 1, 'the close control is not named Close');
    await dialog.getByText('The version you do not choose is not kept.').waitFor({ timeout: 5_000 });
    assert.match(await dialog.innerText(), /yours\. The version you do not choose is not kept\./, 'the two sentences run together');
    await dialog.getByText('Closing this leaves your text unsaved; the next save asks again.').waitFor({ timeout: 5_000 });

    await dialog.getByRole('button', { name: 'Keep the saved version' }).click();
    await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
    await notes.filter({ hasText: THEIRS }).waitFor({ timeout: 5_000 })
      .catch(async () => assert.fail(`the entry shows ${await notes.textContent()}`));
    assert.deepEqual(writes(state), [], 'keeping the saved version wrote something');
    assert.deepEqual(failures, []);
  });

  test(`chromium: ${width} px — overwrite with mine sends one PATCH of the description against the newer version (UC-SYN-02 check 10)`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, context, failures, state, changeElsewhere } = await openEntry(browser, width);
    t.after(() => context.close());

    const notes = await typeWhileChanged(page, changeElsewhere);
    const dialog = await warningAbout(page, 'notes', THEIRS, MINE);
    await dialog.getByRole('button', { name: 'Overwrite with mine' }).click();
    await dialog.waitFor({ state: 'hidden', timeout: 10_000 });

    assert.deepEqual(writes(state), [{ method: 'PATCH', path: `/items/${ENTRY.id}`, body: { notes: MINE } }]);
    // Against the version that holds Mara's text, not the one the reader began from.
    assert.equal(state.sent.find((each) => each.method === 'PATCH').ifMatch, `"${ENTRY.version + 1}"`);
    await notes.filter({ hasText: MINE }).waitFor({ timeout: 5_000 });
    assert.deepEqual(failures, []);
  });

  test(`chromium: ${width} px — a title changed elsewhere while the description was typed is not sent back (UC-SYN-02 check 10)`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, context, failures, state, changeElsewhere } = await openEntry(browser, width);
    t.after(() => context.close());

    const notes = page.getByRole('textbox', { name: 'Notes' });
    await notes.fill('Measure the wall twice.');
    await changeElsewhere({ title: 'Order the tiles, renamed elsewhere' });
    await page.getByRole('heading', { level: 1 }).getByText('Order the tiles, renamed elsewhere').waitFor({ timeout: 10_000 });
    // Its answer, not the request: the stub records a write while it answers it.
    const patched = page.waitForResponse((response) => response.request().method() === 'PATCH' && new URL(response.url()).pathname.endsWith(`/api/v1/items/${ENTRY.id}`), { timeout: 10_000 });
    await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Edit title' }).focus();
    await patched;

    assert.deepEqual(writes(state), [{ method: 'PATCH', path: `/items/${ENTRY.id}`, body: { notes: 'Measure the wall twice.' } }]);
    assert.equal(state.entry.title, 'Order the tiles, renamed elsewhere');
    assert.equal(await page.getByRole('alertdialog').count(), 0, 'a change of another field was asked about');
    assert.deepEqual(failures, []);
  });

  test(`chromium: ${width} px — a refused overwrite keeps the warning and says why (UC-SYN-02 check 10)`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, context, state, changeElsewhere } = await openEntry(browser, width);
    t.after(() => context.close());

    await typeWhileChanged(page, changeElsewhere);
    const dialog = await warningAbout(page, 'notes', THEIRS, MINE);
    state.refusePatches = 1;
    await dialog.getByRole('button', { name: 'Overwrite with mine' }).click();
    await dialog.getByText('Your version was not written; the saved one is still in place.').waitFor({ timeout: 10_000 })
      .catch(async () => assert.fail(`the refusal is not said in the warning: ${await page.locator('main').innerText()}`));
    await dialog.getByText('You do not have permission for this action.').waitFor({ timeout: 5_000 });
    assert.ok(await dialog.isVisible(), 'the refusal closed the warning');
    assert.equal(state.entry.notes, THEIRS);
    assert.equal(writes(state).length, 1, 'a refused overwrite was sent again on its own');
  });

  test(`chromium: ${width} px — Escape leaves the text unsaved, and the next save asks again (UC-SYN-02 check 10)`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, context, failures, state, changeElsewhere } = await openEntry(browser, width);
    t.after(() => context.close());

    const notes = await typeWhileChanged(page, changeElsewhere);
    const dialog = await warningAbout(page, 'notes', THEIRS, MINE);
    await page.keyboard.press('Escape');
    await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
    assert.deepEqual(writes(state), [], 'closing the warning wrote something');
    await notes.filter({ hasText: MINE }).waitFor({ timeout: 5_000 })
      .catch(async () => assert.fail(`closing the warning dropped the reader's text: ${await notes.textContent()}`));

    // The next save: into the description and out again.
    await notes.click();
    await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Edit title' }).focus();
    await warningAbout(page, 'notes', THEIRS, MINE);
    assert.deepEqual(writes(state), [], 'the next save wrote over Mara’s text unasked');
    assert.deepEqual(failures, []);
  });

  test(`chromium: ${width} px — by the keyboard: a title renamed meanwhile is asked about, the focus on the safe answer, and Tab, Enter overwrites (UC-SYN-02 check 10)`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, context, failures, state, changeElsewhere } = await openEntry(browser, width);
    t.after(() => context.close());

    await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Edit title' }).focus();
    await page.keyboard.press('Enter');
    const field = page.getByRole('textbox', { name: 'Title', exact: true });
    await field.waitFor({ timeout: 5_000 });
    await page.keyboard.press('ControlOrMeta+a');
    await page.keyboard.type('Order the tiles, kitchen only');
    await changeElsewhere({ title: 'Order the tiles, bathroom first' });
    await page.keyboard.press('Enter');

    await warningAbout(page, 'title', 'Order the tiles, bathroom first', 'Order the tiles, kitchen only');
    assert.deepEqual(writes(state), [], 'the rename wrote over the other title');
    await page.waitForFunction(() => document.activeElement?.textContent?.trim() === 'Keep the saved version', null, { timeout: 5_000 })
      .catch(async () => assert.fail(`the focus starts on ${await focused(page)}`));
    await page.keyboard.press('Tab');
    assert.equal(await focused(page), 'Overwrite with mine');
    await page.keyboard.press('Enter');

    await page.getByRole('heading', { level: 1 }).getByText('Order the tiles, kitchen only').waitFor({ timeout: 10_000 });
    assert.deepEqual(writes(state), [{ method: 'PATCH', path: `/items/${ENTRY.id}`, body: { title: 'Order the tiles, kitchen only' } }]);
    assert.equal(state.sent.find((each) => each.method === 'PATCH').ifMatch, `"${ENTRY.version + 1}"`);
    assert.deepEqual(failures, []);
  });

  test(`chromium: ${width} px — a change the stream has not brought yet: the refused save asks with both versions and keeps the text (UC-SYN-02 check 10)`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, context, failures, state, changeSilently } = await openEntry(browser, width);
    t.after(() => context.close());

    const notes = page.getByRole('textbox', { name: 'Notes' });
    await notes.fill(MINE);
    changeSilently({ notes: THEIRS });
    await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Edit title' }).focus();
    const dialog = await warningAbout(page, 'notes', THEIRS, MINE);
    // The one write that left was refused as stale; nothing else until the reader answers.
    assert.deepEqual(writes(state), [{ method: 'PATCH', path: `/items/${ENTRY.id}`, body: { notes: MINE } }]);
    await notes.filter({ hasText: MINE }).waitFor({ timeout: 5_000 })
      .catch(async () => assert.fail(`the refused save took the reader's text away: ${await notes.textContent()}`));
    await dialog.getByRole('button', { name: 'Overwrite with mine' }).click();
    await dialog.waitFor({ state: 'hidden', timeout: 10_000 });
    assert.equal(state.sent.filter((each) => each.method === 'PATCH').at(-1).ifMatch, `"${ENTRY.version + 1}"`);
    assert.equal(state.entry.notes, MINE);
    assert.deepEqual(failures, []);
  });

  test(`chromium: ${width} px — a title warning closed: the head shows the saved title, the field holds mine, and saving it asks again (UC-SYN-02 check 10)`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, context, failures, state, changeElsewhere } = await openEntry(browser, width);
    t.after(() => context.close());

    await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Edit title' }).click();
    const field = page.getByRole('textbox', { name: 'Title', exact: true });
    await field.fill('Order the tiles, kitchen only');
    await changeElsewhere({ title: 'Order the tiles, bathroom first' });
    await field.press('Enter');
    const dialog = await warningAbout(page, 'title', 'Order the tiles, bathroom first', 'Order the tiles, kitchen only');
    await page.keyboard.press('Escape');
    await dialog.waitFor({ state: 'hidden', timeout: 5_000 });

    await page.getByRole('heading', { level: 1 }).getByText('Order the tiles, bathroom first').waitFor({ timeout: 5_000 })
      .catch(async () => assert.fail(`the head shows ${await page.getByRole('heading', { level: 1 }).textContent()}`));
    await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Edit title' }).click();
    assert.equal(await field.inputValue(), 'Order the tiles, kitchen only', 'the reader’s unsaved title is not in the field');
    await field.press('Enter');
    await warningAbout(page, 'title', 'Order the tiles, bathroom first', 'Order the tiles, kitchen only');
    assert.deepEqual(writes(state), [], 'a title was written unasked');
    assert.deepEqual(failures, []);
  });

  test(`chromium: ${width} px — both texts changed meanwhile: the second question starts on the safe answer, and only what was overwritten is sent (UC-SYN-02 check 10)`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, context, failures, state, changeElsewhere } = await openEntry(browser, width);
    t.after(() => context.close());

    // The title asked about and left unanswered, then the description typed while it changes too.
    await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Edit title' }).click();
    const field = page.getByRole('textbox', { name: 'Title', exact: true });
    await field.fill('Order the tiles, kitchen only');
    await changeElsewhere({ title: 'Order the tiles, bathroom first' });
    await field.press('Enter');
    await warningAbout(page, 'title', 'Order the tiles, bathroom first', 'Order the tiles, kitchen only');
    await page.keyboard.press('Escape');
    await page.getByRole('alertdialog').waitFor({ state: 'hidden', timeout: 5_000 });
    await typeWhileChanged(page, changeElsewhere);
    const first = await warningAbout(page, 'title', 'Order the tiles, bathroom first', 'Order the tiles, kitchen only');
    await first.getByRole('button', { name: 'Overwrite with mine' }).click();

    await warningAbout(page, 'notes', THEIRS, MINE);
    await page.waitForFunction(() => document.activeElement?.textContent?.trim() === 'Keep the saved version', null, { timeout: 5_000 })
      .catch(async () => assert.fail(`the second question starts on ${await focused(page)}`));
    assert.deepEqual(writes(state), [], 'something was written before the second answer');
    await page.keyboard.press('Enter');
    await page.getByRole('alertdialog').waitFor({ state: 'hidden', timeout: 10_000 });
    assert.deepEqual(writes(state), [{ method: 'PATCH', path: `/items/${ENTRY.id}`, body: { title: 'Order the tiles, kitchen only' } }]);
    assert.deepEqual(failures, []);
  });

  test(`chromium: ${width} px — while the overwrite runs the warning stays, and its refusal is said there (UC-SYN-02 check 10)`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, context, state, changeElsewhere } = await openEntry(browser, width);
    t.after(() => context.close());

    await typeWhileChanged(page, changeElsewhere);
    const dialog = await warningAbout(page, 'notes', THEIRS, MINE);
    let release;
    state.patchHeld = new Promise((resolve) => (release = resolve));
    state.refusePatches = 1;
    const asked = page.waitForRequest((request) => request.method() === 'PATCH', { timeout: 10_000 });
    await dialog.getByRole('button', { name: 'Overwrite with mine' }).click();
    await asked;
    await page.keyboard.press('Escape');
    await dialog.getByRole('button', { name: 'Close', exact: true }).click();
    release();
    await dialog.getByText('Your version was not written; the saved one is still in place.').waitFor({ timeout: 10_000 })
      .catch(async () => assert.fail(`the refusal is not shown: ${await page.locator('main').innerText()}`));
    assert.ok(await dialog.isVisible(), 'the warning closed while the overwrite ran');
  });

  test(`chromium: ${width} px — a newer saved version while the warning is open: it shows the newest, says so, and overwrite writes over that one (UC-SYN-02 check 10)`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, context, failures, state, changeElsewhere } = await openEntry(browser, width);
    t.after(() => context.close());

    await typeWhileChanged(page, changeElsewhere);
    const dialog = await warningAbout(page, 'notes', THEIRS, MINE);
    await changeElsewhere({ notes: 'Tiles ordered online, delivery on Friday.' });
    await dialog.getByRole('region', { name: 'The saved version' }).getByText('Tiles ordered online, delivery on Friday.').waitFor({ timeout: 10_000 });
    await page.getByRole('status').filter({ hasText: 'The saved version changed again.' }).first().waitFor({ timeout: 5_000 })
      .catch(async () => assert.fail('the newer saved version was not said'));
    await dialog.getByRole('button', { name: 'Overwrite with mine' }).click();
    await dialog.waitFor({ state: 'hidden', timeout: 10_000 });
    assert.equal(state.sent.find((each) => each.method === 'PATCH').ifMatch, `"${ENTRY.version + 2}"`);
    assert.deepEqual(failures, []);
  });

  test(`chromium: ${width} px — a formatted description is shown in the warning as the editor shows it, not as its source (UC-SYN-02 check 10)`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, context, failures, changeElsewhere } = await openEntry(browser, width);
    t.after(() => context.close());

    const notes = page.getByRole('textbox', { name: 'Notes' });
    await notes.click();
    await page.keyboard.type('Ask ');
    await page.keyboard.press('ControlOrMeta+b');
    await page.keyboard.type('Ana');
    await page.keyboard.press('ControlOrMeta+b');
    await page.keyboard.type(' first.');
    await changeElsewhere({ notes: 'Tiles **ordered** online.' });
    await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Edit title' }).focus();
    const dialog = page.getByRole('alertdialog', { name: 'The description was changed meanwhile' });
    await dialog.waitFor({ timeout: 10_000 });
    const saved = dialog.getByRole('region', { name: 'The saved version' });
    const mine = dialog.getByRole('region', { name: 'Your version, not saved' });
    await saved.locator('strong').filter({ hasText: 'ordered' }).waitFor({ timeout: 10_000 })
      .catch(async () => assert.fail(`the saved version reads ${await saved.innerText()}`));
    await mine.locator('strong').filter({ hasText: 'Ana' }).waitFor({ timeout: 10_000 });
    assert.doesNotMatch(await dialog.innerText(), /\*\*/, 'Markdown source is shown');
    assert.deepEqual(failures, []);
  });
}

test('chromium: 1280 px — an entry in the pane with an unanswered warning: closing the pane asks before the text is dropped (UC-SYN-02 check 10)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, state, changeElsewhere } = await openEntry(browser, 1280, `/collections/${COLLECTION.id}?item=${ENTRY.id}`);
  t.after(() => context.close());

  await typeWhileChanged(page, changeElsewhere);
  const dialog = await warningAbout(page, 'notes', THEIRS, MINE);
  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
  await page.getByRole('button', { name: 'Close the pane' }).click();
  await page.getByRole('dialog', { name: 'Leave without saving?' }).waitFor({ timeout: 5_000 })
    .catch(async () => assert.fail(`closing the pane dropped the unsaved text: ${new URL(page.url()).search}`));
  assert.deepEqual(writes(state), []);
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

const DISPLACED_HEADING = /Diverging version from Jérôme Winkel, written on another device at .+\. This is the text that lost the merge; the entry carries the other one\./;

test('chromium: 1280 px — an address names a comment: opened at it the entry lands there, and one that is not there says so (UC-SYN-02 check 7)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures, state } = await openEntry(browser, 1280);
  t.after(() => context.close());
  const KEPT = keptComment('01a0e2e0-0000-7000-8000-0000000c0f31', 'Tiles from the big market,\n  ask for Ana.');
  state.comments.push(KEPT);

  await page.goto(`${served.origin}/items/${ENTRY.id}?comment=${KEPT.id}`);
  await landsOnKept(page, KEPT.id, DISPLACED_HEADING, KEPT.body);
  // It is the text a device lost, and it can be removed but not rewritten.
  await page.getByRole('article', { name: /^Kept version/ }).getByRole('button', { name: 'More for this comment' }).click();
  assert.equal(await page.getByRole('menuitem', { name: 'Edit' }).count(), 0, 'a kept version offers to be rewritten');
  await page.getByRole('menuitem', { name: 'Remove' }).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');

  await page.goto(`${served.origin}/items/${ENTRY.id}?comment=01a0e2e0-0000-7000-8000-0000000c0f99`);
  await page.getByRole('paragraph').filter({ hasText: 'This comment could not be found, or you may not see it.' }).waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('[data-marked]').count(), 0, 'another comment was marked in its place');
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — offline, the conflict’s link to the displaced version lands on it and decides nothing (UC-SYN-02 check 7)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures, state } = await openEntry(browser, 1280);
  t.after(() => context.close());

  const strip = await conflictWhileAway(page, context, state);
  await page.getByRole('button', { name: 'Show both versions' }).click();
  const dialog = page.getByRole('dialog', { name: 'Two versions of the notes' });
  await dialog.getByRole('link', { name: 'Your version, as a comment on the entry' }).click();

  await page.waitForURL((url) => url.pathname === `/items/${ENTRY.id}` && url.searchParams.get('comment') === DISPLACED, { timeout: 5_000 });
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
  await landsOnKept(page, DISPLACED, /Diverging version from Jérôme Winkel, written on this device at .+\. This is the text that lost the merge; the entry carries the other one\./, MINE);
  // Looking is not choosing: the conflict waits, and *Write mine again* is still offered.
  assert.equal(await strip.count(), 1, 'following the link decided the conflict');
  await page.getByRole('button', { name: 'Show both versions' }).click();
  await dialog.getByRole('button', { name: 'Write mine again' }).waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — back from a kept version and forward to it again, it is focused again (UC-SYN-02 check 7)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures, state } = await openEntry(browser, 1280);
  t.after(() => context.close());
  const KEPT = keptComment('01a0e2e0-0000-7000-8000-0000000c0f32', THEIRS);
  state.comments.push(KEPT);

  await page.goto(`${served.origin}/items/${ENTRY.id}?comment=${KEPT.id}`);
  await landsOnKept(page, KEPT.id, DISPLACED_HEADING, THEIRS);
  await page.goBack();
  await page.waitForURL((url) => !url.searchParams.has('comment'), { timeout: 5_000 });
  await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Edit title' }).focus();
  await page.goForward();
  await page.waitForURL((url) => url.searchParams.get('comment') === KEPT.id, { timeout: 5_000 });
  await landsOnKept(page, KEPT.id, DISPLACED_HEADING, THEIRS);
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — an address whose older comments could not be read is not said to be gone (UC-SYN-02 check 7)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, state } = await openEntry(browser, 1280);
  t.after(() => context.close());
  state.olderRefused = true;
  state.comments.push(keptComment('01a0e2e0-0000-7000-8000-0000000c0f41', THEIRS));

  await page.goto(`${served.origin}/items/${ENTRY.id}?comment=01a0e2e0-0000-7000-8000-0000000c0f99`);
  await page.getByText('You do not have permission for this action.').first().waitFor({ timeout: 10_000 })
    .catch(async () => assert.fail(`the failed read is not said: ${await page.locator('main').innerText()}`));
  assert.equal(await page.getByText('This comment could not be found, or you may not see it.').count(), 0, 'a comment on a page that did not arrive is said to be gone');
});

test('chromium: 1280 px — an address whose conversation could not be read at all is not said to be gone (UC-SYN-02 check 7)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, state } = await openEntry(browser, 1280);
  t.after(() => context.close());
  state.commentsRefused = true;
  let reads = 0;
  // The thread's own read and the search's read again: both refused.
  const searched = page.waitForResponse((response) => new URL(response.url()).pathname.endsWith(`/api/v1/items/${ENTRY.id}/comments`) && ++reads === 2, { timeout: 10_000 });

  await page.goto(`${served.origin}/items/${ENTRY.id}?comment=01a0e2e0-0000-7000-8000-0000000c0f99`);
  await page.getByText('You do not have permission for this action.').first().waitFor({ timeout: 10_000 })
    .catch(async () => assert.fail(`the failed read is not said: ${await page.locator('main').innerText()}`));
  await searched;
  assert.equal(await page.getByText('This comment could not be found, or you may not see it.').count(), 0, 'a comment in a conversation that did not arrive is said to be gone');
});

/** Notes written offline meet notes somebody else wrote meanwhile; the strip offers both versions. */
async function conflictWhileAway(page, context, state) {
  await state.isNamed;
  state.isOffline = true;
  await context.setOffline(true);
  await page.waitForFunction(() => document.querySelector('.trigger')?.getAttribute('data-connection') === 'offline', null, { timeout: 5_000 });
  await page.getByRole('textbox', { name: 'Notes' }).fill(MINE);
  await page.locator('[data-tour="entry"]').getByRole('button', { name: 'Edit title' }).focus();
  await page.waitForFunction(() => document.querySelector('.trigger')?.getAttribute('aria-label')?.includes('1 change is waiting to be sent'), null, { timeout: 5_000 })
    .catch(() => assert.fail('the notes were not kept as a waiting change'));
  state.theirsAway = THEIRS;
  state.isOffline = false;
  await context.setOffline(false);
  const strip = page.getByText('The notes were changed by somebody else while you were away.');
  await strip.waitFor({ timeout: 15_000 });
  return strip;
}

test('chromium: 1280 px — offline, write mine again writes the notes and files nothing (UC-SYN-02 check 7)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures, state } = await openEntry(browser, 1280);
  t.after(() => context.close());

  const strip = await conflictWhileAway(page, context, state);
  await page.getByRole('button', { name: 'Show both versions' }).click();
  await page.getByRole('dialog', { name: 'Two versions of the notes' }).getByRole('button', { name: 'Write mine again' }).click();
  await strip.waitFor({ state: 'hidden', timeout: 10_000 });
  assert.deepEqual(writes(state), [{ method: 'PATCH', path: `/items/${ENTRY.id}`, body: { notes: MINE } }]);
  assert.equal(state.sent.find((each) => each.method === 'PATCH').ifMatch, `"${ENTRY.version + 1}"`);
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — offline, write mine again refused keeps the conflict and says why (UC-SYN-02 check 7)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, state } = await openEntry(browser, 1280);
  t.after(() => context.close());

  const strip = await conflictWhileAway(page, context, state);
  state.refusePatches = 1;
  await page.getByRole('button', { name: 'Show both versions' }).click();
  await page.getByRole('dialog', { name: 'Two versions of the notes' }).getByRole('button', { name: 'Write mine again' }).click();
  await page.getByText('You do not have permission for this action.').waitFor({ timeout: 10_000 })
    .catch(async () => assert.fail(`the refusal is not said: ${await page.locator('main').innerText()}`));
  assert.equal(await strip.count(), 1, 'the refused choice dropped the conflict');
});

test('chromium: 1280 px — offline, write mine again while the entry cannot be read writes nothing and says why (UC-SYN-02 check 7)', async (t) => {
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
  assert.deepEqual(writes(state).slice(before), [], 'the notes were written without the entry’s current version');
  assert.equal(await strip.count(), 1, 'the conflict was dropped');
});
