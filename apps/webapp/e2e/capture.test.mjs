// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Capture from anywhere (UC-WRK-26), walked against the shared fixture:
//
// * check 1 - `Q` and the bar's *Capture* open the field on every framed page; on a phone the
//   bottom bar's ＋ does; a reader who may create nowhere has none of the three, and a child with
//   one hub and no workspace role has all of them;
// * check 2 - the story's line shows its chips, and a chip taken back stays in the title sent;
// * check 3 - a collection set up as a list takes no date, person or label, and splits lines;
// * check 4 - without a target the entry goes to the collection the reader is in, else the jumble;
//   the message names the place, links to it and undoes it;
// * check 5 - the due date sent is the person's tomorrow, in their zone, near midnight;
// * check 6 - by keyboard alone, and what was recognised is said in the frame's live region.
//
// Chromium; the keyboard sequence also runs in all three engines in `engines.test.mjs`.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, COLLECTION, CREATED_ID, GROCERIES, HUB, ITEMS, JUMBLED, LABELS, MANIFEST, OTHER, PAGE, PERMISSIONS, signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const FIELD = 'What do you want to remember?';
const ZONE = 'Pacific/Kiritimati';
/** 23:59:30 on 21 October 2026 in Kiritimati (UTC+14): the person's tomorrow is the 22nd. */
const NEAR_MIDNIGHT = Date.parse('2026-10-21T09:59:30Z');

const dialogOf = (page) => page.getByRole('dialog', { name: 'Capture' });
const fieldOf = (page) => dialogOf(page).getByRole('textbox', { name: FIELD });

/** The writes the page sent, as `METHOD path` with the body. */
function recordWrites(page) {
  const writes = [];
  page.on('request', (request) => {
    if (request.method() === 'GET') return;
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path.startsWith('/stream') || path.startsWith('/sync')) return;
    writes.push({ call: `${request.method()} ${path}`, body: request.postDataJSON?.() ?? null });
  });
  return writes;
}

/** Waits until a write matching `call` was sent. */
async function sent(page, writes, call) {
  for (let waited = 0; waited < 50; waited++) {
    const found = writes.find((each) => each.call === call);
    if (found) return found;
    await page.waitForTimeout(100);
  }
  assert.fail(`${call} was not sent: ${writes.map((each) => each.call).join(', ')}`);
}

/** Lands on a page and lets the roles arrive, then puts focus on the page, outside any field. */
async function land(page, path) {
  await page.goto(`${served.origin}${path}`);
  await page.getByRole('main').waitFor();
  await page.waitForTimeout(600);
  await page.locator('body').click({ position: { x: 1, y: 300 } });
}

/** A signed-in tab whose reader holds `permissions` on the workspace and `hubRole` on House. */
async function as(browser, permissions, hubRole, width = 1280) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: permissions } });
    if (path === '/memberships') {
      const scope = url.searchParams.get('scope_type');
      const rows =
        scope === 'HUB' && hubRole ? [{ id: 'm2', scope_type: 'HUB', scope_id: HUB.id, account_id: ACCOUNT.id, role: hubRole }] : [];
      return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: [] } });
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
  return { context, page, failures };
}

