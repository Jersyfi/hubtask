// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The reference is one rendering of the contract, and the contract is what it is tested
// against: every operation of the real document appears exactly once, and the shapes a page
// relies on are held on a fixture small enough to read.

import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

import { Reader, humanize, readReference, slugOf } from '../src/lib/api/reference.ts';

const here = path.dirname(fileURLToPath(import.meta.url));
const real = path.resolve(here, '..', '..', '..', 'api', 'openapi.json');

const fixture = {
  info: { title: 'Fixture', version: '1', description: 'A small contract.' },
  servers: [{ url: 'https://{host}/api/v1' }],
  tags: [{ name: 'items', description: 'Entries.\n\nMore about them.' }],
  paths: {
    '/items': {
      post: {
        operationId: 'createItem',
        tags: ['items'],
        summary: 'Create an entry',
        description: 'Refused with `items.parent_type_invalid` when the parent cannot hold it.',
        parameters: [{ $ref: '#/components/parameters/IdempotencyKey' }],
        requestBody: { content: { 'application/json': { schema: { $ref: '#/components/schemas/ItemCreate' } } } },
        responses: { 201: { description: 'Created. The entry.' }, '4XX': { $ref: '#/components/responses/Problem' } },
      },
    },
    '/items/{itemId}': {
      parameters: [{ $ref: '#/components/parameters/ItemId' }],
      get: { operationId: 'getItem', tags: ['items'], summary: 'Read', responses: { 200: { description: 'The entry' } } },
      delete: { operationId: 'trashItem', tags: ['lifecycle'], summary: 'Trash', security: [], responses: { 204: { description: 'Gone' } } },
    },
  },
  components: {
    parameters: {
      ItemId: { name: 'itemId', in: 'path', required: true, schema: { type: 'string', format: 'uuid' } },
      IdempotencyKey: { name: 'Idempotency-Key', in: 'header', schema: { type: 'string', format: 'uuid' }, description: 'A UUID; repeats are answered from the store.' },
    },
    responses: { Problem: { description: 'A problem document. Read the code.' } },
    schemas: {
      ItemCreate: {
        type: 'object',
        required: ['title', 'type'],
        properties: {
          title: { type: 'string', maxLength: 500, example: 'Write the reference' },
          type: { type: 'string', enum: ['TASK', 'ACTIVITY'] },
          due: { $ref: '#/components/schemas/Due' },
          labels: { type: 'array', items: { $ref: '#/components/schemas/Label' } },
          old: { type: 'string', deprecated: true },
        },
      },
      Due: { type: 'object', required: ['at'], properties: { at: { type: 'string', format: 'date-time' }, zone: { type: ['string', 'null'], default: null } } },
      Label: { type: 'object', properties: { id: { type: 'string', format: 'uuid' }, nested: { $ref: '#/components/schemas/Label' } } },
    },
  },
};

test('every operation lands under its first tag, in the document order, declared tags first', () => {
  const reference = readReference(fixture);
  assert.deepEqual(reference.tags.map((tag) => tag.name), ['items', 'lifecycle']);
  assert.deepEqual(reference.tags[0].operations.map((op) => op.id), ['createItem', 'getItem']);
  assert.equal(reference.operationCount, 3);
  assert.equal(reference.tags[0].lede, 'Entries.');
});

test('a schema becomes rows: required, facts, deprecation, one level of unfolding, and a bound', () => {
  const [op] = readReference(fixture).tags[0].operations;
  const rows = op.requestBody.rows;
  assert.deepEqual(rows.map((row) => [row.name, row.type, row.isRequired]), [
    ['title', 'string', true],
    ['type', 'string', true],
    ['due', 'Due', false],
    ['labels', 'array of Label', false],
    ['old', 'string', false],
  ]);
  assert.deepEqual(rows[0].facts, ['maxLength: 500']);
  assert.deepEqual(rows[1].facts, ['TASK', 'ACTIVITY']);
  assert.equal(rows[4].isDeprecated, true);
  assert.deepEqual(rows[2].children.map((row) => [row.name, row.type]), [['at', 'string (date-time)'], ['zone', 'string, or null']]);
  // A recursive schema is finite because the unfolding stops at the bound.
  const label = rows[3].children;
  assert.equal(label[1].name, 'nested');
  assert.equal(label[1].children?.[1]?.children, undefined);
});

test('path, query and header parameters are split, and a path parameter is always required', () => {
  const ops = readReference(fixture).tags[0].operations;
  assert.deepEqual(ops[0].headerParameters.map((row) => row.name), ['Idempotency-Key']);
  assert.deepEqual(ops[1].pathParameters.map((row) => [row.name, row.isRequired]), [['itemId', true]]);
});

test('the curl line names the bearer, the idempotency key and an example body built from the schema', () => {
  const [create, read] = readReference(fixture).tags[0].operations;
  assert.match(create.curl, /^curl -X POST "\$HUBTASK_URL\/items"/);
  assert.match(create.curl, /Authorization: Bearer \$HUBTASK_TOKEN/);
  assert.match(create.curl, /Idempotency-Key: \$\(uuidgen\)/);
  assert.match(create.curl, /"title": "Write the reference"/);
  assert.match(create.curl, /"type": "TASK"/);
  assert.doesNotMatch(read.curl, /Content-Type/);
  const trash = readReference(fixture).tags[1].operations[0];
  assert.equal(trash.needsBearer, false);
  assert.doesNotMatch(trash.curl, /Authorization/);
});

test('a referenced response is resolved and the message codes are read from the description', () => {
  const [create] = readReference(fixture).tags[0].operations;
  assert.deepEqual(create.responses.map((r) => [r.status, r.description]), [
    ['201', 'Created.'],
    ['4XX', 'A problem document.'],
  ]);
  assert.deepEqual(create.codes, ['items.parent_type_invalid']);
});

test('an unknown reference is an error rather than an empty table', () => {
  const reader = new Reader({ ...fixture, components: { ...fixture.components, schemas: {} } });
  assert.throws(() => reader.rowsOf({ $ref: '#/components/schemas/ItemCreate' }), /unknown schema/);
});

test('a slug is a path segment, and an identifier is written out where a summary is missing', () => {
  assert.equal(slugOf('Work items'), 'work-items');
  assert.equal(humanize('createWorkItem'), 'Create work item');
  assert.equal(humanize('listOAuthGrants'), 'List oauth grants');
});

test('the real contract renders every operation exactly once', { skip: !fs.existsSync(real) && 'api/openapi.json is not beside the site' }, () => {
  const document = JSON.parse(fs.readFileSync(real, 'utf8'));
  const reference = readReference(document);
  let expected = 0;
  const ids = new Set();
  for (const item of Object.values(document.paths)) {
    for (const [method, op] of Object.entries(item)) {
      if (['get', 'post', 'put', 'patch', 'delete'].includes(method)) {
        expected++;
        ids.add(op.operationId);
      }
    }
  }
  const rendered = reference.tags.flatMap((tag) => tag.operations.map((op) => op.id));
  assert.equal(rendered.length, expected);
  assert.equal(new Set(rendered).size, expected, 'an operation is rendered twice');
  for (const id of ids) assert.ok(rendered.includes(id), `${id} is not rendered`);
  // Every operation has a curl line that names each of its path parameters.
  for (const tag of reference.tags) {
    for (const op of tag.operations) {
      for (const row of op.pathParameters) assert.ok(op.curl.includes(`{${row.name}}`), `${op.id}: ${row.name} is not in the curl line`);
    }
  }
  assert.ok(reference.schemas.length > 100);
});
