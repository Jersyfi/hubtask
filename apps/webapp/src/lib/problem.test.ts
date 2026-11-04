// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { TransportError } from '@hubtask/sync-engine';

import { formatDateTime } from './i18n/datetime.ts';
import { createMessages } from './i18n/messages.ts';
import { isNoSuchObject, isObjectId, renderProblem, renderRefusal } from './problem.ts';

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

test('field errors without a path are each said, not dropped - a step\'s failing checks', () => {
  const rendered = renderProblem(
    new TransportError('problem', {
      status: 422,
      code: 'validation_failed',
      detailCode: 'workflows.step_refused',
      fieldErrors: [
        { path: '/custom_fields/resolution', code: 'fields.value_required', params: { key: 'custom_fields.resolution' } },
        { path: '', code: 'workflows.check_failed', params: { check: 'tested', message: 'Attach the test report.' } },
        { path: '', code: 'workflows.check_failed', params: { check: 'signed', message: 'A reviewer signs it off.' } },
      ],
    }),
    messages,
  );
  assert.equal(rendered.fields.size, 1);
  assert.deepEqual(rendered.wholes, ['Attach the test report.', 'A reviewer signs it off.']);
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

test('a refusal or a failure carries its reference in the sentence, so every screen shows it', () => {
  for (const status of [403, 409, 422, 503]) {
    const rendered = renderProblem(new TransportError('problem', { status, code: 'forbidden', requestId: 'r-77' }), messages);
    assert.ok(rendered.message.endsWith('Reference: r-77'), `${status}: ${rendered.message}`);
    assert.equal(rendered.reference, undefined, `${status}: said twice`);
  }
});

test('a reference the sentence does not carry is handed to the frame', () => {
  const rendered = renderProblem(new TransportError('problem', { status: 404, code: 'not_found', requestId: 'r-77' }), messages);
  assert.ok(!rendered.message.includes('r-77'));
  assert.equal(rendered.reference, 'r-77');
});

// The copy control copies the reference itself, whether the sentence carries it or the frame does.
test('the reference is handed over for copying whether or not the sentence carries it', () => {
  const said = renderProblem(new TransportError('problem', { status: 403, code: 'forbidden', requestId: 'r-77' }), messages);
  const beside = renderProblem(new TransportError('problem', { status: 404, code: 'not_found', requestId: 'r-78' }), messages);
  const none = renderProblem(new TransportError('timeout'), messages);
  assert.equal(said.requestId, 'r-77');
  assert.equal(beside.requestId, 'r-78');
  assert.equal(none.requestId, undefined);
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
  // The other half. `query.ts` is what stops this client from sending a
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

// UC-AI-07 check 3: a spent budget is the one refusal with the budget's reason and the moment it
// renews, and the reader is told both - the moment in their own locale, never as wire text.
test('a spent AI budget says so and when it renews, the moment written for the reader', () => {
  const renewsAt = '2026-09-09T22:00:00Z';
  const rendered = renderProblem(
    new TransportError('problem', {
      status: 503,
      code: 'dependency_unavailable',
      detailCode: 'ai.unavailable',
      params: { reason: 'budget_spent', renews_at: renewsAt },
    }),
    messages,
  );
  assert.match(rendered.message, /AI budget/);
  assert.ok(rendered.message.includes(formatDateTime(renewsAt, 'en')), rendered.message);
  assert.ok(!rendered.message.includes(renewsAt), rendered.message);

  const off = renderProblem(
    new TransportError('problem', { status: 503, code: 'dependency_unavailable', detailCode: 'ai.unavailable' }),
    messages,
  );
  assert.doesNotMatch(off.message, /budget/);
});

test('a budget spent after the ask, as the job says it, reads like one spent before', () => {
  const renewsAt = '2026-09-09T22:00:00Z';
  const sentence = renderRefusal(
    { code: 'ai.unavailable', params: { reason: 'budget_spent', renews_at: renewsAt } },
    messages,
  );
  assert.match(sentence, /AI budget/);
  assert.ok(sentence.includes(formatDateTime(renewsAt, 'en')), sentence);
  assert.ok(!sentence.includes(renewsAt), sentence);

  assert.doesNotMatch(renderRefusal({ code: 'ai.unavailable' }, messages), /budget/);
  // A code this build has no sentence for still says something true rather than its key.
  assert.ok(!renderRefusal({ code: 'ai.invented_for_this_test' }, messages).includes('ai.invented'));
});

test('a legal hold reads as its own sentence, not as a generic failure', () => {
  // A hold is the one refusal on this screen a person can act on — by asking
  // whoever placed it — and "something went wrong" would tell them nothing to act on. The scope
  // travels as a parameter because *where* the hold is placed is what decides who to ask.
  const rendered = renderProblem(
    new TransportError('problem', {
      status: 409,
      code: 'conflict',
      detailCode: 'lifecycle.legal_hold',
      params: { scope: 'CONTAINER' },
    }),
    messages,
  );
  assert.equal(
    rendered.message,
    'This cannot be deleted for good: a legal hold is in force on the hub or collection it is in. Ask whoever placed it.',
  );
  assert.equal(rendered.isServerFault, false);
  assert.ok(!rendered.message.includes('{'), 'a placeholder reached the reader');
});

test('every scope a hold refusal names reads as words, never as its key', () => {
  // The server sends the hold's scope as stored (Purge.go's legalHoldRefusal), so each one needs
  // its own words: "in force on the CONTAINER" is a key on the screen (P-12).
  for (const scope of ['WORKSPACE', 'TENANT', 'CONTAINER', 'ITEM', 'ACCOUNT']) {
    const rendered = renderProblem(
      new TransportError('problem', {
        status: 409,
        code: 'conflict',
        detailCode: 'lifecycle.legal_hold',
        params: { scope },
      }),
      messages,
    );
    assert.ok(!rendered.message.includes(scope), `${scope} reached the reader: ${rendered.message}`);
    assert.match(rendered.message, /legal hold/);
  }
});

// user-experience.md §5.8: a refusal says what and what next - the permission by its name in the
// role editor, the limit by what it guards, and when to try again from the server's Retry-After.
test('a missing permission is named as the role editor names it, never by its key', () => {
  const rendered = renderProblem(
    new TransportError('problem', {
      status: 403, code: 'forbidden', detailCode: 'access.not_permitted', params: { permission: 'WRITE_ITEMS' },
    }),
    messages,
  );
  assert.ok(rendered.message.includes(messages.t('app.permissions.name.write_items')), rendered.message);
  assert.ok(!rendered.message.includes('WRITE_ITEMS'), rendered.message);

  // A permission this build has no name for stays as it arrived rather than turning into nothing.
  const unknown = renderProblem(
    new TransportError('problem', {
      status: 403, code: 'forbidden', detailCode: 'access.not_permitted', params: { permission: 'INVENTED' },
    }),
    messages,
  );
  assert.ok(unknown.message.includes('INVENTED'), unknown.message);
});

test('a rate limit says what it guards and when to try again', () => {
  const rendered = renderProblem(
    new TransportError('problem', {
      status: 429, code: 'rate_limited', detailCode: 'access.rate_limit_exceeded',
      params: { level: 'credential' }, retryAfterMs: 120_000,
    }),
    messages,
  );
  assert.ok(rendered.message.includes(messages.t('app.limit_level.credential')), rendered.message);
  assert.ok(rendered.message.includes(new Intl.RelativeTimeFormat('en', { numeric: 'auto' }).format(2, 'minute')), rendered.message);
  assert.ok(!rendered.message.includes('credential '), rendered.message);
  assert.ok(!rendered.message.includes('{'), rendered.message);

  // Without Retry-After the sentence still reads, and says soon rather than a placeholder.
  const bare = renderProblem(
    new TransportError('problem', { status: 429, code: 'rate_limited', detailCode: 'access.rate_limit_exceeded', params: { level: 'tenant' } }),
    messages,
  );
  assert.ok(bare.message.includes(messages.t('app.retry_soon')), bare.message);
  assert.ok(!bare.message.includes('{'), bare.message);

  // A numeric level - an approval's step - is not a limit and stays a number.
  const step = renderProblem(
    new TransportError('problem', { status: 409, code: 'conflict', detailCode: 'approvals.too_few_approvers', params: { level: '2' } }),
    messages,
  );
  assert.ok(step.message.includes('Level 2'), step.message);
});

// user-experience.md §5.8: where a screen can fold the reference behind *Details for support*, the
// sentence it shows is the reason alone; the code and the reference are handed over beside it.
test('the sentence without its reference is handed over for a fold that carries the reference', () => {
  const rendered = renderProblem(
    new TransportError('problem', { status: 403, code: 'forbidden', detailCode: 'access.not_permitted', params: { permission: 'READ' }, requestId: 'r-77' }),
    messages,
  );
  assert.ok(rendered.message.includes('r-77'), 'the sentence alone still carries it, for a screen with no fold');
  assert.ok(!rendered.sentence?.includes('r-77'), rendered.sentence);
  assert.ok(rendered.message.startsWith(rendered.sentence ?? '-'));
  assert.equal(rendered.code, 'access.not_permitted');

  const internal = renderProblem(new TransportError('problem', { status: 500, code: 'internal', requestId: 'r-78' }), messages);
  assert.ok(!internal.sentence?.includes('r-78'), internal.sentence);
  assert.ok(internal.sentence !== undefined && !internal.sentence.includes('{'), internal.sentence);
});

// UX-5: an address naming no object - a not-found, or an id in no id's shape - is a not-found
// state with a way back, not "this version cannot read the answer".
test('an address that names no object is told apart from a failure', () => {
  assert.equal(isNoSuchObject(new TransportError('problem', { status: 404, code: 'not_found' })), true);
  assert.equal(isNoSuchObject(new TransportError('problem', { status: 400, code: 'malformed_request', detailCode: 'request.parameter_invalid' })), true);
  assert.equal(isNoSuchObject(new TransportError('problem', { status: 403, code: 'forbidden' })), false);
  assert.equal(isNoSuchObject(new TransportError('malformed', { status: 200 })), false);
  assert.equal(isObjectId('01a0e2e0-0000-7000-8000-0000000018c1'), true);
  assert.equal(isObjectId('dashboards'), false);
});
