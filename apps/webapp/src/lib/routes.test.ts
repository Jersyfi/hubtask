// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The application's real table, as opposed to `router.test.ts`'s invented ones.
//
// What is worth asserting about the real one is not that it matches — that is the router's job —
// but that ADR-0032's areas and the addresses agree. The mobile shell's one restriction is by
// area, so a screen that lives under `/administration` and forgot the tag would ship in the shell,
// and a screen tagged `administration` that lives somewhere else would vanish from it. Neither
// failure is visible in a browser, which is why it is a test.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { ADMINISTRATION_PREFIX, INSTANCE_PREFIX, ROUTES, paneFor } from './routes.ts';
import { normalisePath, resolve } from './router.ts';

test('the administration area is exactly the routes under its prefix', () => {
  const tagged = ROUTES.filter((route) => route.area === 'administration').map((r) => r.pattern);
  const underPrefix = ROUTES.filter(
    (route) => route.pattern === ADMINISTRATION_PREFIX || route.pattern.startsWith(`${ADMINISTRATION_PREFIX}/`),
  ).map((r) => r.pattern);

  assert.deepEqual(
    [...tagged].sort(),
    [...underPrefix].sort(),
    'a route under /administration is missing the tag, or a tagged route lives elsewhere',
  );
  assert.ok(tagged.length >= 1, 'the area is empty, which means the reading is broken');
});

test('the administration area is exactly the screens F4 built, by name', () => {
  // The first test says the tag and the prefix agree; this one says what the set *is*. A screen
  // added under `/administration` later joins this list on purpose, or the walk that found it
  // missing is repeated. F4-21's walk is where the list was read off the running application;
  // `ai` joined it on purpose in F5-05; `rule` and `rule-new`, the editor, in F8-04; and
  // `sign-in-settings` with the sign-in rule a workspace may tighten.
  const built = ROUTES.filter((route) => route.area === 'administration')
    .map((route) => route.name)
    .sort();
  assert.deepEqual(built, [
    'administration',
    'ai',
    'apps',
    'audit',
    'backup',
    'groups',
    'identity-provider',
    'people',
    'permissions',
    'privacy',
    'quotas',
    'restore',
    'retention',
    'rule',
    'rule-new',
    'rules',
    'runs',
    'service-accounts',
    'sign-in-settings',
    'webhooks',
    'workspace-settings',
  ]);
});

test('the instance area is exactly the routes under its prefix', () => {
  // SI-17's acceptance in one assertion, and the same one the administration has: the level above
  // the workspaces is excluded from the shells by its area (ADR-0070 §5), so a screen that lives
  // under `/instance` and forgot the tag would ship the control plane in the mobile shell.
  const tagged = ROUTES.filter((route) => route.area === 'instance').map((r) => r.pattern);
  const underPrefix = ROUTES.filter(
    (route) => route.pattern === INSTANCE_PREFIX || route.pattern.startsWith(`${INSTANCE_PREFIX}/`),
  ).map((r) => r.pattern);

  assert.deepEqual(
    [...tagged].sort(),
    [...underPrefix].sort(),
    'a route under /instance is missing the tag, or a tagged route lives elsewhere',
  );
  assert.ok(tagged.length >= 1, 'the area is empty, which means the reading is broken');
});

test('the instance area is exactly the screens ADR-0070 §5 names', () => {
  // The number is not the rule; the rule is what §5 forbids. What an operator needs to run an
  // installation is counts, states and limits, and a screen showing anything *inside* a workspace
  // does not belong to this area however convenient it would be. The list is written out so that
  // adding one is a decision somebody made against that sentence rather than a file that appeared.
  //
  // Seven since SI-12: the two the control plane had only at a terminal — the providers it offers
  // every workspace, and the keyring's census — joined it when the parity rule was written down
  // (ADR-0070 §5). Neither reads into a workspace: one is the installation's own rows, the other is
  // a count per key that names no workspace at all.
  const built = ROUTES.filter((route) => route.area === 'instance').map((route) => route.name).sort();
  assert.deepEqual(built, [
    'instance',
    'instance-encryption',
    'instance-journal',
    'instance-operators',
    'instance-providers',
    'instance-settings',
    'instance-workspaces',
  ]);
});

