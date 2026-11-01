// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A workspace's first start (UC-WRK-30 check 5), walked on Home: whoever `GET /tenant` offers it
// to is asked what the workspace is used for; the answer is one command that makes a hub and its
// collections on matching presets and closes the question, retried under the same key after a
// failure; a skip closes it and creates nothing;
// where it is not offered nobody is asked. Chromium only; who is offered it is the server's tests'.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { HUB, WORKSPACE, signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

/** This walk's own identifiers: no other fixture record uses them. */
const STARTED_HUB = '01a0e2e0-0000-7000-8000-0000000028a1';

async function as(browser, offered, { failFirstApply = false } = {}) {
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  const sent = [];
  let closed = false;
  let applies = 0;
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/tenant' && request.method() === 'GET') {
      return route.fulfill({ json: { ...WORKSPACE, starter_offered: offered && !closed } });
    }
    if (path === '/tenant/starter:close' && request.method() === 'POST') {
      sent.push({ path });
      closed = true;
      return route.fulfill({ status: 204, body: '' });
    }
    if (path === '/tenant/starter:apply' && request.method() === 'POST') {
      const body = request.postDataJSON();
      sent.push({ path, body, key: request.headers()['idempotency-key'] });
      applies += 1;
      if (failFirstApply && applies === 1) {
        return route.fulfill({ status: 503, contentType: 'application/problem+json', json: { type: 'about:blank', title: 'Unavailable', status: 503, code: 'service.unavailable' } });
      }
      closed = true;
      return route.fulfill({ status: 201, json: { ...HUB, id: STARTED_HUB, type: 'HUB', parent_id: null, name: body.hub_name } });
    }
    if (path === '/containers' && request.method() === 'POST') {
      sent.push({ path, body: request.postDataJSON() });
      return route.fulfill({ status: 500, json: {} });
    }
    return stub(route);
  });
  return { page, failures, context, sent };
}

const dialogOf = (page) => page.locator('[data-starter]');

test('chromium: the first start - asked once, answered by one command making a hub and collections on presets', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, sent } = await as(browser, true);
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  const dialog = dialogOf(page);
  await dialog.waitFor({ timeout: 10_000 });
  for (const name of ['For myself', 'Our household', 'My clients', 'Our team']) {
    assert.equal(await dialog.getByRole('radio', { name }).count(), 1, `${name} is not offered`);
  }
  await dialog.getByRole('radio', { name: 'Our household' }).check();
  await page.getByRole('button', { name: 'Set up' }).click();
  await page.waitForURL(`**/hubs/${STARTED_HUB}`, { timeout: 10_000 });

  assert.equal(sent.length, 1, `one command makes the set, not ${sent.length} writes`);
  assert.equal(sent[0].path, '/tenant/starter:apply');
  assert.match(sent[0].key ?? '', /^[0-9a-f-]{36}$/, 'the set is sent without an Idempotency-Key');
  assert.deepEqual(sent[0].body, {
    hub_name: 'Home',
    hub_preset: 'TASKS',
    collections: [{ name: 'Shopping', preset: 'LIST' }, { name: 'Chores' }, { name: 'Projects', preset: 'PROJECT' }],
  });
  // Not asked again.
  await page.goto(`${served.origin}/`);
  await page.waitForTimeout(1200);
  assert.equal(await dialogOf(page).isVisible(), false, 'asked a second time');
  assert.deepEqual(failures, []);
});

test('chromium: the first start failed once - the retry sends the same key and makes one set', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, sent } = await as(browser, true, { failFirstApply: true });
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  const dialog = dialogOf(page);
  await dialog.waitFor({ timeout: 10_000 });
  await dialog.getByRole('radio', { name: 'Our team' }).check();
  await page.getByRole('button', { name: 'Set up' }).click();
  await dialog.getByRole('alert').waitFor({ timeout: 10_000 });
  await page.getByRole('button', { name: 'Set up' }).click();
  await page.waitForURL(`**/hubs/${STARTED_HUB}`, { timeout: 10_000 });

  const applied = sent.filter((write) => write.path === '/tenant/starter:apply');
  assert.equal(applied.length, 2);
  assert.equal(applied[0].key, applied[1].key, 'the retry was sent under a new key, which could make a second set');
  assert.deepEqual(sent.filter((write) => write.path !== '/tenant/starter:apply'), [], 'something besides the one command was written');
});

test('chromium: the first start skipped - closed, nothing made', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, sent } = await as(browser, true);
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  await dialogOf(page).waitFor({ timeout: 10_000 });
  await page.getByRole('button', { name: 'Skip' }).last().click();
  await page.waitForTimeout(600);
  assert.deepEqual(sent, [{ path: '/tenant/starter:close' }]);
  assert.equal(await dialogOf(page).isVisible(), false);
  assert.deepEqual(failures, []);
});

test('chromium: where the first start is not offered, nobody is asked', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, sent } = await as(browser, false);
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  await page.getByRole('navigation').first().waitFor({ timeout: 10_000 });
  await page.waitForTimeout(1200);
  assert.equal(await dialogOf(page).count() > 0 && (await dialogOf(page).isVisible()), false, 'asked where it is not offered');
  assert.deepEqual(sent, []);
  assert.deepEqual(failures, []);
});
