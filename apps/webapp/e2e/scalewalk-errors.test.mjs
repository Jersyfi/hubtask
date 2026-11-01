// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { answerEvent, fold } from './scalewalk-errors.mjs';

test('one error seen on many entries is one row, counted', () => {
  const rows = fold([
    { level: 'ERROR', message: 'answered 503: GET /api/v1/items/01936f2a-7c1e-7000-8000-000000000001' },
    { level: 'ERROR', message: 'answered 503: GET /api/v1/items/01936f2a-7c1e-7000-8000-000000000002' },
    { level: 'WARN', message: 'slow "Kitchen" took 1200 ms' },
    { level: 'ERROR', message: 'slow "Kitchen" took 1200 ms' },
  ]);
  assert.deepEqual(rows, [
    { source: 'browser', level: 'ERROR', message: 'answered ?: GET /api/v1/items/?', count: 2 },
    { source: 'browser', level: 'ERROR', message: 'slow ? took ? ms', count: 1 },
    { source: 'browser', level: 'WARN', message: 'slow ? took ? ms', count: 1 },
  ]);
  assert.deepEqual(fold([]), []);
});

test('a refusal is a row with its path, told apart from a server error by its words', () => {
  assert.equal(answerEvent(200, 'OK', 'GET', 'http://x/api/v1/items'), null);
  const refused = answerEvent(403, 'Forbidden', 'GET', 'http://x/api/v1/tags?collection_id=1');
  const failed = answerEvent(503, 'Service Unavailable', 'POST', 'http://x/api/v1/sync:snapshot');
  assert.deepEqual(fold([refused, failed]), [
    { source: 'browser', level: 'ERROR', message: 'answered ? Service Unavailable: POST /api/v1/sync:snapshot', count: 1 },
    { source: 'browser', level: 'ERROR', message: 'refused ? Forbidden: GET /api/v1/tags', count: 1 },
  ]);
});
