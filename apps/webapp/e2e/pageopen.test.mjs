// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Opening a page writes nothing (found beside the owner's review of 2026-11-08): the first writer to
// open a page with a body and no live document yet seeds the document from the body, and that
// seeding is the body itself, not an edit - it saved the body again, re-serialised, and the page
// gained a version nobody wrote. A real edit still saves. The same for a whiteboard (R-18): opening
// and leaving it saved its scene again, which re-read the hub's contents in the sidebar.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { sharedChromium } from './browser.mjs';

import { ALL, PD_BOARDS, PD_PAGES, open, served } from './kompassstub.mjs';

test.after(() => served.close());

const varUint = (n) => {
  const out = [];
  while (n > 0x7f) { out.push(0x80 | (n & 0x7f)); n = Math.floor(n / 128); }
  out.push(n);
  return out;
};
/** The server's answer to a new document: sync step 2 with an empty update, then its step 1. */
const EMPTY_DOCUMENT = Buffer.from([0, 1, ...varUint(2), 0, 0]);
const ASK_STATE = Buffer.from([0, 0, ...varUint(1), 0]);
// A text's own keys: on macOS End and Home scroll the document and leave the caret, and Shift+Home
// selects to the document's start - an Enter then replaces what is selected.
const MAC = process.platform === 'darwin';
const LINE_END = MAC ? 'Meta+ArrowRight' : 'End';
const SELECT_TO_LINE_START = MAC ? 'Shift+Meta+ArrowLeft' : 'Shift+Home';

test('the first writer opens a page without saving it; typing saves', async (t) => {
  const browser = await sharedChromium(t);
  const handbook = PD_PAGES[0];
  const { page, failures, close } = await open(browser, { modulesOn: ALL, owner: true });
  t.after(close);
  let synced = false;
  await page.context().routeWebSocket(/\/realtime\/documents\/page\//, (ws) => {
    ws.onMessage(() => {});
    ws.send(EMPTY_DOCUMENT);
    ws.send(ASK_STATE);
    synced = true;
  });
  const saves = [];
  page.on('request', (request) => {
    if (request.method() !== 'GET' && new URL(request.url()).pathname.endsWith(`/pages/${handbook.id}`)) saves.push(request.postData());
  });
  await page.goto(new URL(`/pages/${handbook.id}`, page.url()).href);
  const body = page.getByRole('textbox', { name: /Product handbook/ });
  await body.getByText('Welcome to the team.').waitFor({ timeout: 10_000 });
  assert.ok(synced, 'the live document connected');
  await page.waitForTimeout(2_500);
  assert.deepEqual(saves, [], 'opening the page saved it');

  await body.click();
  await page.keyboard.press(LINE_END);
  await page.keyboard.type(' Glad you are here.');
  await page.waitForTimeout(2_500);
  assert.equal(saves.length, 1, 'an edit is saved');
  assert.match(saves[0], /Glad you are here/);
  assert.deepEqual(failures, []);
});

test('leaving a page right after typing saves the last words and throws nothing', async (t) => {
  const browser = await sharedChromium(t);
  const handbook = PD_PAGES[0];
  const { page, failures, close } = await open(browser, { modulesOn: ALL, owner: true });
  t.after(close);
  await page.context().routeWebSocket(/\/realtime\/documents\/page\//, (ws) => {
    ws.onMessage(() => {});
    ws.send(EMPTY_DOCUMENT);
    ws.send(ASK_STATE);
  });
  const saves = [];
  page.on('request', (request) => {
    if (request.method() !== 'GET' && new URL(request.url()).pathname.endsWith(`/pages/${handbook.id}`)) saves.push(request.postData());
  });
  await page.goto(new URL(`/pages/${handbook.id}`, page.url()).href);
  const body = page.getByRole('textbox', { name: /Product handbook/ });
  await body.getByText('Welcome to the team.').waitFor({ timeout: 10_000 });
  await page.waitForTimeout(500);
  await body.click();
  await page.keyboard.press(LINE_END);
  await page.keyboard.type(' Last words.');
  // Away inside the app before the pause that saves: the page's editor is torn down.
  await page.evaluate(() => {
    history.pushState({}, '', '/');
    window.dispatchEvent(new PopStateEvent('popstate', { state: {} }));
  });
  await body.waitFor({ state: 'detached', timeout: 5_000 });
  await page.waitForTimeout(1_000);
  assert.equal(saves.length, 1, 'the last words are saved on the way out');
  assert.match(saves[0], /Last words\./);
  assert.deepEqual(failures, [], 'leaving writes no state while the page is torn down');
});

