// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SOURCE } from '../i18n/catalogue.ts';
import { PRODUCTS } from '../navigation.ts';
import { ROUTES } from '../routes.ts';
import { resolve } from '../router.ts';
import { agentRows, homeTabs, hubScreenOf, inventorySummary, moreInHubtask, pagesToReview, productDoors, recentBoards, recentPages } from './home.ts';

// user-experience.md §2.2: every product's home leads somewhere - to a hub's screen of it or to a
// workspace-wide screen it already has - and every address resolves to a route.
test('every product home has a way in, and every address it offers is a route', () => {
  for (const row of PRODUCTS) {
    if (row.module === 'TIME' || row.module === 'GOALS') continue; // their own pages are the home
    const doors = productDoors(row.module, { isAdministrationReachable: true });
    const hub = hubScreenOf(row.module, '01a0e2e0-0000-7000-8000-000000000002');
    assert.ok(doors.length > 0 || hub !== undefined, `${row.module}'s home leads nowhere`);
    for (const path of [...doors.map((door) => door.path), ...(hub ? [hub] : []), ...homeTabs(row.module).map((tab) => tab.path)]) {
      assert.ok(resolve(ROUTES, path).name, `${row.module} offers ${path}, which is no route`);
    }
    for (const code of [...doors.map((door) => door.code), ...homeTabs(row.module).map((tab) => tab.code)]) {
      assert.ok(code in SOURCE, code);
    }
  }
});

test('the administration doors are offered only to whoever reaches the administration', () => {
  for (const row of PRODUCTS) {
    const paths = productDoors(row.module, { isAdministrationReachable: false }).map((door) => door.path);
    assert.ok(paths.every((path) => resolve(ROUTES, path).area !== 'administration'), `${row.module} offers ${paths.join(', ')}`);
  }
});

test('the pages home holds My notes as its second tab, at its own address', () => {
  assert.deepEqual(homeTabs('KNOWLEDGE').map((tab) => tab.path), ['/pages', '/notes']);
  assert.deepEqual(homeTabs('WHITEBOARD'), []);
});

// user-experience.md §3.3: the switched-off products are offered to whoever may switch one on, and
// to nobody else; nothing while the manifest is unread.
test('More in Hubtask lists the products that are off, for a holder of STRUCTURE only', () => {
  assert.deepEqual(moreInHubtask({ modulesOn: ['KNOWLEDGE', 'TIME'], mayShape: true }).map((row) => row.module), ['PLANNING', 'WHITEBOARD', 'SERVICE_DESK', 'ASSETS', 'GOALS', 'AGENTS']);
  assert.deepEqual(moreInHubtask({ modulesOn: [], mayShape: false }), []);
  assert.deepEqual(moreInHubtask({ modulesOn: undefined, mayShape: true }), []);
  assert.deepEqual(moreInHubtask({ modulesOn: PRODUCTS.map((row) => row.module), mayShape: true }), []);
});

// §2.2: the whiteboards' home lists every board the reader may read, recent first, across hubs.
test('the boards of every hub are one list, the last changed first, cut where the home stops', () => {
  const board = (id: string, hub: string, at: string) => ({ id, hub_id: hub, title: id, updated_at: at });
  const lists = {
    h1: [board('a', 'h1', '2026-11-01T10:00:00Z'), board('b', 'h1', '2026-10-01T10:00:00Z')],
    h2: [board('c', 'h2', '2026-11-02T08:00:00Z')],
  };
  assert.deepEqual(recentBoards(lists, ['h1', 'h2'], 10).map((each) => each.id), ['c', 'a', 'b']);
  assert.deepEqual(recentBoards(lists, ['h1', 'h2'], 2).map((each) => each.id), ['c', 'a']);
  // A hub the module is off at is not listed, even where an earlier answer is still held.
  assert.deepEqual(recentBoards(lists, ['h1'], 10).map((each) => each.id), ['a', 'b']);
  assert.deepEqual(recentBoards({}, ['h1'], 10), []);
});

