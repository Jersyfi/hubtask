// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A board of 5,000 objects stays smooth (UC-CNV-01 check 10): the canvas draws only
// what the view shows, so panning and zooming over the largest board a scene holds keeps its frames
// coming. Measured in a real browser, because the frame rate is the browser's: requestAnimationFrame
// counts the frames painted while the view is carried across the board.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { HUB, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const BOARD_ID = '01a0e2e0-0000-7000-8000-00000000b0a1';
const COLOURS = ['amber', 'lime', 'green', 'teal', 'blue', 'violet', 'magenta', 'red', 'orange', 'slate'];

/** The largest board a scene holds: 5,000 objects in a grid, notes, shapes, text and connectors. */
function largestBoard() {
  const elements = [];
  for (let i = 0; i < 5000; i += 1) {
    const x = (i % 100) * 220;
    const y = Math.floor(i / 100) * 220;
    const kind = i % 10 === 9 && i > 0 ? 'CONNECTOR' : i % 3 === 0 ? 'SHAPE' : i % 3 === 1 ? 'STICKY' : 'TEXT';
    if (kind === 'CONNECTOR') {
      elements.push({ id: `e${i}`, kind, x, y, width: 0, height: 0, from: `e${i - 1}`, to: `e${i - 2}`, route: 'STRAIGHT', start: 'NONE', end: 'ARROW' });
    } else {
      elements.push({
        id: `e${i}`, kind, x, y, width: 160, height: 120, text: `Idea ${i}`,
        ...(kind === 'SHAPE' ? { shape: 'ROUNDED' } : {}), ...(kind === 'STICKY' ? { color_token: COLOURS[i % COLOURS.length] } : {}),
      });
    }
  }
  return {
    id: BOARD_ID, hub_id: HUB.id, title: 'Big retro', element_count: elements.length, elements, writable: false, trashed_at: null,
    created_by: HUB.id, created_at: '2026-10-30T09:00:00Z', updated_by: HUB.id, updated_at: '2026-10-30T09:00:00Z', version: 1,
  };
}

test('chromium: panning and zooming a board of 5,000 objects keeps its frames coming', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await signedIn(browser, 1440, 900);
  t.after(() => context.close());
  const board = largestBoard();
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith(`/whiteboards/${BOARD_ID}`)) return route.fulfill({ json: board });
    if (path.endsWith(`/whiteboards/${BOARD_ID}/comments`)) return route.fulfill({ json: { data: [] } });
    if (path.endsWith(`/hubs/${HUB.id}/whiteboards`)) return route.fulfill({ json: { data: [] } });
    return stub(route);
  });
  await page.goto(`${served.origin}/whiteboards/${BOARD_ID}`);
  const canvas = page.getByRole('application');
  await canvas.waitFor({ timeout: 20_000 });
  await page.waitForTimeout(500);

  const box = await canvas.boundingBox();
  assert.ok(box, 'the canvas is drawn');
  const centre = { x: box.x + box.width / 2, y: box.y + box.height / 2 };

  /** The frames painted while the hand carries the view across the board and the wheel moves it. */
  async function measured() {
    await page.evaluate(() => {
      const probe = { frames: [], running: true };
      const tick = (at) => {
        probe.frames.push(at);
        if (probe.running) requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
      globalThis.__frames = probe;
    });
    await page.mouse.move(centre.x, centre.y);
    await page.mouse.down();
    for (let step = 1; step <= 60; step += 1) await page.mouse.move(centre.x - step * 6, centre.y - step * 3);
    await page.mouse.up();
    for (let step = 0; step < 10; step += 1) await page.mouse.wheel(0, step < 5 ? 120 : -120);
    await page.waitForTimeout(300);
    const frames = await page.evaluate(() => {
      globalThis.__frames.running = false;
      return globalThis.__frames.frames;
    });
    const gaps = frames.slice(1).map((at, i) => at - frames[i]).sort((a, b) => a - b);
    return {
      frames: gaps.length,
      median: gaps[Math.floor(gaps.length / 2)] ?? Number.POSITIVE_INFINITY,
      worst: gaps[Math.floor(gaps.length * 0.95)] ?? Number.POSITIVE_INFINITY,
    };
  }
  const said = (name, drawn, m) =>
    t.diagnostic(`${name}, ${drawn} groups drawn: ${m.frames} frames, median ${m.median.toFixed(1)} ms (${(1000 / m.median).toFixed(0)} fps), 95th percentile ${m.worst.toFixed(1)} ms`);

  // At the board's own size only what the view shows is drawn.
  assert.match((await canvas.getAttribute('aria-label')) ?? '', /5000/u);
  const near = await canvas.locator('g').count();
  assert.ok(near < 1000, `${near} groups are drawn at 100 %`);
  const close = await measured();
  said('At 100 %', near, close);
  assert.ok(close.frames >= 20, `only ${close.frames} frames were painted while the board moved`);
  // Smooth is a frame at least every 33 ms in the middle of the gesture (30 fps), with no stall of
  // more than a tenth of a second in its slowest twentieth.
  assert.ok(close.median <= 1000 / 30, `the median frame took ${close.median.toFixed(1)} ms`);
  assert.ok(close.worst <= 100, `the slowest frames took ${close.worst.toFixed(1)} ms`);

  // The board zoomed out as far as the view goes: the most of it drawn at once, the worst a board of
  // 5,000 asks.
  await page.getByRole('button', { name: 'Show everything' }).click();
  await page.waitForTimeout(500);
  const whole = await canvas.locator('g').count();
  assert.ok(whole > near, 'showing everything draws more of the board');
  const far = await measured();
  said('Zoomed out', whole, far);
  assert.ok(far.frames >= 10, `only ${far.frames} frames were painted while the whole board moved`);
  assert.ok(far.median <= 1000 / 20, `the median frame of the whole board took ${far.median.toFixed(1)} ms`);
  assert.deepEqual(failures, []);
});