test('a page shows its writing tools only while writing; "/" on an empty line offers the blocks (S-1)', async (t) => {
  const browser = await sharedChromium(t);
  const handbook = PD_PAGES[0];
  const { page, failures, close } = await open(browser, { modulesOn: ALL, owner: true });
  t.after(close);
  await page.context().routeWebSocket(/\/realtime\/documents\/page\//, (ws) => {
    ws.onMessage(() => {});
    ws.send(EMPTY_DOCUMENT);
    ws.send(ASK_STATE);
  });
  await page.goto(new URL(`/pages/${handbook.id}`, page.url()).href);
  const body = page.getByRole('textbox', { name: /Product handbook/ });
  await body.getByText('Welcome to the team.').waitFor({ timeout: 10_000 });
  await page.waitForTimeout(500);
  assert.equal(await page.locator('[data-page-tools], [data-block-handle], [data-floating-toolbar]').count(), 0, 'reading shows no tools');

  // Writing: "+" and the handle at the caret's line, no bar over the page from a tablet on.
  await body.getByText('Welcome to the team.').click();
  await page.locator('[data-block-handle]').waitFor({ timeout: 5_000 });
  assert.equal(await page.locator('[data-page-tools]').count(), 0, 'the bar stands at the text, not over the page');
  const handle = await page.locator('[data-block-handle]').boundingBox();
  const line = await body.getByText('Welcome to the team.').boundingBox();
  assert.ok(handle && line && handle.x + handle.width <= line.x + 1, 'the handle stands before the line');
  assert.ok(Math.abs(handle.y - line.y) < line.height + 8, 'level with the line');
  const column = await page.locator('article.column').boundingBox();
  assert.ok(column && handle.x >= column.x, 'the handle stands in the column\'s own gutter, not over the frame\'s edge');

  // Selected words: the bar above them; Bold makes them bold and keeps them selected.
  await page.keyboard.press(LINE_END);
  await page.keyboard.press(SELECT_TO_LINE_START);
  const bar = page.locator('[data-floating-toolbar]');
  await bar.waitFor({ timeout: 5_000 });
  const barBox = await bar.boundingBox();
  const words = await body.getByText('Welcome to the team.').boundingBox();
  assert.ok(barBox && words && barBox.y + barBox.height <= words.y + 2, 'the bar stands above the selection, not over it');
  const text = await body.boundingBox();
  assert.ok(Math.abs(barBox.x - words.x) < 2 && barBox.x >= text.x - 1, 'the bar starts where the words start, inside the text\'s width');
  await bar.getByRole('button', { name: 'Bold' }).click();
  await body.locator('strong', { hasText: 'Welcome to the team.' }).waitFor({ timeout: 5_000 });

  // The handle's menu moves the line without dragging (WCAG 2.5.7).
  await page.keyboard.press(LINE_END);
  await page.keyboard.press('Enter');
  await page.keyboard.type('Next line');
  await page.keyboard.press('ArrowUp');
  await page.locator('[data-block-grip]').click();
  await page.getByRole('menuitem', { name: 'Move down' }).click();
  await body.locator(':scope > :first-child', { hasText: 'Next line' }).waitFor({ timeout: 5_000 });
  // Writing goes on below the moved line, which keeps its words.
  await page.keyboard.press(LINE_END);
  await page.keyboard.press('Enter');
  await page.keyboard.type('Below the moved');
  await body.locator(':scope > :nth-child(3)', { hasText: 'Below the moved' }).waitFor({ timeout: 5_000 });
  assert.deepEqual((await body.locator(':scope > *').allInnerTexts()).slice(0, 3), ['Next line', 'Welcome to the team.', 'Below the moved']);
  // The menu acts on its own line, though the pointer crosses other lines on its way through it.
  await body.getByText('Next line').click();
  for (const more of ['Third line', 'Fourth line', 'Fifth line']) {
    await page.keyboard.press(LINE_END);
    await page.keyboard.press('Enter');
    await page.keyboard.type(more);
  }
  await body.getByText('Next line').click();
  const count = await body.locator(':scope > *').count();
  await page.locator('[data-block-grip]').click();
  const remove = page.getByRole('menuitem', { name: 'Remove' });
  const over = await remove.boundingBox();
  await page.mouse.move(over.x + 4, over.y + 2, { steps: 4 });
  await remove.click();
  await body.getByText('Next line').waitFor({ state: 'detached', timeout: 5_000 });
  assert.equal(await body.locator(':scope > *').count(), count - 1, 'one line went, the one the menu was opened at');
  for (const more of ['Third line', 'Fourth line', 'Fifth line']) assert.equal(await body.getByText(more).count(), 1, `${more} stays`);
  await body.getByText('Fifth line').click();

  // A checklist line offers *task from it* at its end; the collections open under the line (S-4).
  await page.keyboard.press(LINE_END);
  await page.keyboard.press('Enter');
  await page.keyboard.type('[ ] Book the train');
  const taskFrom = page.locator('[data-page-task-from]');
  await taskFrom.waitFor({ timeout: 5_000 });
  const chip = await taskFrom.boundingBox();
  const item = await body.locator('li p', { hasText: 'Book the train' }).boundingBox();
  assert.ok(chip && item && Math.abs(chip.y + chip.height / 2 - (item.y + item.height / 2)) < item.height, 'level with the line');
  await taskFrom.click();
  await page.locator('[data-box-popover]').getByRole('list', { name: 'Make this line a task in' }).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');
  await page.locator('[data-box-popover]').waitFor({ state: 'detached', timeout: 5_000 });
  await body.locator('li p', { hasText: 'Book the train' }).click();
  await page.keyboard.press(LINE_END);
  await page.keyboard.press('Enter');
  await page.keyboard.press('Enter');

  // "/" on an empty line opens what can be inserted at that line; a block's fields come after the choice.
  await page.keyboard.press(LINE_END);
  await page.keyboard.press('Enter');
  await page.keyboard.type('/');
  const panel = page.locator('[data-box-popover] [data-page-blocks]');
  await panel.waitFor({ timeout: 5_000 });
  assert.equal(await body.getByText('/', { exact: true }).count(), 1, 'the slash stands in the line, as typed');
  assert.equal(await panel.locator('input:not([type=file])').count(), 0, 'no block fields before a choice');
  // A listbox as every menu: the focus stays in the line, the arrows choose (wrapping), Enter takes.
  const list = panel.getByRole('listbox', { name: 'Insert' });
  const activeName = async () => (await page.locator(`[id="${await list.getAttribute('aria-activedescendant')}"]`).innerText()).split('\n')[0];
  assert.equal(await activeName(), 'Show a view', 'the first choice is active');
  assert.match(await panel.innerText(), /↑↓ choose/);
  await page.keyboard.press('ArrowUp');
  assert.equal(await activeName(), 'Embed', 'ArrowUp from the first wraps to the last');
  await page.keyboard.press('ArrowUp');
  await page.keyboard.press('ArrowUp');
  assert.equal(await activeName(), 'Add formula');
  await page.keyboard.press('Enter');
  await panel.locator('input:not([type=file])').waitFor({ timeout: 5_000 });
  assert.equal(await body.getByText('/', { exact: true }).count(), 0, 'the choice replaced the slash');
  await page.keyboard.press('Escape');
  await page.locator('[data-box-popover]').waitFor({ state: 'detached', timeout: 5_000 });
  // What is typed after "/" narrows the list and is marked; with nothing left it says so; erasing the slash closes it.
  await body.locator(':scope > :last-child').click();
  await page.keyboard.press(LINE_END);
  await page.keyboard.press('Enter');
  await page.keyboard.type('/emb');
  await panel.waitFor({ timeout: 5_000 });
  assert.equal(await list.getByRole('option').count(), 1, 'only the matching choice stays');
  assert.match(await list.getByRole('option').innerText(), /^Embed/);
  assert.equal(await list.locator('mark').first().innerText(), 'Emb', 'the typed words are marked');
  await page.keyboard.type('zz');
  await panel.getByText('No block matches').waitFor({ timeout: 5_000 });
  for (let i = 0; i < 6; i += 1) await page.keyboard.press('Backspace');
  await page.locator('[data-box-popover]').waitFor({ state: 'detached', timeout: 5_000 });
  // A file among the choices opens the system's file choice.
  await body.locator(':scope > :last-child').click();
  await page.keyboard.press(LINE_END);
  await page.keyboard.press('Enter');
  await page.keyboard.type('/');
  await panel.waitFor({ timeout: 5_000 });
  for (let i = 0; i < 4; i += 1) await page.keyboard.press('ArrowDown');
  assert.equal(await activeName(), 'Picture or file');
  const [chooser] = await Promise.all([page.waitForEvent('filechooser', { timeout: 5_000 }), page.keyboard.press('Enter')]);
  assert.ok(chooser, 'the file choice opened');

  assert.deepEqual(failures, []);
});