// §2.2: the agents' home lists the agents working here that the reader may see, each leading to
// the screen where the reader may act on it; a retired agent works nowhere and is not listed.
test('the agents working here, live first, each with the screen the reader reaches it on', () => {
  const agent = (id: string, status: string, owner: string) => ({ id, name: id, purpose: '', status, owner_id: owner });
  const list = [agent('old', 'RETIRED', 'me'), agent('draft', 'DRAFT', 'other'), agent('live', 'ACTIVE', 'me'), agent('stopped', 'SUSPENDED', 'other')];
  assert.deepEqual(agentRows(list, 'me', true).map((row) => [row.agent.id, row.path]), [
    ['live', '/profile/agents'], ['stopped', '/administration/agents'], ['draft', '/administration/agents'],
  ]);
  // Without the administration, somebody else's agent is shown but leads nowhere.
  assert.deepEqual(agentRows(list, 'me', false).map((row) => [row.agent.id, row.path]), [
    ['live', '/profile/agents'], ['stopped', undefined], ['draft', undefined],
  ]);
});

// §2.2: the pages' home lists recently changed pages across hubs - the space's content only - and
// the pages waiting for the reader's review.
const pageRow = (id: string, hub: string, at: string, more: Partial<{ owner_id: string | null; template: boolean; archived: boolean; due_for_review: boolean; trashed_at: string | null; daily_for: string | null }> = {}) => ({
  id, hub_id: hub, updated_at: at, owner_id: null, template: false, archived: false, due_for_review: false, trashed_at: null, daily_for: null, ...more,
});

test('recently changed pages leave out templates, archived and trashed pages and daily notes', () => {
  const lists = {
    h1: [pageRow('a', 'h1', '2026-11-01T10:00:00Z'), pageRow('tpl', 'h1', '2026-11-02T10:00:00Z', { template: true }), pageRow('old', 'h1', '2026-11-02T11:00:00Z', { archived: true })],
    h2: [pageRow('b', 'h2', '2026-11-02T08:00:00Z'), pageRow('bin', 'h2', '2026-11-02T09:00:00Z', { trashed_at: '2026-11-02T09:00:00Z' }), pageRow('day', 'h2', '2026-11-02T12:00:00Z', { daily_for: '2026-11-02' })],
  };
  assert.deepEqual(recentPages(lists, ['h1', 'h2'], 10).map((each) => each.id), ['b', 'a']);
  assert.deepEqual(recentPages(lists, ['h1', 'h2'], 1).map((each) => each.id), ['b']);
  assert.deepEqual(recentPages(lists, ['h1'], 10).map((each) => each.id), ['a']);
});

test('the pages to review are the reader\'s own whose verification ran out, the longest unchanged first', () => {
  const lists = {
    h1: [
      pageRow('mine-new', 'h1', '2026-11-01T10:00:00Z', { owner_id: 'me', due_for_review: true }),
      pageRow('mine-ok', 'h1', '2026-10-01T10:00:00Z', { owner_id: 'me' }),
      pageRow('theirs', 'h1', '2026-10-01T10:00:00Z', { owner_id: 'other', due_for_review: true }),
    ],
    h2: [pageRow('mine-old', 'h2', '2026-09-01T10:00:00Z', { owner_id: 'me', due_for_review: true }), pageRow('mine-archived', 'h2', '2026-09-01T10:00:00Z', { owner_id: 'me', due_for_review: true, archived: true })],
  };
  assert.deepEqual(pagesToReview(lists, ['h1', 'h2'], 'me').map((each) => each.id), ['mine-old', 'mine-new']);
  assert.deepEqual(pagesToReview(lists, ['h1', 'h2'], undefined), []);
});

// §2.2: the assets' home opens with a health summary across the hubs where assets are on.
test('the inventory summary adds the hubs up and says when one has not answered yet', () => {
  const health = (total: number, problems: number, overdue: number) => ({ total, review_overdue: overdue, problems: Array.from({ length: problems }, () => ({})) });
  assert.deepEqual(inventorySummary({ h1: health(10, 2, 1), h2: health(5, 1, 0) }, ['h1', 'h2']), { total: 15, wanting: 3, reviewOverdue: 1, isWhole: true });
  assert.deepEqual(inventorySummary({ h1: health(10, 2, 1) }, ['h1', 'h2']), { total: 10, wanting: 2, reviewOverdue: 1, isWhole: false });
  assert.deepEqual(inventorySummary({ h1: health(10, 2, 1), gone: health(9, 9, 9) }, ['h1']), { total: 10, wanting: 2, reviewOverdue: 1, isWhole: true });
});
