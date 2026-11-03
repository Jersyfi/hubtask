// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *Help* leads to a person (user-experience.md §5.8): *Copy details* puts the version, the page
// and the references of recent refusals on the clipboard - the page by its path only, so a search
// typed into the address never leaves with it.
//
// Chromium only: what this asserts is the dialog and the clipboard it writes.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { signedIn } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

test('chromium: Help copies the details for support, and no search with them', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: served.origin });
  await page.goto(`${served.origin}/today?q=salary%20review`);
  await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });
  await page.keyboard.press('?');
  const dialog = page.getByRole('dialog', { name: 'Help and shortcuts' });
  await dialog.getByRole('heading', { name: 'Getting help' }).waitFor({ timeout: 5_000 });
  await dialog.getByRole('button', { name: 'Copy details' }).click();
  await page.getByText('Details copied').first().waitFor({ timeout: 5_000 });

  const copied = await page.evaluate(() => navigator.clipboard.readText());
  assert.match(copied, /^Version: 0\.9\.0$/m);
  assert.match(copied, /^Page: \/today$/m);
  assert.match(copied, /^Workspace: /m);
  assert.match(copied, /^References: /m);
  assert.doesNotMatch(copied, /salary/);
  assert.deepEqual(failures, []);
});
