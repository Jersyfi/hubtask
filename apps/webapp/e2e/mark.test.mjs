// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-WRK-33 check 4, walked: the Hubtask mark in the app bar and as the browser tab's icon, in a
// light and a dark form, and the reduced mark below the use case's threshold.
//
// Each assertion reads what the engine rendered - a computed style, the pixels of the icon drawn
// from the served file - rather than the token file, so a mark that stopped following the theme,
// or an icon the policy blocked, fails here.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';
import { values } from '@hubtask/design-system';

import { signedIn } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

// The policy the binary sends on every answer, the icon's included (security.md §9).
const POLICY =
  "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; font-src 'self'; " +
  "connect-src 'self'; manifest-src 'self'; worker-src 'self' blob:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'";

const served = await serve(DIST, { headers: { 'content-security-policy': POLICY } });
test.after(() => served.close());

/** `#rrggbb` as the `rgb(r, g, b)` a computed style answers. */
const rgb = (hex) => {
  const n = Number.parseInt(hex.slice(1), 16);
  return `rgb(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255})`;
};

for (const scheme of ['light', 'dark']) {
  for (const width of [768, 1280]) {
    test(`chromium: ${width} px, ${scheme} — the bar carries the mark and the name, leading home`, async (t) => {
      const browser = await chromium.launch();
      t.after(() => browser.close());
      const { context, page, failures } = await signedIn(browser, width);
      await page.emulateMedia({ colorScheme: scheme });
      await page.goto(`${served.origin}/`);

      const home = page.locator('header a[href="/"]').filter({ has: page.locator('.logo') });
      await home.waitFor({ timeout: 15_000 });
      assert.match((await home.getAttribute('href')) ?? '', /^\/$/);
      assert.match((await home.innerText()).trim(), /^hubtask$/i, 'the link is named by the wordmark');
      assert.equal(await home.locator('[translate="no"]').count(), 1, 'the name is marked as not to be translated');
      assert.equal(await page.locator('html').getAttribute('data-theme'), scheme);

      const outline = home.locator('svg .outline');
      assert.equal(await outline.evaluate((node) => getComputedStyle(node).stroke), rgb(values[scheme].brand.outline));
      // The full form from the medium step: the core is drawn.
      assert.equal(await home.locator('svg .core').count(), 1);
      assert.equal(await home.locator('svg').getAttribute('aria-hidden'), 'true', 'the name is said once, by the text');

      assert.deepEqual(failures, []);
      await context.close();
    });
  }

  test(`chromium: ${scheme} — the tab icon is the reduced mark in the ${scheme} form, under the policy`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page } = await signedIn(browser, 1280);
    await page.emulateMedia({ colorScheme: scheme });
    const blocked = [];
    page.on('console', (message) => /Content Security Policy/i.test(message.text()) && blocked.push(message.text()));
    await page.goto(`${served.origin}/`);

    const href = await page.locator('link[rel="icon"]').getAttribute('href');
    assert.equal(href, '/favicon.svg');
    const answer = await page.request.get(`${served.origin}${href}`);
    assert.equal(answer.status(), 200);
    assert.equal(answer.headers()['content-type'], 'image/svg+xml');
    assert.ok(!(await answer.text()).includes('class="core"'), 'the tab icon is the reduced form, without a core');

    // Drawn the way a browser draws an icon: the file as an image, at the size of a tab icon's
    // grid, and the pixel on the outline's straight left edge read back.
    const pixel = await page.evaluate(async (source) => {
      const image = new Image();
      image.src = source;
      await image.decode();
      const canvas = document.createElement('canvas');
      canvas.width = 64;
      canvas.height = 64;
      const context2d = canvas.getContext('2d');
      context2d.drawImage(image, 0, 0, 64, 64);
      const [r, g, b] = context2d.getImageData(6, 32, 1, 1).data;
      return `rgb(${r}, ${g}, ${b})`;
    }, href);
    assert.equal(pixel, rgb(values[scheme].brand.outline));
    assert.deepEqual(blocked, []);
    await context.close();
  });
}
