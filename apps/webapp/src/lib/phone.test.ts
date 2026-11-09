// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The phone's frame (the navigation concept §9; UC-WRK-36 checks 13 and 14).

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { TabStacks, ascentOf, browseElsewhere, depthOf, phoneTabs, starOf, tabOf } from './phone.ts';
import { ROUTES } from './routes.ts';
import { resolve } from './router.ts';

const at = (path: string) => resolve(ROUTES, path);

test('the bottom bar is Today · Browse · ＋ · Inbox · Search, narrowed by what the reader has', () => {
  assert.deepEqual(phoneTabs({ hasJumble: true, hasCapture: true }).map((tab) => tab.id), ['today', 'browse', 'capture', 'inbox', 'search']);
  assert.deepEqual(phoneTabs({ hasJumble: false, hasCapture: false }).map((tab) => tab.id), ['today', 'browse', 'search']);
});

test('every Browse step is a route, and a root has no way up', () => {
  for (const path of ['/browse', '/browse/hubs/h1', '/browse/hubs/h1/queues', '/browse/hubs/h1/contents']) assert.ok(at(path).name?.startsWith('browse'), path);
  for (const path of ['/', '/jumble', '/search', '/browse']) assert.equal(ascentOf(at(path), undefined), undefined, path);
});

test('a route belongs to its tab: the roots their own, every other place Browse, the areas too', () => {
  assert.equal(tabOf(at('/')), 'today');
  assert.equal(tabOf(at('/jumble')), 'inbox');
  assert.equal(tabOf(at('/search')), 'search');
  assert.equal(tabOf(at('/browse/hubs/h1')), 'browse');
  assert.equal(tabOf(at('/collections/c1')), 'browse');
  assert.equal(tabOf(at('/plans')), 'browse');
  assert.equal(tabOf(at('/profile')), 'browse');
  assert.equal(tabOf(at('/administration/people')), 'browse');
});

test('one level up: the stack step above, the hub for anything of a hub, Browse for the rest', () => {
  assert.deepEqual(ascentOf(at('/browse/hubs/h1'), undefined), { kind: 'hubs', path: '/browse' });
  assert.deepEqual(ascentOf(at('/browse/hubs/h1/queues'), 'h1'), { kind: 'hub', hubId: 'h1', path: '/browse/hubs/h1' });
  assert.deepEqual(ascentOf(at('/browse/hubs/h1/contents'), 'h1'), { kind: 'hub', hubId: 'h1', path: '/browse/hubs/h1' });
  const parents: Record<string, string | null> = { p2: 'p1', p1: null };
  assert.deepEqual(ascentOf(at('/browse/hubs/h1/contents?under=p2'), 'h1', (id) => parents[id]), { kind: 'contents', hubId: 'h1', pageId: 'p1', path: '/browse/hubs/h1/contents?under=p1' });
  assert.deepEqual(ascentOf(at('/browse/hubs/h1/contents?under=p1'), 'h1', (id) => parents[id]), { kind: 'contents', hubId: 'h1', path: '/browse/hubs/h1/contents' });
  assert.deepEqual(ascentOf(at('/collections/c1'), 'h1'), { kind: 'hub', hubId: 'h1', path: '/browse/hubs/h1' });
  assert.deepEqual(ascentOf(at('/plans'), undefined), { kind: 'browse', path: '/browse' });
});

test('an area is a level of the Browse stack: its groups a step, its screens under it', () => {
  for (const path of ['/administration/browse', '/profile/browse', '/instance/browse']) {
    assert.deepEqual(ascentOf(at(path), undefined), { kind: 'browse', path: '/browse' }, path);
    assert.equal(depthOf(at(path), undefined), 2, path);
  }
  assert.deepEqual(ascentOf(at('/administration/people'), undefined), { kind: 'area', area: 'administration', path: '/administration/browse' });
  assert.deepEqual(ascentOf(at('/administration'), undefined), { kind: 'area', area: 'administration', path: '/administration/browse' });
  assert.deepEqual(ascentOf(at('/profile/sessions'), undefined), { kind: 'area', area: 'profile', path: '/profile/browse' });
  assert.deepEqual(ascentOf(at('/instance/workspaces'), undefined), { kind: 'area', area: 'instance', path: '/instance/browse' });
  assert.equal(depthOf(at('/administration/people'), undefined), 3);
  assert.equal(browseElsewhere(at('/administration/browse')), '/administration');
  assert.equal(browseElsewhere(at('/profile/browse')), '/profile');
  assert.equal(browseElsewhere(at('/instance/browse')), '/instance');
});

test('the level announced: a root 1, a hub 2, what is under it 3', () => {
  assert.equal(depthOf(at('/'), undefined), 1);
  assert.equal(depthOf(at('/browse/hubs/h1'), undefined), 2);
  assert.equal(depthOf(at('/browse/hubs/h1/queues'), 'h1'), 3);
  assert.equal(depthOf(at('/collections/c1'), 'h1'), 3);
  assert.equal(depthOf(at('/browse/hubs/h1/contents?under=p2'), 'h1', 1), 5);
});

test('from medium up a Browse step opens the place the sidebar draws', () => {
  assert.equal(browseElsewhere(at('/browse')), '/');
  assert.equal(browseElsewhere(at('/browse/hubs/h1/queues')), '/hubs/h1/desk');
  assert.equal(browseElsewhere(at('/hubs/h1')), undefined);
});

test('the bar stars the place the screen is', () => {
  assert.deepEqual(starOf(at('/browse/hubs/h1')), { kind: 'HUB', id: 'h1' });
  assert.deepEqual(starOf(at('/collections/c1')), { kind: 'COLLECTION', id: 'c1' });
  assert.deepEqual(starOf(at('/pages/p1')), { kind: 'PAGE', id: 'p1' });
  assert.equal(starOf(at('/plans')), undefined);
});

test('each tab keeps its own stack, and a second tap returns to its root', () => {
  const stacks = new TabStacks();
  const [, browse] = phoneTabs({ hasJumble: true, hasCapture: true });
  assert.equal(stacks.target(browse!, 'today'), '/browse');
  stacks.seen('browse', '/browse/hubs/h1/queues');
  stacks.seen('today', '/');
  assert.equal(stacks.target(browse!, 'today'), '/browse/hubs/h1/queues');
  assert.equal(stacks.target(browse!, 'browse'), '/browse');
});
