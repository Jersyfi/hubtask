// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A team's Matrix room subscribed on Administration → Webhooks (UC-INT-12 check 1): the form sends
// the format, the homeserver, the room, the token and the language as the contract spells them, no
// secret is shown for a room that signs nothing, and a stored room says its format and room and
// offers no new secret. Against the stubbed API; Chromium only, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { MANIFEST, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const CREATED = 'de.hubtask.work.item.created.v1';
const ROOM = '!kitchen:matrix.example.org';

const STORED = {
  id: '01a0e2e0-0000-7000-8000-000000001901',
  target_url: 'https://matrix.example.org',
  event_types: [CREATED],
  state: 'ACTIVE',
  failure_count: 0,
  format: 'MATRIX',
  room: ROOM,
  locale: 'de',
  created_at: '2026-10-01T08:00:00Z',
  version: 1,
};

test('chromium: a Matrix room is subscribed by homeserver, room and token, and shows no secret', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 1280, 1400);
  t.after(() => context.close());

  let sent;
  let listed = [];
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.pathname.endsWith('/meta/capabilities')) {
      return route.fulfill({ json: { ...MANIFEST, event_types: [CREATED] } });
    }
    if (url.pathname.endsWith('/integrations/webhooks') && request.method() === 'POST') {
      sent = request.postDataJSON();
      // The contract answers a chat room without a secret and without the token.
      listed = [STORED];
      return route.fulfill({ status: 201, json: STORED });
    }
    if (url.pathname.endsWith('/integrations/webhooks')) return route.fulfill({ json: listed });
    return stub(route);
  });

  await page.goto(`${served.origin}/administration/webhooks`);

  await page.getByLabel('Delivered as').selectOption({ label: 'Matrix room' });
  await page.getByLabel('Where to deliver').fill('https://matrix.example.org');
  await page.getByLabel('Room', { exact: true }).fill(ROOM);
  await page.getByLabel('Access token').fill('syt_e2e_token');
  await page.getByLabel('Language').fill('de');
  await page.getByLabel(CREATED).check();
  const posted = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/integrations/webhooks'), { timeout: 5_000 });
  await page.getByRole('button', { name: 'Subscribe', exact: true }).click();
  await posted;

  assert.deepEqual(sent, {
    target_url: 'https://matrix.example.org',
    event_types: [CREATED],
    format: 'MATRIX',
    room: ROOM,
    credential: 'syt_e2e_token',
    locale: 'de',
  });

  // The stored room says what it is; a room signs nothing, so no secret is shown or offered anew.
  await page.getByText(`Room ${ROOM}`).waitFor({ timeout: 5_000 });
  assert.ok(await page.getByText('Matrix room', { exact: true }).first().isVisible(), 'the format is not shown');
  assert.equal(await page.getByText('The signing secret').count(), 0, 'a secret is shown for a chat room');
  assert.equal(await page.getByRole('button', { name: 'New secret' }).count(), 0, 'a chat room offers a new secret');
  // The form starts over as a signed event, so the token is not left in it.
  assert.equal(await page.getByLabel('Access token').count(), 0, 'the token stays in the form');
  assert.deepEqual(failures, []);
});
