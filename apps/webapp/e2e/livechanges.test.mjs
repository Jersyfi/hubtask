// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Two signed-in clients of one workspace, one in-memory backend between them: what the first
// writes reaches the second as a change record on its stream, and the second draws it without a
// reload - a board's comment written and resolved (UC-CNV-09 check 6), and a product switched off
// leaving the navigation (UC-INS-21 check 12), the workspace's own name chosen for the header
// reaching the other's header (UC-INS-22 check 7). The stream is answered and closed with a short
// `retry`, so a client comes back for the records written since, the way it does after a drain.
// Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { serve } from './serve.mjs';
import { ACCOUNT, HUB, MANIFEST, OTHER, WORKSPACE, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const BOARD_ID = '01a0e2e0-0000-7000-8000-00000000b1f1';
const ELEMENTS = [{ id: 's1', kind: 'STICKY', x: 20, y: 30, width: 180, height: 150, text: 'Pairing worked', color_token: 'green' }];
const BOARD = {
  id: BOARD_ID, hub_id: HUB.id, title: 'Retro', element_count: ELEMENTS.length, elements: ELEMENTS, writable: true, trashed_at: null,
  created_by: ACCOUNT.id, created_at: '2026-10-30T09:00:00Z', updated_by: OTHER.id, updated_at: '2026-10-30T09:00:00Z', version: 1,
};

function varUint(n) {
  const out = [];
  while (n > 0x7f) { out.push(0x80 | (n & 0x7f)); n = Math.floor(n / 128); }
  out.push(n);
  return out;
}
const EMPTY_DRAWING = Buffer.from([0, 1, ...varUint(2), 0, 0]);

/** The workspace both clients see: its modules, the board's comments, and the change log. */
function workspace(modulesOn) {
  const state = {
    modulesOn: [...modulesOn], comments: [], log: [],
    branding: { header: 'PRODUCT', logo_light_media_id: null, logo_dark_media_id: null, symbol_media_id: null, allowed: true },
  };
  const record = (entity, id, container, payload) => state.log.push({ op: 'UPSERT', entity, entity_id: id, container_id: container, payload });
  const states = () => ['PLANNING', 'KNOWLEDGE', 'WHITEBOARD'].map((module) => ({ module, on: state.modulesOn.includes(module), source: 'WORKSPACE' }));
  let made = 0;

  /** One client: its own place in the log, and the routes the backend answers before the fixture. */
  async function client(browser) {
    const { page, context, failures } = await signedIn(browser, 1280, 860);
    await context.routeWebSocket(/\/realtime\/documents\/whiteboard\//, (ws) => {
      ws.onMessage(() => {});
      ws.send(EMPTY_DRAWING);
    });
    let delivered = 0;
    await context.unroute('**/api/v1/**');
    await context.route('**/api/v1/**', (route) => {
      const request = route.request();
      const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
      const method = request.method();
      if (path === '/stream') {
        const news = state.log.slice(delivered);
        delivered = state.log.length;
        const frames = news.map((each, i) => `id: c-${delivered - news.length + i + 1}\ndata: ${JSON.stringify(each)}\n\n`).join('');
        return route.fulfill({ status: 200, contentType: 'text/event-stream', body: `retry: 250\n\n${frames}` });
      }
      if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, modules_on: state.modulesOn } });
      if (/^\/containers\/[^/]+\/modules$/.test(path)) return route.fulfill({ json: { modules_on: state.modulesOn } });
      if (path === '/tenant/modules' && method === 'GET') return route.fulfill({ json: { modules: states() } });
      const reach = /^\/tenant\/modules\/([A-Z_]+)\/reach$/.exec(path);
      if (reach) return route.fulfill({ json: { module: reach[1], on: state.modulesOn.includes(reach[1]), people: 0, rules: 0 } });
      const switched = /^\/tenant\/modules\/([A-Z_]+)$/.exec(path);
      if (switched && method === 'PUT') {
        const on = request.postDataJSON().switch === 'ON';
        state.modulesOn = state.modulesOn.filter((module) => module !== switched[1]).concat(on ? [switched[1]] : []);
        // The workspace's switches as a device holds them: keys and booleans, under no container.
        record('setting', WORKSPACE.id, null, { hub_id: null, modules: states().map(({ module, on: is }) => ({ module, on: is })) });
        return route.fulfill({ json: { modules: states() } });
      }
      if (path === '/tenant' && method === 'GET') return route.fulfill({ json: { ...WORKSPACE, branding: state.branding } });
      if (path === '/tenant/branding' && method === 'PUT') {
        state.branding = { ...state.branding, header: request.postDataJSON().header };
        // The workspace's look changed: its switches' record, under no container (ADR-0112, O3).
        record('setting', WORKSPACE.id, null, { hub_id: null, modules: states().map(({ module, on: is }) => ({ module, on: is })) });
        return route.fulfill({ json: state.branding });
      }
      if (path === `/whiteboards/${BOARD_ID}`) return route.fulfill({ json: BOARD });
      if (path === `/hubs/${HUB.id}/whiteboards`) return route.fulfill({ json: { data: [BOARD] } });
      if (path === `/whiteboards/${BOARD_ID}/comments` && method === 'POST') {
        const comment = {
          id: `01a0e2e0-0000-7000-8000-00000000c1${String((made += 1)).padStart(2, '0')}`, whiteboard_id: BOARD_ID, parent_id: null,
          author_id: ACCOUNT.id, element_id: null, x: 0, y: 0, resolved: false, resolved_at: null, resolved_by: null,
          created_at: '2026-11-22T09:00:00Z', edited_at: null, deleted: false, version: 1, ...request.postDataJSON(),
        };
        state.comments.push(comment);
        record('whiteboard_comment', comment.id, HUB.id, comment);
        return route.fulfill({ status: 201, json: comment });
      }
      if (path === `/whiteboards/${BOARD_ID}/comments`) return route.fulfill({ json: { data: state.comments } });
      const changed = /^\/whiteboard-comments\/([^/]+)$/.exec(path);
      if (changed && method === 'PATCH') {
        const comment = state.comments.find((each) => each.id === changed[1]);
        const { resolved } = request.postDataJSON();
        Object.assign(comment, { resolved, resolved_at: resolved ? '2026-11-22T09:05:00Z' : null, resolved_by: resolved ? ACCOUNT.id : null, version: comment.version + 1 });
        record('whiteboard_comment', comment.id, HUB.id, comment);
        return route.fulfill({ json: comment });
      }
      return stub(route);
    });
    return { page, context, failures };
  }
  return { state, client };
}

