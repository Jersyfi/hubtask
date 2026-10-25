// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Voice assistants (ADR-0093, UC-WRK-35), the administrator's page, walked against the shared
// fixture with the voice routes answered here:
//
// * check 1 - spoken entries are switched on and a default list chosen, and saved;
// * check 2 - a name people say is set for a list;
// * check 3 - a sentence is tried and the page says where its things would go, nothing added;
// * check 6 - connecting Alexa: the endpoint, the model and the account linking's values to copy,
//   the linking's client made from the vendor ID with Alexa's three addresses.
//
// Chromium, large and compact.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { COLLECTION, signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

for (const [width, size] of [[1280, 'large'], [375, 'compact']]) {
  test(`chromium: ${size} — a workspace switches voice on, names a list, tries a sentence and connects Alexa`, { timeout: 90_000 }, async () => {
    const browser = await chromium.launch();
    const { context, page, failures } = await signedIn(browser, width, 900);
    const writes = [];
    let settings = { enabled: false, aliases: [], installation_skill: false };
    await context.route('**/api/v1/**', async (route) => {
      const request = route.request();
      const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
      if (request.method() !== 'GET') writes.push({ call: `${request.method()} ${path}`, body: request.postDataJSON?.() ?? null });
      if (path === '/voice/settings' && request.method() === 'GET') return route.fulfill({ json: settings });
      if (path === '/voice/settings' && request.method() === 'PUT') {
        settings = { ...request.postDataJSON(), installation_skill: false };
        return route.fulfill({ json: settings });
      }
      if (path === '/voice:try') {
        return route.fulfill({ json: { things: [{ title: 'Milk', amount: '2 l' }, { title: 'Eggs' }], found: true, default: false, place: { collection_id: COLLECTION.id, name: 'Kitchen' }, ambiguous: [] } });
      }
      if (path === '/voice/alexa/model') return route.fulfill({ json: { interactionModel: { languageModel: { invocationName: 'hubtask' } } } });
      if (path === '/oauth/clients' && request.method() === 'POST') {
        return route.fulfill({ status: 201, json: { id: '01a0e2e0-0000-7000-8000-00000000a1e1', name: 'Alexa', redirect_uris: request.postDataJSON().redirect_uris, confidential: true, created_at: '2026-10-25T10:00:00Z', client_secret: 'hbt_ocs_once' } });
      }
      return stub(route);
    });
    try {
      await page.goto(`${served.origin}/administration/voice`);
      const on = page.locator('[data-voice-settings]');
      await on.waitFor({ timeout: 15_000 });

      // Checks 1 and 2: on, a default list, a name - saved together.
      await on.getByText('Take spoken entries').click();
      await on.getByLabel('Default list').selectOption({ value: COLLECTION.id });
      const names = page.locator('[data-voice-names]');
      await names.getByRole('button', { name: 'Add a name' }).click();
      await names.getByLabel('Said as').fill('fridge');
      await page.getByRole('button', { name: 'Save', exact: true }).first().click();
      await page.waitForTimeout(300);
      const saved = writes.find((each) => each.call === 'PUT /voice/settings');
      assert.equal(saved?.body.enabled, true);
      assert.equal(saved?.body.default_collection_id, COLLECTION.id);
      assert.deepEqual(saved?.body.aliases, [{ phrase: 'fridge', collection_id: COLLECTION.id }]);

      // Check 3: a sentence tried, nothing added.
      const trying = page.locator('[data-voice-try]');
      await trying.getByRole('button', { name: 'Try' }).click();
      await trying.locator('[data-voice-result]').getByText('2 l Milk, Eggs would go on Kitchen.').waitFor({ timeout: 5_000 });

      // Check 6: the values to copy, and the linking made from the vendor ID.
      const alexa = page.locator('[data-voice-alexa]');
      await alexa.locator('[data-copy="endpoint"]').getByText(`${served.origin}/api/v1/voice/alexa`).waitFor();
      await alexa.locator('[data-copy="token"]').getByText(`${served.origin}/api/v1/oauth/token`).waitFor();
      await alexa.getByLabel('Vendor ID').fill('M2AB3CD');
      await alexa.getByRole('button', { name: 'Make the linking' }).click();
      await alexa.locator('[data-copy="client"]').getByText('01a0e2e0-0000-7000-8000-00000000a1e1').waitFor({ timeout: 5_000 });
      const made = writes.find((each) => each.call === 'POST /oauth/clients');
      assert.deepEqual(made?.body.redirect_uris, [
        'https://pitangui.amazon.com/api/skill/link/M2AB3CD',
        'https://layla.amazon.com/api/skill/link/M2AB3CD',
        'https://alexa.amazon.co.jp/api/skill/link/M2AB3CD',
      ]);
      assert.equal(made?.body.confidential, true);
      assert.equal(writes.filter((each) => each.call.startsWith('POST /items')).length, 0, 'trying added something');
      assert.deepEqual(failures, []);
    } finally {
      await context.close();
      await browser.close();
    }
  });
}
