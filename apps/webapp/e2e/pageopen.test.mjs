// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Opening a page writes nothing (found beside the owner's review of 2026-11-08): the first writer to
// open a page with a body and no live document yet seeds the document from the body, and that
// seeding is the body itself, not an edit - it saved the body again, re-serialised, and the page
// gained a version nobody wrote. A real edit still saves.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { chromium } from 'playwright';

import { ALL, PD_PAGES, open, served } from './kompassstub.mjs';

test.after(() => served.close());

const varUint = (n) => {
  const out = [];
  while (n > 0x7f) { out.push(0x80 | (n & 0x7f)); n = Math.floor(n / 128); }
  out.push(n);
  return out;
};
/** The server's answer to a new document: sync step 2 with an empty update, then its step 1. */
const EMPTY_DOCUMENT = Buffer.from([0, 1, ...varUint(2), 0, 0]);
const ASK_STATE = Buffer.from([0, 0, ...varUint(1), 0]);

test('the first writer opens a page without saving it; typing saves', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const handbook = PD_PAGES[0];
  const { page, failures, close } = await open(browser, { modulesOn: ALL, owner: true });
  t.after(close);
  let synced = false;
  await page.context().routeWebSocket(/\/realtime\/documents\/page\//, (ws) => {
    ws.onMessage(() => {});
    ws.send(EMPTY_DOCUMENT);
    ws.send(ASK_STATE);
    synced = true;
  });
  const saves = [];
  page.on('request', (request) => {
    if (request.method() !== 'GET' && new URL(request.url()).pathname.endsWith(`/pages/${handbook.id}`)) saves.push(request.postData());
  });
  await page.goto(new URL(`/pages/${handbook.id}`, page.url()).href);
  const body = page.getByRole('textbox', { name: /Product handbook/ });
  await body.getByText('Welcome to the team.').waitFor({ timeout: 10_000 });
  assert.ok(synced, 'the live document connected');
  await page.waitForTimeout(2_500);
  assert.deepEqual(saves, [], 'opening the page saved it');

  await body.click();
  await page.keyboard.press('End');
  await page.keyboard.type(' Glad you are here.');
  await page.waitForTimeout(2_500);
  assert.equal(saves.length, 1, 'an edit is saved');
  assert.match(saves[0], /Glad you are here/);
  assert.deepEqual(failures, []);
});
