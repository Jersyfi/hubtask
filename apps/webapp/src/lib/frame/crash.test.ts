// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { crashDetails, isNoise, newReference, requestIdOf } from './crash.ts';

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
