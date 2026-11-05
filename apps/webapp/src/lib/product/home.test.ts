// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SOURCE } from '../i18n/source.ts';
import { PRODUCTS } from '../navigation.ts';
import { ROUTES } from '../routes.ts';
import { resolve } from '../router.ts';
import { collectionScreenOf, createAt, createOf, CREATE_ASKED, isUnused, recentRuns, homeTabs, hubScreenOf, inventorySummary, moreInHubtask, pagesToReview, recentBoards, recentPages, releasesDue, placesByUse, placesToCreate, nextReleaseOf, hubsWithAssetsFirst, sprintsRunning } from './home.ts';

// user-experience.md §2.2: every product's home leads somewhere - to a hub's screen of it or to a
// workspace-wide screen it already has - and every address resolves to a route.
test('every product home has a way in, and every address it offers is a route', () => {
  for (const row of PRODUCTS) {
    if (row.module === 'TIME' || row.module === 'GOALS') continue; // their own pages are the home
    const hub = hubScreenOf(row.module, '01a0e2e0-0000-7000-8000-000000000002');
    assert.ok(hub !== undefined || homeTabs(row.module).length > 0, `${row.module}'s home leads nowhere`);
    for (const path of [...(hub ? [hub] : []), ...homeTabs(row.module).map((tab) => tab.path)]) {
      assert.ok(resolve(ROUTES, path).name, `${row.module} offers ${path}, which is no route`);
    }
    for (const code of homeTabs(row.module).map((tab) => tab.code)) {
      assert.ok(code in SOURCE, code);
    }
  }
});

test('the pages home holds My notes as its second tab, at its own address', () => {
  assert.deepEqual(homeTabs('KNOWLEDGE').map((tab) => tab.path), ['/pages', '/notes']);
  assert.deepEqual(homeTabs('WHITEBOARD'), []);
});

// user-experience.md §5.1, §7.4: the desk's home is Queues · Desks · Settings, Settings last.
test('the desk home holds its queues, its desks and its settings, settings last', () => {
  assert.deepEqual(homeTabs('SERVICE_DESK').map((tab) => tab.id), ['queues', 'desks', 'settings']);
});

// user-experience.md §5.1, §7.8: the agents' screen is Agents · Waiting · Runs · Settings, Settings last.
test('the agents home holds the agents, their held acts, their runs and their settings, settings last', () => {
  assert.deepEqual(homeTabs('AGENTS').map((tab) => tab.id), ['agents', 'requests', 'runs', 'settings']);
});

