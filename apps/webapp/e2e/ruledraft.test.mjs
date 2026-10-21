// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *Describe it* (UC-AUT-10), driven the way a person drives it: with AI available the new-rule
// editor offers it, a sentence comes back as a rule drawn on the canvas with its sentence, what it
// could not draft named, the probe offered at once - and saving keeps the rule switched off; without
// AI nothing is drawn. Against the shared fixture: what is asserted is what the screen shows and
// what the save sends. Chromium only, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { AI_MANIFEST, DRAFTED_RULE_ID, MANIFEST, RULE_DRAFT_ID, SERVICE_ACCOUNT, signedIn } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

async function openEditor(browser, width, { ai, scheme = 'light' }) {
  const { page, context, failures } = await signedIn(browser, width, 900);
  await page.emulateMedia({ colorScheme: scheme });
  const accepted = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith('/api/v1/meta/capabilities')) return route.fulfill({ json: ai ? AI_MANIFEST : { ...MANIFEST, automation: AI_MANIFEST.automation, event_types: AI_MANIFEST.event_types } });
    if (path.endsWith('/api/v1/auth/service-accounts')) return route.fulfill({ json: [SERVICE_ACCOUNT] });
    if (path.endsWith(`/suggestions/${RULE_DRAFT_ID}:accept`)) accepted.push(request.postDataJSON());
    return route.fallback();
  });
  await page.goto(`${served.origin}/administration/rules/new`);
  await page.getByRole('button', { name: 'Save the rule' }).waitFor({ timeout: 15_000 });
  return { page, failures, accepted, close: () => context.close() };
}

async function evidence(page, name) {
  if (!process.env.EVIDENCE) return;
  await page.screenshot({ path: join(process.env.EVIDENCE, name), fullPage: false });
}

for (const [width, size] of [[1280, 'large'], [800, 'medium'], [375, 'compact']]) {
  for (const scheme of ['light', 'dark']) {
    test(`chromium: ${size}, ${scheme} — a sentence becomes a rule on the canvas, saved switched off`, { timeout: 90_000 }, async () => {
      const browser = await chromium.launch();
      const { page, failures, accepted, close } = await openEditor(browser, width, { ai: true, scheme });
      try {
        await page.getByRole('button', { name: 'Describe it' }).click();
        await page.getByLabel('What should happen?').fill('When a task is created, give it to Mara and set it due in two days.');
        await evidence(page, `ux-23-1-describe-${size}-${scheme}.png`);
        await page.getByRole('button', { name: 'Draft the rule' }).click();

        // The draft on the canvas, as a proposal: its heading, what it could not draft, and the
        // probe offered at once (UC-AUT-10 checks 3 and 4).
        await page.getByText('Suggested rule').first().waitFor({ timeout: 15_000 });
        await page.getByText('“set it due in two days”').waitFor({ timeout: 5_000 });
        await page.locator('[data-card="0"]').waitFor({ timeout: 5_000 });
        assert.equal(await page.getByRole('tab', { name: 'Probe' }).first().getAttribute('aria-selected'), 'true', 'the probe is offered at once');
        await evidence(page, `ux-23-2-drafted-${size}-${scheme}.png`);

        if (width >= 905) {
          // The runner is the person's to choose, and the save is the acceptance.
          await page.locator('button.chip').filter({ hasText: 'Runs as' }).click();
          await page.locator('aside.inspector').getByRole('combobox', { name: 'Runs as' }).selectOption(SERVICE_ACCOUNT.id);
          await page.getByRole('button', { name: 'Save the rule' }).click();
          await page.waitForURL(`**/administration/rules/${DRAFTED_RULE_ID}`, { timeout: 10_000 });
          assert.equal(accepted.length, 1, 'saving accepted the draft once');
          const body = accepted[0].overrides;
          assert.equal(body.run_as, SERVICE_ACCOUNT.id);
          assert.ok(body.name, 'the save carries a name');
          assert.equal(body.trigger.kind, 'EVENT');
          assert.equal(body.enabled, undefined, 'nothing switches the rule on');
          await page.getByText('Off', { exact: true }).first().waitFor({ timeout: 5_000 });
          await evidence(page, `ux-23-3-saved-off-${size}-${scheme}.png`);
        }
        assert.deepEqual(failures, []);
      } finally {
        await close();
        await browser.close();
      }
    });
  }
}

test('chromium: without AI the editor offers nothing to describe a rule with', { timeout: 60_000 }, async () => {
  const browser = await chromium.launch();
  const { page, failures, close } = await openEditor(browser, 1280, { ai: false });
  try {
    assert.equal(await page.getByRole('button', { name: 'Describe it' }).count(), 0);
    assert.equal(await page.locator('[data-ai]').count(), 0, 'no AI area drawn at all');
    assert.deepEqual(failures, []);
  } finally {
    await close();
    await browser.close();
  }
});
