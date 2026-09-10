// SPDX-License-Identifier: BUSL-1.1
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

import { ADMINISTRATION_PREFIX, ROUTES } from './routes.ts';
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
  assert.equal(resolve(ROUTES, '/administration/webhooks').name, 'webhooks');
  assert.equal(resolve(ROUTES, '/administration/identity-provider').name, 'identity-provider');
});
