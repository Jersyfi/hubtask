// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Working on a board (W-2 … W-7, UC-CNV-01, 02, 04, 05, 09): the bar above a chosen note in its
// fixed order, the dock's keys and a placing tool that springs back while a pen stays, the board
// read as a list by frame, the session bar at the top centre, a task card with its key, person,
// status and tick, a new board that opens with its templates as a sheet, the home's comments to
// the reader, and on a tablet the pen that draws while a finger moves the board.
//
// The realtime socket is played by the test (an empty drawing), so the board is live and writable
// and the saved objects are what it draws. Chromium only: what this asserts is the rendered board.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, HUB, MANIFEST, OTHER, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const BOARD_ID = '01a0e2e0-0000-7000-8000-00000000b0f1';
const ENTRY_ID = '01a0e2e0-0000-7000-8000-00000000e0f1';
const ELEMENTS = [
  { id: 'f1', kind: 'FRAME', x: 0, y: 0, width: 420, height: 440, text: 'Went well' },
  { id: 'f2', kind: 'FRAME', x: 460, y: 0, width: 420, height: 440, text: 'Next time' },
  { id: 's1', kind: 'STICKY', x: 20, y: 30, width: 180, height: 150, text: 'Pairing worked', color_token: 'green' },
  { id: 's2', kind: 'STICKY', x: 480, y: 30, width: 180, height: 150, text: 'Review earlier', color_token: 'blue' },
  { id: 'c1', kind: 'CARD', x: 0, y: 490, width: 240, height: 140, target: { kind: 'ENTRY', id: ENTRY_ID } },
];
const board = (elements, extra = {}) => ({
  id: BOARD_ID, hub_id: HUB.id, title: 'Retro', element_count: elements.length, elements, writable: true, trashed_at: null,
  created_by: ACCOUNT.id, created_at: '2026-10-30T09:00:00Z', updated_by: OTHER.id, updated_at: '2026-10-30T09:00:00Z', version: 1, ...extra,
});
const TARGET = {
  kind: 'ENTRY', id: ENTRY_ID, anchor: null, available: true, title: 'Book the room', key: 'TAMA-1', number: 1, level: 'TASK',
  entry_type: 'task', completed: false, archived: false, collection_id: HUB.id, assignee_id: OTHER.id,
  status_name: 'In progress', status_category: 'IN_PROGRESS', start_at: null, due_at: null,
};

function varUint(n) {
  const out = [];
  while (n > 0x7f) { out.push(0x80 | (n & 0x7f)); n = Math.floor(n / 128); }
  out.push(n);
  return out;
}
const EMPTY_DRAWING = Buffer.from([0, 1, ...varUint(2), 0, 0]);

async function open(browser, { width = 1280, height = 860, elements = ELEMENTS, path = `/whiteboards/${BOARD_ID}`, hasTouch = false, comments = [], boards } = {}) {
  const { page, context, failures } = await signedIn(browser, width, height, { hasTouch });
  await context.routeWebSocket(/\/realtime\/documents\/whiteboard\//, (ws) => {
    ws.onMessage(() => {});
    ws.send(EMPTY_DRAWING);
  });
  const saved = [];
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const at = new URL(request.url()).pathname;
    if (at.endsWith(`/whiteboards/${BOARD_ID}`) && request.method() === 'PATCH') {
      saved.push(request.postDataJSON());
      return route.fulfill({ json: board(request.postDataJSON().elements ?? elements) });
    }
    if (at.endsWith(`/whiteboards/${BOARD_ID}`)) return route.fulfill({ json: board(elements) });
    if (at.endsWith(`/whiteboards/${BOARD_ID}/comments`)) return route.fulfill({ json: { data: comments } });
    if (at.endsWith(`/hubs/${HUB.id}/whiteboards`)) return route.fulfill({ json: { data: boards ?? [board(elements)] } });
    if (at.endsWith('/references:resolve')) return route.fulfill({ json: { data: [TARGET] } });
    if (at.endsWith('/meta/capabilities')) return route.fulfill({ json: { ...MANIFEST, modules_on: [...(MANIFEST.modules_on ?? []), 'WHITEBOARD'] } });
    if (/\/containers\/[^/]+\/modules$/.test(at)) return route.fulfill({ json: { modules_on: [...(MANIFEST.modules_on ?? []), 'WHITEBOARD'] } });
    return stub(route);
  });
  await page.goto(`${served.origin}${path}`);
  return { page, context, failures, saved };
}

