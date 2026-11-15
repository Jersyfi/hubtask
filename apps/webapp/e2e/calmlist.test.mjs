// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A collection's list as a calm table (UC-WRK-45, WB-4, the work-surface concept's scene `queue`):
// one region with hairlines and the column heads, groups with their count, a fold and "+ entry in
// <column>" that opens capture there, one act and ⋯ on hover, the toolbar in words, the keys of a
// table, and on a phone a swipe that completes or opens the menu - which does the same. Also the
// rank of an entry by its menu, by key and by drag, and *Move inside* (UC-WRK-06 checks 1 and 3),
// each asserted by the request it sends. Chromium only; the touch path goes through the browser's
// own touch input.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { serve } from './serve.mjs';
import { BUCKETS, COLLECTION, ITEMS, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

/** The Kitchen's list, signed in, every write recorded with its body. */
async function open(browser, { width = 1280, height = 900, hasTouch = false, total } = {}) {
  const written = [];
  const { page, failures, context } = await signedIn(browser, width, height, { hasTouch });
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() !== 'GET' && !/(items:query|sync:pull|sync:snapshot|accounts:resolve|\/search)$/.test(path)) {
      written.push({ method: request.method(), path, body: request.postDataJSON?.() ?? null });
    }
    // A collection longer than its first page: the answer says there is more, and how much.
    if (total !== undefined && path.endsWith('/items:query')) {
      const longer = (options) => route.fulfill(options.json?.data ? { ...options, json: { ...options.json, total, page: { ...(options.json.page ?? {}), has_more: true, next_cursor: 'next' } } } : options);
      return stub(new Proxy(route, { get: (target, name) => (name === 'fulfill' ? longer : typeof target[name] === 'function' ? target[name].bind(target) : target[name]) }));
    }
    return stub(route);
  });
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.locator(`[data-row="${ITEMS[0].id}"]`).waitFor({ timeout: 15_000 });
  return { page, context, failures, written };
}

/** The write of `method` to a path ending in `suffix`, waited for: a loaded runner is slow to send. */
async function sent(written, method, suffix) {
  const find = () => written.find((each) => each.method === method && each.path.endsWith(suffix));
  for (const deadline = Date.now() + 10_000; !find() && Date.now() < deadline; ) await new Promise((done) => setTimeout(done, 50));
  return find();
}

/** A finger drawn sideways over a row, through the browser's own touch input. */
async function swipe(page, context, locator, direction) {
  const box = await locator.boundingBox();
  const y = box.y + box.height / 2;
  const from = box.x + box.width / 2;
  const cdp = await context.newCDPSession(page);
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: from, y }] });
  for (let step = 1; step <= 8; step += 1) {
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: from + direction * step * 18, y }] });
  }
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  await cdp.detach();
}

const tools = (page) => page.getByRole('group', { name: 'The list’s tools' });

test('chromium 1280: one region with the heads, the rows ruled apart, the toolbar in words (UC-WRK-45 checks 1 and 6)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures } = await open(browser);
  t.after(() => context.close());

  const region = page.getByRole('region', { name: `Entries in ${COLLECTION.name}` });
  assert.equal(await region.locator('[data-row]').count(), ITEMS.filter((each) => !each.parent_id).length, 'every row inside the one region');
  const heads = (await region.locator('.fields-head .col-head').allTextContents()).map((each) => each.trim().toLowerCase());
  for (const head of ['title', 'due', 'people', 'labels', 'parts']) assert.ok(heads.includes(head), `the head ${head} among ${heads}`);
  // Ruled apart, not cards with a gap: each row starts where the one above ends.
  const boxes = await region.locator('[data-row]').evaluateAll((rows) => rows.map((row) => row.getBoundingClientRect()));
  for (let index = 1; index < boxes.length; index += 1) {
    assert.ok(Math.abs(boxes[index].y - (boxes[index - 1].y + boxes[index - 1].height)) <= 1, `row ${index} stands ${boxes[index].y - boxes[index - 1].y - boxes[index - 1].height} px off the one above`);
  }
  assert.equal(await region.evaluate((element) => getComputedStyle(element).borderTopStyle), 'solid', 'the region has its hairline');

  // The four menus are one panel *View* (work-surface §6.2, E-21); the density and selecting stay.
  for (const name of ['View', 'Select entries']) {
    const button = tools(page).getByRole('button', { name, exact: false }).first();
    assert.equal(await button.count(), 1, `${name} in the list's tools`);
    assert.ok((await button.innerText()).trim().length > 2, `${name} says its words`);
  }
  assert.equal(await tools(page).getByRole('radiogroup', { name: 'Density' }).count(), 1, 'the density among the list\'s tools');
  assert.deepEqual(failures, []);
});

