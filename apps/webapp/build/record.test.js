// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the walks' recorder writes for one answer (e2e/record.mjs): the line test/contract judges.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { answerOf, guessed } from '../e2e/record.mjs';

const at = (path) => `http://127.0.0.1:5180${path}`;

test('an answer is recorded by method, path below the API, status, content type and body', () => {
  assert.deepEqual(answerOf('GET', at('/api/v1/accounts/me?x=1'), { json: { id: 'a' } }), {
    method: 'GET', path: '/accounts/me', status: 200, content_type: 'application/json', body: '{"id":"a"}',
  });
  assert.deepEqual(answerOf('POST', at('/api/v1/sync:snapshot'), { status: 200, contentType: 'application/x-ndjson', body: '{"cursor":"c"}\n' }), {
    method: 'POST', path: '/sync:snapshot', status: 200, content_type: 'application/x-ndjson', body: '{"cursor":"c"}\n',
  });
});

test('a status and a content type are read from where a stub may state them', () => {
  const refused = answerOf('PATCH', at('/api/v1/items/1'), { status: 409, headers: { 'Content-Type': 'application/problem+json' }, body: '{"code":"conflict"}' });
  assert.equal(refused.status, 409);
  assert.equal(refused.content_type, 'application/problem+json');
  assert.equal(answerOf('DELETE', at('/api/v1/items/1'), { status: 204 }).body, null);
  assert.equal(answerOf('GET', at('/api/v1/items/1'), { body: 'x' }).content_type, null);
});

test('a body that is not text is kept whole, and one from a file is named, not read', () => {
  assert.equal(answerOf('GET', at('/api/v1/media/1'), { body: Buffer.from([1, 2, 3]) }).body_base64, 'AQID');
  assert.equal(answerOf('GET', at('/api/v1/media/1'), { path: '/tmp/x.png' }).body_from, 'path');
});

test('a request outside the API is not an answer of the contract', () => {
  assert.equal(answerOf('GET', at('/assets/index.js'), { body: '' }), undefined);
  assert.equal(answerOf('GET', at('/api/v10/x'), { body: '' }), undefined);
});

test("the fixture's guess is marked, and only for the route it was made for", () => {
  const route = {};
  guessed(route);
  assert.equal(answerOf('GET', at('/api/v1/quotas'), { json: {} }, route).guess, true);
  assert.equal('guess' in answerOf('GET', at('/api/v1/quotas'), { json: {} }, {}), false);
});
