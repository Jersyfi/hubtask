// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Moving in the navigation concept's sidebar (UC-WRK-38, UC-CNV-10 check 6, UC-WRK-37 check 6,
// UC-WRK-39 checks 6-8): the contents dragged and keyed with their zones, refusals, Esc and Undo;
// Move to… with its greyed places; hubs, stars and products ordered; a collection to another hub
// through the pre-check. `SHOTS=<dir>` also writes the screenshots the mockup is compared with.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { chromium } from 'playwright';

import { id, IT, PD, OTHERS, WEB, HUBS, COLLECTIONS, PD_PAGES, PD_BOARDS, ALL, served, open, side } from './kompassstub.mjs';

const SHOTS = process.env.SHOTS;
const PEOPLE = OTHERS[1];
const FINANCE = OTHERS[2];

test.after(() => served.close());

/** Records each move the walk sends; `hold` keeps an answer back until released. */
function recorder({ preview } = {}) {
  const moves = [];
  let held;
  const state = { offline: false };
  const extra = async (route, request, url) => {
    // Offline the browser's own emulation does not stop a route, so the stub refuses every request.
    if (state.offline) return route.abort('internetdisconnected'), true;
    const path = url.pathname;
    if (/\/api\/v1\/(pages|whiteboards|containers)\/[^/]+:(move|reorder)$/.test(path) && request.method() === 'POST') {
      moves.push({ path: path.replace('/api/v1', ''), body: request.postDataJSON() });
      if (held) await held;
      const of = path.split('/')[4].split(':')[0];
      const known = [...PD_PAGES, ...PD_BOARDS, ...HUBS, ...COLLECTIONS].find((each) => each.id === of) ?? { id: of, hub_id: PD.id, title: 'x', version: 2 };
      return route.fulfill({ json: { ...known, version: 2 } }), true;
    }
    if (/:move-preview$/.test(path)) {
      const to = request.postDataJSON().target_parent_id;
      return route.fulfill({ json: preview?.(to) ?? { movable: true, obstacles: [], consequences: [] } }), true;
    }
    // The reader holds STRUCTURE and WRITE_ITEMS at the two hubs, nothing at the workspace.
    if (path.endsWith('/api/v1/memberships')) {
      const scope = url.searchParams.get('scope_type');
      const at = url.searchParams.get('scope_id');
      const rows = scope === 'HUB' && at !== FINANCE.id ? [{ id: `m-${at}`, scope_type: 'HUB', scope_id: at, account_id: id(1), role: 'OWNER' }] : [];
      return route.fulfill({ json: { data: rows, page: { next_cursor: null, has_more: false } } }), true;
    }
    if (path.endsWith('/api/v1/accounts:resolve')) {
      return route.fulfill({ json: { data: [{ id: id(61), display_name: 'Tim Berg' }, { id: id(62), display_name: 'Ana Ruiz' }], page: { next_cursor: null, has_more: false } } }), true;
    }
    return false;
  };
  return { moves, extra, state, hold: () => { let release; held = new Promise((done) => { release = done; }); return () => { held = undefined; release(); }; } };
}

const contents = (page) => side(page).getByRole('tree', { name: 'Contents of Product development' });
const row = (page, name) => contents(page).locator('[role="treeitem"]', { has: page.locator('.name', { hasText: new RegExp(`^${name}$`) }) });

/** Presses on a row, travels, and hovers the target's share of its height (0 top, 1 bottom). */
async function carry(page, from, to, share, { release = true, wait = 0 } = {}) {
  const a = await from.boundingBox();
  const b = await to.boundingBox();
  await page.mouse.move(a.x + a.width / 2, a.y + a.height / 2);
  await page.mouse.down();
  await page.mouse.move(a.x + a.width / 2, a.y + a.height / 2 + 6, { steps: 3 });
  await page.mouse.move(b.x + b.width / 2, b.y + b.height * share, { steps: 8 });
  if (wait) await page.waitForTimeout(wait);
  if (release) await page.mouse.up();
}