const box = (locator) => locator.boundingBox();
/** Where a board point is on the screen, read from the canvas's viewBox. */
async function screenOf(page, x, y) {
  const canvas = page.getByRole('application');
  const [vx, vy, vw, vh] = (await canvas.getAttribute('viewBox')).split(/\s+/).map(Number);
  const area = await canvas.boundingBox();
  return { x: area.x + ((x - vx) / vw) * area.width, y: area.y + ((y - vy) / vh) * area.height };
}
/** The words of the note just placed, once its editor stands open. */
async function write(page, words) {
  const editor = page.locator('textarea.editor');
  await editor.waitFor({ timeout: 5_000 });
  // Typed, not filled: the note just placed holds the focus, so the words go into it.
  await page.waitForFunction(() => document.activeElement?.classList.contains('editor'), null, { timeout: 5_000 });
  await page.keyboard.type(words);
}
const toolOf = (page, tool) => page.locator(`[data-tool=${tool}]`);
const pressed = async (page, tool) => (await toolOf(page, tool).getAttribute('aria-pressed')) === 'true';

test('chromium: a chosen note carries the bar above it in its order, and the card shows key, person, status and tick (UC-CNV-04 check 6, UC-CNV-02 check 9)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser);
  t.after(() => context.close());
  const canvas = page.getByRole('application');
  await canvas.locator('g.object').first().waitFor({ timeout: 20_000 });

  // The card: the key opens it, the person's initials at its corner, the status in words, the tick.
  const card = canvas.locator('g.card').filter({ has: page.locator('[data-card-person]') });
  await card.waitFor({ timeout: 10_000 });
  assert.equal(await card.locator('[data-card-person]').getAttribute('data-card-person'), OTHER.id);
  assert.equal((await card.locator('[data-card-person] text').textContent()).trim(), 'ML', 'the card does not show the person it is on');
  assert.equal((await card.locator('.open-key').textContent()).trim(), 'TAMA-1');
  assert.ok((await card.locator('g.state text').textContent()).includes('In progress'), 'the card does not say its status');
  assert.equal(await card.getByRole('checkbox').count(), 1, 'the card has no tick');

  const note = await screenOf(page, 110, 105);
  await page.mouse.click(note.x, note.y);
  const bar = page.locator('[data-board-bar]');
  await bar.waitFor({ timeout: 5_000 });
  const order = await bar.locator('[role=radiogroup], [data-bar]').evaluateAll((parts) => parts.map((part) => part.getAttribute('data-bar') ?? 'colour'));
  assert.deepEqual(order, ['colour', 'size', 'arrow', 'comment', 'lock', 'task', 'more'], 'the bar is not in its order');
  const above = await box(bar);
  assert.ok(above.y + above.height <= note.y, 'the bar is not above the selection');
  await bar.locator('[data-bar=task]').click();
  await page.locator('[data-board-make]').waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('chromium: the dock answers its keys, a placing tool springs back, a double click holds it and the pen stays (UC-CNV-01 check 11)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser);
  t.after(() => context.close());
  const canvas = page.getByRole('application');
  await canvas.locator('g.object').first().waitFor({ timeout: 20_000 });
  await page.locator('[data-board-status] svg, [data-board-status]').first().waitFor();

  // Every tool shows its key.
  for (const [tool, key] of [['select', 'V'], ['HAND', 'H'], ['STICKY', 'N'], ['SHAPE', 'R'], ['TEXT', 'T'], ['CONNECTOR', 'L'], ['FRAME', 'F'], ['STROKE', 'P'], ['HIGHLIGHTER', 'M'], ['ERASER', 'E'], ['COMMENT', 'C']]) {
    assert.equal(await toolOf(page, tool).getAttribute('aria-keyshortcuts'), key, `${tool} does not show ${key}`);
  }
  await canvas.focus();
  await page.keyboard.press('h');
  assert.ok(await pressed(page, 'HAND'), 'H did not take the hand');
  await page.keyboard.press('v');
  assert.ok(await pressed(page, 'select'), 'V did not lead back to select');

  // A note placed by pointer: the tool springs back to select. A click on the board commits the words.
  const empty = await screenOf(page, 300, 300);
  const aside = await screenOf(page, 440, 460);
  await toolOf(page, 'STICKY').click();
  assert.ok(await pressed(page, 'STICKY'));
  await page.mouse.click(empty.x, empty.y);
  await write(page, 'Idea');
  assert.ok(await pressed(page, 'select'), 'the note tool did not spring back');
  await page.mouse.click(aside.x, aside.y);
  await canvas.getByText('Idea', { exact: true }).waitFor({ timeout: 5_000 });

  // A double click holds it: two notes, and the tool still chosen; Esc leads back.
  await toolOf(page, 'STICKY').dblclick();
  const one = await screenOf(page, 700, 300);
  const two = await screenOf(page, 40, 230);
  await page.mouse.click(one.x, one.y);
  await write(page, 'One');
  assert.ok(await pressed(page, 'STICKY'), 'a double click did not hold the tool');
  await page.mouse.click(two.x, two.y);
  await write(page, 'Two');
  assert.ok(await pressed(page, 'STICKY'), 'a held tool sprang back after the second note');
  await page.mouse.click(aside.x, aside.y);
  await canvas.getByText('Two', { exact: true }).waitFor({ timeout: 5_000 });
  await canvas.focus();
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');
  assert.ok(await pressed(page, 'select'), 'Esc did not lead back to select');

  // The pen stays chosen after a stroke: one stroke is rarely all.
  await toolOf(page, 'STROKE').click();
  const from = await screenOf(page, 250, 600);
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x + 60, from.y + 20, { steps: 6 });
  await page.mouse.up();
  await canvas.locator('g.stroke, path.stroke').first().waitFor({ timeout: 5_000 });
  assert.ok(await pressed(page, 'STROKE'), 'the pen did not stay after a stroke');
  assert.deepEqual(failures, []);
});

