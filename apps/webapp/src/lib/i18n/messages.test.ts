// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SOURCE } from './catalogue.ts';
import { createMessages, humanise } from './messages.ts';

test('a code with parameters renders, in the reader’s language', () => {
  const messages = createMessages({ locale: 'en' });
  assert.equal(messages.t('errors.unauthenticated'), 'Please sign in.');
  assert.equal(
    messages.t('errors.internal', { request_id: '01J9Z2' }),
    'Something went wrong on our side. Reference: 01J9Z2',
  );
});

test('a translation is preferred, and a gap in it falls through to English', () => {
  const german = { 'errors.forbidden': 'Dazu fehlt dir die Berechtigung.' };
  const messages = createMessages({ locale: 'de', catalogues: [german, SOURCE] });
  assert.equal(messages.t('errors.forbidden'), 'Dazu fehlt dir die Berechtigung.');
  assert.equal(messages.t('errors.not_found'), 'This entry does not exist.');
});

test('the source catalogue is in the chain even when a caller forgets it', () => {
  const messages = createMessages({ locale: 'de', catalogues: [{ 'a.b': 'c' }] });
  assert.equal(messages.t('errors.not_found'), 'This entry does not exist.');
});

test('an unknown code renders something a person can read', () => {
  const problems: string[] = [];
  const messages = createMessages({ locale: 'en', onProblem: (m) => problems.push(m) });
  assert.equal(messages.t('items.due_date_in_past'), 'Due date in past');
  assert.equal(messages.has('items.due_date_in_past'), false);
  // The code is not lost: it goes where somebody who can look it up will see it.
  assert.deepEqual(problems, ['no message for the code items.due_date_in_past']);
  // …once, not on every render.
  messages.t('items.due_date_in_past');
  assert.equal(problems.length, 1);
  assert.equal(humanise('errors.capability_not_supported'), 'Capability not supported');
});

test('a message that cannot be rendered does not take the page down', () => {
  // Only reachable from a translation that arrived after the build - catalogue.test.ts parses
  // every message in the source. The reader gets the sentence with its brace in it; the developer
  // gets the reason.
  const problems: string[] = [];
  const broken = { 'errors.not_found': 'Weg: {n, number}' };
  const messages = createMessages({ locale: 'de', catalogues: [broken], onProblem: (m) => problems.push(m) });
  assert.equal(messages.t('errors.not_found'), 'Weg: {n, number}');
  assert.match(problems[0]!, /cannot be rendered/);
});

test('has answers for the two codes a problem document carries', () => {
  const messages = createMessages({ locale: 'en' });
  assert.equal(messages.has('errors.validation_failed'), true);
  assert.equal(messages.has('errors.detail_that_does_not_exist'), false);
});