test('chromium: Q and the bar’s Capture open the field on every framed page (check 1)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280);
  t.after(() => context.close());

  for (const path of [`/collections/${COLLECTION.id}`, `/items/${ITEMS[0].id}`, '/search', '/profile', '/jumble', `/hubs/${HUB.id}`]) {
    await land(page, path);
    await page.keyboard.press('q');
    await fieldOf(page).waitFor({ timeout: 5_000 });
    assert.equal(await page.evaluate(() => document.activeElement?.tagName), 'TEXTAREA', `Q on ${path}: focus is not in the field`);
    // `Q` in the field is a letter, not the shortcut.
    await page.keyboard.press('q');
    assert.equal(await fieldOf(page).inputValue(), 'q');
    await page.keyboard.press('Escape');
    await dialogOf(page).waitFor({ state: 'hidden', timeout: 5_000 });

    await page.getByRole('banner').getByRole('button', { name: 'Capture' }).click();
    await fieldOf(page).waitFor({ timeout: 5_000 });
    assert.equal(await fieldOf(page).inputValue(), '', 'the field opens empty');
    await page.keyboard.press('Escape');
    await dialogOf(page).waitFor({ state: 'hidden', timeout: 5_000 });
  }
  assert.equal(await page.getByRole('banner').getByRole('button', { name: 'Capture' }).getAttribute('aria-keyshortcuts'), 'Q');
  assert.deepEqual(failures, []);
});

test('chromium: 375 px — the bottom bar’s ＋ opens the field (check 1)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 375, 800, { hasTouch: true });
  t.after(() => context.close());

  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.getByText(ITEMS[2].title).first().waitFor();
  const bar = page.getByRole('navigation', { name: 'Sections' });
  const names = await bar.locator('li').allTextContents();
  assert.deepEqual(names.map((name) => name.trim()), ['Overview', 'Search', 'Capture', 'Jumble', 'You']);
  await bar.getByRole('button', { name: 'Capture' }).tap();
  await fieldOf(page).waitFor({ timeout: 5_000 });
  assert.equal(await page.getByRole('banner').getByRole('button', { name: 'Capture' }).count(), 0, 'the bar carries Capture on a phone too');
  assert.deepEqual(failures, []);
});

test('chromium: the story — chips, the entry in the right place on the person’s tomorrow, Open and Undo (checks 2, 4, 5)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280, 900, { timezoneId: ZONE });
  t.after(() => context.close());
  await context.clock.setFixedTime(NEAR_MIDNIGHT);
  const writes = recordWrites(page);

  await land(page, `/collections/${COLLECTION.id}`);
  await page.keyboard.press('q');
  await fieldOf(page).fill('Call the electrician tomorrow 11am +Mara !Urgent');
  const chips = dialogOf(page).getByRole('list', { name: 'Recognised' }).getByRole('listitem');
  await page.waitForFunction(() => document.querySelectorAll('dialog[open] ul li').length === 4, null, { timeout: 5_000 });
  assert.equal(await chips.count(), 4);
  assert.ok((await dialogOf(page).textContent()).includes('Into Kitchen in House'), 'the target line does not name Kitchen');

  await page.keyboard.press('Enter');
  const create = await sent(page, writes, 'POST /items');
  assert.equal(create.body.collection_id, COLLECTION.id);
  assert.equal(create.body.title, 'Call the electrician');
  assert.equal(create.body.assignee_id, OTHER.id);
  assert.deepEqual(create.body.label_ids, [LABELS.find((label) => label.name === 'Urgent').id]);
  assert.equal(create.body.due_time_zone, ZONE);
  assert.equal(create.body.due_date_only, false);
  // 11:00 on the 22nd in Kiritimati is 21:00 on the 21st in Greenwich.
  assert.equal(create.body.due_at, '2026-10-21T21:00:00.000Z');

  await dialogOf(page).waitFor({ state: 'hidden', timeout: 5_000 });
  const message = page.locator('.stack').getByText('Added to Kitchen');
  await message.waitFor({ timeout: 5_000 });
  assert.equal(await page.getByRole('link', { name: 'Open' }).getAttribute('href'), `/items/${CREATED_ID}`);
  await page.getByRole('button', { name: 'Undo' }).click();
  await sent(page, writes, `DELETE /items/${CREATED_ID}`);

  // A chip taken back stays in the title sent.
  await land(page, `/collections/${COLLECTION.id}`);
  await page.keyboard.press('q');
  await fieldOf(page).fill('Call the electrician tomorrow');
  await dialogOf(page).getByRole('button', { name: 'Keep “tomorrow” in the title' }).click();
  await page.waitForFunction(() => document.querySelectorAll('dialog[open] ul li').length === 0, null, { timeout: 5_000 });
  const before = writes.length;
  await fieldOf(page).press('Enter');
  await sent(page, writes.slice(before), 'POST /items').catch(() => {});
  const second = writes.slice(before).find((each) => each.call === 'POST /items');
  assert.equal(second?.body.title, 'Call the electrician tomorrow');
  assert.equal(second?.body.due_at, undefined);
  assert.deepEqual(failures, []);
});

