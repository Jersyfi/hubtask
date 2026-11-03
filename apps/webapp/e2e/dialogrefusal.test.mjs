// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A refusal while a dialog is open is said inside the dialog, above its actions, and the dialog
// keeps what was typed (user-experience.md §5.10): a new whiteboard the server refuses leaves the
// dialog open with the title and the reason, never a toast over a closed dialog.
//
// Chromium only: what this asserts is the rendered dialog.

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

test('chromium: a refused new whiteboard keeps its dialog open with the reason inside it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith(`/hubs/${HUB.id}/whiteboards`) && request.method() === 'POST') {
      return route.fulfill({
        status: 422,
        contentType: 'application/problem+json',
        json: { type: 'about:blank', title: 'validation.failed', status: 422, code: 'validation.failed', detail: 'validation.failed' },
      });
    }
    if (path.endsWith(`/hubs/${HUB.id}/whiteboards`)) return route.fulfill({ json: { data: [] } });
    return stub(route);
  });
  await page.goto(`${served.origin}/hubs/${HUB.id}/whiteboards`);
  await page.getByRole('button', { name: 'New whiteboard' }).first().click();
  const dialog = page.getByRole('dialog', { name: 'New whiteboard' });
  await dialog.getByLabel('Title').fill('Retro');
  await dialog.getByRole('button', { name: 'Make the whiteboard' }).click();

  await dialog.locator('.banner[data-tone=danger]').first().waitFor({ timeout: 5_000 });
  assert.equal(await dialog.isVisible(), true, 'the dialog closed on a refusal');
  assert.equal(await dialog.getByLabel('Title').inputValue(), 'Retro', 'the title was lost');
});
