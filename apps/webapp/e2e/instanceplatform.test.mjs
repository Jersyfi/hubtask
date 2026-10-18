// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Events to the purchase platform, walked as an operator meets them (UC-INS-14 checks 1, 5, 6, 7):
// a subscription is added and its secret shown once, its dead letters are counted on its row, the
// notice says it stays on, the dead letters are delivered again, it is paused and removed - and a
// single-workspace installation offers none of it.
//
// Against a stubbed API: what is asserted is what the screen sends and what it shows. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { MANIFEST, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const SUBSCRIPTION = {
  id: '01a0e2e0-0000-7000-8000-0000000007a1',
  target_url: 'https://shop.provider.example/hubtask',
  state: 'ACTIVE',
  dead_letters: 4,
  last_delivery_at: new Date(Date.now() - 3 * 3_600_000).toISOString(),
  last_error: 'webhooks.target_unavailable',
  from_file: false,
  created_at: '2026-10-16T08:00:00Z',
  version: 1,
};

async function open(browser, { tenancy = 'multi', width = 1280, height = 900, fromFile = false } = {}) {
  const context = await browser.newContext({ viewport: { width, height } });
  const sent = [];
  let subscriptions = [{ ...SUBSCRIPTION, from_file: fromFile }];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    if (path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, tenancy_mode: tenancy, instance: { reachable: true } } });
    }
    if (path === '/admin/subscriptions' && method === 'GET') {
      sent.push({ method, path });
      return route.fulfill({ json: subscriptions });
    }
    if (path === '/admin/subscriptions' && method === 'POST') {
      const body = request.postDataJSON();
      sent.push({ method, path, body });
      const added = {
        ...SUBSCRIPTION, id: '01a0e2e0-0000-7000-8000-0000000007a2', target_url: body.target_url,
        dead_letters: 0, last_delivery_at: null, last_error: null,
      };
      subscriptions = [...subscriptions, added];
      return route.fulfill({ status: 201, json: { ...added, secret: 'the-shown-once-secret' } });
    }
    if (path.endsWith(':redeliver') && method === 'POST') {
      sent.push({ method, path });
      subscriptions = subscriptions.map((each) => (path.includes(each.id) ? { ...each, dead_letters: 0 } : each));
      return route.fulfill({ status: 202, json: { queued: 4 } });
    }
    if (path.startsWith('/admin/subscriptions/') && method === 'PATCH') {
      const body = request.postDataJSON();
      sent.push({ method, path, body });
      subscriptions = subscriptions.map((each) => (path.endsWith(each.id) ? { ...each, state: body.state } : each));
      const changed = subscriptions.find((each) => path.endsWith(each.id));
      return route.fulfill({ json: changed });
    }
    if (path.startsWith('/admin/subscriptions/') && method === 'DELETE') {
      sent.push({ method, path });
      subscriptions = subscriptions.filter((each) => !path.endsWith(each.id));
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

test('chromium: an operator sees the dead letters, is told the subscription stays on, and delivers them again', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/instance/platform`);
  const row = page.getByRole('row').filter({ hasText: 'shop.provider.example' });
  await row.waitFor({ timeout: 15_000 });
  assert.match((await row.textContent()) ?? '', /4 undeliverable/);
  assert.match((await row.textContent()) ?? '', /3 hours ago/);
  const main = page.locator('main');
  assert.match((await main.textContent()) ?? '', /The platform could not be reached\./);
  assert.match((await main.textContent()) ?? '', /The subscription stays on/);
  assert.match((await main.textContent()) ?? '', /never content/);

  await row.getByRole('button', { name: 'Deliver again' }).click();
  await page.getByRole('row').filter({ hasText: 'Delivering' }).waitFor({ timeout: 10_000 });
  assert.ok(sent.some((each) => each.method === 'POST' && each.path === `/admin/subscriptions/${SUBSCRIPTION.id}:redeliver`));
  assert.equal(await page.getByRole('button', { name: 'Deliver again' }).count(), 0, 'redelivery is offered with nothing to redeliver');
  assert.doesNotMatch((await main.textContent()) ?? '', /could not be reached/);
});

test('chromium: an operator subscribes a platform and sees its secret once, then pauses and removes one', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/instance/platform`);
  await page.getByRole('row').filter({ hasText: 'shop.provider.example' }).waitFor({ timeout: 15_000 });

  await page.getByRole('button', { name: 'Add subscription' }).first().click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Address of the platform').fill('https://billing.provider.example/events');
  await dialog.getByRole('button', { name: 'Subscribe' }).click();
  await page.getByText("The subscription's signing secret").waitFor({ timeout: 10_000 });
  const created = sent.find((each) => each.method === 'POST' && each.path === '/admin/subscriptions');
  assert.deepEqual(created?.body, { target_url: 'https://billing.provider.example/events' });
  await page.getByRole('row').filter({ hasText: 'billing.provider.example' }).waitFor({ timeout: 10_000 });

  const first = page.getByRole('row').filter({ hasText: 'shop.provider.example' });
  await first.getByRole('button', { name: 'Pause' }).click();
  await first.getByRole('button', { name: 'Resume' }).waitFor({ timeout: 10_000 });
  assert.ok(sent.some((each) => each.method === 'PATCH' && each.body?.state === 'PAUSED'));
  assert.match((await first.textContent()) ?? '', /Paused/);
  assert.equal(await first.getByRole('button', { name: 'Deliver again' }).count(), 0, 'a paused subscription offers a redelivery');

  await first.getByRole('button', { name: 'Remove' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Remove' }).click();
  await page.getByRole('row').filter({ hasText: 'shop.provider.example' }).waitFor({ state: 'detached', timeout: 10_000 });
  assert.ok(sent.some((each) => each.method === 'DELETE' && each.path === `/admin/subscriptions/${SUBSCRIPTION.id}`));
});

// On a phone each subscription reads top to bottom, its address whole, its controls reachable.
test('chromium: on a phone the dead letters are seen and delivered again', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser, { width: 375, height: 812 });
  t.after(close);

  await page.goto(`${served.origin}/instance/platform`);
  const card = page.locator('li').filter({ hasText: 'shop.provider.example' });
  await card.waitFor({ timeout: 15_000 });
  assert.match((await card.textContent()) ?? '', /4 undeliverable/);
  await card.getByRole('button', { name: 'Deliver again' }).click();
  await page.locator('li').filter({ hasText: 'Delivering' }).waitFor({ timeout: 10_000 });
  assert.ok(sent.some((each) => each.path === `/admin/subscriptions/${SUBSCRIPTION.id}:redeliver`));
});

