// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// No use case check: the design system's unsized Dialog (design-system.md §4, wave 1) with a body
// taller than the window. It scrolls whole: its body keeps its height inside its frame and its foot
// follows the body, never drawn over it. Walked in the built workbench under the product's
// `style-src 'self'`.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium, firefox, webkit } from 'playwright';

import { serve } from './serve.mjs';

const WORKBENCH = join(dirname(fileURLToPath(import.meta.url)), '..', '..', '..', 'packages', 'design-system', 'dist', 'workbench');
const ENGINES = { chromium, firefox, webkit };

for (const engine of (process.env.ENGINES ?? 'chromium').split(',')) {
  test(`an unsized dialog taller than the window scrolls whole, its foot after its body, in ${engine}`, { timeout: 2 * 60_000 }, async () => {
    assert.ok(existsSync(join(WORKBENCH, 'index.html')), 'no built workbench: run `make workbench-build` first');
    const server = await serve(WORKBENCH, { headers: { 'content-security-policy': "style-src 'self'" } });
    const browser = await ENGINES[engine].launch().catch(async (error) => { await server.close(); throw error; });
    try {
      const page = await browser.newPage({ viewport: { width: 1280, height: 700 } });
      await page.goto(`${server.origin}/?story=wave-1-overlays-dialog--confirm`);
      await page.getByRole('button', { name: 'Delete permanently', exact: true }).click();
      const dialog = page.getByRole('dialog');
      await dialog.waitFor();
      assert.equal(await dialog.evaluate((node) => node.hasAttribute('data-size')), false, 'the story\'s dialog is sized: this walk does not reach its case');
      // Content a caller could put there: many more lines than the window holds.
      const drawn = await dialog.evaluate((node) => {
        const body = node.querySelector('.body');
        for (let line = 0; line < 40; line++) body.append(Object.assign(document.createElement('p'), { textContent: `Line ${line + 1}` }));
        const box = (element) => element.getBoundingClientRect();
        const frame = body.parentElement;
        return { body: box(body).height, frame: box(frame).height, bodyEnd: box(body).bottom, footStart: box(node.querySelector('.actions')).top };
      });
      assert.ok(drawn.frame >= drawn.body - 0.5, `the frame (${drawn.frame} px) is shorter than its body (${drawn.body} px): the body spills out of it`);
      assert.ok(drawn.footStart >= drawn.bodyEnd - 0.5, `the foot starts at ${drawn.footStart}, over the body that ends at ${drawn.bodyEnd}`);
    } finally {
      await browser.close();
      await server.close();
    }
  });
}
