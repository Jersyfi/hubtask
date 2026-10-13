// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The AI models an installation offers, walked as an operator meets them (UC-AI-06 checks 1, 2):
// a model is offered with its name, processor and where, the key goes in and never comes back, a
// withdrawal says what it costs, and the defaults screen says in words which AI workspaces may use.
//
// Against a stubbed API: what is asserted is what the screen sends and what it shows. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { MANIFEST, OFFERED_AI, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const LEVEL = {
  sign_in: {}, legal: {}, localisation: {}, quotas: {},
  ai: {
    sources: { set: true, value: 'OFFERED', locked: true, kind: 'CHOICE', options: ['EITHER', 'OFFERED', 'OWN', 'NONE'], workspace_may: 'TIGHTEN' },
    min_jurisdiction: { set: true, value: 'EEA', locked: false, kind: 'CHOICE', options: ['ANY', 'ADEQUACY', 'EEA', 'SELF_HOSTED'], workspace_may: 'TIGHTEN' },
  },
  source: 'database', is_enforced_from_file: false,
};

async function open(browser, { tenancy = 'multi' } = {}) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const sent = [];
  let models = [...OFFERED_AI];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, tenancy_mode: tenancy, instance: { reachable: true } } });
    }
    if (path === '/admin/settings' && request.method() === 'GET') return route.fulfill({ json: LEVEL });
    if (path === '/admin/ai-providers' && request.method() === 'GET') return route.fulfill({ json: models });
    if (path === '/admin/ai-providers' && request.method() === 'POST') {
      const body = request.postDataJSON();
      sent.push({ method: 'POST', path, body });
      const added = { ...body, id: '01a0e2e0-0000-7000-8000-0000000006a2', has_api_key: Boolean(body.api_key), created_at: '2026-10-12T08:00:00Z', version: 1 };
      delete added.api_key;
      models = [...models, added];
      return route.fulfill({ status: 201, json: added });
    }
    if (path.startsWith('/admin/ai-providers/') && request.method() === 'DELETE') {
      sent.push({ method: 'DELETE', path });
      models = models.filter((each) => !path.endsWith(each.id));
      return route.fulfill({ status: 204, body: '' });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  return { page, sent, close: () => context.close() };
}

test('chromium: an operator offers a model, and withdraws one knowing what it costs', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/instance/ai`);
  const row = page.getByRole('row').filter({ hasText: 'Large (EU)' });
  await row.waitFor({ timeout: 15_000 });
  // Name, processor and where - in words, never the contract's value.
  assert.match(await row.textContent() ?? '', /Example AI SAS/);
  assert.match(await row.textContent() ?? '', /In the European Economic Area/);
  assert.doesNotMatch(await row.textContent() ?? '', /\bEEA\b/);

  await page.getByRole('button', { name: 'Offer a model' }).click();
  const dialog = page.getByRole('dialog', { name: 'Offer a model' });
  await dialog.waitFor();
  await dialog.getByLabel(/^Name/).fill('Small (EU)');
  await dialog.getByLabel(/^Processor/).fill('Example AI SAS');
  await dialog.getByLabel(/^Address/).fill('https://models.example.eu/v1');
  await dialog.getByLabel(/^Model for proposals/).fill('small-1');
  await dialog.getByLabel(/^API key/).fill('the-offered-key');
  await dialog.getByRole('button', { name: 'Save' }).click();
  await dialog.waitFor({ state: 'hidden' });

  assert.equal(sent[0].method, 'POST');
  assert.equal(sent[0].body.display_name, 'Small (EU)');
  assert.equal(sent[0].body.processor, 'Example AI SAS');
  assert.equal(sent[0].body.jurisdiction, 'EEA');
  assert.equal(sent[0].body.api_key, 'the-offered-key');
  await page.getByRole('row').filter({ hasText: 'Small (EU)' }).waitFor();
  assert.doesNotMatch(await page.content(), /the-offered-key/);

  await row.getByRole('button', { name: 'Withdraw' }).click();
  const withdraw = page.getByRole('dialog', { name: 'Withdraw this model' });
  await withdraw.waitFor();
  assert.match(await withdraw.textContent() ?? '', /Every workspace that uses Large \(EU\) gets no AI answer until it chooses again/);
  await withdraw.getByRole('button', { name: 'Withdraw' }).click();
  await withdraw.waitFor({ state: 'hidden' });
  assert.equal(sent[1].method, 'DELETE');
  assert.match(sent[1].path, /0000000006a1$/);
});

test('chromium: the defaults say in words which AI workspaces may use', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/instance/settings`);
  const sources = page.getByRole('row').filter({ hasText: 'Workspaces may use' });
  await sources.waitFor({ timeout: 15_000 });
  assert.match(await sources.textContent() ?? '', /Only the offered model/);
  const minimum = page.getByRole('row').filter({ hasText: 'Their own model must process content' });
  assert.match(await minimum.textContent() ?? '', /In the EEA/);
  assert.doesNotMatch(await sources.textContent() ?? '', /OFFERED|ai\.sources/);
});

// UC-AI-06 check 8: a private installation (one workspace) has no model to offer, so neither the
// installation's navigation nor its defaults speak of an offered one.
test('chromium: a single-workspace installation draws no offered AI', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, { tenancy: 'single' });
  t.after(close);

  const read = page.waitForResponse((response) => response.url().endsWith('/api/v1/admin/settings'));
  await page.goto(`${served.origin}/instance/settings`);
  await read;
  await page.locator('nav').first().waitFor();
  await page.waitForTimeout(500);
  assert.equal(await page.getByRole('row').filter({ hasText: 'Workspaces may use' }).count(), 0, 'the AI area is drawn');
  const text = (await page.locator('main').textContent()) ?? '';
  assert.doesNotMatch(text, /offered model/i, 'the defaults speak of an offered model');
  assert.equal(await page.locator('a[href="/instance/ai"]').count(), 0, 'the installation offers an AI row');
});