// An enforcing instance file decides the subscriptions: the screen offers no add, pause or remove,
// and the dead letters are still delivered again from here.
test('chromium: subscriptions the instance file decides offer only a redelivery', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, { fromFile: true });
  t.after(close);

  await page.goto(`${served.origin}/instance/platform`);
  const row = page.getByRole('row').filter({ hasText: 'shop.provider.example' });
  await row.waitFor({ timeout: 15_000 });
  assert.equal(await row.getByRole('button', { name: 'Deliver again' }).count(), 1);
  assert.equal(await row.getByRole('button', { name: 'Pause' }).count(), 0);
  assert.equal(await row.getByRole('button', { name: 'Remove' }).count(), 0);
  assert.equal(await page.getByRole('button', { name: 'Add subscription' }).count(), 0);
});

// UC-INS-14 check 7: a single-workspace installation has no purchase platform to tell - no row in
// the installation's navigation, and the address reads nothing.
test('chromium: a single-workspace installation offers no events to a platform', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser, { tenancy: 'single' });
  t.after(close);

  await page.goto(`${served.origin}/instance/platform`);
  await page.getByText('there is no purchase platform to tell').waitFor({ timeout: 15_000 });
  assert.equal(await page.locator('a[href="/instance/platform"]').count(), 0, 'the installation offers a platform row');
  assert.equal(sent.filter((each) => each.path === '/admin/subscriptions').length, 0, 'the subscriptions were read');
  assert.equal(await page.getByRole('button', { name: 'Add subscription' }).count(), 0);
});
