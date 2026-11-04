// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Who else is on a board (UC-CNV-05 check 3): a reader who may not write sees another person's
// cursor with their name and the list of who is there, follows their view, and stops following
// once they move the board themselves. The realtime socket is played by the test, speaking the
// editors' framing: a sync step 2 with an empty drawing, then one awareness update for Ada.

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

const BOARD_ID = '01a0e2e0-0000-7000-8000-00000000b0e1';
const BOARD = {
  id: BOARD_ID, hub_id: HUB.id, title: 'Retro', element_count: 1, writable: false, trashed_at: null,
  elements: [{ id: 's1', kind: 'STICKY', x: 40, y: 40, width: 160, height: 160, text: 'Pairing', color_token: 'green' }],
  created_by: ACCOUNT.id, created_at: '2026-10-30T09:00:00Z', updated_by: ACCOUNT.id, updated_at: '2026-10-30T09:00:00Z', version: 1,
};

const ADA = 4242;

function varUint(n) {
  const out = [];
  while (n > 0x7f) { out.push(0x80 | (n & 0x7f)); n = Math.floor(n / 128); }
  out.push(n);
  return out;
}
const varBytes = (bytes) => [...varUint(bytes.length), ...bytes];

/** One awareness message for Ada at a clock, in y-protocols' encoding inside the editors' frame. */
function awareness(clock, state) {
  const update = [...varUint(1), ...varUint(ADA), ...varUint(clock), ...varBytes([...Buffer.from(JSON.stringify(state))])];
  return Buffer.from([...varUint(1), ...varBytes(update)]);
}
const EMPTY_DRAWING = Buffer.from([0, 1, ...varBytes([0, 0])]);

/** The middle of what the canvas shows, read from its viewBox. */
async function centreOf(page) {
  const [x, y, w, h] = (await page.getByRole('application').getAttribute('viewBox')).split(/\s+/).map(Number);
  return { x: x + w / 2, y: y + h / 2 };
}

test('chromium: a reader sees who is there, follows them, and stops when they move the board', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  const told = [];
  let socket;
  await context.routeWebSocket(/\/realtime\/documents\/whiteboard\//, (ws) => {
    socket = ws;
    ws.onMessage((message) => told.push(message));
    ws.send(EMPTY_DRAWING);
    ws.send(awareness(1, { name: 'Ada', cursor: { x: 120, y: 80 }, view: { x: 2000, y: 1500, zoom: 1 } }));
  });
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const at = new URL(route.request().url()).pathname;
    if (at.endsWith(`/whiteboards/${BOARD_ID}`)) return route.fulfill({ json: BOARD });
    if (at.endsWith(`/whiteboards/${BOARD_ID}/comments`)) return route.fulfill({ json: { data: [] } });
    if (at.endsWith(`/hubs/${HUB.id}/whiteboards`)) return route.fulfill({ json: { data: [] } });
    return stub(route);
  });
  await page.goto(`${served.origin}/whiteboards/${BOARD_ID}`);

  const canvas = page.getByRole('application');
  await canvas.locator('g.peer').getByText('Ada', { exact: true }).waitFor({ timeout: 15_000 });
  assert.ok(told.length > 0, 'the reader told the others nothing of where they are');

  await page.locator('[data-board-sheet="session"]').click();
  const follow = page.getByRole('button', { name: 'Follow', exact: true });
  await page.getByRole('heading', { name: 'Here now' }).waitFor();
  await follow.click();
  await page.getByRole('button', { name: 'Stop following' }).waitFor();
  let centre = await centreOf(page);
  assert.ok(Math.abs(centre.x - 2000) < 2 && Math.abs(centre.y - 1500) < 2, `the follower looks at ${JSON.stringify(centre)}`);

  // Ada moves on, and the follower with her.
  socket.send(awareness(2, { name: 'Ada', cursor: { x: 300, y: 200 }, view: { x: 600, y: 400, zoom: 1 } }));
  await page.waitForFunction(() => {
    const [x, , w] = document.querySelector('[role="application"]').getAttribute('viewBox').split(/\s+/).map(Number);
    return Math.abs(x + w / 2 - 600) < 2;
  });

  // The reader carries the board themselves: following ends, and Ada's next move leaves the view.
  const area = await canvas.boundingBox();
  await page.mouse.move(area.x + area.width / 2, area.y + area.height / 2);
  await page.mouse.down();
  await page.mouse.move(area.x + area.width / 2 - 80, area.y + area.height / 2 - 40, { steps: 5 });
  await page.mouse.up();
  await follow.waitFor();
  centre = await centreOf(page);
  socket.send(awareness(3, { name: 'Ada', cursor: { x: 0, y: 0 }, view: { x: -3000, y: -3000, zoom: 1 } }));
  await page.waitForTimeout(300);
  const after = await centreOf(page);
  assert.ok(Math.abs(after.x - centre.x) < 2 && Math.abs(after.y - centre.y) < 2, 'the view still followed after the reader moved it');
  assert.deepEqual(failures, []);
});
