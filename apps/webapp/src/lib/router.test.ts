// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { Router, isOwnAddress, normalisePath, queryOf, resolve, type Route } from './router.ts';

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

test('a route declares its area, and end-user is what most screens are', () => {
  // ADR-0032's one restriction is by area: the mobile shell ships end-user and profile in full and
  // reaches administration through the web app. Declaring it here is what stops the shell having
  // to classify a route by reading it.
  const declared: Route[] = [
    { name: 'home', pattern: '/' },
    { name: 'profile', pattern: '/profile', area: 'profile' },
    { name: 'installation', pattern: '/installation', area: 'administration' },
  ];

  assert.equal(resolve(declared, '/').area, 'end-user', 'an undeclared route is an end-user one');
  assert.equal(resolve(declared, '/profile').area, 'profile');
  assert.equal(resolve(declared, '/installation').area, 'administration');
  // A path that matches nothing is not administration by accident.
  assert.equal(resolve(declared, '/nowhere').area, 'end-user');
});

test('the query travels with the resolution, and the path stays clean of it', () => {
  const routes = [{ name: 'collection', pattern: '/collections/:id' }];
  const resolution = resolve(routes, '/collections/c1?item=i1&x=y#frag');
  assert.equal(resolution.path, '/collections/c1');
  assert.deepEqual(resolution.params, { id: 'c1' });
  assert.deepEqual(resolution.query, { item: 'i1', x: 'y' });
  assert.deepEqual(queryOf('/collections/c1'), {});
  assert.deepEqual(queryOf('/a?item=%2Fx'), { item: '/x' });
});

test('a query name an object already answers to is a value like any other', () => {
  const query = queryOf('/a?__proto__=x&constructor=y&item=i1');
  assert.deepEqual(Object.keys(query).sort(), ['__proto__', 'constructor', 'item']);
  assert.equal(Object.getOwnPropertyDescriptor(query, '__proto__')?.value, 'x');
  assert.equal(Object.getPrototypeOf(query), Object.prototype, 'the name replaced the prototype');
});

test('a link of the shell at tauri://localhost stays in the application, though its origin is opaque', () => {
  const shell = { protocol: 'tauri:', host: 'localhost' };
  const link = new URL('/hubs/7', 'tauri://localhost/today');
  assert.equal(link.origin, 'null');
  assert.ok(isOwnAddress(link, shell));
  assert.ok(isOwnAddress(new URL('/hubs/7', 'http://tauri.localhost/'), { protocol: 'http:', host: 'tauri.localhost' }));
  assert.ok(isOwnAddress(new URL('https://hub.example.org:443/x'), { protocol: 'https:', host: 'hub.example.org' }));
  assert.ok(!isOwnAddress(new URL('https://localhost/hubs/7'), shell));
  assert.ok(!isOwnAddress(new URL('tauri://elsewhere/hubs/7'), shell));
  assert.ok(!isOwnAddress(new URL('https://hub.example.org:8443/x'), { protocol: 'https:', host: 'hub.example.org' }));
});

/**
 * A session history as the browser keeps it, enough for the stateful half: entries with their
 * state and address, `back` answered by a `popstate` a task later.
 */
function sessionHistory(start: string) {
  const entries: { state: unknown; path: string }[] = [{ state: null, path: start }];
  let at = 0;
  const events = new EventTarget();
  const location = { pathname: start, search: '' };
  const move = (to: number) => {
    at = to;
    const [pathname = '/', search = ''] = (entries[at]?.path ?? '/').split(/(?=\?)/u);
    location.pathname = pathname;
    location.search = search;
  };
  const history = {
    get state() { return entries[at]?.state ?? null; },
    get length() { return entries.length; },
    pushState(state: unknown, _title: string, path: string) { entries.splice(at + 1); entries.push({ state, path }); move(at + 1); },
    replaceState(state: unknown, _title: string, path?: string) { entries[at] = { state, path: path ?? entries[at]?.path ?? '/' }; },
    back() {
      setTimeout(() => {
        if (at === 0) return;
        move(at - 1);
        events.dispatchEvent(Object.assign(new Event('popstate'), { state: entries[at]?.state ?? null }));
      }, 0);
    },
    forward() {
      setTimeout(() => {
        if (at >= entries.length - 1) return;
        move(at + 1);
        events.dispatchEvent(Object.assign(new Event('popstate'), { state: entries[at]?.state ?? null }));
      }, 0);
    },
  };
  Object.assign(globalThis, {
    history,
    window: { location, addEventListener: events.addEventListener.bind(events), removeEventListener: events.removeEventListener.bind(events) },
    document: { addEventListener: () => {}, removeEventListener: () => {} },
  });
  return { history, at: () => at, path: () => entries[at]?.path };
}
const settle = () => new Promise((done) => setTimeout(done, 5));

test('a held step is closed by back, and the screen under it stays where it was', async () => {
  const session = sessionHistory('/hubs');
  const router = new Router(routes, '/hubs', {});
  const stop = router.start();
  router.navigate('/containers/new-hub');
  const seen: string[] = [];
  router.subscribe((resolution) => seen.push(resolution.path));
  let closed = 0;
  router.hold(() => (closed += 1));
  assert.equal(session.path(), '/containers/new-hub', 'holding a step changed the address');

  session.history.back();
  await settle();
  assert.equal(closed, 1, 'back did not close the step');
  assert.deepEqual(seen, ['/containers/new-hub'], 'back moved the screen');
  assert.equal(router.move, 'push', 'back over a step counted as a move of the screen');

  session.history.back();
  await settle();
  assert.equal(closed, 1);
  assert.deepEqual(seen, ['/containers/new-hub', '/hubs'], 'the next back did not leave the screen');
  stop();
});

test('a step closed another way takes its entry out, so the next back leaves the screen', async () => {
  const session = sessionHistory('/hubs');
  const router = new Router(routes, '/hubs', {});
  const stop = router.start();
  router.navigate('/containers/new-hub');
  const seen: string[] = [];
  router.subscribe((resolution) => seen.push(resolution.path));
  let closed = 0;
  const release = router.hold(() => (closed += 1));
  release();
  release();
  await settle();
  assert.equal(closed, 0, 'its own release called back into the step');
  assert.deepEqual(seen, ['/containers/new-hub'], 'the release moved the screen');

  session.history.forward();
  await settle();
  assert.deepEqual(seen, ['/containers/new-hub'], 'forward into a closed step moved the screen');

  session.history.back();
  await settle();
  assert.deepEqual(seen, ['/containers/new-hub', '/hubs'], 'back did not leave the screen');
  stop();
});
