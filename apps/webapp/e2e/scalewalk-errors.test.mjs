// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fold } from './scalewalk-errors.mjs';

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