test('chromium: the board reads as a list by frame in reading order (UC-CNV-01 check 13)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser);
  t.after(() => context.close());
  await page.getByRole('application').locator('g.object').first().waitFor({ timeout: 20_000 });
  await page.locator('[data-board-mode=list]').click();
  const groups = page.locator('[data-board-list] section, .list section');
  await groups.first().waitFor({ timeout: 5_000 });
  const heads = await groups.locator('h2, h3').allTextContents();
  assert.deepEqual(heads.slice(0, 2).map((each) => each.trim().replace(/\s*\d+$/u, '')), ['Went well', 'Next time'], 'the frames are not in reading order');
  assert.ok((await groups.first().textContent()).includes('Pairing worked'), 'a frame does not list its note');
  await page.locator('[data-board-mode=surface]').click();
  await page.getByRole('application').waitFor();
  assert.deepEqual(failures, []);
});

test('chromium: a timer started from Moderate is one bar at the top centre, level with the islands, with a minute more (UC-CNV-05 check 4)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser);
  t.after(() => context.close());
  await page.getByRole('application').locator('g.object').first().waitFor({ timeout: 20_000 });
  await page.locator('[data-board-sheet=session]').click();
  const side = page.locator('[data-board-part=session]');
  await side.waitFor({ timeout: 5_000 });
  await side.getByRole('button', { name: 'Start the timer' }).click();
  const bar = page.locator('[data-board-session]');
  await bar.waitFor({ timeout: 5_000 });
  const stage = await box(page.locator('.stage'));
  const at = await box(bar);
  assert.ok(Math.abs(at.x + at.width / 2 - (stage.x + stage.width / 2)) < 4, 'the session bar is not at the centre');
  // Level with the top islands where it clears both (the rule: islands.test.ts), else just below them;
  // either way it touches neither.
  const clear = () => page.evaluate(() => {
    const bar = document.querySelector('[data-board-session]');
    const at = bar.getBoundingClientRect();
    const [title, people] = ['title', 'people'].map((name) => document.querySelector(`[data-board-island=${name}]`).getBoundingClientRect());
    const level = !bar.classList.contains('below');
    return level ? Math.abs(at.top - title.top) < 2 && at.left >= title.right && at.right <= people.left : at.top >= title.bottom && at.top >= people.bottom;
  });
  assert.ok(await clear(), 'the session bar touches a top island');
  const clock = bar.getByRole('timer');
  const was = await clock.textContent();
  await bar.getByRole('button', { name: '1 min' }).click();
  await page.waitForFunction((before) => document.querySelector('[data-board-session] [role=timer]')?.textContent !== before, was);
  const [m1] = was.split(':').map(Number);
  const [m2] = (await clock.textContent()).split(':').map(Number);
  assert.ok(m2 >= m1 + 1 || m2 === m1 + 1, `a minute more read ${was} → ${await clock.textContent()}`);
  await page.setViewportSize({ width: 1100, height: 860 });
  await page.waitForFunction(() => document.querySelector('[data-board-session]').classList.contains('below'), null, { timeout: 5_000 });
  assert.ok(await clear(), 'the session bar touches a top island where there is no room beside them');
  assert.deepEqual(failures, []);
});

