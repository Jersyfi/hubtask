// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { TransportError } from '@hubtask/sync-engine';

import { createMessages } from './i18n/messages.ts';
import { renderProblem } from './problem.ts';

const messages = createMessages({ locale: 'en', onProblem: () => {} });

test('the specific code wins over the category it falls into', () => {
  const rendered = renderProblem(
    new TransportError('problem', {
      status: 409,
      code: 'conflict',
      detailCode: 'items.parent_in_own_subtree',
    }),
    messages,
  );
  // `errors.conflict` is true and useless; the detail code is the same failure said usefully.
  assert.equal(rendered.message, 'An entry cannot be moved into something that sits inside it.');
  assert.notEqual(rendered.message, messages.t('errors.conflict'));
});

test('a detail code nobody has a message for falls back to the category', () => {
  // Preferring the specific one blindly would put "Invented code" on the screen as a humanised
  // key, when the catalogue has a real sentence one level up.
  const rendered = renderProblem(
    new TransportError('problem', { status: 403, code: 'forbidden', detailCode: 'items.invented_code' }),
    messages,
  );
  assert.equal(rendered.message, 'You do not have permission for this action.');
});

test('a field error lands on its field, by path', () => {
  const rendered = renderProblem(
    new TransportError('problem', {
      status: 422,
      code: 'validation_failed',
      fieldErrors: [
        { path: 'title', code: 'usecase.field_required' },
        { path: 'status', code: 'usecase.field_not_in_enum', params: { allowed: 'OPEN, DONE' } },
        // No path: nothing to attach it to, so it is not invented onto one.
        { code: 'usecase.input_invalid' },
      ],
    }),
    messages,
  );
  assert.equal(rendered.fields.size, 2);
  assert.equal(rendered.fields.get('title'), 'This field is required.');
  assert.equal(rendered.fields.get('status'), 'This field accepts one of: OPEN, DONE.');
  assert.equal(rendered.message, 'The request contains invalid values.');
});

test('a 500 renders its reference, and does not say it twice', () => {
  const rendered = renderProblem(
    new TransportError('problem', { status: 500, code: 'internal', requestId: '01J9Z2ABCD' }),
    messages,
  );
  assert.equal(rendered.message, 'Something went wrong on our side. Reference: 01J9Z2ABCD');
  // The sentence already carries it, so the frame is not asked to show it a second time.
  assert.equal(rendered.reference, undefined);
  assert.equal(rendered.isServerFault, true);
});

test('a reference the sentence does not carry is handed to the frame', () => {
  const rendered = renderProblem(
    new TransportError('problem', { status: 503, code: 'dependency_unavailable', requestId: 'r-77' }),
    messages,
  );
  assert.equal(rendered.message, 'A required service is temporarily unavailable.');
  assert.equal(rendered.reference, 'r-77');
  assert.equal(rendered.isServerFault, true);
});

test('a failure that never reached the server still says something true', () => {
  for (const [kind, expected] of [
    ['offline', 'Hubtask cannot reach the server. Check the connection and try again.'],
    ['timeout', 'The server took too long to answer. Try again.'],
    ['malformed', 'The server sent an answer this version of Hubtask cannot read.'],
  ] as const) {
    const rendered = renderProblem(new TransportError(kind), messages);
    assert.equal(rendered.message, expected);
    assert.equal(rendered.isServerFault, true, `${kind} is not the reader's fault`);
    assert.equal(rendered.fields.size, 0);
  }
});

test('a failure with nothing to quote does not show a placeholder instead', () => {
  // A gateway answering 502 with an empty body: no code, no detail code, no request id. The
  // server's own sentence for this carries the reference inside it, so offering it here would put
  // a literal `{request_id}` in front of the reader.
  const rendered = renderProblem(new TransportError('problem', { status: 502 }), messages);
  assert.equal(rendered.message, 'Something went wrong. Try again in a moment.');
  assert.ok(!rendered.message.includes('{'), 'a placeholder reached the reader');
  assert.equal(rendered.reference, undefined);
  assert.equal(rendered.isServerFault, true);
});

test('a validation failure is not the server’s fault', () => {
  const rendered = renderProblem(new TransportError('problem', { status: 422, code: 'validation_failed' }), messages);
  assert.equal(rendered.isServerFault, false);
});

test('a query the server refuses by name still reads as a sentence', () => {
  // The other half of F2-13's acceptance. `query.ts` is what stops this client from sending a
  // field the manifest does not report; this is what happens when a refusal arrives anyway —
  // a manifest read before the installation changed, an automation that wrote the view, a filter
  // that came from somewhere this client cannot see. The reader is told which field and why,
  // rather than being shown "the request contains invalid values".
  const rendered = renderProblem(
    new TransportError('problem', {
      status: 422,
      code: 'validation_failed',
      detailCode: 'query.field_unknown',
      params: { field: 'invented_field' },
      fieldErrors: [
        { path: '/filter/field', code: 'query.field_unknown', params: { field: 'invented_field' } },
      ],
    }),
    messages,
  );
  assert.equal(rendered.message, 'invented_field is not something you can filter on here.');
  assert.equal(
    rendered.fields.get('/filter/field'),
    'invented_field is not something you can filter on here.',
  );
  assert.ok(!rendered.message.includes('{'), 'a placeholder reached the reader');
});