test('UC-WRK-38 checks 1-5, UC-CNV-10 check 6: a page carried into a folded page, the reason on a board, Esc, Undo and Cmd/Ctrl+Z', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const record = recorder();
  const { page, failures, close } = await open(browser, { path: `/hubs/${PD.id}`, modulesOn: ALL, owner: true, extra: record.extra });
  t.after(close);
  await row(page, 'Onboarding').waitFor({ timeout: 10_000 });
  // Check 1: the grip only where ordering is allowed, and a short press stays a click.
  assert.equal(await row(page, 'Onboarding').getAttribute('data-movable'), '');

  // Check 2: the middle of a folded page is *inside*; after 600 ms it opens.
  assert.equal(await row(page, 'Release notes').getAttribute('aria-expanded'), 'false');
  await carry(page, row(page, 'Onboarding'), row(page, 'Release notes'), 0.5, { release: false, wait: 900 });
  assert.equal(await row(page, 'Release notes').getAttribute('data-drop'), 'inside');
  assert.equal(await row(page, 'Release notes').getAttribute('aria-expanded'), 'true');
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/drag-inside.png`, clip: { x: 0, y: 0, width: 420, height: 900 } });
  // Check 2: the lower quarter of a row is *after*: a line at its level.
  const notes = await row(page, 'Meeting notes').boundingBox();
  await page.mouse.move(notes.x + notes.width / 2, notes.y + notes.height * 0.9, { steps: 4 });
  await contents(page).locator('.line').waitFor();
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/drag-line.png`, clip: { x: 0, y: 0, width: 420, height: 900 } });
  // Check 3: a board takes nothing in - no line, one sentence on the drag image.
  await row(page, 'Retro board').scrollIntoViewIfNeeded();
  const retro = await row(page, 'Retro board').boundingBox();
  await page.mouse.move(retro.x + retro.width / 2, retro.y + retro.height / 2, { steps: 4 });
  await page.locator('.ghost .why', { hasText: 'A whiteboard has no subpages' }).waitFor();
  assert.equal(await contents(page).locator('.line').count(), 0);
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/drag-refused.png`, clip: { x: 0, y: 0, width: 420, height: 900 } });
  // Check 4: Esc cancels; nothing is written.
  await page.keyboard.press('Escape');
  await page.mouse.up();
  assert.equal(record.moves.length, 0);
  await page.locator('#announcer, [aria-live]').filter({ hasText: 'Cancelled. Nothing changed.' }).first().waitFor({ state: 'attached' });

  // Check 5 and UC-CNV-10 check 6: a board dropped under a page lands there, *Undo* takes it back.
  await carry(page, row(page, 'Retro board'), row(page, 'Onboarding'), 0.5);
  await page.getByRole('button', { name: 'Undo' }).waitFor();
  assert.deepEqual(record.moves[0], { path: `/whiteboards/${id(411)}:move`, body: { parent_page_id: id(317), after_id: null } });
  await page.locator('.slot', { hasText: 'Moved. Position 1 of 1 under Onboarding.' }).waitFor();
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/drag-dropped.png`, clip: { x: 0, y: 0, width: 420, height: 900 } });
  await page.locator('main').click({ position: { x: 600, y: 400 } });
  await page.keyboard.press('ControlOrMeta+z');
  await page.locator('.slot', { hasText: 'Undone.' }).waitFor();
  assert.deepEqual(record.moves[1], { path: `/whiteboards/${id(411)}:move`, body: { parent_page_id: null, after_id: id(318) } });
  // Check 1: a press that travels less than 4 px stays a click.
  const box = await row(page, 'Meeting notes').boundingBox();
  await page.mouse.move(box.x + 60, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + 62, box.y + box.height / 2);
  await page.mouse.up();
  await page.waitForURL(`**/pages/${id(318)}`);
  assert.equal(record.moves.length, 2);
  assert.deepEqual(failures, []);
});

