// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-WRK-25 on a phone (checks 1-6): the grocery list of the shared fixture at 375 × 812 with a
// touch screen, online and offline. Lines typed into capture become entries, a leading amount
// becomes the entry's amount, a tap on a row's start or a swipe ticks it and a tap on the rest opens
// it, a still hold opens the entry menu and ticks nothing, a ticked
// entry moves under the collection's name for *Done*, an entry under *Recently* comes back as the
// same entry, open entries sit under their categories in order, and offline the ticks and the
// captures queue and are pushed when the network returns.
//
// Chromium only: what is walked here is the app's own gesture code and layout, not an engine
// feature; the engines job loads the bundle in all three.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, GROCERIES, GROCERY_ITEMS, GROCERY_LABELS, PAGE, pushed, resetGroceries, signedIn } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
const browser = await chromium.launch();
test.after(async () => {
  await browser.close();
  await served.close();
});

const idOf = (title) => GROCERY_ITEMS.find((entry) => entry.title === title)?.id;

/**
 * The list, signed in on a phone, with every write request it makes recorded. With `role` the
 * reader holds that role on the workspace and nothing below it, as `entrybyrole` stubs one.
 */
async function phone(role) {
  resetGroceries();
  const session = await signedIn(browser, 375, 812, { hasTouch: true });
  if (role) {
    await session.context.route('**/api/v1/memberships**', (route) => {
      const scope = new URL(route.request().url()).searchParams.get('scope_type');
      const rows = scope === 'TENANT' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role }] : [];
      return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: [] } });
    });
  }
  const writes = [];
  session.page.on('request', (request) => {
    if (request.method() !== 'GET' && !request.url().includes(':query') && !request.url().includes('sync:')) {
      writes.push({ method: request.method(), path: new URL(request.url()).pathname, body: request.postDataJSON?.() ?? null });
    }
  });
  await session.page.goto(`${served.origin}/collections/${GROCERIES.id}`);
  await session.page.locator('[data-row]').first().waitFor();
  return { ...session, writes };
}

/** A finger on a row: down, the moves, up - pointer events of a touch, as the browser sends them. */
async function finger(page, rowId, moves, holdMs = 0) {
  const box = await page.locator(`[data-row="${rowId}"]`).boundingBox();
  const start = { x: box.x + box.width / 2, y: box.y + box.height / 2 };
  await page.evaluate(
    async ({ id, start, moves, holdMs }) => {
      const row = document.querySelector(`[data-row="${id}"] .title`) ?? document.querySelector(`[data-row="${id}"]`);
      const fire = (type, x, y) =>
        row.dispatchEvent(new PointerEvent(type, { bubbles: true, cancelable: true, pointerType: 'touch', isPrimary: true, clientX: x, clientY: y }));
      fire('pointerdown', start.x, start.y);
      if (holdMs > 0) await new Promise((done) => setTimeout(done, holdMs));
      for (const [dx, dy] of moves) {
        await new Promise((done) => setTimeout(done, 16));
        fire('pointermove', start.x + dx, start.y + dy);
      }
      const [dx, dy] = moves.at(-1) ?? [0, 0];
      fire('pointerup', start.x + dx, start.y + dy);
    },
    { id: rowId, start, moves, holdMs },
  );
}

/** Capture, from the bottom bar's ＋: pointed at the list the phone stands in. */
async function captureField(page) {
  await page.getByRole('navigation', { name: 'Sections' }).getByRole('button', { name: 'Capture' }).tap();
  const field = page.getByRole('dialog', { name: 'Capture' }).getByRole('textbox', { name: 'What do you want to remember?' });
  await field.waitFor({ timeout: 5_000 });
  return field;
}

const sectionOf = (page, rowId) =>
  page.evaluate((id) => {
    let node = document.querySelector(`[data-row="${id}"]`);
    while (node && !node.matches?.('h2.group-head, .section-row')) node = node.previousElementSibling;
    return (node?.matches('.section-row') ? node.querySelector('h2') : node)?.querySelector('span')?.textContent?.trim();
  }, rowId);