test('chromium 1280: grouped by column, each group counts, folds and adds in place (UC-WRK-45 check 2)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures, written } = await open(browser);
  t.after(() => context.close());

  await tools(page).getByRole('button', { name: 'View' }).click();
  await page.locator('[data-view-panel] [data-grouping="column"]').click();
  await page.keyboard.press('Escape');
  const first = BUCKETS[0].name;
  const head = page.locator('.level h2.group-head', { hasText: first });
  await head.waitFor({ timeout: 5_000 });
  const inFirst = ITEMS.filter((each) => each.bucket_id === BUCKETS[0].id && !each.completion?.is_completed).length;
  assert.equal((await head.locator('.group-count').innerText()).trim(), String(inFirst));

  const twist = head.getByRole('button', { name: `Fold “${first}”` });
  await twist.click();
  await page.waitForFunction((id) => !document.querySelector(`[data-row="${id}"]`), ITEMS[0].id, { timeout: 5_000 });
  await head.getByRole('button', { name: `Show “${first}”` }).click();
  await page.locator(`[data-row="${ITEMS[0].id}"]`).waitFor({ timeout: 5_000 });

  await page.getByRole('button', { name: `Entry in “${first}”` }).click();
  const dialog = page.getByRole('dialog', { name: 'Capture' });
  const field = dialog.getByRole('textbox', { name: 'What do you want to remember?' });
  await field.fill('Seal the grout');
  await field.press('Enter');
  const made = await sent(written, 'POST', '/items');
  assert.ok(made, 'capture created nothing');
  assert.equal(made.body.bucket_id, BUCKETS[0].id, 'the entry lands in the group\'s column');
  assert.deepEqual(failures, []);
});

