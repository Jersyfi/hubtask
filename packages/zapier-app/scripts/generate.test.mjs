// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The app against the contract it is generated from (P-05): every event type a trigger, every
// create and search present with a sample, the definition in the shape the platform loads, and
// the generated code doing what the contract expects when driven with a fake `z`.

import assert from 'node:assert/strict';
import fs from 'node:fs';
import { createRequire } from 'node:module';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';

import { CREATES, SEARCHES, loadApp, triggerKey, words, writeApp } from './generate.mjs';
import { appProblems, selftest } from './schema.mjs';

const require = createRequire(import.meta.url);
const document = require('@hubtask/api-client/openapi.json');
const events = require('@hubtask/api-client/events.json');

const into = fs.mkdtempSync(path.join(os.tmpdir(), 'hubtask-zapier-test-'));
const written = writeApp({ document, events, into });
const app = loadApp(into);
test.after(() => fs.rmSync(into, { recursive: true, force: true }));

/** A `z` that records requests and answers what the test says. */
function fakeZ(answer) {
  const calls = [];
  return {
    calls,
    request: async (options) => {
      calls.push(options);
      return typeof answer === 'function' ? answer(options) : answer;
    },
    hash: (_, value) => `hash-of-${value.length}`,
    errors: { Error: class ZapierError extends Error { constructor(message, code, status) { super(message); this.code = code; this.status = status; } } },
  };
}

const bundle = (inputData = {}) => ({ authData: { base_url: 'https://hubtask.example/', access_token: 't' }, inputData, meta: { zap: { id: 7 } }, targetUrl: 'https://hooks.zapier.com/x', subscribeData: { id: 'sub-1' }, cleanedRequest: { type: 'de.hubtask.work.item.created.v1', data: {} } });

test('the format validator catches what it claims to catch', () => {
  assert.equal(selftest(), true);
});

test('the app validates, and names every event type as a trigger and every create and search', () => {
  assert.deepEqual(appProblems(app), []);
  assert.equal(Object.keys(app.triggers).length, Object.keys(events).length);
  for (const type of Object.keys(events)) assert.ok(app.triggers[triggerKey(type)], `${type} has no trigger`);
  assert.deepEqual(Object.keys(app.creates).sort(), CREATES.map((c) => c.id).sort());
  assert.deepEqual(Object.keys(app.searches).sort(), SEARCHES.map((s) => s.id).sort());
  assert.equal(written.triggers.length, Object.keys(events).length);
  assert.equal(triggerKey('de.hubtask.work.item.created.v1'), 'workItemCreated');
  assert.equal(words('workItemCreated'), 'Work Item Created');
});

test('a create sends the body fields the person filled, an idempotency key, and the bearer path', async () => {
  const z = fakeZ({ status: 201, data: { id: 'i1' } });
  const created = await app.creates.createWorkItem.operation.perform(z, bundle({ type: 'TASK', title: 'Buy oat milk', notes: '' }));
  assert.deepEqual(created, { id: 'i1' });
  const [call] = z.calls;
  assert.equal(call.method, 'POST');
  assert.equal(call.url, 'https://hubtask.example/api/v1/items');
  assert.deepEqual(call.body, { type: 'TASK', title: 'Buy oat milk' });
  assert.ok(call.headers['Idempotency-Key'].startsWith('hash-of-'));
  assert.equal(call.headers['Content-Type'], 'application/json');
});

test('an update fills the path and sends a merge patch; a complete fills its action path', async () => {
  const z = fakeZ({ status: 200, data: { id: 'i1' } });
  await app.creates.updateWorkItem.operation.perform(z, bundle({ itemId: 'a/b', title: 'Renamed' }));
  await app.creates.completeWorkItem.operation.perform(z, bundle({ itemId: 'i1' }));
  assert.equal(z.calls[0].url, 'https://hubtask.example/api/v1/items/a%2Fb');
  assert.equal(z.calls[0].headers['Content-Type'], 'application/merge-patch+json');
  assert.deepEqual(z.calls[0].body, { title: 'Renamed' });
  assert.equal(z.calls[1].url, 'https://hubtask.example/api/v1/items/i1:complete');
  assert.deepEqual(z.calls[1].body ?? {}, {});
});

test('a refusal becomes a Zapier error carrying the problem code', async () => {
  const z = fakeZ({ status: 422, data: { code: 'validation_failed', detail_code: 'items.title_required' } });
  await assert.rejects(app.creates.createWorkItem.operation.perform(z, bundle({ type: 'TASK' })), (error) => {
    assert.equal(error.code, 'items.title_required');
    assert.equal(error.status, 422);
    return true;
  });
});

test('a trigger subscribes over the REST hooks pattern, unsubscribes by id, and lists for the sample', async () => {
  const trigger = app.triggers.workItemCreated;
  const z = fakeZ((options) => (options.method === 'GET' ? { status: 200, data: { data: [{ id: 'e1' }] } } : { status: 201, data: { id: 'sub-1', secret: 's' } }));
  await trigger.operation.performSubscribe(z, bundle());
  await trigger.operation.performUnsubscribe(z, bundle());
  const listed = await trigger.operation.performList(z, bundle());
  assert.deepEqual(z.calls[0].body, { target_url: 'https://hooks.zapier.com/x', event_types: ['de.hubtask.work.item.created.v1'] });
  assert.equal(z.calls[1].method, 'DELETE');
  assert.equal(z.calls[1].url, 'https://hubtask.example/api/v1/integrations/webhooks/sub-1');
  assert.equal(z.calls[2].url, 'https://hubtask.example/api/v1/integrations/triggers/de.hubtask.work.item.created.v1');
  assert.deepEqual(listed, [{ id: 'e1' }]);
  assert.deepEqual(trigger.operation.perform(z, bundle()), [bundle().cleanedRequest]);
  assert.equal(trigger.operation.sample.type, 'de.hubtask.work.item.created.v1');
});

test('a search answers the page\'s entries', async () => {
  const z = fakeZ({ status: 200, data: { data: [{ id: 'i1' }, { id: 'i2' }], page: {} } });
  const found = await app.searches.searchItems.operation.perform(z, bundle({ query: 'milk' }));
  assert.equal(found.length, 2);
  assert.equal(z.calls[0].url, 'https://hubtask.example/api/v1/search');
  assert.deepEqual(z.calls[0].body, { query: 'milk' });
});

test('the published manifest names the platform library and the workspace\'s does not', () => {
  const published = JSON.parse(fs.readFileSync(path.join(into, 'package.json'), 'utf8'));
  assert.ok(published.dependencies['zapier-platform-core']);
  const own = require('../package.json');
  assert.equal(own.dependencies, undefined);
  assert.equal(own.peerDependencies, undefined);
});