test('every route resolves to one of the four areas, and the profile ones are named', () => {
  // The areas are the whole of what the mobile shell switches on. A route that resolved to none
  // would be one the shell had to classify by reading it; a profile route that was not tagged would
  // ship as end-user and be excluded from nothing, which is not what "own security is not
  // administration" means.
  //
  // Four since SI-17: ADR-0070 §5 puts the level above the workspaces in this same app, and the
  // shells exclude it as they exclude administration.
  for (const route of ROUTES) {
    const area = resolve(ROUTES, route.pattern.replaceAll(/:\w+/g, 'x')).area;
    assert.ok(
      ['end-user', 'profile', 'administration', 'instance'].includes(area),
      `${route.name} is in ${area}`,
    );
  }
  // Your settings is a section since ADR-0065 decision 3, so the area is its eight screens rather
  // than the two it began with. Every one of them is about the reader themselves, which is what
  // `profile` means and why the mobile shell ships them all.
  const profile = ROUTES.filter((route) => route.area === 'profile').map((route) => route.name).sort();
  assert.deepEqual(profile, [
    'appearance',
    'devices',
    'grants',
    'notifications',
    'profile',
    'security',
    'sessions',
    'tokens',
  ]);
});

test('every route has a unique name and a unique pattern', () => {
  // Two routes with one name is a `route.name` switch that renders the wrong screen; two with one
  // pattern is a table where the second is unreachable, because the first match wins.
  const names = ROUTES.map((route) => route.name);
  const patterns = ROUTES.map((route) => route.pattern);
  assert.equal(new Set(names).size, names.length, 'two routes share a name');
  assert.equal(new Set(patterns).size, patterns.length, 'two routes share a pattern');
});

test('every pattern is already normalised', () => {
  // `resolve` normalises the path it is given, not the patterns: a pattern with a trailing slash
  // would have one segment more than the path it is meant to match and would never fire.
  for (const route of ROUTES) {
    assert.equal(route.pattern, normalisePath(route.pattern), `${route.pattern} is not normalised`);
  }
});

test('the addresses this application publishes resolve to their screens', () => {
  // The three that are somebody else's: the invitation mail links to /redeem, the identity
  // provider redirects to /auth/callback, and the frame links to /administration. A rename here is
  // a link somewhere else that stops working, which is what makes them worth naming in a test.
  assert.equal(resolve(ROUTES, '/redeem').name, 'redeem');
  assert.equal(resolve(ROUTES, '/auth/callback').name, 'oidc-callback');
  assert.equal(resolve(ROUTES, '/auth/callback?code=a&state=b').name, 'oidc-callback');
  assert.equal(resolve(ROUTES, '/administration').name, 'administration');
  assert.equal(resolve(ROUTES, '/administration/workspace').name, 'workspace-settings');
  assert.equal(resolve(ROUTES, '/administration/quotas').name, 'quotas');
  assert.equal(resolve(ROUTES, '/administration/backup').name, 'backup');
  assert.equal(resolve(ROUTES, '/administration/audit').name, 'audit');
  assert.equal(resolve(ROUTES, '/administration/privacy').name, 'privacy');
  assert.equal(resolve(ROUTES, '/administration/webhooks').name, 'webhooks');
  assert.equal(resolve(ROUTES, '/administration/retention').name, 'retention');
  assert.equal(resolve(ROUTES, '/administration/restore').name, 'restore');
  assert.equal(resolve(ROUTES, '/administration/identity-provider').name, 'identity-provider');
});

test('the detail pane is a place, not a feature: a pane from large, a redirect below, the entry page untouched', () => {
  // ADR-0061 decision 4. `/collections/:id?item=:itemId` is the collection with an entry open;
  // below `large` there is no room beside the list, and the entry's own address is where it goes.
  const open = resolve(ROUTES, '/collections/c1?item=i1');
  assert.deepEqual(paneFor(open, { isLarge: true }), { kind: 'pane', collectionId: 'c1', itemId: 'i1' });
  assert.deepEqual(paneFor(open, { isLarge: false }), { kind: 'redirect', path: '/items/i1' });
  // Without the parameter there is nothing to open, on any width.
  assert.deepEqual(paneFor(resolve(ROUTES, '/collections/c1'), { isLarge: true }), { kind: 'none' });
  // The entry's own address is not a pane and is not redirected, whatever its query says.
  assert.deepEqual(paneFor(resolve(ROUTES, '/items/i1?item=i2'), { isLarge: false }), { kind: 'none' });
  // A hub has no list of entries to open one beside.
  assert.deepEqual(paneFor(resolve(ROUTES, '/hubs/h1?item=i1'), { isLarge: true }), { kind: 'none' });
});