/** The board with its comments open beside it. */
async function openComments(page) {
  await page.goto(`${served.origin}/whiteboards/${BOARD_ID}`);
  await page.locator('[data-board-sheet=comments]').click();
  await page.locator('[data-board-part=comments]').waitFor({ timeout: 15_000 });
}

test('chromium: a board comment written and resolved on one client appears on the other without a reload (UC-CNV-09 check 6)', async (t) => {
  const browser = await sharedChromium(t);
  const shared = workspace([...(MANIFEST.modules_on ?? []), 'WHITEBOARD']);
  const writer = await shared.client(browser);
  const reader = await shared.client(browser);
  t.after(() => Promise.all([writer.context.close(), reader.context.close()]));
  await Promise.all([openComments(writer.page), openComments(reader.page)]);
  const threads = (page) => page.locator('[data-board-part=comments] [data-board-thread]');
  assert.equal(await threads(reader.page).count(), 0, 'the board starts without a comment');
  let reloads = 0;
  reader.page.on('load', () => (reloads += 1));

  await writer.page.locator('[data-board-part=comments]').getByPlaceholder('New comment').fill('Move this to the left');
  await writer.page.locator('[data-board-part=comments]').getByRole('button', { name: 'Send', exact: true }).click();
  await threads(writer.page).first().waitFor({ timeout: 10_000 });

  // The other client was told by the record alone: nothing there asked for the thread.
  const arrived = threads(reader.page).first();
  await arrived.waitFor({ timeout: 15_000 });
  assert.match((await arrived.textContent()) ?? '', /Move this to the left/);

  await threads(writer.page).first().getByRole('button', { name: 'Resolve', exact: true }).click();
  await reader.page.waitForFunction(() => document.querySelectorAll('[data-board-part=comments] [data-board-thread]:not(.resolved)').length === 0, null, { timeout: 15_000 });
  assert.equal(reloads, 0, 'the other client reloaded');
  assert.deepEqual(shared.state.log.map((each) => each.entity), ['whiteboard_comment', 'whiteboard_comment']);
  assert.deepEqual([...writer.failures, ...reader.failures], []);
});