test('chromium: without a collection the jumble, and its Undo dismisses (check 4)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280);
  t.after(() => context.close());
  const writes = recordWrites(page);

  await land(page, '/search');
  await page.keyboard.press('q');
  await fieldOf(page).fill('Call the plumber tomorrow');
  assert.ok((await dialogOf(page).textContent()).includes('Into the jumble'));
  // The jumble decides nothing: no chip for the day.
  assert.equal(await dialogOf(page).getByRole('list', { name: 'Recognised' }).count(), 0);
  await page.keyboard.press('Enter');
  const submitted = await sent(page, writes, 'POST /jumble/entries');
  assert.deepEqual(submitted.body, { channel: 'QUICK_CAPTURE', raw_subject: 'Call the plumber tomorrow' });
  await page.locator('.stack').getByText('Put in the jumble').waitFor({ timeout: 5_000 });
  await page.getByRole('button', { name: 'Undo' }).click();
  await sent(page, writes, `POST /jumble/entries/${JUMBLED.id}:dismiss`);
  await page.locator('.stack').getByText(/^Dismissed\./).waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('chromium: a list takes no date, person or label, and makes one entry per line; Undo is one bulk (check 3)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280);
  t.after(() => context.close());
  await context.route('**/api/v1/containers**', async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith(`/containers/${GROCERIES.id}`)) return route.fulfill({ json: GROCERIES });
    if (url.pathname.endsWith('/containers') && url.searchParams.get('parent_id') === HUB.id) {
      return route.fulfill({ json: { ...PAGE, data: [COLLECTION, GROCERIES] } });
    }
    return stub(route);
  });
  const writes = recordWrites(page);

  await land(page, `/collections/${GROCERIES.id}`);
  await page.keyboard.press('q');
  await fieldOf(page).fill('Milk tomorrow +Mara !Urgent');
  await page.waitForTimeout(500);
  assert.ok((await dialogOf(page).textContent()).includes('Into Groceries'));
  assert.equal(await dialogOf(page).getByRole('list', { name: 'Recognised' }).count(), 0, 'a list recognised something');
  await fieldOf(page).press('Shift+Enter');
  await fieldOf(page).pressSequentially('Eggs');
  await fieldOf(page).press('Enter');
  for (let waited = 0; writes.filter((each) => each.call === 'POST /items').length < 2 && waited < 50; waited++) await page.waitForTimeout(100);
  const creates = writes.filter((each) => each.call === 'POST /items');
  assert.deepEqual(creates.map((each) => each.body.title), ['Milk tomorrow +Mara !Urgent', 'Eggs']);
  assert.ok(creates.every((each) => each.body.collection_id === GROCERIES.id && each.body.due_at === undefined));
  await page.locator('.stack').getByText('2 entries added to Groceries').waitFor({ timeout: 5_000 });
  await page.getByRole('button', { name: 'Undo' }).click();
  const bulk = await sent(page, writes, 'POST /items:bulk');
  assert.equal(bulk.body.atomic, true);
  assert.deepEqual(bulk.body.operations.map((each) => each.op), ['TRASH_ITEM', 'TRASH_ITEM']);
  assert.deepEqual(failures, []);
});

