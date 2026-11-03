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
import { productWalk } from './productwalk.mjs';

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

const SERVER_CLASS = {
  key: 'server', name: 'Server', parent: 'ci', system: false, lineage: ['ci', 'server'], version: 1,
  attributes: [], identifier_rules: [], required_relations: [], staleness_days: 0,
  all_attributes: [{ key: 'serial', kind: 'TEXT', need: 'OPTIONAL' }],
};

test('chromium: a refused new configuration item says why inside its dialog', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith('/cmdb/classes')) return route.fulfill({ json: { data: [SERVER_CLASS] } });
    if (path.endsWith(`/hubs/${HUB.id}/cis`) && request.method() === 'POST') {
      return route.fulfill({
        status: 422,
        contentType: 'application/problem+json',
        json: { type: 'about:blank', title: 'validation.failed', status: 422, code: 'validation.failed', detail: 'validation.failed' },
      });
    }
    return stub(route);
  });
  await page.goto(`${served.origin}/hubs/${HUB.id}/assets`);
  await page.getByRole('button', { name: 'New configuration item' }).first().click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Class', { exact: false }).first().selectOption('server');
  await dialog.getByLabel('Name', { exact: true }).fill('web-02');
  await dialog.getByRole('button', { name: 'Create', exact: true }).click();

  await dialog.locator('.banner[data-tone=danger]').first().waitFor({ timeout: 5_000 });
  assert.equal(await dialog.getByLabel('Name', { exact: true }).inputValue(), 'web-02');
});

const REFUSED = {
  status: 422,
  contentType: 'application/problem+json',
  json: { type: 'about:blank', title: 'validation.failed', status: 422, code: 'validation.failed', detail: 'validation.failed' },
};

test('chromium: a refused new forge keeps its dialog open with the name and the reason', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context } = await signedIn(browser, 1280, 1000);
  t.after(() => context.close());
  await context.route(`**/api/v1/hubs/${HUB.id}/code-hooks`, (route) =>
    route.request().method() === 'POST' ? route.fulfill(REFUSED) : route.fulfill({ json: { data: [] } }));
  await page.goto(`${served.origin}/hubs/${HUB.id}/settings/code`);
  await page.getByRole('button', { name: 'Add a forge' }).click();
  const dialog = page.getByRole('dialog', { name: 'Add a forge' });
  await dialog.getByRole('textbox', { name: 'Name' }).fill('website');
  await dialog.getByRole('button', { name: 'Add', exact: true }).click();

  await dialog.locator('.banner[data-tone=danger]').first().waitFor({ timeout: 5_000 });
  assert.equal(await dialog.getByRole('textbox', { name: 'Name' }).inputValue(), 'website');
  assert.equal(await page.locator('.toast[data-tone=danger]').count(), 0, 'the refusal was also a toast');
});

test('chromium: a refused new model says why inside its dialog', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const walk = await productWalk(browser, {
    modules: ['AGENTS'],
    answer: (method, path) => {
      if (path === '/agent-models' && method === 'POST') return REFUSED;
      if (path === '/agent-models' || path === '/agents' || path === '/agents:metering' || path === '/agent-requests') return { json: { data: [] } };
      if (path === '/agents:halt') return { json: { halted: false, set_by: null, set_at: null } };
      if (path === '/agents:policy') return { json: { 'agents.registered_only': { value: false, source: 'DEFAULT' } } };
      return undefined;
    },
  });
  t.after(walk.close);
  const { page } = walk;
  await page.goto(`${served.origin}/administration/agents`);
  await page.getByRole('tab', { name: 'Models' }).click();
  await page.getByRole('button', { name: 'Register a model' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Key').fill('local-small');
  await dialog.getByLabel('Name').fill('Local small');
  await dialog.getByLabel('Model', { exact: true }).fill('small-7b');
  await dialog.getByRole('button', { name: 'Save' }).click();

  await dialog.locator('.banner[data-tone=danger]').first().waitFor({ timeout: 5_000 });
  assert.equal(await dialog.getByLabel('Name').inputValue(), 'Local small');
});