test('chromium: a product switched off on one client leaves the other\'s navigation without a reload (UC-INS-21 check 12)', async (t) => {
  const browser = await sharedChromium(t);
  const shared = workspace(['PLANNING', 'KNOWLEDGE', 'WHITEBOARD']);
  const admin = await shared.client(browser);
  const other = await shared.client(browser);
  t.after(() => Promise.all([admin.context.close(), other.context.close()]));

  await other.page.goto(`${served.origin}/`);
  // The products are a tree in the sidebar, each row by its name.
  const pages = other.page.getByRole('treeitem', { name: 'Pages', exact: true });
  await pages.first().waitFor({ timeout: 15_000 });
  let reloads = 0;
  other.page.on('load', () => (reloads += 1));

  await admin.page.goto(`${served.origin}/administration/modules`);
  const row = admin.page.locator('[data-module="KNOWLEDGE"]');
  await row.getByRole('switch').waitFor({ timeout: 15_000 });
  assert.equal(await row.getByRole('switch').isChecked(), true, 'Pages starts on');
  await row.getByRole('switch').click();
  await admin.page.getByRole('dialog').getByRole('button', { name: 'Switch off', exact: true }).click();

  await pages.first().waitFor({ state: 'detached', timeout: 15_000 });
  assert.ok(await other.page.getByRole('treeitem', { name: 'Whiteboards', exact: true }).count() > 0, 'a product still on left the navigation too');
  assert.equal(reloads, 0, 'the other client reloaded');
  assert.deepEqual(shared.state.log.map((each) => [each.entity, each.container_id]), [['setting', null]]);
  assert.deepEqual([...admin.failures, ...other.failures], []);
});

test('chromium: the header\'s choice saved on one client reaches the other\'s header without a reload (UC-INS-22 check 7)', async (t) => {
  const browser = await sharedChromium(t);
  const shared = workspace(MANIFEST.modules_on ?? []);
  const admin = await shared.client(browser);
  const other = await shared.client(browser);
  t.after(() => Promise.all([admin.context.close(), other.context.close()]));

  await other.page.goto(`${served.origin}/`);
  const bar = other.page.getByRole('banner');
  await bar.getByRole('link', { name: 'Hubtask', exact: true }).waitFor({ timeout: 15_000 });
  let reloads = 0;
  other.page.on('load', () => (reloads += 1));

  await admin.page.goto(`${served.origin}/administration/workspace/name`);
  await admin.page.getByRole('group', { name: 'What the header shows' }).getByRole('radio', { name: 'Workspace name' }).check({ timeout: 15_000 });
  await admin.page.getByRole('button', { name: 'Save', exact: true }).click();

  // The other device read the workspace again on its record: its own name, two letters beside it.
  await bar.getByRole('link', { name: WORKSPACE.display_name, exact: true }).waitFor({ timeout: 15_000 });
  assert.equal(await bar.getByRole('link', { name: 'Hubtask', exact: true }).count(), 0);
  assert.equal(reloads, 0, 'the other client reloaded');
  assert.deepEqual(shared.state.log.map((each) => [each.entity, each.container_id]), [['setting', null]]);
  assert.deepEqual([...admin.failures, ...other.failures], []);
});