test('chromium 1280: one act and ⋯ on hover, the menu opens the same; ↓, Space and Enter (UC-WRK-45 checks 3 and 5)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures } = await open(browser);
  t.after(() => context.close());

  const row = page.locator(`[data-row="${ITEMS[0].id}"]`);
  await page.mouse.move(0, 0);
  const act = row.getByRole('button', { name: 'Open', exact: true });
  assert.equal(await act.evaluate((element) => getComputedStyle(element.closest('.tools')).clipPath !== 'none'), true, 'the act waits for the pointer');
  await row.hover();
  await page.waitForFunction((id) => getComputedStyle(document.querySelector(`[data-row="${id}"] .tools`)).clipPath === 'none', ITEMS[0].id);
  await row.getByRole('button', { name: `Actions for ${ITEMS[0].title}` }).click();
  const menu = page.getByRole('menu', { name: `Actions for ${ITEMS[0].title}` });
  assert.equal(await menu.getByRole('menuitem', { name: 'Open', exact: true }).count(), 1, 'the menu does what the act does');
  await page.keyboard.press('Escape');

  // The keys of a table: ↓ walks to the next row's title, Space picks it, Enter opens it.
  await row.locator('.content').focus();
  await page.keyboard.press('ArrowDown');
  const second = ITEMS[1];
  assert.equal(await page.evaluate(() => document.activeElement?.closest('[data-row]')?.getAttribute('data-row')), second.id, '↓ reaches the next row');
  await page.keyboard.press('ArrowUp');
  assert.equal(await page.evaluate(() => document.activeElement?.closest('[data-row]')?.getAttribute('data-row')), ITEMS[0].id, '↑ comes back');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press(' ');
  const pick = page.getByRole('checkbox', { name: `Select ${second.title}` });
  await pick.waitFor({ timeout: 5_000 });
  assert.equal(await pick.isChecked(), true, 'Space picks the row');
  // Selecting leaves the entry as it was: the pick is not its completion.
  assert.equal(await page.locator(`[data-row="${second.id}"] .task-row`).getAttribute('data-completed'), null);
  await pick.click();
  assert.equal(await pick.isChecked(), false, 'the box only picks and unpicks');
  assert.equal(await page.locator(`[data-row="${second.id}"] .task-row`).getAttribute('data-completed'), null);

  // Enter on a row's title opens the entry - beside the list at this width.
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.locator(`[data-row="${ITEMS[2].id}"] .content`).focus();
  await page.keyboard.press('Enter');
  await page.waitForFunction((id) => location.pathname.includes(id) || location.search.includes(id), ITEMS[2].id, { timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('chromium 390: a row swiped right is done, swiped left opens its menu, which offers the same (UC-WRK-45 check 7)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures, written } = await open(browser, { width: 390, height: 844, hasTouch: true });
  t.after(() => context.close());

  assert.equal(await page.getByText('Swipe right to complete, left for the menu').count(), 1, 'the hint says it');
  const row = page.locator(`[data-row="${ITEMS[2].id}"]`);
  await swipe(page, context, row.locator('.content'), -1);
  const menu = page.getByRole('menu', { name: `Actions for ${ITEMS[2].title}` });
  await menu.waitFor({ timeout: 5_000 });
  assert.equal(await menu.getByRole('menuitem', { name: 'Mark as done' }).count(), 1, 'the menu completes too');
  await page.keyboard.press('Escape');
  await menu.waitFor({ state: 'detached', timeout: 5_000 });

  await swipe(page, context, row.locator('.content'), 1);
  assert.ok(await sent(written, 'POST', `/items/${ITEMS[2].id}:complete`), 'the swipe completed nothing');
  assert.equal(page.url().includes(ITEMS[2].id), false, 'the swipe opened the entry');
  assert.deepEqual(failures, []);
});

test('chromium 1280: an entry ranks by its menu, by Alt+↑ and by its grip; Move inside stands refused with its reason where the type may not go (UC-WRK-06 checks 1 and 3)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures, written } = await open(browser);
  t.after(() => context.close());

  const [first, second] = ITEMS;
  const menuOf = async (item) => {
    const row = page.locator(`[data-row="${item.id}"]`);
    await row.hover();
    await row.getByRole('button', { name: `Actions for ${item.title}` }).click();
    return page.getByRole('menu', { name: `Actions for ${item.title}` });
  };
  await (await menuOf(first)).getByRole('menuitem', { name: 'Move down' }).click();
  assert.ok(await sent(written, 'POST', `/items/${first.id}:reorder`), 'the menu ranked nothing');

  written.length = 0;
  await page.locator(`[data-row="${second.id}"] .content`).focus();
  await page.keyboard.press('Alt+ArrowUp');
  assert.ok(await sent(written, 'POST', `/items/${second.id}:reorder`), 'Alt+↑ ranked nothing');

  written.length = 0;
  const grip = page.locator(`[data-row="${first.id}"] [data-grip]`);
  await page.locator(`[data-row="${first.id}"]`).hover();
  const from = await grip.boundingBox();
  const to = await page.locator(`[data-row="${ITEMS[2].id}"]`).boundingBox();
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
  await page.mouse.down();
  for (let step = 1; step <= 10; step += 1) await page.mouse.move(from.x + from.width / 2, from.y + ((to.y + to.height - from.y) * step) / 10);
  await page.mouse.up();
  assert.ok(await sent(written, 'POST', `/items/${first.id}:reorder`), 'the drag ranked nothing');

  // *Move inside* only where the profile lets the type go there: a task under a task is offered
  // with its reason and sends nothing (the refusals' codes are the service's tests).
  written.length = 0;
  const inside = (await menuOf(second)).getByRole('menuitem', { name: /^Move inside/ });
  assert.equal(await inside.getAttribute('aria-disabled'), 'true');
  const reason = await inside.getAttribute('aria-describedby');
  assert.ok(reason && (await page.locator(`#${reason}`).textContent()).trim().length > 0, 'the refusal says why');
  await page.keyboard.press('Escape');
  assert.equal(written.some((each) => each.path.endsWith(':move')), false, 'a refused move was sent');
  assert.deepEqual(failures, []);
});

test('chromium 1280: a list longer than a page ends in "show more" at its foot, no page numbers (UC-WRK-45 check 8)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures } = await open(browser, { total: 40 });
  t.after(() => context.close());
  const region = page.getByRole('region', { name: `Entries in ${COLLECTION.name}` });
  const more = region.getByRole('button', { name: /^Show more - \d+ of 40 shown$/ });
  await more.waitFor({ timeout: 10_000 });
  const [foot, last] = [await more.boundingBox(), await region.locator('[data-row]').last().boundingBox()];
  assert.ok(foot.y >= last.y + last.height - 1, 'the control stands under the last row');
  assert.equal(await page.getByRole('navigation', { name: /page/i }).count(), 0, 'page numbers');
  assert.equal(await page.getByRole('link', { name: /^2$/ }).count(), 0, 'a page number');
  assert.deepEqual(failures, []);
});