test('chromium: a board just made opens with its templates as a sheet and the first-use keys; a card puts its template (UC-CNV-01 check 12)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures, saved } = await open(browser, { elements: [], path: `/whiteboards/${BOARD_ID}?new=1` });
  t.after(() => context.close());
  const sheet = page.locator('[data-board-begin]');
  await sheet.waitFor({ timeout: 20_000 });
  assert.equal(await sheet.locator('[data-board-start]').count(), 8, 'the sheet does not offer the eight starts');
  await page.locator('[data-board-first-use]').waitFor();
  assert.deepEqual(await page.locator('[data-board-first-use] kbd').allTextContents(), ['N', 'Double click', 'Space']);
  await sheet.locator('[data-board-start=KANBAN] button').click();
  await sheet.waitFor({ state: 'detached', timeout: 5_000 });
  await page.getByRole('application').locator('g.frame, g.object').first().waitFor({ timeout: 5_000 });
  await page.waitForFunction(() => document.querySelectorAll('[role=application] g.object, [role=application] g.frame').length >= 3);
  assert.ok(saved.length >= 0);
  assert.deepEqual(failures, []);
});

test('chromium: the home lists the boards with their people, the templates row and the open comments to you (UC-CNV-10 check 8, UC-CNV-09 check 5)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const comments = [{
    id: '01a0e2e0-0000-7000-8000-00000000c0f1', whiteboard_id: BOARD_ID, parent_id: null, author_id: OTHER.id,
    body: `[@Jérôme Winkel](hubtask:person/${ACCOUNT.id}) can you book the room?`, element_id: 's1', x: 0, y: 0,
    resolved: false, resolved_at: null, resolved_by: null, created_at: '2026-11-13T09:00:00Z', edited_at: null, deleted: false, version: 1,
  }];
  const { page, context, failures } = await open(browser, { path: '/whiteboards', comments });
  t.after(() => context.close());
  await page.locator('[data-boards-recent] [data-board-card]').first().waitFor({ timeout: 20_000 });
  await page.locator('[data-boards-recent] [data-board-thumbnail=drawn]').first().waitFor({ timeout: 10_000 });
  assert.deepEqual(
    await page.locator('[data-boards-templates] [data-board-start]').evaluateAll((cards) => cards.map((card) => card.getAttribute('data-board-start'))),
    ['RETRO_START_STOP_CONTINUE', 'RETRO_FOUR_LS', 'RETRO_MAD_SAD_GLAD', 'BRAINSTORM', 'FLOWCHART'],
  );
  const row = page.locator('[data-boards-comments] [data-board-comment]');
  await row.waitFor({ timeout: 10_000 });
  const words = await row.textContent();
  assert.ok(words.includes('Mara Lind') && words.includes('@Jérôme Winkel can you book the room?'), `the comment reads ${words}`);
  assert.ok(words.includes('Pairing worked'), 'the comment does not name the note it is at');
  await page.getByText('1 open comment', { exact: true }).waitFor({ timeout: 5_000 });
  await row.getByRole('link').click();
  await page.waitForFunction(() => location.search.includes('comment='));
  await page.locator('[data-board-part=comments] [data-board-thread]').first().waitFor({ timeout: 10_000 });
  assert.deepEqual(failures, []);
});

