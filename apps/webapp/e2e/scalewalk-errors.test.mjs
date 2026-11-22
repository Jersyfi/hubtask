// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { EventEmitter } from 'node:events';
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { answerEvent, fold, record } from './scalewalk-errors.mjs';

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

// A page as record() listens to it, and the events in the order Playwright 1.65's Chromium emits
// them when a page is left: the next document's request, the old one's cancelled requests, the
// commit.
function fakePage() {
  const page = new EventEmitter();
  const main = {};
  page.mainFrame = () => main;
  const request = (path, { navigation = false, frame = main } = {}) => {
    const each = {
      method: () => 'GET', url: () => `http://x${path}`, isNavigationRequest: () => navigation, frame: () => frame,
      failure: () => each.failed,
    };
    page.emit('request', each);
    return each;
  };
  const fail = (each, errorText) => { each.failed = { errorText }; page.emit('requestfailed', each); };
  const answer = (each, status) => page.emit('response', {
    status: () => status, statusText: () => '', request: () => each, url: () => each.url(),
  });
  const commit = (frame = main) => page.emit('framenavigated', frame);
  return { page, request, fail, answer, commit };
}

const messages = (events) => events.map((each) => each.message);

test('a request the page gives up by being left is no error; a failure while it stays is', () => {
  const { page, request, fail, commit } = fakePage();
  const events = [];
  record(page, events);
  request('/', { navigation: true });
  commit();
  const chunk = request('/assets/Screen.js');
  const stream = request('/api/v1/stream');
  fail(request('/api/v1/items'), 'net::ERR_FAILED');
  fail(request('/api/v1/search'), 'net::ERR_ABORTED');
  commit(); // a history change: the document stays
  fail(request('/api/v1/tags'), 'net::ERR_ABORTED');
  request('/other', { navigation: true });
  const late = request('/assets/Other.js'); // the old document still runs until the commit
  fail(chunk, 'net::ERR_ABORTED');
  fail(stream, 'net::ERR_ABORTED');
  commit();
  fail(late, 'net::ERR_ABORTED');
  fail(request('/api/v1/hubs'), 'net::ERR_ABORTED');
  assert.deepEqual(messages(events), [
    'request failed: GET /api/v1/items net::ERR_FAILED',
    'request failed: GET /api/v1/search net::ERR_ABORTED',
    'request failed: GET /api/v1/tags net::ERR_ABORTED',
    'request failed: GET /api/v1/hubs net::ERR_ABORTED',
  ]);
});

test('a request left behind that fails in another way is still an error', () => {
  const { page, request, fail, commit } = fakePage();
  const events = [];
  record(page, events);
  request('/', { navigation: true });
  commit();
  const broken = request('/api/v1/items');
  request('/other', { navigation: true });
  fail(broken, 'net::ERR_CONNECTION_RESET');
  commit();
  assert.deepEqual(messages(events), ['request failed: GET /api/v1/items net::ERR_CONNECTION_RESET']);
});

test('a frame that is left keeps its cancelled request an error, and the page opens no window', () => {
  const { page, request, fail, commit } = fakePage();
  const events = [];
  record(page, events);
  request('/', { navigation: true });
  commit();
  const frame = {};
  request('/embed', { navigation: true, frame });
  commit(frame);
  const inFrame = request('/api/v1/embed', { frame });
  request('/embed?2', { navigation: true, frame });
  fail(inFrame, 'net::ERR_ABORTED');
  commit(frame);
  fail(request('/api/v1/items'), 'net::ERR_ABORTED');
  assert.deepEqual(messages(events), [
    'request failed: GET /api/v1/embed net::ERR_ABORTED',
    'request failed: GET /api/v1/items net::ERR_ABORTED',
  ]);
});

test('a navigation that fails is an error, and the page it did not leave keeps its failures', () => {
  const { page, request, fail, commit } = fakePage();
  const events = [];
  record(page, events);
  request('/', { navigation: true });
  commit();
  fail(request('/other', { navigation: true }), 'net::ERR_ABORTED');
  fail(request('/api/v1/items'), 'net::ERR_ABORTED');
  assert.deepEqual(messages(events), [
    'request failed: GET /other net::ERR_ABORTED',
    'request failed: GET /api/v1/items net::ERR_ABORTED',
  ]);
});

for (const status of [204, 205]) {
  test(`a navigation answered ${status} leaves no document, so the page's own aborts stay errors`, () => {
    const { page, request, fail, answer, commit } = fakePage();
    const events = [];
    record(page, events);
    request('/', { navigation: true });
    commit();
    answer(request('/download', { navigation: true }), status);
    fail(request('/api/v1/items'), 'net::ERR_ABORTED');
    assert.deepEqual(messages(events), ['request failed: GET /api/v1/items net::ERR_ABORTED']);
  });
}

test('the answer of a navigation that was superseded does not end the window of the next', () => {
  const { page, request, fail, answer, commit } = fakePage();
  const events = [];
  record(page, events);
  request('/', { navigation: true });
  commit();
  const stream = request('/api/v1/stream');
  const first = request('/download', { navigation: true });
  request('/other', { navigation: true });
  answer(first, 204);
  fail(stream, 'net::ERR_ABORTED');
  commit();
  assert.deepEqual(messages(events), []);
});