test('several lines become one entry each, and a leading amount becomes the amount (checks 1, 2)', async () => {
  const { page, writes, failures, context } = await phone();
  const field = await captureField(page);
  await field.fill('3 eggs');
  await page.getByRole('button', { name: 'Keep “3” in the title' }).waitFor();
  await field.fill('- 2 milk\n\nbread\n1 kg apples');
  await field.press('Enter');
  await page.waitForFunction(() => document.querySelector('[data-row]') !== null);
  await page.waitForTimeout(500);
  const created = writes.filter((write) => write.method === 'POST' && write.path === '/api/v1/items');
  assert.deepEqual(
    created.map((write) => [write.body.title, write.body.amount ?? null, write.body.collection_id]),
    [
      ['milk', '2', GROCERIES.id],
      ['bread', null, GROCERIES.id],
      ['apples', '1 kg', GROCERIES.id],
    ],
  );
  assert.equal(await field.inputValue(), '', 'the created lines leave the field');
  assert.equal(await field.isVisible(), true, 'a list keeps capture open for the next line');
  await page.keyboard.press('Escape');
  // The amount is the list's field: a chip beside the title (concept round 2, §2).
  const apples = page.locator(`[data-row="${idOf('apples')}"]`);
  assert.equal(await apples.locator('[data-field="amount"]').innerText(), '1 kg');
  assert.match(await apples.innerText(), /apples/);
  assert.deepEqual(failures, []);
  await context.close();
});

test('open entries sit under their categories in the shop\'s order (check 5)', async () => {
  const { page, context } = await phone();
  // One group head in every list (concept round 2, §2): its name, then how many.
  const heads = await page.locator('.level h2.group-head > span:first-child').allInnerTexts();
  assert.deepEqual(heads, ['Produce', 'Dairy', 'Bakery', 'Other', 'In the trolley', 'Recently']);
  assert.equal(await sectionOf(page, idOf('milk')), 'Dairy');
  assert.equal(await sectionOf(page, idOf('salt')), 'Other');
  await context.close();
});

test('a tap on the row\'s start ticks it once, on a target of at least 44 by 44, and it moves to the trolley (checks 3, 6)', async () => {
  const { page, writes, context } = await phone();
  const row = page.locator(`[data-row="${idOf('salt')}"]`);
  const box = await row.locator('.leading').boundingBox();
  assert.ok(box.width >= 44 && box.height >= 44, `the tick target is ${box.width} × ${box.height}`);

  await row.locator('.leading').tap({ position: { x: 2, y: 2 } });
  await page.waitForFunction((id) => {
    let node = document.querySelector(`[data-row="${id}"]`);
    while (node && !node.matches?.('h2.group-head, .section-row')) node = node.previousElementSibling;
    return (node?.matches('.section-row') ? node.querySelector('h2') : node)?.querySelector('span')?.textContent?.trim() === 'In the trolley';
  }, idOf('salt'));
  // The checkbox inside the row is the checkbox's own: one press, one tick.
  await page.locator(`[data-row="${idOf('bread')}"] input[type="checkbox"]`).tap();
  await page.waitForTimeout(400);
  const ticks = writes.filter((write) => write.path.endsWith(':complete'));
  assert.deepEqual(ticks.map((write) => write.path.split('/').pop()), [`${idOf('salt')}:complete`, `${idOf('bread')}:complete`]);
  // The rest of the row opens the entry, and ticks nothing.
  await page.locator(`[data-row="${idOf('milk')}"] .title`).tap();
  await page.waitForURL(new RegExp(`/items/${idOf('milk')}$`, 'u'), { timeout: 5_000 });
  assert.equal(writes.filter((write) => write.path.endsWith(`${idOf('milk')}:complete`)).length, 0, 'opening ticked it');
  await context.close();
});

test('a swipe ticks, and a still hold opens the entry menu with Select first and ticks nothing (check 6, ADR-0088)', async () => {
  const { page, writes, context } = await phone();
  await finger(page, idOf('apples'), [[20, 0], [60, 2], [90, 4]]);
  await page.waitForTimeout(400);
  assert.ok(writes.some((write) => write.path.endsWith(`${idOf('apples')}:complete`)), 'the swipe did not tick');

  const before = writes.length;
  await finger(page, idOf('milk'), [], 450);
  // The entry menu every surface draws (UC-WRK-28 check 1), with the list's own items after Open
  // and no level to move inside or out of.
  const menu = page.getByRole('menu', { name: 'Actions for milk' });
  await menu.waitFor();
  const items = (await menu.getByRole('menuitem').allInnerTexts()).map((each) => each.trim());
  assert.deepEqual(items.slice(0, 4), ['Select', 'Open', 'Amount', 'Category']);
  for (const shared of ['Move up', 'Move to another collection…', 'Copy link', 'Move to the trash']) {
    // An item that is off carries its reason under its name.
    assert.ok(items.some((each) => each.split('\n')[0] === shared), `${shared} is missing: ${items.join(', ')}`);
  }
  assert.ok(!items.some((each) => each.startsWith('Move inside') || each === 'Move out one level'), items.join(', '));
  await page.waitForTimeout(300);
  assert.equal(writes.slice(before).filter((write) => write.path.includes(':complete')).length, 0, 'the hold ticked');

  // A right click opens the same menu, as on every surface (UC-WRK-28 check 1).
  await page.keyboard.press('Escape');
  await page.locator(`[data-row="${idOf('bread')}"] .title`).click({ button: 'right' });
  await page.getByRole('menu', { name: 'Actions for bread' }).waitFor();
  assert.equal(writes.slice(before).filter((write) => write.path.includes(':complete')).length, 0, 'the right click ticked');
  await context.close();
});

