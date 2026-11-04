// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The tour survives a full page load (design-system.md §8): a tab whose server names a newer
// build moves to it at the next navigation (api-guidelines.md §8), and the tour's first move to
// another screen is such a navigation. The tour then went on nowhere - no mark, the field still
// null - and came back only at the next sign-in. Walked to its end here at both widths.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, forgetUnstubbed, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const FRESH = { ...ACCOUNT, id: '01a0e2e0-0000-7000-8000-0000000032b7', onboarding_completed_at: null };

for (const [width, height] of [[1280, 860], [375, 812]]) {
  test(`chromium: a newer server build mid-tour does not end the tour at ${width}x${height}`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const context = await browser.newContext({ viewport: { width, height } });
    t.after(() => context.close());
    forgetUnstubbed();
    const documents = [];
    context.on('request', (request) => { if (request.resourceType() === 'document') documents.push(new URL(request.url()).pathname); });
    await context.route('**/api/v1/**', async (route) => {
      const url = new URL(route.request().url());
      // Every answer names another build than the bundle's: the next navigation is a full load.
      const newer = new Proxy(route, {
        get(target, name) {
          if (name === 'fulfill') {
            return (options = {}) => target.fulfill({ ...options, headers: { ...(options.headers ?? {}), 'X-Hubtask-Build': '/assets/index-newer.js' } });
          }
          const value = Reflect.get(target, name);
          return typeof value === 'function' ? value.bind(target) : value;
        },
      });
      if (url.pathname.endsWith('/api/v1/accounts/me')) return newer.fulfill({ json: FRESH });
      if (url.pathname.endsWith('/preferences') && route.request().method() === 'PATCH') {
        return newer.fulfill({ json: { ...FRESH, ...route.request().postDataJSON() } });
      }
      return stub(newer);
    });
    await context.addInitScript(() => {
      sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
      sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    });
    const page = await context.newPage();
    await page.goto(`${served.origin}/`);
    const spotlight = page.locator('.spotlight');
    await spotlight.waitFor({ state: 'attached', timeout: 15_000 });

    const seen = [];
    const another = () => spotlight.waitFor({ state: 'attached', timeout: 8_000 }).then(() => true, () => false);
    for (let step = 0; step < 20 && (await another()); step += 1) {
      const mark = page.locator('[role="dialog"][aria-labelledby^="coach-"]');
      await mark.waitFor({ state: 'visible', timeout: 5_000 });
      const count = (await mark.locator('.count').textContent())?.trim() ?? '';
      if (seen.at(-1) !== count) seen.push(count);
      await mark.getByRole('button').first().click({ timeout: 3_000 });
      await page.waitForTimeout(200);
    }
    assert.ok(documents.length > 1, `no full load happened: ${JSON.stringify(documents)}`);
    const of = /of (\d+)/.exec(seen[0] ?? '')?.[1];
    // The fixture draws no entry opener, so the tour passes over its last step, as in tourclick.
    assert.ok(of && seen.includes(`Step ${Number(of) - 1} of ${of}`), `the tour stopped after ${JSON.stringify(seen)} (documents ${JSON.stringify(documents)})`);
  });
}
