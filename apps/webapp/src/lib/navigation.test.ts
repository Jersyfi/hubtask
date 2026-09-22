// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The one navigation list against the route table and the catalogue (ADR-0061 decision 1).
//
// What is worth asserting is not what the list says — that is read off the screen — but that it
// cannot disagree with the two things it points at: every route target resolves to a route the
// destination claims as its own, every word is a code the catalogue has, and the two areas
// ADR-0032 switches on are the route table's, not this file's opinion. And that the bottom bar
// can draw it: three to five, or the component refuses.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SOURCE } from './i18n/catalogue.ts';
import { DESTINATIONS, KEEPING, TRASH, YOU_CODE, account, currentDestination, primary } from './navigation.ts';
import { ROUTES } from './routes.ts';
import { resolve } from './router.ts';

test('every destination with a route lands on a route it claims as current', () => {
  for (const destination of DESTINATIONS) {
    if (destination.target.kind !== 'route') continue;
    const resolution = resolve(ROUTES, destination.target.path);
    assert.ok(resolution.name, `${destination.id} points at ${destination.target.path}, which resolves to nothing`);
    assert.ok(destination.routes.includes(resolution.name), `${destination.id} lands on ${resolution.name} but does not claim it`);
    assert.equal(currentDestination(resolution), destination.id);
  }
  for (const row of KEEPING) {
    const resolution = resolve(ROUTES, row.path);
    assert.ok(resolution.name, `${row.id} points at ${row.path}, which resolves to nothing`);
    assert.ok(row.routes.includes(resolution.name), `${row.id} lands on ${resolution.name} but does not claim it`);
    assert.equal(currentDestination(resolution), row.id);
  }
});

test('nothing is in two bands, and nothing is outside one', () => {
  // The three bands are `places` (the primary group), the tree (the containers, which are data
  // rather than a list here) and `keeping`. A destination in both would be drawn twice, which is
  // the failure the one list exists to prevent (ADR-0063 decision 1).
  const places = new Set(primary().map((destination) => destination.id));
  const keeping = new Set(KEEPING.map((row) => row.id));
  for (const id of keeping) assert.equal(places.has(id), false, `${id} is in two bands`);
  assert.equal(keeping.size, KEEPING.length, 'a row of the keeping band is in it twice');
  // The account group is behind the avatar, not in the column: no band holds it.
  for (const destination of DESTINATIONS) {
    if (destination.group !== 'account') continue;
    assert.equal(places.has(destination.id) || keeping.has(destination.id), false, `${destination.id} is in the column`);
  }
});

test('every route a destination claims exists in the table', () => {
  const names = new Set(ROUTES.map((route) => route.name));
  for (const destination of DESTINATIONS) {
    for (const name of destination.routes) assert.ok(names.has(name), `${destination.id} claims ${name}, which no route is named`);
  }
});

test('the areas are the route table’s, not the list’s', () => {
  // A destination's `area` is what the mobile build reads; the route the destination lands on
  // is what the shell excludes. The two are the same fact and must say the same thing.
  for (const destination of DESTINATIONS) {
    if (destination.target.kind !== 'route') continue;
    assert.equal(resolve(ROUTES, destination.target.path).area, destination.area ?? 'end-user', destination.id);
  }
  const administration = DESTINATIONS.filter((destination) => destination.area === 'administration');
  assert.equal(administration.length, 1, 'the administration is one row of the list');
});

test('every word is a code the catalogue has', () => {
  for (const destination of DESTINATIONS) assert.ok(destination.code in SOURCE, destination.code);
  for (const row of KEEPING) assert.ok(row.code in SOURCE, row.code);
  assert.ok(TRASH.code in SOURCE);
  assert.ok(YOU_CODE in SOURCE);
});

test('the bottom bar can draw the primary group and "You"', () => {
  // `BottomBar` takes three to five destinations: the primary group plus the account group's
  // head. Fewer is a switch, more is narrower than a thumb.
  const count = primary().length + 1;
  assert.ok(count >= 3 && count <= 5, `${count} destinations on a phone`);
});

test('the search exists once', () => {
  assert.equal(DESTINATIONS.filter((destination) => destination.target.kind === 'route' && destination.target.path === '/search').length, 1);
});

test('the administration row is offered only where the server says so', () => {
  const ids = (reachable: boolean) => account({ isAdministrationReachable: reachable }).map((destination) => destination.id);
  assert.deepEqual(ids(true), ['profile', 'administration', 'tour', 'sign-out', 'about']);
  assert.deepEqual(ids(false), ['profile', 'tour', 'sign-out', 'about']);
});

test('the installation is still in the list, at the foot and under another name', () => {
  // It stopped being a destination called "This installation" and became "About Hubtask" at the
  // end of the menu (ADR-0063 decision 6) — a move, not a removal: the route is the same, the
  // page is the same, and every reader still reaches it. Parity (ADR-0032) is about what a
  // person can do, and nothing here is one thing fewer.
  const about = DESTINATIONS.find((destination) => destination.id === 'about');
  assert.equal(about?.target.kind === 'route' && about.target.path, '/installation');
  assert.deepEqual(about?.routes, ['installation']);
  assert.equal(about, account({ isAdministrationReachable: false }).at(-1), 'it is not at the foot');
  assert.equal(about?.area, undefined, 'everyone may read what this installation is');
});

test('every screen under the administration is the administration destination', () => {
  for (const route of ROUTES.filter((each) => each.area === 'administration')) {
    assert.equal(currentDestination(resolve(ROUTES, route.pattern.replaceAll(/:\w+/g, 'x'))), 'administration', route.name);
  }
  // And a screen outside the list belongs to nothing: the bar marks no destination current.
  assert.equal(currentDestination(resolve(ROUTES, '/redeem')), undefined);
  assert.equal(currentDestination({ name: null, area: 'end-user' }), undefined);
});
