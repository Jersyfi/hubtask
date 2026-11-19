// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A page's history (UC-DOC-06): an older version brought back stands in the live document too.
// The restore wrote the saved body only; the editors' live document still held the text before it
// and wrote it back at the next save, so the restored version was gone again.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { sharedChromium } from './browser.mjs';

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
  const browser = await sharedChromium(t);
  const { page, body, failures, seen } = await opened(browser, t);

  await page.getByRole('button', { name: 'More for this page' }).click();
  await page.getByRole('menuitem', { name: 'History' }).click();
  await page.getByRole('button', { name: 'Back to the page' }).click();
  await body.getByText('Welcome to the team.').waitFor({ timeout: 5_000 });
  await page.getByRole('button', { name: 'More for this page' }).click();
  await page.getByRole('menuitem', { name: 'History' }).click();
  await page.getByRole('button', { name: /Sent to legal/ }).click();
  await page.getByRole('button', { name: 'Bring this version back' }).click();
  await page.getByRole('button', { name: 'Back to the page' }).click();
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

test('the history lists the versions with person, time and name, and compares one side by side or by its changes (phist)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, body, failures } = await opened(browser, t);

  await page.getByRole('button', { name: 'More for this page' }).click();
  await page.getByRole('menuitem', { name: 'History' }).click();
  const history = page.locator('[data-page-history]');
  await history.waitFor({ timeout: 5_000 });
  assert.equal(await body.isVisible(), false, 'the history stands in the text\'s place');
  await history.getByRole('heading', { name: 'Versions · 2' }).waitFor();
  const versions = history.getByRole('listitem');
  assert.match(await versions.nth(0).innerText(), /Now/);
  assert.match(await versions.nth(2).innerText(), /Nov 4, 2026.*\n(Jérôme Winkel|Somebody in this workspace)/, 'time and person');
  assert.match(await versions.nth(2).innerText(), /Sent to legal/, 'a named version carries its name');
  assert.match(await history.innerText(), /Bringing one back adds a new version; nothing is lost\./);

  // The version before the newest is compared at once, side by side, each drawn as the page draws
  // its text: the block that went out struck, the one that came in marked.
  const sides = history.locator('[data-history-sides]');
  await sides.locator('[data-page-version] .page-body > p[data-change="removed"]', { hasText: OLDER }).waitFor({ timeout: 5_000 });
  await sides.locator('[data-page-version] .page-body > p[data-change="added"]', { hasText: 'Welcome to the team.' }).waitFor();
  assert.equal(await sides.getByRole('textbox').count(), 0, 'a version is read, not written');
  assert.equal(await history.getByRole('button', { name: /Sent to legal/ }).getAttribute('aria-pressed'), 'true');
  assert.match(await history.innerText(), /1 place/);
  await history.getByRole('radio', { name: 'Changes only' }).click();
  const changes = history.locator('[data-history-changes]');
  await changes.locator('[data-change="removed"]', { hasText: OLDER }).waitFor({ timeout: 5_000 });
  assert.equal(await changes.locator('[data-change]').count(), 2, 'only the changed blocks');

  // Naming is a button on top, for the version compared.
  await history.getByRole('button', { name: 'Name this version' }).click();
  const dialog = page.getByRole('dialog', { name: 'Name this version' });
  assert.equal(await dialog.getByRole('textbox').inputValue(), 'Sent to legal');
  assert.deepEqual(failures, []);
});
