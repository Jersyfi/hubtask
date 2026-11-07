// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A custom field is labelled by its name, never its key (user-experience.md §5.6): the settings'
// list names it with the key beside, a new field's key follows the name typed until the key is
// written, the name travels with the definition, and an entry's field reads by the name.
//
// Chromium, large and at a phone's width.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { COLLECTION, ITEMS, signedIn, stub } from './fixture.mjs';
import { unfoldFields } from './reveal.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const NAMED = {
  id: '01a0e2e0-0000-7000-8000-0000000000c1', collection_id: COLLECTION.id, key: 'first_response', name: 'First response',
  kind: 'TEXT', options: [], currency: null, is_required: false, restricted: false, read_restricted: false, applies_to: ['TASK'], version: 1,
};
const ENTRY = { ...ITEMS[0], custom_fields: { first_response: 'Within the hour' } };

async function open(width, path) {
  const browser = await chromium.launch();
  const { context, page } = await signedIn(browser, width, 900);
  const written = [];
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const read = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (read === '/custom-fields' && request.method() === 'POST') {
      written.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: { ...NAMED, ...request.postDataJSON(), id: '01a0e2e0-0000-7000-8000-0000000000c2' } });
    }
    if (read === '/custom-fields') return route.fulfill({ json: [NAMED] });
    if (read === `/items/${ENTRY.id}`) return route.fulfill({ json: ENTRY });
    return stub(route);
  });
  await page.goto(`${served.origin}${path}`);
  await page.getByRole('main').waitFor();
  return { page, written, close: async () => { await context.close(); await browser.close(); } };
}

test('chromium: the field settings name a field, and a new key follows the name', { timeout: 90_000 }, async () => {
  for (const width of [1280, 390]) {
    const { page, written, close } = await open(width, `/collections/${COLLECTION.id}/settings/fields`);
    try {
      const part = page.getByRole('region', { name: 'Custom fields' });
      await part.getByText('First response', { exact: true }).waitFor({ timeout: 15_000 });
      assert.equal(await part.getByText('first_response', { exact: true }).count(), 1, 'the key stands beside the name');

      await part.getByLabel('Name', { exact: true }).fill('Customer account');
      assert.equal(await part.getByLabel('Key', { exact: true }).inputValue(), 'customer_account');
      await part.getByLabel('Key', { exact: true }).fill('client');
      await part.getByLabel('Name', { exact: true }).fill('Client account');
      assert.equal(await part.getByLabel('Key', { exact: true }).inputValue(), 'client', 'a key written by hand stays');

      await part.getByRole('button', { name: 'Define a field' }).click();
      for (let tries = 0; tries < 50 && written.length === 0; tries++) await page.waitForTimeout(100);
      assert.equal(written[0]?.name, 'Client account');
      assert.equal(written[0]?.key, 'client');
    } finally {
      await close();
    }
  }
});

test('chromium: an entry\'s custom field reads by its name', { timeout: 60_000 }, async () => {
  const { page, close } = await open(1280, `/items/${ENTRY.id}`);
  try {
    await page.getByRole('heading', { name: ENTRY.title }).first().waitFor({ timeout: 15_000 });
    await unfoldFields(page);
    await page.getByText('First response', { exact: true }).first().waitFor({ timeout: 15_000 });
    assert.equal(await page.getByText('first_response', { exact: true }).count(), 0, 'the key is not the label');
  } finally {
    await close();
  }
});
