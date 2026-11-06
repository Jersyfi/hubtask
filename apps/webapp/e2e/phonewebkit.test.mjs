// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// WebKit at a phone's width: a page whose layout answers its own size answers it in the next
// frame, so no resize observation is left undelivered - which WebKit throws as an uncaught error.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { webkit } from 'playwright';

import { serve } from './serve.mjs';
import { productWalk } from './productwalk.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

// CI runs Chromium only (ADR-0089); ENGINES=webkit runs this by hand.
const skip = Boolean(process.env.ENGINES) && !process.env.ENGINES.split(',').includes('webkit');

test('webkit: 390 px - the pages throw no resize observation left undelivered', { skip }, async (t) => {
  const browser = await webkit.launch();
  t.after(() => browser.close());
  const walk = await productWalk(browser, { modules: ['PLANNING', 'SERVICE_DESK', 'WORKFLOWS'], answer: () => undefined, width: 390, height: 844 });
  t.after(walk.close);
  const { page } = walk;
  for (const path of ['/', '/plans', '/desk', '/administration']) {
    await page.goto(`${served.origin}${path}`);
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(300);
    const thrown = walk.errors().filter((row) => /ResizeObserver/.test(JSON.stringify(row)));
    assert.deepEqual(thrown, [], `on ${path}`);
  }
});
