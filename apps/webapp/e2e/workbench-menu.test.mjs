// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// No use case check: the design system's Menu (design-system.md §4, wave 1) drawn as the bottom
// sheet at a phone's width, opened from inside a dialog that fills the screen. `Escape` closes one
// layer at a time - the sheet first, the dialog with what was typed in it only on a second press
// (layers.ts) - as MultiSelect's walk holds it for a popover's sheet. Walked in the built workbench
// under the product's `style-src 'self'`.

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
