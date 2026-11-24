// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// No use case check: the design system's Menu (design-system.md §4, wave 1) drawn as the bottom
// sheet at a phone's width, opened from inside a dialog that fills the screen. `Escape` closes one
// layer at a time - the sheet first, the dialog with what was typed in it only on a second press
// (layers.ts) - as MultiSelect's walk holds it for a popover's sheet. And its scroll shades
// (scrollEdges): a shade only at an edge with more beyond it, painted over the rows and never past
// the surface's rounded corners, in the story's four states, darkening the surface in both themes.
// Walked in the built workbench under
// the product's `style-src 'self'`.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium, firefox, webkit } from 'playwright';

import { serve } from './serve.mjs';
import { assertShades, frameBox, readEdges, scrollToShare, shadeChanges, shadeLightness } from './scrolledges.mjs';

const WORKBENCH = join(dirname(fileURLToPath(import.meta.url)), '..', '..', '..', 'packages', 'design-system', 'dist', 'workbench');
const ENGINES = { chromium, firefox, webkit };

for (const engine of (process.env.ENGINES ?? 'chromium').split(',')) {
  test(`a menu's sheet over a full-screen dialog is closed first by Escape, in ${engine}`, { timeout: 2 * 60_000 }, async () => {
    assert.ok(existsSync(join(WORKBENCH, 'index.html')), 'no built workbench: run `make workbench-build` first');
    const server = await serve(WORKBENCH, { headers: { 'content-security-policy': "style-src 'self'" } });
    const browser = await ENGINES[engine].launch().catch(async (error) => { await server.close(); throw error; });
    try {
      const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
      await page.goto(`${server.origin}/?story=wave-1-overlays-menu--in-dialog`);
      await page.getByRole('button', { name: 'Edit the step' }).click();
      const dialog = page.getByRole('dialog', { name: 'Edit the step' });
      await dialog.waitFor();
      await dialog.getByRole('textbox', { name: 'Title' }).fill('Ship it');

      const trigger = dialog.getByRole('button', { name: 'Step actions' });
      await trigger.click();
      const sheet = page.getByRole('dialog', { name: 'Step actions' });
      await sheet.waitFor();
      assert.equal(await sheet.evaluate((node) => node.tagName), 'DIALOG', 'the menu is not the sheet at 390');

      await page.keyboard.press('Escape');
      await sheet.waitFor({ state: 'hidden' });
      assert.equal(await dialog.isVisible(), true, 'Escape on the menu’s sheet closed the dialog under it');
      assert.equal(await dialog.getByRole('textbox', { name: 'Title' }).inputValue(), 'Ship it', 'the dialog lost its answer');

      await page.keyboard.press('Escape');
      await dialog.waitFor({ state: 'hidden' });
    } finally {
      await browser.close();
      await server.close();
    }
  });
}

for (const engine of (process.env.ENGINES ?? 'chromium').split(',')) {
  test(`a list's scroll shades show only at an edge with more beyond it, over the rows and inside the corners, in ${engine}`, { timeout: 2 * 60_000 }, async () => {
    assert.ok(existsSync(join(WORKBENCH, 'index.html')), 'no built workbench: run `make workbench-build` first');
    const server = await serve(WORKBENCH, { headers: { 'content-security-policy': "style-src 'self'" } });
    const browser = await ENGINES[engine].launch().catch(async (error) => { await server.close(); throw error; });
    try {
      const page = await browser.newPage({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 2 });
      await page.goto(`${server.origin}/?story=wave-1-overlays-menu--edges`);
      const list = (name) => page.getByRole('region', { name }).getByRole('list');
      await list('More on both sides').waitFor();

      // The four states the story holds.
      const cases = [
        ['Nothing to scroll', { start: false, end: false }],
        ['More below', { start: false, end: true }],
        ['More on both sides', { start: true, end: true }],
        ['More above', { start: true, end: false }],
      ];
      for (const [name, want] of cases) {
        const read = await readEdges(list(name));
        assert.equal(read.scrolls, name !== 'Nothing to scroll', `${name}: the story's list ${read.scrolls ? 'scrolls' : 'does not scroll'}`);
        assertShades(assert, read, want, name);
      }

      // Scrolled by hand, the shades follow: top, middle, end of the same list.
      const moving = list('More below');
      for (const [share, want] of [[0.5, { start: true, end: true }], [1, { start: true, end: false }], [0, { start: false, end: true }]]) {
        await scrollToShare(moving, share);
        assertShades(assert, await readEdges(moving), want, `More below, scrolled to ${share}`);
      }

      // Over the rows: with the row at the top under the pointer, the shade still changes the pixels
      // there - a shade behind the rows would be covered by the hovered row's colour.
      const both = list('More on both sides');
      const frame = await frameBox(both);
      await page.mouse.move(frame.x + frame.width / 2, frame.y + 6);
      const strip = { x: frame.x + frame.width / 4, y: frame.y + 2, width: frame.width / 2, height: 3 };
      assert.ok(await shadeChanges(page, both, 'start', strip), 'the shade at the top is hidden by the hovered row');
      // Inside the corners: the corner of the surface outside its rounding is the same with the shade
      // and without it - a shade that stood past the radius would darken it.
      const corner = { x: frame.x, y: frame.y, width: 3, height: 3 };
      assert.equal(await shadeChanges(page, both, 'start', corner), false, 'the shade at the top darkens the surface\'s corner past its rounding');
      const foot = { x: frame.x + frame.width - 3, y: frame.y + frame.height - 3, width: 3, height: 3 };
      assert.equal(await shadeChanges(page, both, 'end', foot), false, 'the shade at the bottom darkens the surface\'s corner past its rounding');

      // A shade darkens the surface in either theme - a lighter band on a dark surface reads as a
      // bar, not as more to come.
      for (const theme of ['light', 'dark']) {
        await page.goto(`${server.origin}/?story=wave-1-overlays-menu--edges&theme=${theme}`);
        await list('More on both sides').waitFor();
        const box = await frameBox(list('More on both sides'));
        for (const [edge, y] of [['start', box.y + 1], ['end', box.y + box.height - 5]]) {
          const { shown, hidden } = await shadeLightness(page, list('More on both sides'), edge, { x: box.x + box.width / 4, y, width: box.width / 2, height: 4 });
          assert.ok(shown < hidden, `${theme}: the shade at the ${edge} lightens the surface (${shown.toFixed(3)} with it, ${hidden.toFixed(3)} without)`);
        }
      }
    } finally {
      await browser.close();
      await server.close();
    }
  });
}
