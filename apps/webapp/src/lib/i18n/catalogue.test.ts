// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The gate that makes a hand-written ICU subset safe.
//
// `format.ts` refuses syntax it does not implement rather than guessing at it. That refusal is
// only worth anything if somebody hears it, and the moment to hear it is when a message is written
// into the catalogue - not when a reader of Polish sees the wrong plural. So every message in
// `locales/en.json` is parsed here, and a construct the client cannot render turns this red.
//
// It is the client's counterpart to `infrastructure/i18n/Catalogue_test.go`, which refuses a
// plural on the Go side because the Go renderer implements only simple arguments. The two gates
// draw the line in different places on purpose: the server renders a handful of messages for
// email, and the client renders all of them.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SOURCE, SOURCE_LOCALE, patternFor } from './catalogue.ts';
import { format, parse } from './format.ts';

test('the catalogue is the one at the repository root, and it is not empty', () => {
  assert.ok(Object.keys(SOURCE).length > 500, 'the catalogue looks truncated');
  assert.equal(SOURCE['errors.unauthenticated'], 'Please sign in.');
});

test('no note to the translators is offered as a message', () => {
  for (const code of Object.keys(SOURCE)) {
    assert.ok(!code.startsWith('_'), `${code} is metadata and would be rendered as a sentence`);
  }
});

test('every message in the catalogue is one this renderer can render', () => {
  const failures: string[] = [];
  for (const [code, pattern] of Object.entries(SOURCE)) {
    try {
      parse(pattern);
    } catch (error) {
      failures.push(`${code}: ${(error as Error).message}`);
    }
  }
  assert.deepEqual(failures, [], 'add the construct to format.ts, or write the message another way');
});

test('the errors.* codes render as the sentences the catalogue gives them', () => {
  assert.equal(format(SOURCE['errors.forbidden']!), 'You do not have permission for this action.');
  assert.equal(
    format(SOURCE['errors.internal']!, { request_id: '01J9Z2' }),
    'Something went wrong on our side. Reference: 01J9Z2',
  );
});

test('a missing translation falls back to the source language, never to a key', () => {
  const german: Record<string, string> = { 'errors.forbidden': 'Dazu fehlt dir die Berechtigung.' };
  assert.equal(patternFor('errors.forbidden', [german, SOURCE]), 'Dazu fehlt dir die Berechtigung.');
  assert.equal(patternFor('errors.not_found', [german, SOURCE]), 'This entry does not exist.');
  assert.equal(patternFor('errors.invented', [german, SOURCE]), undefined);
  assert.equal(SOURCE_LOCALE, 'en');
});
