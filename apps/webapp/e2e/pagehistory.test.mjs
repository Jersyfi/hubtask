// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A page's history (UC-DOC-06): an older version brought back stands in the live document too.
// The restore wrote the saved body only; the editors' live document still held the text before it
// and wrote it back at the next save, so the restored version was gone again.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { chromium } from 'playwright';

import { ACCOUNT } from './fixture.mjs';
import { ALL, PD_PAGES, open, pageRead, served } from './kompassstub.mjs';

test.after(() => served.close());

const varUint = (n) => {
  const out = [];
  while (n > 0x7f) { out.push(0x80 | (n & 0x7f)); n = Math.floor(n / 128); }
  out.push(n);
  return out;
};
const EMPTY_DOCUMENT = Buffer.from([0, 1, ...varUint(2), 0, 0]);
const ASK_STATE = Buffer.from([0, 0, ...varUint(1), 0]);
const OLDER = 'An older welcome, brought back.';
const VERSIONS = [
  { seq: 2, title: 'Product handbook', author_id: ACCOUNT.id, name: null, restored_from: null, size: 20, created_at: '2026-11-06T10:00:00Z' },
  { seq: 1, title: 'Product handbook', author_id: ACCOUNT.id, name: 'Sent to legal', restored_from: null, size: 31, created_at: '2026-11-04T16:05:00Z' },
];

/** The handbook opened by its owner with its history stubbed; answers the saves and restores seen. */
async function opened(browser, t) {
  const handbook = PD_PAGES[0];
  const { page, failures, close } = await open(browser, { modulesOn: ALL, owner: true });
  t.after(close);
  await page.context().routeWebSocket(/\/realtime\/documents\/page\//, (ws) => {
    ws.onMessage(() => {});
    ws.send(EMPTY_DOCUMENT);
    ws.send(ASK_STATE);
  });
  const seen = { saves: [], restores: [] };
  await page.route('**/api/v1/pages/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace('/api/v1', '');
    if (path === `/pages/${handbook.id}/versions`) return route.fulfill({ json: { data: VERSIONS, page: { next_cursor: null, has_more: false } } });
    if (path === `/pages/${handbook.id}/versions/1`) return route.fulfill({ json: { ...VERSIONS[1], page_id: handbook.id, body: OLDER } });
    if (path === `/pages/${handbook.id}/versions/2`) return route.fulfill({ json: { ...VERSIONS[0], page_id: handbook.id, body: 'Welcome to the team.' } });
    if (path === `/pages/${handbook.id}/versions/1:restore` && request.method() === 'POST') {
      seen.restores.push(1);
      return route.fulfill({ json: { ...pageRead(handbook), body: OLDER, version: 3 } });
    }
    if (path === `/pages/${handbook.id}` && request.method() !== 'GET') seen.saves.push(request.postData() ?? '');
    return route.fallback();
  });
  await page.goto(new URL(`/pages/${handbook.id}`, page.url()).href);
  const body = page.getByRole('textbox', { name: /Product handbook/ });
  await body.getByText('Welcome to the team.').waitFor({ timeout: 10_000 });
  return { page, body, failures, seen };
}

test('a version brought back stands in the live document, and the next save keeps it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, body, failures, seen } = await opened(browser, t);

  await page.getByRole('button', { name: 'More for this page' }).click();
  await page.getByRole('menuitem', { name: 'History' }).click();
  await page.getByRole('button', { name: /Sent to legal/ }).click();
  await page.getByRole('button', { name: 'Bring this version back' }).click();
  await body.getByText(OLDER).waitFor({ timeout: 5_000 });
  assert.equal(await body.getByText('Welcome to the team.').count(), 0, 'the live document still holds the text before the restore');
  assert.deepEqual(seen.restores, [1]);

  await page.waitForTimeout(2_500);
  assert.deepEqual(seen.saves, [], 'bringing a version back saved the body a second time');
  await body.getByText(OLDER).click();
  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+ArrowRight' : 'End');
  await page.keyboard.type(' Still here.');
  await page.waitForTimeout(2_500);
  assert.equal(seen.saves.length, 1, 'the edit is saved');
  assert.match(seen.saves[0], /An older welcome, brought back\. Still here\./);
  assert.doesNotMatch(seen.saves[0], /Welcome to the team/);
  assert.deepEqual(failures, []);
});