test('an entry under Recently comes back as the same entry, no new one (check 4)', async () => {
  const { page, writes, context } = await phone();
  // Most used first.
  const recently = await page.evaluate(() => {
    const head = [...document.querySelectorAll('h2.group-head')].find((each) => each.querySelector('span')?.textContent.trim() === 'Recently');
    const ids = [];
    for (let node = head?.nextElementSibling; node && !node.matches('h2'); node = node.nextElementSibling) {
      if (node.dataset.row) ids.push(node.dataset.row);
    }
    return ids;
  });
  assert.deepEqual(recently, [idOf('oat milk'), idOf('coffee'), idOf('tea')]);

  // What capture holds is matched against it, under the field.
  await (await captureField(page)).fill('oat');
  const before = page.getByRole('dialog', { name: 'Capture' }).getByRole('region', { name: 'Ticked before' });
  await before.waitFor({ timeout: 5_000 });
  assert.equal(await before.getByRole('button', { name: /tea/u }).count(), 0);

  await before.getByRole('button', { name: 'Put oat milk back on the list' }).tap();
  await page.waitForTimeout(400);
  assert.ok(writes.some((write) => write.path.endsWith(`${idOf('oat milk')}:reopen`)));
  assert.equal(writes.filter((write) => write.method === 'POST' && write.path === '/api/v1/items').length, 0);
  await context.close();
});

test('the entry page shows and changes the amount where the list offers one (check 2)', async () => {
  const { page, writes, context } = await phone();
  await page.goto(`${served.origin}/items/${idOf('milk')}`);
  const row = page.locator('[data-detail="amount"]');
  await row.waitFor({ state: 'attached' });
  // On a phone the details are folded under the head.
  await page.locator('summary.details-summary').click();
  assert.match(await row.innerText(), /2/);
  await row.click();
  const field = page.getByRole('textbox', { name: 'Amount', exact: true });
  await field.fill('3');
  await page.getByRole('button', { name: 'Save' }).click();
  await page.waitForTimeout(400);
  assert.ok(writes.some((write) => write.method === 'PATCH' && write.path.endsWith(idOf('milk')) && write.body?.amount === '3'));

  // A collection that does not offer an amount draws no such row.
  await page.goto(`${served.origin}/items/01a0e2e0-0000-7000-8000-000000000033`);
  await page.locator('[data-detail]').first().waitFor({ state: 'attached' });
  assert.equal(await page.locator('[data-detail="amount"]').count(), 0);
  await context.close();
});