// user-experience.md §5.1, §8.2: the assets' settings are their home's last tab, not a door to the
// administration.
test('the assets home holds the inventory and its settings, settings last', () => {
  assert.deepEqual(homeTabs('ASSETS').map((tab) => tab.id), ['inventory', 'settings']);
  assert.deepEqual(homeTabs('ASSETS').map((tab) => tab.path), ['/assets', '/assets/settings']);
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

// §2.2: the plans' home lists the releases still to go out, the soonest first, across hubs.
test('the releases to come are the planned ones of the hubs named, the soonest first', () => {
  const release = (id: string, hub: string, on: string, state = 'PLANNED') => ({ id, hub_id: hub, release_on: on, state });
  const lists = { h1: [release('b', 'h1', '2026-12-01'), release('gone', 'h1', '2026-10-01', 'RELEASED')], h2: [release('a', 'h2', '2026-11-15')], off: [release('x', 'off', '2026-11-01')] };
  assert.deepEqual(releasesDue(lists, ['h1', 'h2'], 10).map((each) => each.id), ['a', 'b']);
  assert.deepEqual(releasesDue(lists, ['h1', 'h2'], 1).map((each) => each.id), ['a']);
});

// §2.2: the agents' home shows their recent runs, the newest first across the agents.
test('the recent runs across agents, the newest first, at most the limit', () => {
  const job = (id: string, agent: string, at: string) => ({ id, agent_id: agent, started_at: at });
  const at = { a1: [job('j1', 'a1', '2026-11-03T08:00:00Z'), job('j2', 'a1', '2026-11-01T08:00:00Z')], a2: [job('j3', 'a2', '2026-11-02T08:00:00Z')] };
  assert.deepEqual(recentRuns(at, ['a1', 'a2'], 2).map((each) => each.id), ['j1', 'j3']);
  assert.deepEqual(recentRuns(at, ['a2'], 5).map((each) => each.id), ['j3'], 'an agent no longer listed is left out');
  assert.deepEqual(recentRuns({}, ['a1'], 5), []);
});

// UX-2, user-experience.md §5.1/§5.2: every product home's head carries its primary create act,
// worded for the product and leading to the place's screen of it where the place's own act stands;
// a home that is its own screen carries the act in that screen's head.
test('every product home carries its primary create act', () => {
  const ownScreen: Record<string, string> = {
    SERVICE_DESK: 'DeskHome words its own',
    TIME: 'time records on its own page (TimeView, Record time)',
    GOALS: 'goals create on their own page',
    AGENTS: 'AgentsView is the home, Add an agent in its head',
  };
  const id = '01a0e2e0-0000-7000-8000-000000000002';
  for (const row of PRODUCTS) {
    if (ownScreen[row.module]) {
      assert.equal(createOf(row.module), undefined, `${row.module}: ${ownScreen[row.module]}`);
      continue;
    }
    const create = createOf(row.module);
    assert.ok(create, `${row.module}'s home has no create act`);
    assert.ok(create.code in SOURCE && create.inCode in SOURCE, `${row.module}: ${create.code}, ${create.inCode}`);
    const screen = create.level === 'HUB' ? hubScreenOf(row.module, id) : collectionScreenOf(row.module, id);
    assert.ok(screen && resolve(ROUTES, screen).name, `${row.module}'s create leads to no route: ${screen}`);
  }
  assert.equal(createOf('PLANNING')?.code, 'app.sprints.plan', 'the plans lead to a collection\'s Plan a sprint');
});

// §5.2: the home's create act creates - it opens the place's create dialog, not the place's list.
test('a product home\'s create act opens the create dialog at the place', () => {
  const id = '01a0e2e0-0000-7000-8000-000000000002';
  for (const module of ['KNOWLEDGE', 'WHITEBOARD', 'ASSETS', 'PLANNING']) {
    const path = createAt(module, id);
    assert.ok(path, `${module} has no create address`);
    const url = new URL(path, 'https://hubtask.invalid');
    assert.equal(url.searchParams.get(CREATE_ASKED), '1', `${module}: ${path}`);
    assert.ok(resolve(ROUTES, url.pathname).name, `${module}'s create leads to no route: ${path}`);
  }
  assert.equal(createAt('TIME', id), undefined);
});

// §4: the first visit is decided by whether anything of the product exists yet, not by whether a
// hub has it on - switching a product on at the workspace reaches every hub at once.
test('a product is unused until one of its hubs holds something of it', () => {
  assert.equal(isUnused({}, []), true, 'no hub with the product holds nothing of it');
  assert.equal(isUnused({ a: 0 }, ['a', 'b']), undefined, 'a hub still being read decides nothing yet');
  assert.equal(isUnused({ a: 0, b: 0 }, ['a', 'b']), true);
  assert.equal(isUnused({ a: 0, b: 2 }, ['a', 'b']), false);
});

test('a home\'s create act offers the place holding most of the product first, ties in their order', () => {
  const places = [{ id: 'general' }, { id: 'it' }, { id: 'hr' }];
  assert.deepEqual(placesByUse(places, { general: 0, it: 12, hr: 3 }).map((p) => p.id), ['it', 'hr', 'general']);
  assert.deepEqual(placesByUse(places, { general: 0, hr: 0 }).map((p) => p.id), ['general', 'it', 'hr']);
  assert.deepEqual(placesByUse(places, undefined).map((p) => p.id), ['general', 'it', 'hr']);
});

// user-experience.md §5.2: a create dialog's *In:* chip offers the hubs where the product is on and
// the reader may write - never one whose answer is still out, never one the workspace has it off for.
test('the place chip offers the hubs where the product is on and the reader may write', () => {
  const hubs = [{ id: 'general', name: 'General' }, { id: 'it', name: 'IT' }, { id: 'hr', name: 'HR' }, { id: 'new', name: 'New' }];
  const onAt = { general: ['KNOWLEDGE', 'ASSETS'], it: ['KNOWLEDGE', 'ASSETS'], hr: ['ASSETS'] };
  const mayWrite = (path: { hubId: string }) => (path.hubId === 'general' ? undefined : path.hubId !== 'hr');
  const of = (module: string, workspaceOn: readonly string[] | undefined) =>
    placesToCreate({ module, level: 'HUB', hubs, workspaceOn, onAt, collectionsOf: () => [], mayWrite }).map((place) => place.id);
  assert.deepEqual(of('KNOWLEDGE', ['KNOWLEDGE', 'ASSETS']), ['it']);
  assert.deepEqual(of('ASSETS', ['KNOWLEDGE', 'ASSETS']), ['it']);
  assert.deepEqual(of('ASSETS', ['KNOWLEDGE']), []);
});

test('the place chip of a product made at a collection offers the collections, named with their hub', () => {
  const places = placesToCreate({
    module: 'PLANNING',
    level: 'COLLECTION',
    hubs: [{ id: 'dev', name: 'Development' }, { id: 'ops', name: 'Operations' }],
    workspaceOn: ['PLANNING'],
    onAt: { dev: ['PLANNING'], ops: [] },
    collectionsOf: (hubId) => (hubId === 'dev' ? [{ id: 'app', name: 'App' }, { id: 'web', name: 'Web' }] : [{ id: 'run', name: 'Run' }]),
    mayWrite: (path) => path.collectionId !== 'web',
  });
  assert.deepEqual(places, [{ id: 'app', label: 'Development › App' }]);
});

// user-experience.md §2.2: a home's hub rows say their state - a plan's next release, a hub's
// inventory - and the assets' home lists the hubs that hold an inventory, not every hub.
test('a hub row names its next planned release, and the assets home lists the hubs with an inventory', () => {
  const release = (id: string, hub_id: string, release_on: string, state = 'PLANNED') => ({ id, hub_id, release_on, state });
  const lists = { a: [release('1', 'a', '2026-12-10'), release('2', 'a', '2026-11-20'), release('3', 'a', '2026-10-01', 'RELEASED')], b: [] };
  assert.equal(nextReleaseOf(lists, 'a')?.id, '2');
  assert.equal(nextReleaseOf(lists, 'b'), undefined);
  const hubs = [{ id: 'a' }, { id: 'b' }, { id: 'c' }];
  assert.deepEqual(hubsWithAssetsFirst(hubs, { a: { total: 0 }, b: { total: 4 } }).map((hub) => hub.id), ['b', 'c']);
  // Before any hub answers, and where none holds anything, every hub stays.
  assert.deepEqual(hubsWithAssetsFirst(hubs, {}).map((hub) => hub.id), ['a', 'b', 'c']);
  assert.deepEqual(hubsWithAssetsFirst(hubs, { a: { total: 0 }, b: { total: 0 } }).map((hub) => hub.id), ['a', 'b', 'c']);
});

test('the plans home lists the sprints running in the hubs\' collections, the soonest to end first (§2.2, §7.9)', () => {
  const sprint = (id: string, collection_id: string, state: string, end_on: string) => ({ id, collection_id, state, end_on });
  const lists = {
    c1: [sprint('s1', 'c1', 'ACTIVE', '2026-11-20'), sprint('s2', 'c1', 'PLANNED', '2026-11-06')],
    c2: [sprint('s3', 'c2', 'ACTIVE', '2026-11-10'), sprint('s4', 'c2', 'CLOSED', '2026-11-01')],
    c3: [sprint('s5', 'c3', 'ACTIVE', '2026-11-08')],
  };
  assert.deepEqual(sprintsRunning(lists, ['c1', 'c2'], 5).map((each) => each.id), ['s3', 's1']);
  assert.deepEqual(sprintsRunning(lists, ['c1', 'c2', 'c3'], 1).map((each) => each.id), ['s5']);
  assert.deepEqual(sprintsRunning(lists, ['gone'], 5), []);
});