test('chromium: a reader who may create nowhere has no capture at all (check 1, P-05)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  for (const width of [1280, 375]) {
    const { context, page, failures } = await as(browser, PERMISSIONS.VIEWER, 'VIEWER', width);
    await land(page, `/collections/${COLLECTION.id}`);
    await page.waitForTimeout(600);
    await page.keyboard.press('q');
    await page.waitForTimeout(300);
    assert.equal(await dialogOf(page).count(), 0, `${width}: Q opened a field`);
    assert.equal(await page.getByRole('button', { name: 'Capture' }).count(), 0, `${width}: a capture control is drawn`);
    assert.deepEqual(failures, []);
    await context.close();
  }
});

test('chromium: a child with one hub and no workspace role captures into their own collection (check 1)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await as(browser, PERMISSIONS.BELOW, 'MEMBER');
  t.after(() => context.close());
  const writes = recordWrites(page);

  await land(page, '/search');
  const button = page.getByRole('banner').getByRole('button', { name: 'Capture' });
  await button.waitFor({ timeout: 10_000 });
  await button.click();
  await fieldOf(page).fill('Homework');
  await page.waitForTimeout(300);
  assert.ok((await dialogOf(page).textContent()).includes('Nowhere yet'), 'no current collection and no jumble: nowhere yet');
  await page.keyboard.press('Enter');
  assert.ok((await dialogOf(page).textContent()).includes('Nowhere yet'));
  assert.equal(writes.filter((each) => each.call.startsWith('POST /items') || each.call.startsWith('POST /jumble')).length, 0);

  await fieldOf(page).fill('Homework @Kitchen');
  await page.waitForFunction(() => document.querySelector('dialog[open]')?.textContent?.includes('Into Kitchen in House'), null, { timeout: 5_000 });
  await page.keyboard.press('Enter');
  const create = await sent(page, writes, 'POST /items');
  assert.equal(create.body.collection_id, COLLECTION.id);
  assert.equal(create.body.title, 'Homework');
  assert.deepEqual(failures, []);
});

test('chromium: by keyboard alone, and said to a screen reader (check 6)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280);
  t.after(() => context.close());
  const writes = recordWrites(page);

  await land(page, `/collections/${COLLECTION.id}`);
  // Focus on something real before Q, to see it come back.
  await page.getByRole('banner').getByRole('button', { name: 'Capture' }).focus();
  await page.keyboard.press('Tab');
  const before = await page.evaluate(() => document.activeElement?.outerHTML.slice(0, 80));
  await page.keyboard.press('q');
  await fieldOf(page).waitFor();
  await page.keyboard.type('Order tiles !Materials tomorrow');
  // What was recognised is said once typing pauses, through the frame's one live region.
  await page.waitForFunction(() => (document.querySelector('.announcement[role="status"]')?.textContent ?? '').includes('Into Kitchen'), null, { timeout: 5_000 });
  const said = await page.locator('.announcement[role="status"]').textContent();
  assert.ok(said.includes('Materials'), `the live region says: ${said}`);

  // Tab to the first chip's control and take it back by Enter: focus returns to the field, where
  // Enter captures.
  await page.keyboard.press('Tab');
  const focused = await page.evaluate(() => document.activeElement?.getAttribute('aria-label'));
  assert.match(focused ?? '', /^Keep “/);
  await page.keyboard.press('Enter');
  assert.equal(await page.evaluate(() => document.activeElement?.tagName), 'TEXTAREA', 'focus did not come back to the field');
  await page.keyboard.press('Enter');
  const create = await sent(page, writes, 'POST /items');
  assert.ok(create.body.title.startsWith('Order tiles'));

  // Escape returns focus where it was.
  await page.keyboard.press('q');
  await fieldOf(page).waitFor();
  await page.keyboard.press('Escape');
  await dialogOf(page).waitFor({ state: 'hidden' });
  const after = await page.evaluate(() => document.activeElement?.outerHTML.slice(0, 80));
  assert.equal(after, before);
  assert.deepEqual(failures, []);
});
