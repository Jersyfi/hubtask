// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The whiteboard screens as user-experience.md §7.2 draws them: the canvas full-bleed with its
// tools floating over it, every side sheet closed until its control opens it, a sheet that lies
// over the canvas without moving it or hiding the tools, the tools at the foot on a phone with no
// sideways scroll, and an empty hub that starts a board from a template in one press.
//
// Chromium only: what this asserts is the rendered layout.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, HUB, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const BOARD_ID = '01a0e2e0-0000-7000-8000-00000000b0d1';
const ELEMENTS = [
  { id: 'f1', kind: 'FRAME', x: 0, y: 0, width: 420, height: 720, text: 'Went well' },
  { id: 's1', kind: 'STICKY', x: 40, y: 40, width: 160, height: 160, text: 'Pairing', color_token: 'green' },
  { id: 's2', kind: 'STICKY', x: 2400, y: 1600, width: 160, height: 160, text: 'Far away', color_token: 'amber' },
];
const BOARD = {
  id: BOARD_ID, hub_id: HUB.id, title: 'Retro', element_count: ELEMENTS.length, elements: ELEMENTS, writable: true, trashed_at: null,
  created_by: ACCOUNT.id, created_at: '2026-10-30T09:00:00Z', updated_by: ACCOUNT.id, updated_at: '2026-10-30T09:00:00Z', version: 1,
};

async function open(browser, width, height, path, { boards = [], posted } = {}) {
  const { page, context, failures } = await signedIn(browser, width, height);
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const at = new URL(request.url()).pathname;
    if (at.endsWith(`/whiteboards/${BOARD_ID}`)) return route.fulfill({ json: BOARD });
    if (at.endsWith(`/whiteboards/${BOARD_ID}/comments`)) return route.fulfill({ json: { data: [] } });
    if (at.endsWith(`/hubs/${HUB.id}/whiteboards`) && request.method() === 'POST') {
      posted?.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: { ...BOARD, id: '01a0e2e0-0000-7000-8000-00000000b0d2' } });
    }
    if (at.endsWith(`/hubs/${HUB.id}/whiteboards`)) return route.fulfill({ json: { data: boards } });
    return stub(route);
  });
  await page.goto(`${served.origin}${path}`);
  return { page, context, failures };
}

const box = (locator) => locator.boundingBox();
const inside = (inner, outer) =>
  inner.x >= outer.x - 1 && inner.y >= outer.y - 1 && inner.x + inner.width <= outer.x + outer.width + 1 && inner.y + inner.height <= outer.y + outer.height + 1;

test('chromium: the board is full-bleed, its sheets closed until opened, and a sheet moves neither the canvas nor hides the tools', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, 1280, 860, `/whiteboards/${BOARD_ID}`);
  t.after(() => context.close());
  const canvas = page.getByRole('application');
  await canvas.locator('g.object').first().waitFor({ timeout: 20_000 });

  const stage = await box(page.locator('.stage'));
  const tools = await box(page.locator('[data-board-tools]'));
  assert.ok(inside(tools, stage), 'the tools lie over the canvas');
  assert.ok(Math.abs(tools.x + tools.width / 2 - (stage.x + stage.width / 2)) < 2, 'the tools stand at the top centre');
  assert.ok(tools.y - stage.y < 40, 'the tools stand at the top');
  assert.equal(await page.locator('[data-board-side]').count(), 0, 'a sheet was open on arrival');
  // The board opens showing all it holds: the far note is on the screen.
  assert.ok(await canvas.locator('g.object').count() >= 2, 'the board did not open on everything it holds');

  await page.locator('[data-board-sheet=comments]').click();
  const side = page.locator('[data-board-side]');
  await side.waitFor({ timeout: 5_000 });
  assert.equal(await page.evaluate(() => document.activeElement?.id), 'board-sheet-title', 'focus did not move to the sheet');
  const after = await box(page.locator('.stage'));
  assert.deepEqual([after.width, after.height], [stage.width, stage.height], 'opening the sheet resized the canvas');
  const sheet = await box(side);
  const moved = await box(page.locator('[data-board-tools]'));
  assert.ok(moved.x + moved.width <= sheet.x, 'the sheet covers the tools');

  await page.keyboard.press('Escape');
  await side.waitFor({ state: 'detached', timeout: 5_000 });
  assert.equal(await page.evaluate(() => document.activeElement?.getAttribute('data-board-sheet')), 'comments', 'focus did not return to the sheet’s control');
  assert.equal(await page.evaluate(() => document.scrollingElement.scrollWidth - innerWidth), 0);
  assert.deepEqual(failures, []);
});

test('chromium: on a phone the tools are at the foot of the board, all on the screen, and nothing scrolls sideways', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, 375, 812, `/whiteboards/${BOARD_ID}`);
  t.after(() => context.close());
  await page.getByRole('application').locator('g.object').first().waitFor({ timeout: 20_000 });

  const stage = await box(page.locator('.stage'));
  const tools = page.locator('[data-board-tools]');
  const bar = await box(tools);
  assert.ok(inside(bar, stage), 'the tools leave the board');
  assert.ok(stage.y + stage.height - (bar.y + bar.height) < 24, 'the tools are not at the foot');
  const viewport = { x: 0, y: 0, width: 375, height: 812 };
  for (const button of await tools.locator('button').all()) assert.ok(inside(await box(button), viewport), 'a tool is off the screen');
  const corner = await box(page.locator('.corner'));
  assert.ok(corner.y + corner.height <= bar.y, 'the view controls sit on the tools');
  assert.equal(await page.evaluate(() => document.scrollingElement.scrollWidth - innerWidth), 0, 'the page scrolls sideways');
  assert.deepEqual(failures, []);
});

test('chromium: an empty hub starts a board from a template in one press, named and drawn from it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const posted = [];
  const { page, context, failures } = await open(browser, 1280, 860, `/hubs/${HUB.id}/whiteboards`, { posted });
  t.after(() => context.close());
  const start = page.locator('[data-boards-start]');
  await start.waitFor({ timeout: 20_000 });
  assert.deepEqual(
    await start.locator('[data-board-start]').evaluateAll((cards) => cards.map((card) => card.getAttribute('data-board-start'))),
    ['BLANK', 'RETRO', 'BRAINSTORM', 'USER_STORY_MAP', 'KANBAN', 'FLOWCHART'],
  );
  await start.locator('[data-board-start=RETRO] button').click();
  const dialog = page.getByRole('dialog', { name: 'New whiteboard' });
  await dialog.waitFor({ timeout: 5_000 });
  assert.equal(await dialog.getByLabel('Title').inputValue(), 'Retrospective', 'the title is not the template’s name');
  assert.equal(await dialog.locator('[data-board-start=RETRO] button').getAttribute('aria-pressed'), 'true');
  await dialog.getByLabel('Questions of the retrospective').selectOption('RETRO_MAD_SAD_GLAD');
  await dialog.getByRole('button', { name: 'Make the whiteboard' }).click();
  await page.waitForFunction(() => location.pathname.startsWith('/whiteboards/'), null, { timeout: 5_000 });

  assert.equal(posted.length, 1);
  assert.equal(posted[0].title, 'Retrospective');
  const frames = posted[0].elements.filter((e) => e.kind === 'FRAME').map((e) => e.text);
  assert.deepEqual(frames, ['Mad', 'Sad', 'Glad'], 'the board was not made from the chosen questions');
  assert.deepEqual(failures, []);
});