test('UC-WRK-38 checks 6 and 8: Alt+arrows indent a page, M offers greyed places, a move shows waiting until answered', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const record = recorder();
  const { page, failures, close } = await open(browser, { path: `/hubs/${PD.id}`, modulesOn: ALL, owner: true, extra: record.extra });
  t.after(close);
  await row(page, 'Onboarding').waitFor({ timeout: 10_000 });
  // Check 8: the move stands at once with *waiting* until the server answers.
  const release = record.hold();
  await row(page, 'Onboarding').focus();
  await page.keyboard.press('Alt+ArrowRight');
  await row(page, 'Onboarding').locator('.waiting').waitFor({ state: 'attached' });
  release();
  await page.getByRole('button', { name: 'Undo' }).waitFor();
  assert.deepEqual(record.moves[0], { path: `/pages/${id(317)}:move`, body: { parent_id: id(316) } });
  await row(page, 'Onboarding').locator('.waiting').waitFor({ state: 'detached' });

  // Check 6: M opens Move to…, the board and the row itself greyed with the reason.
  await row(page, 'Release notes').focus();
  await page.keyboard.press('m');
  const picker = page.getByRole('dialog', { name: 'Move “Release notes” to…' });
  await picker.waitFor();
  const greyed = picker.getByRole('button', { name: /Retro board/ });
  assert.equal(await greyed.getAttribute('aria-disabled'), 'true');
  assert.ok((await greyed.innerText()).includes('A whiteboard has no subpages'));
  assert.ok((await picker.getByRole('button', { name: /Release 3.1/ }).innerText()).includes('Not under itself'));
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/move-picker.png` });
  await picker.getByRole('textbox').fill('hand');
  await picker.getByRole('button', { name: /Product handbook/ }).click();
  await page.waitForFunction(() => document.querySelectorAll('[role="dialog"]').length === 0);
  assert.deepEqual(record.moves[1], { path: `/pages/${id(313)}:move`, body: { parent_id: id(310), after_id: id(312) } });
  assert.deepEqual(failures, []);
});

test('UC-WRK-38 check 7, UC-WRK-37 check 6: hubs ordered in the shared order with Undo, stars and products personally', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const record = recorder();
  const { page, failures, written, close } = await open(browser, { modulesOn: ALL, owner: true, extra: record.extra, stars: [{ kind: 'HUB', id: IT.id }, { kind: 'COLLECTION', id: WEB.id }] });
  t.after(close);
  const hubs = side(page).getByRole('tree', { name: 'Hubs' });
  await hubs.getByRole('treeitem', { name: /^IT operations/ }).waitFor({ timeout: 10_000 });
  await carry(page, hubs.getByRole('treeitem', { name: /^IT operations/ }), hubs.getByRole('treeitem', { name: /^Product development/ }), 0.1);
  await page.getByRole('button', { name: 'Undo' }).click();
  await page.locator('.slot', { hasText: 'Undone.' }).waitFor();
  assert.deepEqual(record.moves.map((each) => each.path), [`/containers/${IT.id}:reorder`, `/containers/${IT.id}:reorder`]);
  assert.equal(record.moves[0].body.before_container_id, PD.id);

  // Stars: Web app dragged above IT operations, written as the whole list.
  const starred = side(page).getByRole('tree', { name: 'Starred' });
  await carry(page, starred.getByRole('treeitem', { name: /^Web app/ }), starred.getByRole('treeitem', { name: /^IT operations/ }), 0.1);
  await page.getByRole('button', { name: 'Undo' }).last().waitFor();
  assert.deepEqual(written.stars.at(-1).stars.map((each) => each.kind), ['COLLECTION', 'HUB']);

  // Products: Alt+↓ on Plans writes the person's order.
  const products = side(page).getByRole('tree', { name: 'Products' });
  await products.getByRole('treeitem', { name: /^Plans/ }).focus();
  await page.keyboard.press('Alt+ArrowDown');
  await page.waitForFunction(() => document.body.innerText.includes('Position 2 of'));
  assert.deepEqual(written.preferences.at(-1).nav_products.slice(0, 2), ['KNOWLEDGE', 'PLANNING']);
  assert.deepEqual(failures, []);
});

const PREVIEW = (to) =>
  to === IT.id
    ? {
        movable: false,
        obstacles: [{ code: 'containers.name_taken', params: {} }, { code: 'entry_types.hub_bound', params: {} }],
        consequences: [
          { code: 'containers.move_loses_access', params: { count: '2', account_ids: `${id(61)},${id(62)}` } },
          { code: 'containers.move_takes_structure', params: { hub_id: IT.id } },
          { code: 'containers.move_keeps_releases', params: { count: '4', hub_id: PD.id } },
        ],
      }
    : { movable: true, obstacles: [], consequences: [{ code: 'containers.move_loses_access', params: { count: '1', account_ids: id(61) } }] };

/** A finger's long press on an element: the hold, then the lift (ADR-0088). */
async function longPress(page, locator) {
  const box = await locator.boundingBox();
  const at = { x: box.x + box.width / 3, y: box.y + box.height / 2 };
  await locator.dispatchEvent('pointerdown', { pointerType: 'touch', button: 0, clientX: at.x, clientY: at.y, isPrimary: true });
  await page.waitForTimeout(450);
  await page.evaluate((point) => window.dispatchEvent(new PointerEvent('pointerup', { pointerType: 'touch', clientX: point.x, clientY: point.y, bubbles: true })), at);
  // The lift's own click is swallowed for the hold's length; a real finger sends it at once.
  await page.waitForTimeout(400);
}

/** Walks the dialog of UC-WRK-39 checks 6-8, wherever it was opened. */
async function walkMoveDialog(page, record, isPhone) {
  const dialog = page.getByRole('dialog', { name: 'Move “Web app”' });
  await dialog.waitFor();
  // Check 6: a hub without STRUCTURE is greyed with the reason.
  const finance = dialog.getByRole('radio', { name: /^Finance/ });
  assert.equal(await finance.getAttribute('aria-disabled'), 'true');
  assert.ok((await finance.innerText()).includes('No permission to order there'));
  // Check 7: obstacles with their ways out, warnings in words, never a code.
  await dialog.getByRole('radio', { name: /^IT operations/ }).click();
  const checks = dialog.getByRole('list', { name: 'Before moving' });
  await checks.getByText('The name is taken in IT operations.', { exact: false }).waitFor();
  const said = await checks.innerText();
  for (const words of ['Entries here use a type of Product development', '2 people will no longer see it: Tim Berg, Ana Ruiz.', 'It takes on the fields, views and rules of IT operations.', '4 entries stay in releases of Product development.']) {
    assert.ok(said.includes(words), said);
  }
  assert.ok(!/[a-z]+\.[a-z]+_[a-z_]+/.test(said), `a code shows: ${said}`);
  await checks.getByRole('button', { name: "Change the entries' types" }).waitFor();
  // Check 8: Move is enabled only while no obstacle remains; on a phone it names the destination.
  const confirm = dialog.getByRole('button', { name: /^Move/ });
  assert.equal(await confirm.getAttribute('aria-disabled'), 'true');
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/move-${isPhone ? 'phone' : 'desk'}.png` });
  await dialog.getByRole('radio', { name: /^People/ }).click();
  await checks.getByText('1 person will no longer see it: Tim Berg.').waitFor();
  assert.equal((await confirm.innerText()).trim(), isPhone ? 'Move to “People”' : 'Move');
  await page.waitForFunction(() => [...document.querySelectorAll('[role="dialog"] button, dialog button')].some((b) => /^Move/.test(b.textContent?.trim() ?? '') && b.getAttribute('aria-disabled') !== 'true'));
  await confirm.click();
  await dialog.waitFor({ state: 'hidden' });
  assert.deepEqual(record.moves.at(-1), { path: `/containers/${WEB.id}:move`, body: { target_parent_id: PEOPLE.id, before_container_id: null } });
}

