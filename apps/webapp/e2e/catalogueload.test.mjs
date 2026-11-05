// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Every catalogue is a chunk loaded on demand (`i18n-l10n.md` §3): a reader whose browser asks for
// German, signed out, reads the door in German and downloads the German catalogue only - not the
// English one beside it. And one whose browser asks for English reads English.
//
// Against a stubbed API. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';
import { readFileSync } from 'node:fs';

import { chromium } from 'playwright';

import { stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const GERMAN = new Set(Object.values(JSON.parse(readFileSync(join(dirname(fileURLToPath(import.meta.url)), '..', '..', '..', 'locales', 'de.json'), 'utf8'))));
const served = await serve(DIST);
test.after(() => served.close());

async function door(browser, locale) {
  const context = await browser.newContext({ locale, viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', (route) => stub(route));
  const page = await context.newPage();
  const catalogues = [];
  page.on('request', (request) => {
    const match = /\/assets\/(en|de)-[^/]*\.js$/.exec(new URL(request.url()).pathname);
    if (match) catalogues.push(match[1]);
  });
  await page.goto(`${served.origin}/`);
  await page.waitForFunction((lang) => document.documentElement.lang === lang, locale.slice(0, 2), { timeout: 15_000 });
  await page.waitForSelector('h1', { timeout: 15_000 });
  await page.waitForLoadState('networkidle');
  return { context, page, catalogues };
}

test('chromium: a German browser loads the German catalogue and not the English one', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, catalogues } = await door(browser, 'de-DE');
  t.after(() => context.close());
  assert.deepEqual(catalogues, ['de']);
  const heading = (await page.locator('h1').first().innerText()).trim();
  assert.ok(GERMAN.has(heading), `the heading is not a German sentence: ${heading}`);
});

test('chromium: an English browser loads the English catalogue', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, catalogues } = await door(browser, 'en-US');
  t.after(() => context.close());
  assert.deepEqual(catalogues, ['en']);
});