test('390 px: writing a page draws one compact row of tools at the foot, above the keyboard (§7, pwrite)', async (t) => {
  const browser = await sharedChromium(t);
  const handbook = PD_PAGES[0];
  const { page, failures, close } = await open(browser, { modulesOn: ALL, owner: true, width: 390, height: 844, hasTouch: true });
  t.after(close);
  await page.context().routeWebSocket(/\/realtime\/documents\/page\//, (ws) => {
    ws.onMessage(() => {});
    ws.send(EMPTY_DOCUMENT);
    ws.send(ASK_STATE);
  });
  await page.goto(new URL(`/pages/${handbook.id}`, page.url()).href);
  const body = page.getByRole('textbox', { name: /Product handbook/ });
  await body.getByText('Welcome to the team.').waitFor({ timeout: 10_000 });
  await body.getByText('Welcome to the team.').click();
  const tools = page.locator('[data-page-tools]');
  await tools.waitFor({ timeout: 5_000 });
  const bar = await tools.boundingBox();
  assert.ok(bar && bar.height <= 64, `the bar is ${bar?.height} px high, not one row`);
  assert.ok(bar && Math.abs(bar.y + bar.height - 844) <= 1, 'the bar does not stand at the foot');
  const buttons = await tools.getByRole('button').evaluateAll((all) => all.map((each) => each.getAttribute('aria-label') ?? each.textContent?.trim()));
  assert.ok(buttons.includes('Bold') && buttons.includes('Insert'), `the row holds ${buttons.join(', ')}`);
  const sizes = await tools.getByRole('button').evaluateAll((all) => all.map((each) => each.getBoundingClientRect()).map((r) => [Math.round(r.width), Math.round(r.height), Math.round(r.right)]));
  assert.ok(sizes.every(([w, h, right]) => w >= 44 && w <= 56 && h >= 44 && right <= 390), `the tools are ${JSON.stringify(sizes)}`);
  await tools.getByRole('button', { name: 'Insert' }).click();
  await page.locator('[data-page-blocks]').waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('the first writer opens and leaves a whiteboard without saving it; removing a sticky saves', async (t) => {
  const browser = await sharedChromium(t);
  const retro = PD_BOARDS.find((each) => each.title === 'Retro board');
  const { page, failures, close } = await open(browser, { modulesOn: ALL, owner: true });
  t.after(close);
  let synced = false;
  await page.context().routeWebSocket(/\/realtime\/documents\/whiteboard\//, (ws) => {
    ws.onMessage(() => {});
    ws.send(EMPTY_DOCUMENT);
    ws.send(ASK_STATE);
    synced = true;
  });
  const saves = [];
  page.on('request', (request) => {
    if (request.method() === 'PATCH' && new URL(request.url()).pathname.endsWith(`/whiteboards/${retro.id}`)) saves.push(request.postData());
  });
  await page.goto(new URL(`/whiteboards/${retro.id}`, page.url()).href);
  const sticky = page.getByText('What went well');
  await sticky.waitFor({ timeout: 10_000 });
  await page.waitForTimeout(2_500);
  assert.ok(synced, 'the live document connected');
  assert.deepEqual(saves, [], 'opening the board saved it');
  // Leaving through the sidebar: the board is torn down, and nothing changed.
  await page.locator('aside.sidenav').getByRole('treeitem', { name: 'Product handbook' }).first().click();
  await page.waitForFunction(() => location.pathname.startsWith('/pages/'), undefined, { timeout: 10_000 });
  await page.waitForTimeout(500);
  assert.deepEqual(saves, [], 'leaving the board saved it');

  await page.goBack();
  await sticky.waitFor({ timeout: 10_000 });
  await sticky.click();
  await page.keyboard.press('Delete');
  await page.waitForTimeout(2_500);
  assert.equal(saves.length, 1, 'a removal is saved');
  assert.doesNotMatch(saves[0], /What went well/);
  assert.deepEqual(failures, []);
});
