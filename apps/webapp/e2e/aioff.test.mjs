// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// With no provider and no consent, no screen shows an AI control, an AI placeholder or a prompt to
// set one up (UC-AI-01 check 1): the shared fixture's manifest announces no AI, and each screen a
// person reaches outside the administration's AI section is walked for anything AI draws - a
// `data-ai` mark, the ✦, an "AI" or "Suggest" control.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { COLLECTION, HUB, ITEMS, MANIFEST, signedIn } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const SCREENS = [
  '/',
  `/hubs/${HUB.id}`,
  `/collections/${COLLECTION.id}`,
  `/items/${ITEMS[0].id}`,
  '/jumble',
  '/search',
  '/pages',
  '/trash',
  '/profile',
  '/administration/rules',
];

test('chromium: with AI off no screen draws an AI control, placeholder or prompt (UC-AI-01 check 1)', { timeout: 120_000 }, async (t) => {
  assert.ok(!MANIFEST.features.ai_suggestions, 'the shared manifest announces no AI');
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());

  for (const screen of SCREENS) {
    await page.goto(`${served.origin}${screen}`);
    await page.locator('main').first().waitFor({ timeout: 10_000 }).catch(() => assert.fail(`${screen}: no main region`));
    await page.waitForLoadState('networkidle');
    assert.ok((await page.locator('main h1').count()) > 0, `${screen}: the screen drew no heading`);
    assert.equal(await page.getByText('This screen stopped working').count(), 0, `${screen}: the screen failed`);
    assert.equal(await page.locator('[data-ai]').count(), 0, `${screen}: an element marked as AI`);
    assert.equal(await page.getByText('✦').count(), 0, `${screen}: the AI mark`);
    assert.equal(await page.getByRole('button', { name: /\bAI\b|Suggest/ }).count(), 0, `${screen}: an AI control`);
    assert.equal(await page.getByRole('link', { name: /\bAI\b/ }).count(), 0, `${screen}: a way to set AI up`);
  }
  assert.deepEqual(failures, []);
});
