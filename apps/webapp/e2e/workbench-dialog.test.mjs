// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// No use case check: the design system's unsized Dialog (design-system.md §4, wave 1) with a body
// taller than the window. It scrolls whole: its body keeps its height inside its frame and its foot
// follows the body, never drawn over it. And with the classic scrollbars a mouse's system draws,
// a dialog or drawer keeps one bar's room, on the box that scrolls (design-system.md §6, "One
// scrollbar"): the body of a sized dialog, of a sheet and of a drawer from the bottom, the panel of
// an unsized dialog and of a side drawer. Walked in the built workbench under the product's
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

/**
 * What `.panel` and `.body` keep for a scrollbar where the opener's overlay is open: the
 * `scrollbar-gutter` the stylesheet gives each with classic bars, and the pixels each keeps where
 * the system draws classic bars (`null` where it overlays them, as macOS without a mouse does).
 */
async function gutters(page, origin, story, opener) {
  await page.goto(`${origin}/?story=${story}`);
  await page.getByRole('button', { name: opener, exact: true }).click();
  const overlay = page.getByRole('dialog');
  await overlay.waitFor();
  return overlay.evaluate((node) => {
    const isClassic = document.documentElement.dataset.scrollbars !== 'overlay';
    delete document.documentElement.dataset.scrollbars;
    const kept = (box) => {
      const style = getComputedStyle(box);
      const pixels = box.offsetWidth - box.clientWidth - Number.parseFloat(style.borderInlineStartWidth) - Number.parseFloat(style.borderInlineEndWidth);
      return { declared: style.scrollbarGutter, pixels: isClassic ? pixels : null };
    };
    return { panel: kept(node.querySelector('.panel')), body: kept(node.querySelector('.body')) };
  });
}

test('with classic scrollbars a dialog or drawer keeps one bar\'s room, on the box that scrolls, in chromium', { timeout: 2 * 60_000 }, async () => {
  assert.ok(existsSync(join(WORKBENCH, 'index.html')), 'no built workbench: run `make workbench-build` first');
  const server = await serve(WORKBENCH, { headers: { 'content-security-policy': "style-src 'self'" } });
  const browser = await chromium.launch({ ignoreDefaultArgs: ['--hide-scrollbars'] }).catch(async (error) => { await server.close(); throw error; });
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
    await page.goto(server.origin);
    const zone = await page.evaluate(() => Number.parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--layout-nav-scrollbar')));
    assert.ok(zone > 0, 'no hit zone for the bar');
    const cases = [
      ['wave-1-overlays-dialog--size-m', 'Add people', 'body'],
      ['wave-2-structure-drawer--bottom', 'Open the details', 'body'],
      ['wave-2-structure-drawer--sized', 'Open the details', 'body'],
      ['wave-1-overlays-dialog--confirm', 'Delete permanently', 'panel'],
      ['wave-2-structure-drawer--end', 'Open the details', 'panel'],
    ];
    for (const [story, opener, scroller] of cases) {
      const kept = await gutters(page, server.origin, story, opener);
      const other = scroller === 'body' ? 'panel' : 'body';
      assert.equal(kept[scroller].declared, 'stable', `${story}: its ${scroller}, the box that scrolls, keeps no gutter`);
      assert.equal(kept[other].declared, 'auto', `${story}: its ${other} keeps a second gutter`);
      if (kept[scroller].pixels === null) continue;
      assert.equal(kept[scroller].pixels, zone, `${story}: its ${scroller} keeps ${kept[scroller].pixels} px, not the bar's ${zone}`);
      assert.equal(kept[other].pixels, 0, `${story}: its ${other} keeps a second gutter of ${kept[other].pixels} px`);
    }
  } finally {
    await browser.close();
    await server.close();
  }
});
