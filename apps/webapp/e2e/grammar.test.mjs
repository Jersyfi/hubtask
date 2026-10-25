// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The layout walk (design-system.md §13): every screen the fixture draws, at the four widths, held to
// G1-G4 by measurement - one edge for the head, the trail and the toolbar; one control size in the
// bar; no word broken across lines and no name cut without an ellipsis; an empty state centred. A
// screen added later is added to ROUTES with it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { COLLECTION, HUB, showToday, signedIn } from './fixture.mjs';
import { measure } from './grammar.mjs';

const served = await serve(join(dirname(fileURLToPath(import.meta.url)), '..', 'dist'));
test.after(() => served.close());

const ROUTES = [
  '/', '/search', '/jumble', '/archive', '/trash', `/hubs/${HUB.id}`, `/collections/${COLLECTION.id}`,
  `/hubs/${HUB.id}/settings/general`, `/collections/${COLLECTION.id}/settings/structure`, `/collections/${COLLECTION.id}/settings/labels`,
  '/profile', '/profile/appearance', '/profile/notifications', '/profile/security', '/profile/sessions', '/profile/devices', '/profile/apps', '/profile/tokens',
  '/administration', '/administration/workspace', '/administration/presets', '/administration/quotas', '/administration/people', '/administration/machines',
  '/administration/sign-in', '/administration/legal', '/administration/rules', '/administration/runs', '/administration/webhooks',
  '/administration/private-hubs', '/administration/backup', '/administration/retention', '/administration/restore', '/administration/audit',
  '/administration/privacy', '/administration/ai', '/installation',
];

for (const width of [390, 800, 1280, 1600]) {
  test(`chromium: ${width} px — every screen keeps the grammar`, async (t) => {
    showToday();
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, context } = await signedIn(browser, width, 900, { timezoneId: 'UTC' });
    t.after(() => context.close());
    const findings = [];
    for (const route of ROUTES) {
      await page.goto(`${served.origin}${route}`);
      await page.locator('main').waitFor({ timeout: 10_000 });
      // The screen's reads land and its layout settles before it is measured.
      await page.waitForTimeout(500);
      for (const found of await page.evaluate(measure)) findings.push(`${route}: ${found}`);
    }
    assert.deepEqual(findings, []);
  });
}