test('a holder of STRUCTURE names the done section and orders the categories where the list is used (checks 3, 5)', async () => {
  const { page, writes, context } = await phone();
  await page.getByRole('button', { name: 'Rename the section of done entries' }).click();
  await page.getByLabel('What done entries are shown under').fill('Im Wagen');
  await page.getByRole('button', { name: 'Save', exact: true }).first().click();
  await page.waitForTimeout(400);
  const renamed = writes.find((write) => write.method === 'PATCH' && write.path.endsWith(`/containers/${GROCERIES.id}/structure`));
  assert.deepEqual(renamed?.body?.settings?.levels, [{ type: 'TASK', name: null, done_name: 'Im Wagen' }]);

  await page.getByRole('button', { name: 'Categories' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Bakery').uncheck();
  await dialog.getByRole('button', { name: 'Move Dairy up' }).click();
  await dialog.getByRole('button', { name: 'Save', exact: true }).click();
  await page.waitForTimeout(400);
  const ordered = writes.filter((write) => write.method === 'PATCH' && write.path.endsWith('/structure')).at(-1);
  assert.deepEqual(ordered?.body?.settings?.fields?.TASK?.categories, [GROCERY_LABELS[1].id, GROCERY_LABELS[0].id]);
  assert.ok(ordered?.body?.settings?.fields?.TASK?.capabilities.includes('LABELS'));
  await context.close();
});

// The collection's structure can change while its categories are being chosen - the answer to
// the rename just made, or another device's change arriving by sync. The choice in the open dialog
// is the person's and stays; on a slow runner the late answer used to reset it.
test('a structure that arrives while the categories are chosen keeps the choice (check 5)', async () => {
  const { page, writes, context } = await phone();
  let release;
  const held = new Promise((done) => (release = done));
  await context.route('**/containers/*/structure', async (route) => {
    if (route.request().method() !== 'PATCH') return route.fallback();
    await held;
    return route.fallback();
  });
  await page.getByRole('button', { name: 'Rename the section of done entries' }).click();
  await page.getByLabel('What done entries are shown under').fill('Im Wagen');
  await page.getByRole('button', { name: 'Save', exact: true }).first().click();

  await page.getByRole('button', { name: 'Categories' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Bakery').uncheck();
  release();
  await page.waitForResponse((response) => response.url().endsWith('/structure') && response.request().method() === 'PATCH');
  await page.waitForTimeout(400);
  assert.equal(await dialog.getByLabel('Bakery').isChecked(), false, 'the arriving structure ticked Bakery again');

  await dialog.getByRole('button', { name: 'Save', exact: true }).click();
  await page.waitForTimeout(400);
  const ordered = writes.filter((write) => write.method === 'PATCH' && write.path.endsWith('/structure')).at(-1);
  assert.deepEqual(ordered?.body?.settings?.fields?.TASK?.categories, [GROCERY_LABELS[0].id, GROCERY_LABELS[1].id]);
  await context.close();
});

test('a viewer reads the list and is offered nothing that writes (P-05)', async () => {
  const { page, writes, context } = await phone('VIEWER');
  await page.waitForTimeout(800);
  assert.equal(await page.locator('main').getByRole('button', { name: /^Add/u }).count(), 0, 'an add for a viewer');
  assert.equal(await page.getByRole('button', { name: 'Categories' }).count(), 0);
  assert.equal(await page.getByRole('button', { name: 'Rename the section of done entries' }).count(), 0);
  await page.locator(`[data-row="${idOf('salt')}"] .leading`).tap({ position: { x: 2, y: 2 } });
  await finger(page, idOf('apples'), [[20, 0], [60, 2], [90, 4]]);
  await page.waitForTimeout(400);
  assert.equal(writes.filter((write) => write.path.includes(':complete')).length, 0, 'a viewer ticked');
  await context.close();
});

test('the list\'s head counts what is open and shows no percentage, and offline no figure (check 7)', async () => {
  const { page, context } = await phone();
  const open = GROCERY_ITEMS.filter((entry) => !entry.completion?.is_completed).length;
  const figure = page.locator('[data-figure]').first();
  await figure.waitFor({ timeout: 5_000 });
  assert.equal((await figure.locator('.text').textContent())?.trim(), `${open} open`);
  assert.doesNotMatch(await page.locator('main').innerText(), /\d\s?%/, 'a list shows a percentage');

  // Offline the replica answers the list, and a figure is the server's alone (UX-24 D10).
  await context.route('**/api/v1/containers**', (route) => route.abort('internetdisconnected'));
  await page.reload();
  await page.locator('[data-row]').first().waitFor({ timeout: 15_000 });
  assert.equal(await page.locator('[data-figure]').count(), 0, 'a figure was drawn from the replica');
  await context.close();
});

test('offline, ticks and captures wait and are sent when the network returns (check 6)', async () => {
  const { page, context } = await phone();
  pushed.length = 0;
  // A route answers whatever the browser's own offline switch says, so the stub is cut off too:
  // while the device is offline, every request to the server fails as a dropped network would.
  let isOffline = false;
  await context.route('**/api/v1/**', (route) => (isOffline ? route.abort('internetdisconnected') : route.fallback()));
  isOffline = true;
  await context.setOffline(true);
  await page.waitForFunction(() => document.querySelector('.trigger')?.getAttribute('data-connection') === 'offline', null, { timeout: 5_000 });

  await page.locator(`[data-row="${idOf('bread')}"] .leading`).tap({ position: { x: 2, y: 2 } });
  const field = await captureField(page);
  await field.fill('2 lemons');
  await field.press('Enter');
  // Both writes wait in the queue, which the sync mark counts.
  await page.waitForFunction(() => document.body.textContent?.includes('2 changes are waiting to be sent'), null, { timeout: 5_000 });

  isOffline = false;
  await context.setOffline(false);
  await page.waitForFunction(() => document.querySelector('.trigger')?.getAttribute('data-connection') !== 'offline', null, { timeout: 10_000 });
  await page.waitForTimeout(2_000);
  const mutations = pushed.flatMap((push) => push.mutations ?? []);
  assert.ok(
    mutations.some((mutation) => mutation.kind === 'ITEM_PATCH' && mutation.item_id === idOf('bread')),
    `no tick was pushed: ${JSON.stringify(mutations)}`,
  );
  assert.ok(
    mutations.some((mutation) => mutation.kind === 'ITEM_CREATE' && mutation.payload?.title === 'lemons' && mutation.payload?.amount === '2'),
    `no capture was pushed: ${JSON.stringify(mutations)}`,
  );
  await context.close();
});