test('UC-WRK-39 checks 6-8: M on a collection row opens the move with its pre-check', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const record = recorder({ preview: PREVIEW });
  const { page, failures, close } = await open(browser, { path: `/hubs/${PD.id}`, modulesOn: ALL, owner: true, extra: record.extra });
  t.after(close);
  const web = side(page).getByRole('tree', { name: 'Collections of Product development' }).getByRole('treeitem', { name: /^Web app/ });
  await web.waitFor({ timeout: 10_000 });
  await web.focus();
  await page.keyboard.press('m');
  await walkMoveDialog(page, record, false);
  assert.deepEqual(failures, []);
});

const phone = (browser, path, options) =>
  open(browser, { path, modulesOn: ALL, owner: true, width: 390, height: 844, hasTouch: true, ready: (page) => page.locator('[data-phone-bar]'), ...options });

test('UC-WRK-38 check 9, UC-WRK-39 check 8: a long press opens the row\'s sheet; Move to… names the destination on its button', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const record = recorder({ preview: PREVIEW });
  const { page, failures, close } = await phone(browser, `/browse/hubs/${PD.id}`, { extra: record.extra });
  t.after(close);
  const web = page.locator('[data-press-row] li[data-id]', { hasText: 'Web app' });
  await web.waitFor({ timeout: 10_000 });
  await longPress(page, web);
  const sheet = page.getByRole('dialog', { name: 'Web app' });
  await sheet.waitFor();
  const items = (await sheet.locator('li').allInnerTexts()).map((each) => each.trim());
  assert.deepEqual(items.slice(0, 3), ['Star', 'Move to…', 'Change order'], JSON.stringify(items));
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/phone-7.png` });
  await sheet.getByRole('button', { name: 'Move to…' }).click();
  await walkMoveDialog(page, record, true);
  assert.deepEqual(failures, []);
});

test('UC-WRK-38 check 9: Change order opens reorder mode - grips at the end, Done - and no row drags outside it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const record = recorder();
  const { page, failures, close } = await phone(browser, `/browse/hubs/${PD.id}`, { extra: record.extra });
  t.after(close);
  const rows = page.locator('[data-press-row] li[data-id]');
  await rows.filter({ hasText: 'Web app' }).waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('.reorder').count(), 0);
  await longPress(page, rows.filter({ hasText: 'Mobile' }));
  await page.getByRole('dialog', { name: 'Mobile' }).getByRole('button', { name: 'Change order' }).click();
  await page.getByRole('button', { name: 'Done' }).waitFor();
  const grips = page.locator('[data-reordering] .reorder');
  assert.ok((await grips.count()) >= 3);
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/phone-8.png` });
  // The last collection carried by its grip above the first.
  const ordered = page.locator('[data-reordering] li[data-id]');
  const last = ordered.last();
  const lastName = (await last.innerText()).split('\n')[0];
  const grip = await last.locator('.reorder').boundingBox();
  const first = await ordered.first().boundingBox();
  await page.mouse.move(grip.x + grip.width / 2, grip.y + grip.height / 2);
  await page.mouse.down();
  await page.mouse.move(grip.x + grip.width / 2, first.y + 4, { steps: 6 });
  await page.mouse.up();
  await page.getByRole('button', { name: 'Undo' }).waitFor();
  assert.match(record.moves.at(-1).path, /^\/containers\/[^/]+:move$/);
  assert.ok(lastName.length > 0);
  await page.getByRole('button', { name: 'Done' }).click();
  await page.locator('.reorder').first().waitFor({ state: 'detached' });
  assert.deepEqual(failures, []);
});

test('UC-WRK-38 check 8: offline a move is not kept to send later, and the sidebar says so', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const record = recorder();
  const { page, failures, close } = await open(browser, { path: `/hubs/${PD.id}`, modulesOn: ALL, owner: true, extra: record.extra });
  t.after(close);
  await row(page, 'Onboarding').waitFor({ timeout: 10_000 });
  record.state.offline = true;
  await page.context().setOffline(true);
  await row(page, 'Onboarding').focus();
  await page.keyboard.press('Alt+ArrowRight');
  await side(page).locator('.failure', { hasText: 'Moving needs a connection' }).waitFor({ timeout: 10_000 });
  // It springs back: still at the top, nothing sent.
  assert.equal(await row(page, 'Onboarding').getAttribute('aria-level'), '1');
  assert.equal(record.moves.length, 0);
  record.state.offline = false;
  await page.context().setOffline(false);
  assert.deepEqual(failures, []);
});
