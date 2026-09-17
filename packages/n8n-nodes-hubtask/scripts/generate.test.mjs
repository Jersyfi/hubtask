// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The node against the contract it is generated from (P-04): every operation reachable, every
// event type offered, the description in the shape n8n loads, and the validator able to fail.

import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import test from 'node:test';

import { describeNode, generate, readDocument, words } from './generate.mjs';
import { describeProblems, selftest } from './schema.mjs';

const require = createRequire(import.meta.url);
const document = require('@hubtask/api-client/openapi.json');
const events = require('@hubtask/api-client/events.json');

test('the format validator catches what it claims to catch', () => {
  assert.equal(selftest(), true);
});

test('names are written out', () => {
  assert.equal(words('createWorkItem'), 'Create Work Item');
  assert.equal(words('WORK_PACKAGE'), 'Work Package');
  assert.equal(words('itemId'), 'Item ID');
  assert.equal(words('ai'), 'AI');
});

test('every operation of the contract is an operation of the node, under its tag', () => {
  const { contract, description, problems } = generate({ document, events });
  assert.deepEqual(problems, []);
  const expected = [];
  for (const item of Object.values(document.paths)) {
    for (const [method, op] of Object.entries(item)) {
      if (['get', 'post', 'put', 'patch', 'delete'].includes(method)) expected.push(op.operationId);
    }
  }
  const offered = description.properties
    .filter((property) => property.name === 'operation')
    .flatMap((property) => property.options.map((option) => option.value));
  assert.equal(offered.length, expected.length);
  for (const id of expected) assert.ok(offered.includes(id), `${id} is not an operation of the node`);
  assert.equal(new Set(offered).size, expected.length, 'an operation is offered twice');
  assert.equal(contract.tags.length, description.properties[0].options.length);
});

test('a path parameter is a required property under its operation, and the url names it', () => {
  const { description } = generate({ document, events });
  const operation = description.properties.find((p) => p.name === 'operation' && p.displayOptions.show.resource[0] === 'items');
  const get = operation.options.find((option) => option.value === 'getWorkItem');
  assert.equal(get.routing.request.method, 'GET');
  assert.equal(get.routing.request.url, '=/items/{{ encodeURIComponent($parameter["itemId"]) }}');
  const itemId = description.properties.find((p) => p.name === 'itemId' && p.displayOptions?.show.operation?.[0] === 'getWorkItem');
  assert.equal(itemId.required, true);
  assert.equal(itemId.type, 'string');
});

test('a body becomes fields with routing, required ones on their own and the rest in a collection', () => {
  const { description } = generate({ document, events });
  const under = (name) => description.properties.find((p) => p.name === name && p.displayOptions?.show.operation?.[0] === 'createWorkItem');
  assert.equal(under('title').required, true);
  assert.deepEqual(under('title').routing, { send: { type: 'body', property: 'title' } });
  assert.equal(under('type').type, 'options');
  const additional = under('additionalFields');
  assert.equal(additional.type, 'collection');
  assert.ok(additional.options.some((option) => option.name === 'notes'));
  const patch = description.properties.find((p) => p.name === 'IfMatch' && p.displayOptions?.show.operation?.[0] === 'updateWorkItem');
  assert.deepEqual(patch.routing, { request: { headers: { 'If-Match': '={{ $value }}' } } });
});

test('query parameters are options that route to the query', () => {
  const { description } = generate({ document, events });
  const options = description.properties.find((p) => p.name === 'options' && p.displayOptions?.show.operation?.[0] === 'listContainers');
  assert.equal(options.type, 'collection');
  const size = options.options.find((option) => option.name === 'size');
  assert.deepEqual(size.routing, { send: { type: 'query', property: 'size' } });
});

test('every event type of the contract is offered by the trigger', () => {
  const { eventTypes } = generate({ document, events });
  assert.equal(eventTypes.length, Object.keys(events).length);
  assert.ok(eventTypes.includes('de.hubtask.work.item.created.v1'));
});

test('a document with a nameless operation is refused rather than rendered', () => {
  const broken = structuredClone(document);
  delete broken.paths['/items'].post.operationId;
  const description = describeNode(readDocument(broken));
  assert.ok(describeProblems(description).length > 0);
});
