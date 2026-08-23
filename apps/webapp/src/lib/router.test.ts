// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { normalisePath, resolve, type Route } from './router.ts';

const routes: Route[] = [
  { name: 'home', pattern: '/' },
  { name: 'container', pattern: '/containers/:id' },
  { name: 'container-activity', pattern: '/containers/:id/activity/:activityId' },
];

test('normalisePath strips query, fragment, and the trailing slash', () => {
  assert.equal(normalisePath('/'), '/');
  assert.equal(normalisePath(''), '/');
  assert.equal(normalisePath('/hubs/'), '/hubs');
  assert.equal(normalisePath('/hubs?cursor=abc'), '/hubs');
  assert.equal(normalisePath('/hubs#section'), '/hubs');
  assert.equal(normalisePath('/hubs/?a=1#b'), '/hubs');
});

test('the root resolves to home', () => {
  const r = resolve(routes, '/');
  assert.equal(r.name, 'home');
  assert.deepEqual(r.params, {});
});

test('a parameter captures one segment and is decoded', () => {
  const r = resolve(routes, '/containers/01JBXR%20TEST');
  assert.equal(r.name, 'container');
  assert.deepEqual(r.params, { id: '01JBXR TEST' });
});

test('two parameters in one pattern both capture', () => {
  const r = resolve(routes, '/containers/abc/activity/def');
  assert.equal(r.name, 'container-activity');
  assert.deepEqual(r.params, { id: 'abc', activityId: 'def' });
});

test('a missing segment is not an empty parameter', () => {
  // `/containers/` is the collection, not a container with an empty id.
  assert.equal(resolve(routes, '/containers/').name, null);
});

test('an extra segment does not match a shorter pattern', () => {
  assert.equal(resolve(routes, '/containers/abc/extra').name, null);
});

test('an unknown path resolves to no route, keeping the normalised path', () => {
  const r = resolve(routes, '/nowhere?x=1');
  assert.equal(r.name, null);
  assert.equal(r.path, '/nowhere');
});

test('a query string does not disturb the match', () => {
  const r = resolve(routes, '/containers/abc?tab=activity');
  assert.equal(r.name, 'container');
  assert.deepEqual(r.params, { id: 'abc' });
});

test('the first match wins, so the table is ordered specific-first', () => {
  const shadowing: Route[] = [
    { name: 'wildcard', pattern: '/x/:anything' },
    { name: 'specific', pattern: '/x/fixed' },
  ];
  assert.equal(resolve(shadowing, '/x/fixed').name, 'wildcard');
});