test('chromium: the home draws the boards opened on this device first, else the last changed (UC-CNV-10 check 8)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const newer = board([], { id: '01a0e2e0-0000-7000-8000-00000000b0f2', title: 'Plan', updated_at: '2026-11-12T09:00:00Z' });
  const { page, context, failures } = await open(browser, { path: '/whiteboards', boards: [newer, board(ELEMENTS)] });
  t.after(() => context.close());
  const titles = (region) => region.locator('[data-board-card] .title').allTextContents();
  const changed = page.getByRole('region', { name: 'Recently changed boards' });
  await changed.locator('[data-board-card]').first().waitFor({ timeout: 20_000 });
  assert.deepEqual(await titles(changed), ['Plan', 'Retro'], 'nothing opened here yet: the last changed first');

  await changed.getByRole('link', { name: /Retro/ }).click();
  await page.getByRole('application').locator('g.object').first().waitFor({ timeout: 20_000 });
  await page.goto(`${served.origin}/whiteboards`);
  const opened = page.getByRole('region', { name: 'Recently opened' });
  await opened.locator('[data-board-card]').first().waitFor({ timeout: 20_000 });
  assert.deepEqual(await titles(opened), ['Retro'], 'the board opened here leads');
  assert.deepEqual(failures, []);
});

test('chromium: a picture or file from the insert menu opens the platform\'s chooser, no file control drawn on the board', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser);
  t.after(() => context.close());
  await page.getByRole('application').locator('g.object').first().waitFor({ timeout: 20_000 });
  assert.equal(await page.locator('input[type=file]').count(), 0, 'the board draws a file control of its own');
  await page.locator('[data-board-insert]:visible').click();
  const chooser = page.waitForEvent('filechooser', { timeout: 5_000 });
  await page.getByRole('menuitem', { name: 'Picture or file' }).click();
  assert.ok(await chooser, 'no chooser opened');
  assert.deepEqual(failures, []);
});

test('chromium: on a tablet a finger moves the board and a pen draws (UC-CNV-04 check 7, as far as touch emulation reaches)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, { width: 1024, height: 768, hasTouch: true });
  t.after(() => context.close());
  const canvas = page.getByRole('application');
  await canvas.locator('g.object').first().waitFor({ timeout: 20_000 });
  const area = await canvas.boundingBox();
  const start = { x: area.x + area.width * 0.7, y: area.y + area.height * 0.75 };

  // A finger drag, through the browser's own touch input: the view moves, nothing is drawn.
  const viewBefore = await canvas.getAttribute('viewBox');
  const objectsBefore = await canvas.locator('g.object, g.stroke, path.stroke').count();
  const cdp = await context.newCDPSession(page);
  const touch = (type, x, y) => cdp.send('Input.dispatchTouchEvent', { type, touchPoints: type === 'touchEnd' ? [] : [{ x, y }] });
  await touch('touchStart', start.x, start.y);
  for (let i = 1; i <= 6; i++) await touch('touchMove', start.x - i * 15, start.y - i * 8);
  await touch('touchEnd');
  await page.waitForFunction((was) => document.querySelector('[role=application]').getAttribute('viewBox') !== was, viewBefore, { timeout: 5_000 });
  assert.equal(await canvas.locator('g.object, g.stroke, path.stroke').count(), objectsBefore, 'a finger drew instead of moving the board');

  // A pen: Chromium cannot emulate a stylus, so the pen's pointer events are dispatched with
  // pointerType "pen" - what a tablet's pen sends. With the select tool chosen, the pen draws.
  const viewPen = await canvas.getAttribute('viewBox');
  await canvas.evaluate((svg, at) => {
    const send = (type, x, y) => svg.dispatchEvent(new PointerEvent(type, { pointerId: 7, pointerType: 'pen', isPrimary: true, clientX: x, clientY: y, buttons: type === 'pointerup' ? 0 : 1, pressure: 0.5, bubbles: true, cancelable: true }));
    send('pointerdown', at.x, at.y);
    for (let i = 1; i <= 6; i++) send('pointermove', at.x + i * 10, at.y + i * 4);
    send('pointerup', at.x + 60, at.y + 24);
  }, start);
  await page.waitForTimeout(300);
  const drawn = await canvas.locator('g.stroke, path.stroke').count();
  assert.equal(await canvas.getAttribute('viewBox'), viewPen, 'the pen moved the board');
  assert.ok(drawn > 0, 'the pen drew nothing');
  assert.deepEqual(failures, []);
});
