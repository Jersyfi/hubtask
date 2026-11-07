// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { crashDetails, framesOf, isNoise, newReference, requestIdOf } from './crash.ts';

test('a reference is short, readable and differs between failures', () => {
  let n = 0;
  const reference = newReference(() => (n++ % 7) / 7);
  assert.match(reference, /^C-[0-9A-Z]{8}$/);
  assert.notEqual(newReference(), newReference());
});

test('the details name the build, the address, the browser and the request ID, never the message', () => {
  const error = Object.assign(new TypeError('Cannot read properties of null (reading "Quarterly plan")'), { requestId: 'r-42' });
  const details = crashDetails({
    reference: 'C-ABCDEFGH', build: '/assets/index-1a2b.js', path: '/collections/7',
    userAgent: 'Test/1.0', at: new Date('2026-11-01T10:00:00Z'), error,
  });
  assert.match(details, /reference: C-ABCDEFGH/);
  assert.match(details, /build: \/assets\/index-1a2b\.js/);
  assert.match(details, /path: \/collections\/7/);
  assert.match(details, /browser: Test\/1\.0/);
  assert.match(details, /failure: TypeError/);
  assert.match(details, /request_id: r-42/);
  assert.doesNotMatch(details, /Quarterly plan/);
});

test('only a string request ID is taken', () => {
  assert.equal(requestIdOf({ requestId: 'r-1' }), 'r-1');
  assert.equal(requestIdOf({ requestId: 7 }), undefined);
  assert.equal(requestIdOf(new Error('x')), undefined);
  assert.equal(requestIdOf(null), undefined);
});

test('a skipped resize frame and a cancelled request are not failures', () => {
  assert.ok(isNoise(undefined, 'ResizeObserver loop completed with undelivered notifications.'));
  assert.ok(isNoise(Object.assign(new Error('x'), { name: 'AbortError' })));
  assert.ok(!isNoise(new TypeError('x')));
});

test('the frames say where the code failed, never what the message quoted', () => {
  const webkit = Object.assign(new TypeError('Quarterly plan'), {
    stack: 'derived@tauri://localhost/assets/ContainerView-1a2b.js:3:1207\nmodule code@tauri://localhost/assets/index-9f.js:12:44\nforEach@[native code]',
  });
  assert.deepEqual(framesOf(webkit), [
    'derived@tauri://localhost/assets/ContainerView-1a2b.js:3:1207',
    'module code@tauri://localhost/assets/index-9f.js:12:44',
  ]);
  const v8 = Object.assign(new TypeError('Quarterly plan\nat Budget:1:2'), {
    stack: 'TypeError: Quarterly plan\nat Budget:1:2\n    at derived (http://localhost/assets/ContainerView-1a2b.js:3:1207)',
  });
  assert.deepEqual(framesOf(v8), ['at derived (http://localhost/assets/ContainerView-1a2b.js:3:1207)']);
  const details = crashDetails({
    reference: 'C-ABCDEFGH', build: null, path: '/hubs/7', userAgent: 'Test/1.0', at: new Date('2026-11-01T10:00:00Z'), error: v8,
  });
  assert.match(details, /frame: at derived \(http:\/\/localhost\/assets\/ContainerView-1a2b\.js:3:1207\)/);
  assert.doesNotMatch(details, /Quarterly|Budget/);
  assert.deepEqual(framesOf('not an error'), []);
});
